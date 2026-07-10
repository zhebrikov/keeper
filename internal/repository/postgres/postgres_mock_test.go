package postgres

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"

	domainerrors "github.com/zhebrikov/gophkeeper/internal/domain/errors"
	"github.com/zhebrikov/gophkeeper/internal/domain/models"
)

func newMockDB(t *testing.T) (*DB, sqlmock.Sqlmock) {
	t.Helper()
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS users")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	db, err := NewWithConn(conn)
	if err != nil {
		t.Fatalf("NewWithConn: %v", err)
	}
	return db, mock
}

func TestNewWithConnMigrateError(t *testing.T) {
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer conn.Close()

	mock.ExpectExec("CREATE TABLE").WillReturnError(sql.ErrConnDone)

	_, err = NewWithConn(conn)
	if err == nil {
		t.Fatal("expected migrate error")
	}
}

func TestCreateUserSuccess(t *testing.T) {
	db, mock := newMockDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	userID := uuid.New()

	rows := sqlmock.NewRows([]string{"id", "login", "password_hash", "created_at", "updated_at"}).
		AddRow(userID, "alice", "hash", now, now)
	mock.ExpectQuery("INSERT INTO users").
		WithArgs(sqlmock.AnyArg(), "alice", "hash", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(rows)

	user, err := db.CreateUser(ctx, "alice", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if user.Login != "alice" {
		t.Fatalf("got login %q", user.Login)
	}
}

func TestCreateUserDuplicate(t *testing.T) {
	db, mock := newMockDB(t)
	ctx := context.Background()

	mock.ExpectQuery("INSERT INTO users").
		WillReturnError(&pq.Error{Code: "23505"})

	_, err := db.CreateUser(ctx, "alice", "hash")
	if err != domainerrors.ErrAlreadyExists {
		t.Fatalf("got %v, want ErrAlreadyExists", err)
	}
}

func TestGetUserByLogin(t *testing.T) {
	db, mock := newMockDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	userID := uuid.New()

	rows := sqlmock.NewRows([]string{"id", "login", "password_hash", "created_at", "updated_at"}).
		AddRow(userID, "alice", "hash", now, now)
	mock.ExpectQuery("SELECT id, login").
		WithArgs("alice").
		WillReturnRows(rows)

	user, err := db.GetUserByLogin(ctx, "alice")
	if err != nil {
		t.Fatalf("GetUserByLogin: %v", err)
	}
	if user.ID != userID {
		t.Fatal("unexpected user id")
	}

	mock.ExpectQuery("SELECT id, login").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	_, err = db.GetUserByLogin(ctx, "missing")
	if err != domainerrors.ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestGetUserByID(t *testing.T) {
	db, mock := newMockDB(t)
	ctx := context.Background()
	userID := uuid.New()
	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{"id", "login", "password_hash", "created_at", "updated_at"}).
		AddRow(userID, "alice", "hash", now, now)
	mock.ExpectQuery("SELECT id, login").
		WithArgs(userID).
		WillReturnRows(rows)

	user, err := db.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if user.Login != "alice" {
		t.Fatalf("got login %q", user.Login)
	}
}

func TestRefreshTokenOps(t *testing.T) {
	db, mock := newMockDB(t)
	ctx := context.Background()
	userID := uuid.New()
	now := time.Now().UTC()

	mock.ExpectExec("INSERT INTO refresh_tokens").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := db.SaveRefreshToken(ctx, userID, "hash", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("SaveRefreshToken: %v", err)
	}

	tokenID := uuid.New()
	rows := sqlmock.NewRows([]string{"id", "user_id", "token_hash", "expires_at", "created_at"}).
		AddRow(tokenID, userID, "hash", now.Add(time.Hour), now)
	mock.ExpectQuery("SELECT id, user_id").
		WithArgs("hash").
		WillReturnRows(rows)

	token, err := db.GetRefreshToken(ctx, "hash")
	if err != nil {
		t.Fatalf("GetRefreshToken: %v", err)
	}
	if token.UserID != userID {
		t.Fatal("unexpected user id")
	}

	mock.ExpectExec("DELETE FROM refresh_tokens").
		WithArgs("hash").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := db.RevokeRefreshToken(ctx, "hash"); err != nil {
		t.Fatalf("RevokeRefreshToken: %v", err)
	}
}

func TestCreateSecretOnly(t *testing.T) {
	db, mock := newMockDB(t)
	ctx := context.Background()
	userID := uuid.New()
	secretID := uuid.New()
	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "type", "name", "encrypted_data", "metadata", "version", "created_at", "updated_at", "deleted_at",
	}).AddRow(secretID, userID, "text", "note", []byte("enc"), "meta", int64(1), now, now, nil)

	mock.ExpectQuery("INSERT INTO secrets").
		WillReturnRows(rows)

	secret, err := db.CreateSecret(ctx, &models.Secret{
		ID:            secretID,
		UserID:        userID,
		Type:          models.SecretTypeText,
		Name:          "note",
		EncryptedData: []byte("enc"),
		Metadata:      "meta",
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	if secret.Version != 1 {
		t.Fatalf("expected version 1, got %d", secret.Version)
	}
}

func TestGetSecretOnly(t *testing.T) {
	db, mock := newMockDB(t)
	ctx := context.Background()
	userID := uuid.New()
	secretID := uuid.New()
	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "type", "name", "encrypted_data", "metadata", "version", "created_at", "updated_at", "deleted_at",
	}).AddRow(secretID, userID, "text", "note", []byte("enc"), "meta", int64(1), now, now, nil)

	mock.ExpectQuery("FROM secrets WHERE id").
		WithArgs(secretID, userID).
		WillReturnRows(rows)

	got, err := db.GetSecret(ctx, userID, secretID)
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got.Name != "note" {
		t.Fatalf("got name %q", got.Name)
	}
}

