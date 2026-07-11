package client

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/zhebrikov/gophkeeper/api/gen/gophkeeper/v1"
	"github.com/zhebrikov/gophkeeper/internal/auth"
	gkclient "github.com/zhebrikov/gophkeeper/internal/client"
	"github.com/zhebrikov/gophkeeper/internal/config"
	"github.com/zhebrikov/gophkeeper/internal/crypto"
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

func startTestServer(t *testing.T) *testServer {
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

	return &testServer{listener: lis, server: grpcServer}
}

type testEnv struct {
	ctx context.Context
	cfg config.ClientConfig
}

func setupTestEnv(t *testing.T) testEnv {
	t.Helper()

	ts := startTestServer(t)
	cfg := config.ClientConfig{
		ServerAddr: "passthrough:///bufnet",
		ConfigPath: t.TempDir(),
		Insecure:   true,
	}

	dial := func() *grpc.ClientConn {
		conn, err := grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return ts.listener.Dial()
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		require.NoError(t, err)
		return conn
	}

	origFactory := gophkeeperClientFactory
	gophkeeperClientFactory = func(c config.ClientConfig) (*gophkeeperClient, error) {
		return gkclient.NewWithConn(dial(), sessionPath(c))
	}
	t.Cleanup(func() {
		gophkeeperClientFactory = origFactory
	})

	return testEnv{
		ctx: context.Background(),
		cfg: cfg,
	}
}

func stubExit(t *testing.T) {
	t.Helper()
	orig := exitFunc
	exitFunc = func(code int) {
		panic(code)
	}
	t.Cleanup(func() {
		exitFunc = orig
	})
}

func withStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)

	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = r.Close()
	})

	_, err = w.WriteString(input)
	require.NoError(t, err)
	require.NoError(t, w.Close())
}

func executeCmd(t *testing.T, cmd *cobra.Command, args ...string) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	oldStdout := os.Stdout
	os.Stdout = w

	var errBuf bytes.Buffer
	cmd.SetArgs(args)
	cmd.SetErr(&errBuf)
	execErr := cmd.Execute()

	require.NoError(t, w.Close())
	os.Stdout = oldStdout

	var outBuf bytes.Buffer
	_, err = outBuf.ReadFrom(r)
	require.NoError(t, err)
	require.NoError(t, execErr)
	return outBuf.String()
}

func loginTestUser(t *testing.T, env testEnv, login, password string) {
	t.Helper()
	api, err := newGophkeeperClient(env.cfg)
	require.NoError(t, err)
	require.NoError(t, api.Register(env.ctx, login, password))
	require.NoError(t, api.Close())
}

func TestSessionPath(t *testing.T) {
	cfg := config.ClientConfig{ConfigPath: "/tmp/gophkeeper"}
	require.Equal(t, filepath.Join("/tmp/gophkeeper", "session.json"), sessionPath(cfg))
}

func TestRequireAuth(t *testing.T) {
	env := setupTestEnv(t)

	_, err := requireAuth(env.cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not authenticated")

	loginTestUser(t, env, "alice", "password1")

	api, err := requireAuth(env.cfg)
	require.NoError(t, err)
	require.True(t, api.IsAuthenticated())
	require.NoError(t, api.Close())
}

func TestReadPasswordAndLine(t *testing.T) {
	withStdin(t, "alice\nsecret\n")

	login, err := readLine("Login: ")
	require.NoError(t, err)
	require.Equal(t, "alice", login)

	password, err := readPassword("Password: ")
	require.NoError(t, err)
	require.Equal(t, "secret", password)
}

func TestVersionCmd(t *testing.T) {
	stdout := executeCmd(t, newVersionCmd())
	require.Contains(t, stdout, "gophkeeper-client")
}

func TestRegisterCmd(t *testing.T) {
	env := setupTestEnv(t)
	stdout := executeCmd(t, newRegisterCmd(env.ctx, env.cfg),
		"--login", "alice", "--password", "password1")
	require.Contains(t, stdout, "Registration successful")

	api, err := newGophkeeperClient(env.cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = api.Close() })
	require.True(t, api.IsAuthenticated())
}

func TestLoginCmd(t *testing.T) {
	env := setupTestEnv(t)

	api, err := newGophkeeperClient(env.cfg)
	require.NoError(t, err)
	require.NoError(t, api.Register(env.ctx, "alice", "password1"))
	require.NoError(t, api.Logout())
	require.NoError(t, api.Close())

	stdout := executeCmd(t, newLoginCmd(env.ctx, env.cfg),
		"--login", "alice", "--password", "password1")
	require.Contains(t, stdout, "Login successful")
}

func TestLogoutCmd(t *testing.T) {
	env := setupTestEnv(t)
	loginTestUser(t, env, "alice", "password1")

	stdout := executeCmd(t, newLogoutCmd(env.cfg))
	require.Contains(t, stdout, "Logged out")

	api, err := newGophkeeperClient(env.cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = api.Close() })
	require.False(t, api.IsAuthenticated())
}

