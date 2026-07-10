// Package postgres implements [repository.Repository] using PostgreSQL.
//
// [New] opens a connection pool, verifies connectivity, and runs schema
// migrations automatically. Secrets use soft deletion; updates rely on
// optimistic locking via the version column.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	domainerrors "github.com/zhebrikov/gophkeeper/internal/domain/errors"
	"github.com/zhebrikov/gophkeeper/internal/domain/models"
	"github.com/zhebrikov/gophkeeper/internal/repository"
)

// DB wraps a PostgreSQL connection pool.
type DB struct {
	conn *sql.DB
}

// New opens a PostgreSQL repository using the given connection string.
func New(dsn string) (*DB, error) {
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(5 * time.Minute)

	db := &DB{conn: conn}
	if err := db.Ping(context.Background()); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if err := db.migrate(context.Background()); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return db, nil
}

// NewWithConn creates a repository from an existing database connection and runs migrations.
// It is intended for testing with sqlmock or other test doubles.
func NewWithConn(conn *sql.DB) (*DB, error) {
	db := &DB{conn: conn}
	if err := db.migrate(context.Background()); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return db, nil
}

// Close closes the underlying database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Ping verifies the database connection is alive.
func (db *DB) Ping(ctx context.Context) error {
	return db.conn.PingContext(ctx)
}

var _ repository.Repository = (*DB)(nil)

const migrationSQL = `
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    login TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS secrets (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    encrypted_data BYTEA NOT NULL,
    metadata TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_secrets_user_updated ON secrets(user_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_hash ON refresh_tokens(token_hash);
`

func (db *DB) migrate(ctx context.Context) error {
	_, err := db.conn.ExecContext(ctx, migrationSQL)
	return err
}

// CreateUser inserts a new user record.
func (db *DB) CreateUser(ctx context.Context, login, passwordHash string) (*models.User, error) {
	id := uuid.New()
	now := time.Now().UTC()

	const query = `
		INSERT INTO users (id, login, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, login, password_hash, created_at, updated_at`

	user := &models.User{}
	err := db.conn.QueryRowContext(ctx, query, id, login, passwordHash, now, now).Scan(
		&user.ID, &user.Login, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, domainerrors.ErrAlreadyExists
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}
	return user, nil
}

// GetUserByLogin returns a user by login name.
func (db *DB) GetUserByLogin(ctx context.Context, login string) (*models.User, error) {
	const query = `
		SELECT id, login, password_hash, created_at, updated_at
		FROM users WHERE login = $1`

	user := &models.User{}
	err := db.conn.QueryRowContext(ctx, query, login).Scan(
		&user.ID, &user.Login, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domainerrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select user: %w", err)
	}
	return user, nil
}

// GetUserByID returns a user by ID.
func (db *DB) GetUserByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	const query = `
		SELECT id, login, password_hash, created_at, updated_at
		FROM users WHERE id = $1`

	user := &models.User{}
	err := db.conn.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Login, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domainerrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select user: %w", err)
	}
	return user, nil
}

