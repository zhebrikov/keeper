// Package client provides the gRPC client for communicating with the GophKeeper server.
//
// [API] manages the gRPC connection, attaches JWT authorization headers,
// and persists session state via [storage.Store].
package client

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	pb "github.com/zhebrikov/gophkeeper/api/gen/gophkeeper/v1"
	"github.com/zhebrikov/gophkeeper/internal/client/storage"
	"github.com/zhebrikov/gophkeeper/internal/config"
)

// API wraps the gRPC client and session management.
type API struct {
	conn    *grpc.ClientConn
	client  pb.GophKeeperClient
	store   *storage.Store
	session *storage.Session
}

// New creates a new API client connected to the configured server address.
func New(cfg config.ClientConfig) (*API, error) {
	creds, err := transportCredentials(cfg)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(cfg.ServerAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("dial server: %w", err)
	}

	api, err := newAPI(conn, filepath.Join(cfg.ConfigPath, "session.json"))
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return api, nil
}

// NewWithConn creates an API client using an existing gRPC connection.
// It is intended for testing with in-memory transports such as bufconn.
func NewWithConn(conn *grpc.ClientConn, configPath string) (*API, error) {
	return newAPI(conn, configPath)
}

func newAPI(conn *grpc.ClientConn, configPath string) (*API, error) {
	store := storage.New(configPath)
	session, err := store.Load()
	if err != nil {
		return nil, err
	}

	return &API{
		conn:    conn,
		client:  pb.NewGophKeeperClient(conn),
		store:   store,
		session: session,
	}, nil
}

// Close closes the underlying gRPC connection.
func (a *API) Close() error {
	return a.conn.Close()
}

// Register creates a new account and stores the session.
func (a *API) Register(ctx context.Context, login, password string) error {
	resp, err := a.client.Register(ctx, &pb.RegisterRequest{Login: login, Password: password})
	if err != nil {
		return err
	}
	return a.saveSession(resp)
}

// Login authenticates and stores the session.
func (a *API) Login(ctx context.Context, login, password string) error {
	resp, err := a.client.Login(ctx, &pb.LoginRequest{Login: login, Password: password})
	if err != nil {
		return err
	}
	return a.saveSession(resp)
}

// Logout clears the local session.
func (a *API) Logout() error {
	a.session = nil
	return a.store.Clear()
}

// IsAuthenticated reports whether the client has a stored session.
func (a *API) IsAuthenticated() bool {
	return a.session != nil && a.session.AccessToken != ""
}

// CreateSecret sends a new encrypted secret to the server.
func (a *API) CreateSecret(ctx context.Context, req *pb.CreateSecretRequest) (*pb.SecretResponse, error) {
	return a.client.CreateSecret(a.authCtx(ctx), req)
}

// UpdateSecret updates an existing secret on the server.
func (a *API) UpdateSecret(ctx context.Context, req *pb.UpdateSecretRequest) (*pb.SecretResponse, error) {
	return a.client.UpdateSecret(a.authCtx(ctx), req)
}

// GetSecret retrieves a secret by ID.
func (a *API) GetSecret(ctx context.Context, id string) (*pb.SecretResponse, error) {
	return a.client.GetSecret(a.authCtx(ctx), &pb.GetSecretRequest{Id: id})
}

// ListSecrets returns secrets optionally filtered by update time.
func (a *API) ListSecrets(ctx context.Context, since time.Time) (*pb.ListSecretsResponse, error) {
	req := &pb.ListSecretsRequest{}
	if !since.IsZero() {
		req.UpdatedSince = since.Unix()
	}
	return a.client.ListSecrets(a.authCtx(ctx), req)
}

// DeleteSecret removes a secret on the server.
func (a *API) DeleteSecret(ctx context.Context, id string) error {
	_, err := a.client.DeleteSecret(a.authCtx(ctx), &pb.DeleteSecretRequest{Id: id})
	return err
}

// Sync synchronizes secrets with the server.
func (a *API) Sync(ctx context.Context) (*pb.SyncResponse, error) {
	req := &pb.SyncRequest{}
	if a.session != nil && !a.session.LastSyncAt.IsZero() {
		req.LastSyncAt = a.session.LastSyncAt.Unix()
	}

	resp, err := a.client.Sync(a.authCtx(ctx), req)
	if err != nil {
		return nil, err
	}

	if a.session != nil {
		a.session.LastSyncAt = time.Unix(resp.GetSyncAt(), 0).UTC()
		_ = a.store.Save(a.session)
	}
	return resp, nil
}

func (a *API) authCtx(ctx context.Context) context.Context {
	if a.session == nil {
		return ctx
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+a.session.AccessToken))
}

func (a *API) saveSession(resp *pb.AuthResponse) error {
	a.session = &storage.Session{
		AccessToken:  resp.GetAccessToken(),
		RefreshToken: resp.GetRefreshToken(),
		UserID:       resp.GetUserId(),
	}
	return a.store.Save(a.session)
}

func transportCredentials(cfg config.ClientConfig) (credentials.TransportCredentials, error) {
	if cfg.Insecure {
		return insecure.NewCredentials(), nil
	}
	if cfg.TLSEnabled || cfg.TLSCAFile != "" {
		if cfg.TLSCAFile == "" {
			return nil, fmt.Errorf("GOPHKEEPER_TLS_CA is required when TLS is enabled")
		}
		creds, err := credentials.NewClientTLSFromFile(cfg.TLSCAFile, "")
		if err != nil {
			return nil, fmt.Errorf("load TLS credentials: %w", err)
		}
		return creds, nil
	}
	return insecure.NewCredentials(), nil
}
