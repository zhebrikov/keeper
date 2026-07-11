package grpc_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/zhebrikov/gophkeeper/api/gen/gophkeeper/v1"
	"github.com/zhebrikov/gophkeeper/internal/auth"
	domainerrors "github.com/zhebrikov/gophkeeper/internal/domain/errors"
	"github.com/zhebrikov/gophkeeper/internal/domain/models"
	grpchandler "github.com/zhebrikov/gophkeeper/internal/handler/grpc"
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
	sinceUnix := since.Unix()
	for _, s := range m.secrets {
		if s.UserID == userID && (since.IsZero() || s.UpdatedAt.Unix() > sinceUnix) {
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

type testEnv struct {
	repo *mockRepo
	jwt  *auth.Manager
	svc  *service.Service
	h    *grpchandler.Handler
}

func newTestEnv() *testEnv {
	repo := newMockRepo()
	jwt := auth.NewManager("secret", time.Minute)
	svc := service.New(repo, jwt, time.Hour)
	h := grpchandler.NewHandler(svc, jwt)
	return &testEnv{repo: repo, jwt: jwt, svc: svc, h: h}
}

func (e *testEnv) authCtx(ctx context.Context, userID uuid.UUID) context.Context {
	return grpchandler.ContextWithUserID(ctx, userID)
}

func (e *testEnv) register(t *testing.T, login string) (uuid.UUID, string) {
	t.Helper()
	resp, err := e.h.Register(context.Background(), &pb.RegisterRequest{
		Login:    login,
		Password: "secret12",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetAccessToken())
	userID, err := uuid.Parse(resp.GetUserId())
	require.NoError(t, err)
	return userID, resp.GetAccessToken()
}

func TestRegisterHandler(t *testing.T) {
	e := newTestEnv()
	resp, err := e.h.Register(context.Background(), &pb.RegisterRequest{
		Login:    "bob",
		Password: "secret12",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetAccessToken())
	require.NotEmpty(t, resp.GetRefreshToken())
}

func TestRegisterDuplicate(t *testing.T) {
	e := newTestEnv()
	_, err := e.h.Register(context.Background(), &pb.RegisterRequest{Login: "bob", Password: "secret12"})
	require.NoError(t, err)

	_, err = e.h.Register(context.Background(), &pb.RegisterRequest{Login: "bob", Password: "secret12"})
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.AlreadyExists, st.Code())
}

func TestLoginHandler(t *testing.T) {
	e := newTestEnv()
	_, err := e.h.Register(context.Background(), &pb.RegisterRequest{Login: "bob", Password: "secret12"})
	require.NoError(t, err)

	resp, err := e.h.Login(context.Background(), &pb.LoginRequest{Login: "bob", Password: "secret12"})
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetAccessToken())
}

func TestLoginInvalidCredentials(t *testing.T) {
	e := newTestEnv()
	_, err := e.h.Register(context.Background(), &pb.RegisterRequest{Login: "bob", Password: "secret12"})
	require.NoError(t, err)

	_, err = e.h.Login(context.Background(), &pb.LoginRequest{Login: "bob", Password: "wrong"})
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Unauthenticated, st.Code())
}

func TestRefreshTokenHandler(t *testing.T) {
	e := newTestEnv()
	reg, err := e.h.Register(context.Background(), &pb.RegisterRequest{Login: "bob", Password: "secret12"})
	require.NoError(t, err)

	resp, err := e.h.RefreshToken(context.Background(), &pb.RefreshTokenRequest{
		RefreshToken: reg.GetRefreshToken(),
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetAccessToken())
}

func TestRefreshTokenInvalid(t *testing.T) {
	e := newTestEnv()
	_, err := e.h.RefreshToken(context.Background(), &pb.RefreshTokenRequest{
		RefreshToken: "invalid",
	})
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Unauthenticated, st.Code())
}

func TestGetSecretUnauthorized(t *testing.T) {
	e := newTestEnv()
	_, err := e.h.GetSecret(context.Background(), &pb.GetSecretRequest{Id: uuid.New().String()})
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Unauthenticated, st.Code())
}

func TestSecretCRUDHandlers(t *testing.T) {
	e := newTestEnv()
	ctx := context.Background()
	userID, _ := e.register(t, "alice")
	authCtx := e.authCtx(ctx, userID)

	created, err := e.h.CreateSecret(authCtx, &pb.CreateSecretRequest{
		Type:          pb.SecretType_SECRET_TYPE_TEXT,
		Name:          "note",
		EncryptedData: []byte("enc"),
		Metadata:      "meta",
	})
	require.NoError(t, err)
	require.Equal(t, "note", created.GetName())
	require.Equal(t, pb.SecretType_SECRET_TYPE_TEXT, created.GetType())

	got, err := e.h.GetSecret(authCtx, &pb.GetSecretRequest{Id: created.GetId()})
	require.NoError(t, err)
	require.Equal(t, created.GetId(), got.GetId())

	updated, err := e.h.UpdateSecret(authCtx, &pb.UpdateSecretRequest{
		Id:            created.GetId(),
		Name:          "note-v2",
		EncryptedData: []byte("enc2"),
		Metadata:      "meta2",
		Version:       created.GetVersion(),
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), updated.GetVersion())

	list, err := e.h.ListSecrets(authCtx, &pb.ListSecretsRequest{})
	require.NoError(t, err)
	require.Len(t, list.GetSecrets(), 1)

	syncResp, err := e.h.Sync(authCtx, &pb.SyncRequest{})
	require.NoError(t, err)
	require.NotZero(t, syncResp.GetSyncAt())
	require.Len(t, syncResp.GetSecrets(), 1)

	_, err = e.h.DeleteSecret(authCtx, &pb.DeleteSecretRequest{Id: created.GetId()})
	require.NoError(t, err)

	_, err = e.h.GetSecret(authCtx, &pb.GetSecretRequest{Id: created.GetId()})
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.NotFound, st.Code())
}