func TestListCmd(t *testing.T) {
	env := setupTestEnv(t)
	loginTestUser(t, env, "alice", "password1")

	api, err := newGophkeeperClient(env.cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = api.Close() })

	_, err = api.CreateSecret(env.ctx, &pb.CreateSecretRequest{
		Type:          pb.SecretType_SECRET_TYPE_TEXT,
		Name:          "note",
		EncryptedData: []byte("encrypted"),
		Metadata:      "meta",
	})
	require.NoError(t, err)
	require.NoError(t, api.Close())

	stdout := executeCmd(t, newListCmd(env.ctx, env.cfg))
	require.Contains(t, stdout, "ID")
	require.Contains(t, stdout, "TYPE")
	require.Contains(t, stdout, "NAME")
	require.Contains(t, stdout, "note")
	require.Contains(t, stdout, "text")
}

func TestListCmdRequiresAuth(t *testing.T) {
	env := setupTestEnv(t)
	stubExit(t)

	cmd := newListCmd(env.ctx, env.cfg)
	cmd.SetArgs(nil)

	require.Panics(t, func() {
		_ = cmd.Execute()
	})
}

func TestAddCmd(t *testing.T) {
	env := setupTestEnv(t)
	loginTestUser(t, env, "alice", "password1")
	withStdin(t, "master\n")

	stdout := executeCmd(t, newAddCmd(env.ctx, env.cfg),
		"--type", "text", "--name", "note", "--data", "hello", "--metadata", "meta")
	require.Contains(t, stdout, "Secret created")

	api, err := newGophkeeperClient(env.cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = api.Close() })

	list, err := api.ListSecrets(env.ctx, time.Time{})
	require.NoError(t, err)
	require.Len(t, list.GetSecrets(), 1)
	require.Equal(t, "note", list.GetSecrets()[0].GetName())
}

func TestGetCmd(t *testing.T) {
	env := setupTestEnv(t)
	loginTestUser(t, env, "alice", "password1")

	const master = "masterpass"
	encrypted, err := crypto.EncryptWithPassword(master, []byte("secret data"))
	require.NoError(t, err)

	api, err := newGophkeeperClient(env.cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = api.Close() })

	created, err := api.CreateSecret(env.ctx, &pb.CreateSecretRequest{
		Type:          pb.SecretType_SECRET_TYPE_TEXT,
		Name:          "note",
		EncryptedData: encrypted,
		Metadata:      "meta",
	})
	require.NoError(t, err)

	withStdin(t, master+"\n")
	stdout := executeCmd(t, newGetCmd(env.ctx, env.cfg), created.GetId())
	require.Contains(t, stdout, "Name:     note")
	require.Contains(t, stdout, "Data:     secret data")
}

func TestUpdateCmd(t *testing.T) {
	env := setupTestEnv(t)
	loginTestUser(t, env, "alice", "password1")

	const master = "masterpass"
	encrypted, err := crypto.EncryptWithPassword(master, []byte("old data"))
	require.NoError(t, err)

	api, err := newGophkeeperClient(env.cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = api.Close() })

	created, err := api.CreateSecret(env.ctx, &pb.CreateSecretRequest{
		Type:          pb.SecretType_SECRET_TYPE_TEXT,
		Name:          "note",
		EncryptedData: encrypted,
	})
	require.NoError(t, err)

	withStdin(t, master+"\n")
	stdout := executeCmd(t, newUpdateCmd(env.ctx, env.cfg),
		created.GetId(), "--data", "new data")
	require.Contains(t, stdout, "Secret updated")
}

func TestDeleteCmd(t *testing.T) {
	env := setupTestEnv(t)
	loginTestUser(t, env, "alice", "password1")

	api, err := newGophkeeperClient(env.cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = api.Close() })

	created, err := api.CreateSecret(env.ctx, &pb.CreateSecretRequest{
		Type:          pb.SecretType_SECRET_TYPE_TEXT,
		Name:          "note",
		EncryptedData: []byte("encrypted"),
	})
	require.NoError(t, err)

	stdout := executeCmd(t, newDeleteCmd(env.ctx, env.cfg), created.GetId())
	require.Contains(t, stdout, "Secret deleted")
}

func TestSyncCmd(t *testing.T) {
	env := setupTestEnv(t)
	loginTestUser(t, env, "alice", "password1")

	api, err := newGophkeeperClient(env.cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = api.Close() })

	_, err = api.CreateSecret(env.ctx, &pb.CreateSecretRequest{
		Type:          pb.SecretType_SECRET_TYPE_TEXT,
		Name:          "note",
		EncryptedData: []byte("encrypted"),
	})
	require.NoError(t, err)

	stdout := executeCmd(t, newSyncCmd(env.ctx, env.cfg))
	require.True(t, strings.HasPrefix(stdout, "Synced "))
}

func TestRegisterCmdInteractive(t *testing.T) {
	env := setupTestEnv(t)
	withStdin(t, "bob\nsecret\n")

	stdout := executeCmd(t, newRegisterCmd(env.ctx, env.cfg))
	require.Contains(t, stdout, "Registration successful")
}
