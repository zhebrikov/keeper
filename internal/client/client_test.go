package client_test

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/zhebrikov/gophkeeper/api/gen/gophkeeper/v1"
	"github.com/zhebrikov/gophkeeper/internal/auth"
	"github.com/zhebrikov/gophkeeper/internal/client"
	domainerrors "github.com/zhebrikov/gophkeeper/internal/domain/errors"
	"github.com/zhebrikov/gophkeeper/internal/domain/models"
	grpchandler "github.com/zhebrikov/gophkeeper/internal/handler/grpc"
	"github.com/zhebrikov/gophkeeper/internal/service"
)

const bufSize = 1024 * 1024

type mockRepo struct {
	users   map[string]*models.User
	secrets map[uuid.UUID]*models.Secret
	tokens  map[string]*models.RefreshToken
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		users:   make(map[string]*models.User),
		secrets: make(map[uuid.UUID]*models.Secret),
		tokens:  make(map[string]*models.RefreshToken),
	}
}

func (m *mockRepo) Close() error                   { return nil }
func (m *mockRepo) Ping(ctx context.Context) error { return nil }

func (m *mockRepo) CreateUser(ctx context.Context, login, passwordHash string) (*models.User, error) {
	if _, ok := m.users[login]; ok {
		return nil, domainerrors.ErrAlreadyExists
	}
	user := &models.User{
		ID:           uuid.New(),
		Login:        login,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	m.users[login] = user
	return user, nil
}

func (m *mockRepo) GetUserByLogin(ctx context.Context, login string) (*models.User, error) {
	user, ok := m.users[login]
	if !ok {
		return nil, domainerrors.ErrNotFound
	}
	return user, nil
}

func (m *mockRepo) GetUserByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, domainerrors.ErrNotFound
}

func (m *mockRepo) SaveRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	m.tokens[tokenHash] = &models.RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}
	return nil
}

func (m *mockRepo) GetRefreshToken(ctx context.Context, tokenHash string) (*models.RefreshToken, error) {
	token, ok := m.tokens[tokenHash]
	if !ok {
		return nil, domainerrors.ErrNotFound
	}
	return token, nil
}

func (m *mockRepo) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	delete(m.tokens, tokenHash)
	return nil
}

func (m *mockRepo) CreateSecret(ctx context.Context, secret *models.Secret) (*models.Secret, error) {
	if secret.ID == uuid.Nil {
		secret.ID = uuid.New()
	}
	secret.Version = 1
	now := time.Now().UTC()
	secret.CreatedAt = now
	secret.UpdatedAt = now
	m.secrets[secret.ID] = secret
	return secret, nil
}

func (m *mockRepo) GetSecret(ctx context.Context, userID, secretID uuid.UUID) (*models.Secret, error) {
	secret, ok := m.secrets[secretID]
	if !ok || secret.UserID != userID {
		return nil, domainerrors.ErrNotFound
	}
	copy := *secret
	return &copy, nil
}

func (m *mockRepo) UpdateSecret(ctx context.Context, secret *models.Secret) (*models.Secret, error) {
	existing, ok := m.secrets[secret.ID]
	if !ok || existing.Version != secret.Version {
		return nil, domainerrors.ErrConflict
	}
	secret.Version++
	secret.UpdatedAt = time.Now().UTC()
	m.secrets[secret.ID] = secret
	return secret, nil
}

func (m *mockRepo) ListSecrets(ctx context.Context, userID uuid.UUID, since time.Time) ([]*models.Secret, error) {
	var result []*models.Secret
	for _, s := range m.secrets {
		if s.UserID == userID && (since.IsZero() || s.UpdatedAt.After(since)) {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *mockRepo) DeleteSecret(ctx context.Context, userID, secretID uuid.UUID) error {
	secret, ok := m.secrets[secretID]
	if !ok || secret.UserID != userID {
		return domainerrors.ErrNotFound
	}
	now := time.Now().UTC()
	secret.DeletedAt = &now
	secret.Version++
	return nil
}

type testServer struct {
	listener *bufconn.Listener
	server   *grpc.Server
}

func startTestServer(t *testing.T) (*testServer, *auth.Manager) {
	t.Helper()

	repo := newMockRepo()
	jwt := auth.NewManager("test-secret", time.Minute)
	svc := service.New(repo, jwt, time.Hour)
	handler := grpchandler.NewHandler(svc, jwt)

	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(grpchandler.AuthUnaryInterceptor(jwt)),
	)
	pb.RegisterGophKeeperServer(grpcServer, handler)

	go func() {
		_ = grpcServer.Serve(lis)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
	})

	return &testServer{listener: lis, server: grpcServer}, jwt
}

func dialTestServer(t *testing.T, ts *testServer) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return ts.listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func newTestAPI(t *testing.T, conn *grpc.ClientConn) *client.API {
	t.Helper()
	dir := t.TempDir()
	api, err := client.NewWithConn(conn, filepath.Join(dir, "session.json"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = api.Close() })
	return api
}

func TestRegisterAndLogin(t *testing.T) {
	ts, _ := startTestServer(t)
	conn := dialTestServer(t, ts)
	api := newTestAPI(t, conn)
	ctx := context.Background()

	require.NoError(t, api.Register(ctx, "alice", "password1"))
	require.True(t, api.IsAuthenticated())

	require.NoError(t, api.Logout())
	require.False(t, api.IsAuthenticated())

	require.NoError(t, api.Login(ctx, "alice", "password1"))
	require.True(t, api.IsAuthenticated())
}

func TestSecretCRUD(t *testing.T) {
	ts, _ := startTestServer(t)
	conn := dialTestServer(t, ts)
	api := newTestAPI(t, conn)
	ctx := context.Background()

	require.NoError(t, api.Register(ctx, "alice", "password1"))

	created, err := api.CreateSecret(ctx, &pb.CreateSecretRequest{
		Type:          pb.SecretType_SECRET_TYPE_TEXT,
		Name:          "note",
		EncryptedData: []byte("encrypted"),
		Metadata:      "meta",
	})
	require.NoError(t, err)
	require.Equal(t, "note", created.GetName())

	got, err := api.GetSecret(ctx, created.GetId())
	require.NoError(t, err)
	require.Equal(t, created.GetId(), got.GetId())

	updated, err := api.UpdateSecret(ctx, &pb.UpdateSecretRequest{
		Id:            created.GetId(),
		Name:          "note-v2",
		EncryptedData: []byte("encrypted-v2"),
		Metadata:      "meta2",
		Version:       created.GetVersion(),
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), updated.GetVersion())

	list, err := api.ListSecrets(ctx, time.Time{})
	require.NoError(t, err)
	require.Len(t, list.GetSecrets(), 1)

	syncResp, err := api.Sync(ctx)
	require.NoError(t, err)
	require.NotZero(t, syncResp.GetSyncAt())
	require.Len(t, syncResp.GetSecrets(), 1)

	require.NoError(t, api.DeleteSecret(ctx, created.GetId()))
}

func TestUnauthenticatedRequests(t *testing.T) {
	ts, _ := startTestServer(t)
	conn := dialTestServer(t, ts)
	api := newTestAPI(t, conn)

	_, err := api.GetSecret(context.Background(), uuid.New().String())
	require.Error(t, err)
}

func TestIsAuthenticatedWithoutSession(t *testing.T) {
	ts, _ := startTestServer(t)
	conn := dialTestServer(t, ts)
	api := newTestAPI(t, conn)
	require.False(t, api.IsAuthenticated())
}