// SaveRefreshToken stores a refresh token hash.
func (db *DB) SaveRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	const query = `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := db.conn.ExecContext(ctx, query, uuid.New(), userID, tokenHash, expiresAt, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("insert refresh token: %w", err)
	}
	return nil
}

// GetRefreshToken returns a refresh token by its hash.
func (db *DB) GetRefreshToken(ctx context.Context, tokenHash string) (*models.RefreshToken, error) {
	const query = `
		SELECT id, user_id, token_hash, expires_at, created_at
		FROM refresh_tokens WHERE token_hash = $1`

	token := &models.RefreshToken{}
	err := db.conn.QueryRowContext(ctx, query, tokenHash).Scan(
		&token.ID, &token.UserID, &token.TokenHash, &token.ExpiresAt, &token.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domainerrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select refresh token: %w", err)
	}
	return token, nil
}

// RevokeRefreshToken removes a refresh token.
func (db *DB) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	const query = `DELETE FROM refresh_tokens WHERE token_hash = $1`
	_, err := db.conn.ExecContext(ctx, query, tokenHash)
	if err != nil {
		return fmt.Errorf("delete refresh token: %w", err)
	}
	return nil
}

// CreateSecret inserts a new secret record.
func (db *DB) CreateSecret(ctx context.Context, secret *models.Secret) (*models.Secret, error) {
	if secret.ID == uuid.Nil {
		secret.ID = uuid.New()
	}
	now := time.Now().UTC()
	secret.CreatedAt = now
	secret.UpdatedAt = now
	if secret.Version == 0 {
		secret.Version = 1
	}

	const query = `
		INSERT INTO secrets (id, user_id, type, name, encrypted_data, metadata, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, user_id, type, name, encrypted_data, metadata, version, created_at, updated_at, deleted_at`

	return db.scanSecret(db.conn.QueryRowContext(ctx, query,
		secret.ID, secret.UserID, secret.Type, secret.Name, secret.EncryptedData,
		secret.Metadata, secret.Version, secret.CreatedAt, secret.UpdatedAt,
	))
}

// GetSecret returns a secret by ID for the given user.
func (db *DB) GetSecret(ctx context.Context, userID, secretID uuid.UUID) (*models.Secret, error) {
	const query = `
		SELECT id, user_id, type, name, encrypted_data, metadata, version, created_at, updated_at, deleted_at
		FROM secrets WHERE id = $1 AND user_id = $2`

	secret, err := db.scanSecret(db.conn.QueryRowContext(ctx, query, secretID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domainerrors.ErrNotFound
	}
	return secret, err
}

// UpdateSecret updates a secret with optimistic locking on version.
func (db *DB) UpdateSecret(ctx context.Context, secret *models.Secret) (*models.Secret, error) {
	now := time.Now().UTC()
	newVersion := secret.Version + 1

	const query = `
		UPDATE secrets
		SET name = $1, encrypted_data = $2, metadata = $3, version = $4, updated_at = $5
		WHERE id = $6 AND user_id = $7 AND version = $8 AND deleted_at IS NULL
		RETURNING id, user_id, type, name, encrypted_data, metadata, version, created_at, updated_at, deleted_at`

	row := db.conn.QueryRowContext(ctx, query,
		secret.Name, secret.EncryptedData, secret.Metadata, newVersion, now,
		secret.ID, secret.UserID, secret.Version,
	)

	updated, err := db.scanSecret(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domainerrors.ErrConflict
	}
	return updated, err
}

// ListSecrets returns secrets for a user, optionally filtered by update time.
func (db *DB) ListSecrets(ctx context.Context, userID uuid.UUID, since time.Time) ([]*models.Secret, error) {
	const query = `
		SELECT id, user_id, type, name, encrypted_data, metadata, version, created_at, updated_at, deleted_at
		FROM secrets
		WHERE user_id = $1 AND ($2::timestamptz IS NULL OR updated_at > $2)
		ORDER BY updated_at ASC`

	var sinceArg any
	if since.IsZero() {
		sinceArg = nil
	} else {
		sinceArg = since
	}

	rows, err := db.conn.QueryContext(ctx, query, userID, sinceArg)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	defer rows.Close()

	var secrets []*models.Secret
	for rows.Next() {
		secret, err := db.scanSecret(rows)
		if err != nil {
			return nil, err
		}
		secrets = append(secrets, secret)
	}
	return secrets, rows.Err()
}

// DeleteSecret soft-deletes a secret.
func (db *DB) DeleteSecret(ctx context.Context, userID, secretID uuid.UUID) error {
	now := time.Now().UTC()
	const query = `
		UPDATE secrets
		SET deleted_at = $1, updated_at = $1, version = version + 1
		WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL`

	result, err := db.conn.ExecContext(ctx, query, now, secretID, userID)
	if err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return domainerrors.ErrNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (db *DB) scanSecret(row rowScanner) (*models.Secret, error) {
	secret := &models.Secret{}
	var deletedAt sql.NullTime
	err := row.Scan(
		&secret.ID, &secret.UserID, &secret.Type, &secret.Name, &secret.EncryptedData,
		&secret.Metadata, &secret.Version, &secret.CreatedAt, &secret.UpdatedAt, &deletedAt,
	)
	if err != nil {
		return nil, err
	}
	if deletedAt.Valid {
		secret.DeletedAt = &deletedAt.Time
	}
	return secret, nil
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
