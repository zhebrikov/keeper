package postgres_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	domainerrors "github.com/zhebrikov/gophkeeper/internal/domain/errors"
	"github.com/zhebrikov/gophkeeper/internal/domain/models"
	"github.com/zhebrikov/gophkeeper/internal/repository/postgres"
)

func setupTestDB(t *testing.T) *postgres.DB {
	t.Helper()

	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		db, err := postgres.New(dsn)
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		return db
	}

	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("postgres unavailable: set DATABASE_URL or start Docker")
	}
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("gophkeeper"),
		tcpostgres.WithUsername("gk"),
		tcpostgres.WithPassword("gk"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, testcontainers.TerminateContainer(container))
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := postgres.New(dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	return db
}

func TestPing(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Ping(context.Background()))
}

func TestUserCRUD(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	user, err := db.CreateUser(ctx, "alice", "hash1")
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, user.ID)
	require.Equal(t, "alice", user.Login)

	byLogin, err := db.GetUserByLogin(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, user.ID, byLogin.ID)

	byID, err := db.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, "alice", byID.Login)

	_, err = db.CreateUser(ctx, "alice", "hash2")
	require.ErrorIs(t, err, domainerrors.ErrAlreadyExists)

	_, err = db.GetUserByLogin(ctx, "nobody")
	require.ErrorIs(t, err, domainerrors.ErrNotFound)

	_, err = db.GetUserByID(ctx, uuid.New())
	require.ErrorIs(t, err, domainerrors.ErrNotFound)
}

func TestRefreshTokenLifecycle(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	user, err := db.CreateUser(ctx, "bob", "hash")
	require.NoError(t, err)

	expires := time.Now().Add(time.Hour).UTC()
	require.NoError(t, db.SaveRefreshToken(ctx, user.ID, "token-hash-1", expires))

	token, err := db.GetRefreshToken(ctx, "token-hash-1")
	require.NoError(t, err)
	require.Equal(t, user.ID, token.UserID)

	require.NoError(t, db.RevokeRefreshToken(ctx, "token-hash-1"))

	_, err = db.GetRefreshToken(ctx, "token-hash-1")
	require.ErrorIs(t, err, domainerrors.ErrNotFound)
}

func TestSecretCRUD(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	user, err := db.CreateUser(ctx, "carol", "hash")
	require.NoError(t, err)

	secret := &models.Secret{
		UserID:        user.ID,
		Type:          models.SecretTypeText,
		Name:          "note",
		EncryptedData: []byte("encrypted"),
		Metadata:      "meta",
	}
	created, err := db.CreateSecret(ctx, secret)
	require.NoError(t, err)
	require.Equal(t, int64(1), created.Version)
	require.NotEqual(t, uuid.Nil, created.ID)

	got, err := db.GetSecret(ctx, user.ID, created.ID)
	require.NoError(t, err)
	require.Equal(t, "note", got.Name)

	created.Name = "note-v2"
	created.EncryptedData = []byte("encrypted-v2")
	created.Metadata = "meta2"
	updated, err := db.UpdateSecret(ctx, created)
	require.NoError(t, err)
	require.Equal(t, int64(2), updated.Version)
	require.Equal(t, "note-v2", updated.Name)

	_, err = db.UpdateSecret(ctx, created)
	require.ErrorIs(t, err, domainerrors.ErrConflict)

	all, err := db.ListSecrets(ctx, user.ID, time.Time{})
	require.NoError(t, err)
	require.Len(t, all, 1)

	since := updated.UpdatedAt
	time.Sleep(10 * time.Millisecond)

	secret2 := &models.Secret{
		UserID:        user.ID,
		Type:          models.SecretTypeCredential,
		Name:          "github",
		EncryptedData: []byte("enc2"),
	}
	_, err = db.CreateSecret(ctx, secret2)
	require.NoError(t, err)

	recent, err := db.ListSecrets(ctx, user.ID, since)
	require.NoError(t, err)
	require.Len(t, recent, 1)
	require.Equal(t, "github", recent[0].Name)

	require.NoError(t, db.DeleteSecret(ctx, user.ID, created.ID))

	deleted, err := db.GetSecret(ctx, user.ID, created.ID)
	require.NoError(t, err)
	require.True(t, deleted.IsDeleted())

	err = db.DeleteSecret(ctx, user.ID, uuid.New())
	require.ErrorIs(t, err, domainerrors.ErrNotFound)

	_, err = db.GetSecret(ctx, user.ID, uuid.New())
	require.ErrorIs(t, err, domainerrors.ErrNotFound)

	_, err = db.GetSecret(ctx, uuid.New(), created.ID)
	require.ErrorIs(t, err, domainerrors.ErrNotFound)
}