func TestInvalidSecretID(t *testing.T) {
	e := newTestEnv()
	userID, _ := e.register(t, "alice")
	authCtx := e.authCtx(context.Background(), userID)

	_, err := e.h.GetSecret(authCtx, &pb.GetSecretRequest{Id: "not-uuid"})
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.InvalidArgument, st.Code())

	_, err = e.h.UpdateSecret(authCtx, &pb.UpdateSecretRequest{Id: "bad"})
	st, ok = status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.InvalidArgument, st.Code())

	_, err = e.h.DeleteSecret(authCtx, &pb.DeleteSecretRequest{Id: "bad"})
	st, ok = status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.InvalidArgument, st.Code())
}

func TestUpdateSecretConflict(t *testing.T) {
	e := newTestEnv()
	userID, _ := e.register(t, "alice")
	authCtx := e.authCtx(context.Background(), userID)

	created, err := e.h.CreateSecret(authCtx, &pb.CreateSecretRequest{
		Type:          pb.SecretType_SECRET_TYPE_BINARY,
		Name:          "file",
		EncryptedData: []byte("data"),
	})
	require.NoError(t, err)

	_, err = e.h.UpdateSecret(authCtx, &pb.UpdateSecretRequest{
		Id:            created.GetId(),
		Name:          "file-v2",
		EncryptedData: []byte("data2"),
		Version:       99,
	})
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Aborted, st.Code())
}

func TestSecretTypeMapping(t *testing.T) {
	e := newTestEnv()
	userID, _ := e.register(t, "alice")
	authCtx := e.authCtx(context.Background(), userID)

	types := []pb.SecretType{
		pb.SecretType_SECRET_TYPE_CREDENTIAL,
		pb.SecretType_SECRET_TYPE_TEXT,
		pb.SecretType_SECRET_TYPE_BINARY,
		pb.SecretType_SECRET_TYPE_CARD,
		pb.SecretType_SECRET_TYPE_OTP,
		pb.SecretType_SECRET_TYPE_UNSPECIFIED,
	}
	for _, st := range types {
		created, err := e.h.CreateSecret(authCtx, &pb.CreateSecretRequest{
			Type:          st,
			Name:          st.String(),
			EncryptedData: []byte("x"),
		})
		require.NoError(t, err)
		require.NotEmpty(t, created.GetType())
	}
}

func TestListSecretsWithSince(t *testing.T) {
	e := newTestEnv()
	userID, _ := e.register(t, "alice")
	authCtx := e.authCtx(context.Background(), userID)

	old, err := e.h.CreateSecret(authCtx, &pb.CreateSecretRequest{
		Type:          pb.SecretType_SECRET_TYPE_TEXT,
		Name:          "old",
		EncryptedData: []byte("x"),
	})
	require.NoError(t, err)

	since := old.GetUpdatedAt()
	time.Sleep(time.Second)

	_, err = e.h.CreateSecret(authCtx, &pb.CreateSecretRequest{
		Type:          pb.SecretType_SECRET_TYPE_TEXT,
		Name:          "new",
		EncryptedData: []byte("y"),
	})
	require.NoError(t, err)

	list, err := e.h.ListSecrets(authCtx, &pb.ListSecretsRequest{UpdatedSince: since})
	require.NoError(t, err)
	require.Len(t, list.GetSecrets(), 1)
	require.Equal(t, "new", list.GetSecrets()[0].GetName())
}

func TestAuthUnaryInterceptor(t *testing.T) {
	e := newTestEnv()
	interceptor := grpchandler.AuthUnaryInterceptor(e.jwt)

	publicHandler := func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	}
	protectedHandler := func(ctx context.Context, req any) (any, error) {
		return "protected", nil
	}

	resp, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{
		FullMethod: "/gophkeeper.v1.GophKeeper/Login",
	}, publicHandler)
	require.NoError(t, err)
	require.Equal(t, "ok", resp)

	_, err = interceptor(context.Background(), nil, &grpc.UnaryServerInfo{
		FullMethod: "/gophkeeper.v1.GophKeeper/GetSecret",
	}, protectedHandler)
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Unauthenticated, st.Code())

	userID, accessToken := e.register(t, "alice")
	md := metadata.Pairs("authorization", "Bearer "+accessToken)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	resp, err = interceptor(ctx, nil, &grpc.UnaryServerInfo{
		FullMethod: "/gophkeeper.v1.GophKeeper/GetSecret",
	}, func(ctx context.Context, req any) (any, error) {
		return "protected", nil
	})
	require.NoError(t, err)
	require.Equal(t, "protected", resp)
	_ = userID
}

func TestAuthInterceptorWithInvalidToken(t *testing.T) {
	e := newTestEnv()
	interceptor := grpchandler.AuthUnaryInterceptor(e.jwt)

	md := metadata.Pairs("authorization", "Bearer invalid-token")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{
		FullMethod: "/gophkeeper.v1.GophKeeper/GetSecret",
	}, func(ctx context.Context, req any) (any, error) {
		return nil, nil
	})
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Unauthenticated, st.Code())
}

func TestContextWithUserID(t *testing.T) {
	e := newTestEnv()
	userID, _ := e.register(t, "alice")
	authCtx := e.authCtx(context.Background(), userID)

	_, err := e.h.GetSecret(authCtx, &pb.GetSecretRequest{Id: uuid.New().String()})
	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.NotFound, st.Code())
}
