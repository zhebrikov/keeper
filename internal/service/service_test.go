package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zhebrikov/gophkeeper/internal/auth"
	"github.com/zhebrikov/gophkeeper/internal/crypto"
	domainerrors "github.com/zhebrikov/gophkeeper/internal/domain/errors"
	"github.com/zhebrikov/gophkeeper/internal/domain/models"
	"github.com/zhebrikov/gophkeeper/internal/service"
)

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
	secret.CreatedAt = time.Now().UTC()
	secret.UpdatedAt = secret.CreatedAt
	m.secrets[secret.ID] = secret
	return secret, nil
}

func (m *mockRepo) GetSecret(ctx context.Context, userID, secretID uuid.UUID) (*models.Secret, error) {
	secret, ok := m.secrets[secretID]
	if !ok || secret.UserID != userID {
		return nil, domainerrors.ErrNotFound
	}
	return secret, nil
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

func newTestService(repo *mockRepo) *service.Service {
	jwt := auth.NewManager("test-secret", time.Minute)
	return service.New(repo, jwt, time.Hour)
}

func TestRegisterAndLogin(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo)
	ctx := context.Background()

	tokens, err := svc.Register(ctx, "alice", "password1")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatal("expected tokens")
	}

	tokens2, err := svc.Login(ctx, "alice", "password1")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if tokens2.AccessToken == "" {
		t.Fatal("expected access token")
	}
}

func TestRegisterDuplicate(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo)
	ctx := context.Background()

	_, err := svc.Register(ctx, "alice", "password1")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, err = svc.Register(ctx, "alice", "password2")
	if !errors.Is(err, domainerrors.ErrAlreadyExists) {
		t.Fatalf("got %v, want ErrAlreadyExists", err)
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo)
	ctx := context.Background()

	_, err := svc.Register(ctx, "alice", "password1")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, err = svc.Login(ctx, "alice", "wrong")
	if !errors.Is(err, domainerrors.ErrInvalidCredentials) {
		t.Fatalf("got %v, want ErrInvalidCredentials", err)
	}
}

func TestRefreshToken(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo)
	ctx := context.Background()

	tokens, err := svc.Register(ctx, "alice", "password1")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	newTokens, err := svc.RefreshToken(ctx, tokens.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if newTokens.AccessToken == "" {
		t.Fatal("expected new access token")
	}
}

func TestSecretCRUD(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo)
	ctx := context.Background()

	tokens, err := svc.Register(ctx, "alice", "password1")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	_ = tokens

	encrypted, err := crypto.EncryptWithPassword("master", []byte("secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	user := repo.users["alice"]
	created, err := svc.CreateSecret(ctx, service.CreateSecretInput{
		UserID:        user.ID,
		Type:          models.SecretTypeText,
		Name:          "note",
		EncryptedData: encrypted,
		Metadata:      "meta",
	})
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	got, err := svc.GetSecret(ctx, user.ID, created.ID)
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got.Name != "note" {
		t.Fatalf("got name %q", got.Name)
	}

	updated, err := svc.UpdateSecret(ctx, service.UpdateSecretInput{
		UserID:        user.ID,
		ID:            created.ID,
		Name:          "note-v2",
		EncryptedData: encrypted,
		Metadata:      "meta2",
		Version:       created.Version,
	})
	if err != nil {
		t.Fatalf("UpdateSecret: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("expected version 2, got %d", updated.Version)
	}

	secrets, syncAt, err := svc.Sync(ctx, user.ID, time.Time{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(secrets) == 0 || syncAt.IsZero() {
		t.Fatal("expected sync results")
	}

	if err := svc.DeleteSecret(ctx, user.ID, created.ID); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	_, err = svc.GetSecret(ctx, user.ID, created.ID)
	if !errors.Is(err, domainerrors.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestCreateSecretInvalidInput(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo)
	_, err := svc.CreateSecret(context.Background(), service.CreateSecretInput{})
	if !errors.Is(err, domainerrors.ErrInvalidInput) {
		t.Fatalf("got %v, want ErrInvalidInput", err)
	}
}
