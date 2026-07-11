// Package service contains GophKeeper business logic.
//
// [Service] orchestrates authentication (register, login, token refresh)
// and encrypted secret management. The server stores only client-encrypted
// blobs; decryption happens exclusively on the client.
package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zhebrikov/gophkeeper/internal/auth"
	"github.com/zhebrikov/gophkeeper/internal/crypto"
	domainerrors "github.com/zhebrikov/gophkeeper/internal/domain/errors"
	"github.com/zhebrikov/gophkeeper/internal/domain/models"
	"github.com/zhebrikov/gophkeeper/internal/repository"
)

// Service implements GophKeeper business operations.
type Service struct {
	repo       repository.Repository
	jwt        *auth.Manager
	refreshTTL time.Duration
}

// New creates a Service with the given dependencies.
func New(repo repository.Repository, jwt *auth.Manager, refreshTTL time.Duration) *Service {
	return &Service{
		repo:       repo,
		jwt:        jwt,
		refreshTTL: refreshTTL,
	}
}

// Register creates a new user account and returns authentication tokens.
func (s *Service) Register(ctx context.Context, login, password string) (*models.TokenPair, error) {
	login = strings.TrimSpace(login)
	if login == "" || len(password) < 6 {
		return nil, domainerrors.ErrInvalidInput
	}

	hash, err := crypto.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.repo.CreateUser(ctx, login, hash)
	if err != nil {
		return nil, err
	}

	return s.issueTokens(ctx, user.ID)
}

// Login authenticates a user and returns authentication tokens.
func (s *Service) Login(ctx context.Context, login, password string) (*models.TokenPair, error) {
	user, err := s.repo.GetUserByLogin(ctx, strings.TrimSpace(login))
	if err != nil {
		if errors.Is(err, domainerrors.ErrNotFound) {
			return nil, domainerrors.ErrInvalidCredentials
		}
		return nil, err
	}

	if !crypto.CheckPassword(user.PasswordHash, password) {
		return nil, domainerrors.ErrInvalidCredentials
	}

	return s.issueTokens(ctx, user.ID)
}

// RefreshToken validates a refresh token and issues a new token pair.
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*models.TokenPair, error) {
	tokenHash := crypto.HashToken(refreshToken)
	stored, err := s.repo.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		return nil, domainerrors.ErrUnauthorized
	}

	if time.Now().After(stored.ExpiresAt) {
		_ = s.repo.RevokeRefreshToken(ctx, tokenHash)
		return nil, domainerrors.ErrUnauthorized
	}

	_ = s.repo.RevokeRefreshToken(ctx, tokenHash)
	return s.issueTokens(ctx, stored.UserID)
}

// CreateSecretInput holds parameters for creating a secret.
type CreateSecretInput struct {
	// UserID is the owner of the new secret.
	UserID uuid.UUID
	// Type describes the kind of secret data.
	Type models.SecretType
	// Name is a human-readable label.
	Name string
	// EncryptedData is the client-encrypted secret payload.
	EncryptedData []byte
	// Metadata stores optional plaintext context.
	Metadata string
}

// CreateSecret stores a new encrypted secret for the user.
func (s *Service) CreateSecret(ctx context.Context, in CreateSecretInput) (*models.Secret, error) {
	if in.UserID == uuid.Nil || in.Name == "" || len(in.EncryptedData) == 0 {
		return nil, domainerrors.ErrInvalidInput
	}

	secret := &models.Secret{
		UserID:        in.UserID,
		Type:          in.Type,
		Name:          in.Name,
		EncryptedData: in.EncryptedData,
		Metadata:      in.Metadata,
	}

	return s.repo.CreateSecret(ctx, secret)
}

// UpdateSecretInput holds parameters for updating a secret.
type UpdateSecretInput struct {
	// UserID is the owner of the secret.
	UserID uuid.UUID
	// ID is the secret identifier.
	ID uuid.UUID
	// Name is the updated label.
	Name string
	// EncryptedData is the updated encrypted payload.
	EncryptedData []byte
	// Metadata is the updated plaintext context.
	Metadata string
	// Version is the expected version for optimistic locking.
	Version int64
}

// UpdateSecret updates an existing secret with optimistic locking.
func (s *Service) UpdateSecret(ctx context.Context, in UpdateSecretInput) (*models.Secret, error) {
	if in.UserID == uuid.Nil || in.ID == uuid.Nil || in.Name == "" || len(in.EncryptedData) == 0 {
		return nil, domainerrors.ErrInvalidInput
	}

	existing, err := s.repo.GetSecret(ctx, in.UserID, in.ID)
	if err != nil {
		return nil, err
	}
	if existing.IsDeleted() {
		return nil, domainerrors.ErrNotFound
	}

	existing.Name = in.Name
	existing.EncryptedData = in.EncryptedData
	existing.Metadata = in.Metadata
	existing.Version = in.Version

	return s.repo.UpdateSecret(ctx, existing)
}

// GetSecret returns a secret by ID for the authenticated user.
func (s *Service) GetSecret(ctx context.Context, userID, secretID uuid.UUID) (*models.Secret, error) {
	secret, err := s.repo.GetSecret(ctx, userID, secretID)
	if err != nil {
		return nil, err
	}
	if secret.IsDeleted() {
		return nil, domainerrors.ErrNotFound
	}
	return secret, nil
}

// ListSecrets returns all secrets for a user, optionally since a timestamp.
func (s *Service) ListSecrets(ctx context.Context, userID uuid.UUID, since time.Time) ([]*models.Secret, error) {
	return s.repo.ListSecrets(ctx, userID, since)
}

// DeleteSecret soft-deletes a secret.
func (s *Service) DeleteSecret(ctx context.Context, userID, secretID uuid.UUID) error {
	return s.repo.DeleteSecret(ctx, userID, secretID)
}

// Sync returns all secrets changed since the given timestamp.
func (s *Service) Sync(ctx context.Context, userID uuid.UUID, lastSyncAt time.Time) ([]*models.Secret, time.Time, error) {
	secrets, err := s.repo.ListSecrets(ctx, userID, lastSyncAt)
	if err != nil {
		return nil, time.Time{}, err
	}
	return secrets, time.Now().UTC(), nil
}

func (s *Service) issueTokens(ctx context.Context, userID uuid.UUID) (*models.TokenPair, error) {
	accessToken, err := s.jwt.GenerateAccessToken(userID)
	if err != nil {
		return nil, err
	}

	refreshToken, err := generateRefreshToken()
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(s.refreshTTL)
	if err := s.repo.SaveRefreshToken(ctx, userID, crypto.HashToken(refreshToken), expiresAt); err != nil {
		return nil, err
	}

	return &models.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		UserID:       userID,
	}, nil
}

func generateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