func TestUpdateSecret(t *testing.T) {
	db, mock := newMockDB(t)
	ctx := context.Background()
	userID := uuid.New()
	secretID := uuid.New()
	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "type", "name", "encrypted_data", "metadata", "version", "created_at", "updated_at", "deleted_at",
	}).AddRow(secretID, userID, "text", "note-v2", []byte("enc2"), "meta2", int64(2), now, now, nil)

	mock.ExpectQuery("UPDATE secrets").
		WillReturnRows(rows)

	updated, err := db.UpdateSecret(ctx, &models.Secret{
		ID:            secretID,
		UserID:        userID,
		Name:          "note-v2",
		EncryptedData: []byte("enc2"),
		Metadata:      "meta2",
		Version:       1,
	})
	if err != nil {
		t.Fatalf("UpdateSecret: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("expected version 2, got %d", updated.Version)
	}

	mock.ExpectQuery("UPDATE secrets").
		WillReturnError(sql.ErrNoRows)

	_, err = db.UpdateSecret(ctx, &models.Secret{ID: secretID, UserID: userID, Version: 99})
	if err != domainerrors.ErrConflict {
		t.Fatalf("got %v, want ErrConflict", err)
	}
}

func TestListSecrets(t *testing.T) {
	db, mock := newMockDB(t)
	ctx := context.Background()
	userID := uuid.New()
	secretID := uuid.New()
	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "type", "name", "encrypted_data", "metadata", "version", "created_at", "updated_at", "deleted_at",
	}).AddRow(secretID, userID, "text", "note", []byte("enc"), "", int64(1), now, now, nil)

	mock.ExpectQuery("FROM secrets").
		WithArgs(userID, nil).
		WillReturnRows(rows)

	secrets, err := db.ListSecrets(ctx, userID, time.Time{})
	if err != nil {
		t.Fatalf("ListSecrets: %v", err)
	}
	if len(secrets) != 1 {
		t.Fatalf("expected 1 secret, got %d", len(secrets))
	}
}

func TestListSecretsSince(t *testing.T) {
	db, mock := newMockDB(t)
	ctx := context.Background()
	userID := uuid.New()
	secretID := uuid.New()
	now := time.Now().UTC()
	since := now.Add(-time.Hour)

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "type", "name", "encrypted_data", "metadata", "version", "created_at", "updated_at", "deleted_at",
	}).AddRow(secretID, userID, "text", "note", []byte("enc"), "", int64(1), now, now, nil)

	mock.ExpectQuery("FROM secrets").
		WithArgs(userID, sqlmock.AnyArg()).
		WillReturnRows(rows)

	secrets, err := db.ListSecrets(ctx, userID, since)
	if err != nil {
		t.Fatalf("ListSecrets with since: %v", err)
	}
	if len(secrets) != 1 {
		t.Fatalf("expected 1 secret, got %d", len(secrets))
	}
}

func TestDeleteSecret(t *testing.T) {
	db, mock := newMockDB(t)
	ctx := context.Background()
	userID := uuid.New()
	secretID := uuid.New()

	mock.ExpectExec("UPDATE secrets").
		WithArgs(sqlmock.AnyArg(), secretID, userID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := db.DeleteSecret(ctx, userID, secretID); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}

	mock.ExpectExec("UPDATE secrets").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := db.DeleteSecret(ctx, userID, uuid.New())
	if err != domainerrors.ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestPingMock(t *testing.T) {
	conn, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer conn.Close()

	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS users")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	db, err := NewWithConn(conn)
	if err != nil {
		t.Fatalf("NewWithConn: %v", err)
	}

	mock.ExpectPing()
	if err := db.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestClose(t *testing.T) {
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	mock.ExpectClose()
	db := &DB{conn: conn}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestScanSecretWithDeletedAt(t *testing.T) {
	db, mock := newMockDB(t)
	ctx := context.Background()
	userID := uuid.New()
	secretID := uuid.New()
	now := time.Now().UTC()
	deleted := now.Add(time.Minute)

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "type", "name", "encrypted_data", "metadata", "version", "created_at", "updated_at", "deleted_at",
	}).AddRow(secretID, userID, "text", "note", []byte("enc"), "", int64(2), now, now, deleted)

	mock.ExpectQuery("SELECT id, user_id").
		WithArgs(secretID, userID).
		WillReturnRows(rows)

	got, err := db.GetSecret(ctx, userID, secretID)
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if !got.IsDeleted() {
		t.Fatal("expected deleted secret")
	}
}
