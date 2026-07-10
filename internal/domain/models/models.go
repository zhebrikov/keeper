// Package models defines domain entities for GophKeeper.
//
// Core types are [User], [Secret], [RefreshToken], and [TokenPair].
// Secrets support optimistic locking via the Version field and soft deletion
// via DeletedAt.
package models

import (
	"time"

	"github.com/google/uuid"
)

// SecretType identifies the kind of stored secret data.
type SecretType string

const (
	// SecretTypeCredential stores login/password pairs.
	SecretTypeCredential SecretType = "credential"
	// SecretTypeText stores arbitrary text data.
	SecretTypeText SecretType = "text"
	// SecretTypeBinary stores arbitrary binary data.
	SecretTypeBinary SecretType = "binary"
	// SecretTypeCard stores bank card data.
	SecretTypeCard SecretType = "card"
	// SecretTypeOTP stores one-time password data.
	SecretTypeOTP SecretType = "otp"
)

// User represents a registered GophKeeper account.
type User struct {
	// ID is the unique user identifier.
	ID uuid.UUID
	// Login is the unique account name.
	Login string
	// PasswordHash is the bcrypt hash of the user's password.
	PasswordHash string
	// CreatedAt is the account creation timestamp.
	CreatedAt time.Time
	// UpdatedAt is the last account update timestamp.
	UpdatedAt time.Time
}

// Secret represents an encrypted data entry owned by a user.
type Secret struct {
	// ID is the unique secret identifier.
	ID uuid.UUID
	// UserID is the owner of the secret.
	UserID uuid.UUID
	// Type describes the kind of stored data.
	Type SecretType
	// Name is a human-readable label for the secret.
	Name string
	// EncryptedData holds client-encrypted secret bytes.
	EncryptedData []byte
	// Metadata stores optional plaintext context (e.g. URL or note).
	Metadata string
	// Version is incremented on each update for optimistic locking.
	Version int64
	// CreatedAt is the creation timestamp.
	CreatedAt time.Time
	// UpdatedAt is the last modification timestamp.
	UpdatedAt time.Time
	// DeletedAt is set when the secret is soft-deleted.
	DeletedAt *time.Time
}

// IsDeleted reports whether the secret has been soft-deleted.
func (s *Secret) IsDeleted() bool {
	return s.DeletedAt != nil
}

// RefreshToken represents a stored refresh token for a user session.
type RefreshToken struct {
	// ID is the unique token record identifier.
	ID uuid.UUID
	// UserID is the token owner.
	UserID uuid.UUID
	// TokenHash is the SHA-256 hash of the refresh token value.
	TokenHash string
	// ExpiresAt is when the refresh token becomes invalid.
	ExpiresAt time.Time
	// CreatedAt is when the token was stored.
	CreatedAt time.Time
}

// TokenPair holds access and refresh tokens issued after authentication.
type TokenPair struct {
	// AccessToken is the short-lived JWT for API calls.
	AccessToken string
	// RefreshToken is the long-lived token used to obtain new access tokens.
	RefreshToken string
	// UserID is the authenticated user's identifier.
	UserID uuid.UUID
}
