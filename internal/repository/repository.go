// Package repository provides data access abstractions for GophKeeper.
//
// The [Repository] interface aggregates user, token, and secret storage.
// The default implementation is provided by package postgres.
package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zhebrikov/gophkeeper/internal/domain/models"
)

// UserStore defines user persistence operations.
type UserStore interface {
	// CreateUser inserts a new user with the given login and bcrypt password hash.
	CreateUser(ctx context.Context, login, passwordHash string) (*models.User, error)
	// GetUserByLogin returns a user by unique login name.
	GetUserByLogin(ctx context.Context, login string) (*models.User, error)
	// GetUserByID returns a user by identifier.
	GetUserByID(ctx context.Context, id uuid.UUID) (*models.User, error)
}

// TokenStore defines refresh token persistence operations.
type TokenStore interface {
	// SaveRefreshToken stores a hashed refresh token with an expiration time.
	SaveRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	// GetRefreshToken returns a refresh token record by its hash.
	GetRefreshToken(ctx context.Context, tokenHash string) (*models.RefreshToken, error)
	// RevokeRefreshToken removes a refresh token, invalidating the session.
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
}

// SecretStore defines secret persistence operations.
type SecretStore interface {
	// CreateSecret inserts a new encrypted secret for a user.
	CreateSecret(ctx context.Context, secret *models.Secret) (*models.Secret, error)
	// GetSecret returns a secret by ID scoped to the given user.
	GetSecret(ctx context.Context, userID, secretID uuid.UUID) (*models.Secret, error)
	// UpdateSecret updates a secret using optimistic locking on version.
	UpdateSecret(ctx context.Context, secret *models.Secret) (*models.Secret, error)
	// ListSecrets returns secrets for a user, optionally filtered by update time.
	ListSecrets(ctx context.Context, userID uuid.UUID, since time.Time) ([]*models.Secret, error)
	// DeleteSecret soft-deletes a secret by setting deleted_at.
	DeleteSecret(ctx context.Context, userID, secretID uuid.UUID) error
}

// Repository combines all storage interfaces and lifecycle methods.
type Repository interface {
	UserStore
	TokenStore
	SecretStore
	// Close releases database connections and other resources.
	Close() error
	// Ping verifies that the storage backend is reachable.
	Ping(ctx context.Context) error
}
