// Package grpc provides gRPC transport handlers for the GophKeeper API.
//
// [Handler] delegates business logic to [service.Service] and maps domain
// errors to gRPC status codes. [AuthUnaryInterceptor] validates JWT tokens
// for all methods except Register, Login, and RefreshToken.
package grpc

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/zhebrikov/gophkeeper/api/gen/gophkeeper/v1"
	"github.com/zhebrikov/gophkeeper/internal/auth"
	domainerrors "github.com/zhebrikov/gophkeeper/internal/domain/errors"
	"github.com/zhebrikov/gophkeeper/internal/domain/models"
	"github.com/zhebrikov/gophkeeper/internal/service"
)

// Handler implements the GophKeeper gRPC service.
type Handler struct {
	pb.UnimplementedGophKeeperServer
	svc *service.Service
	jwt *auth.Manager
}

// NewHandler creates a gRPC handler with the given service and JWT manager.
func NewHandler(svc *service.Service, jwt *auth.Manager) *Handler {
	return &Handler{svc: svc, jwt: jwt}
}

// Register implements GophKeeper.Register.
func (h *Handler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.AuthResponse, error) {
	tokens, err := h.svc.Register(ctx, req.GetLogin(), req.GetPassword())
	if err != nil {
		return nil, mapError(err)
	}
	return toAuthResponse(tokens), nil
}

// Login implements GophKeeper.Login.
func (h *Handler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.AuthResponse, error) {
	tokens, err := h.svc.Login(ctx, req.GetLogin(), req.GetPassword())
	if err != nil {
		return nil, mapError(err)
	}
	return toAuthResponse(tokens), nil
}

// RefreshToken implements GophKeeper.RefreshToken.
func (h *Handler) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.AuthResponse, error) {
	tokens, err := h.svc.RefreshToken(ctx, req.GetRefreshToken())
	if err != nil {
		return nil, mapError(err)
	}
	return toAuthResponse(tokens), nil
}

// CreateSecret implements GophKeeper.CreateSecret.
func (h *Handler) CreateSecret(ctx context.Context, req *pb.CreateSecretRequest) (*pb.SecretResponse, error) {
	userID, err := h.userIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	secret, err := h.svc.CreateSecret(ctx, service.CreateSecretInput{
		UserID:        userID,
		Type:          fromProtoSecretType(req.GetType()),
		Name:          req.GetName(),
		EncryptedData: req.GetEncryptedData(),
		Metadata:      req.GetMetadata(),
	})
	if err != nil {
		return nil, mapError(err)
	}
	return toSecretResponse(secret), nil
}

// UpdateSecret implements GophKeeper.UpdateSecret.
func (h *Handler) UpdateSecret(ctx context.Context, req *pb.UpdateSecretRequest) (*pb.SecretResponse, error) {
	userID, err := h.userIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	secretID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid secret id")
	}

	secret, err := h.svc.UpdateSecret(ctx, service.UpdateSecretInput{
		UserID:        userID,
		ID:            secretID,
		Name:          req.GetName(),
		EncryptedData: req.GetEncryptedData(),
		Metadata:      req.GetMetadata(),
		Version:       req.GetVersion(),
	})
	if err != nil {
		return nil, mapError(err)
	}
	return toSecretResponse(secret), nil
}

// GetSecret implements GophKeeper.GetSecret.
func (h *Handler) GetSecret(ctx context.Context, req *pb.GetSecretRequest) (*pb.SecretResponse, error) {
	userID, err := h.userIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	secretID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid secret id")
	}

	secret, err := h.svc.GetSecret(ctx, userID, secretID)
	if err != nil {
		return nil, mapError(err)
	}
	return toSecretResponse(secret), nil
}

// ListSecrets implements GophKeeper.ListSecrets.
func (h *Handler) ListSecrets(ctx context.Context, req *pb.ListSecretsRequest) (*pb.ListSecretsResponse, error) {
	userID, err := h.userIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	var since time.Time
	if req.GetUpdatedSince() > 0 {
		since = time.Unix(req.GetUpdatedSince(), 0).UTC()
	}

	secrets, err := h.svc.ListSecrets(ctx, userID, since)
	if err != nil {
		return nil, mapError(err)
	}

	resp := &pb.ListSecretsResponse{}
	for _, s := range secrets {
		resp.Secrets = append(resp.Secrets, toSecretResponse(s))
	}
	return resp, nil
}

// DeleteSecret implements GophKeeper.DeleteSecret.
func (h *Handler) DeleteSecret(ctx context.Context, req *pb.DeleteSecretRequest) (*pb.DeleteSecretResponse, error) {
	userID, err := h.userIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	secretID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid secret id")
	}

	if err := h.svc.DeleteSecret(ctx, userID, secretID); err != nil {
		return nil, mapError(err)
	}
	return &pb.DeleteSecretResponse{}, nil
}

// Sync implements GophKeeper.Sync.
func (h *Handler) Sync(ctx context.Context, req *pb.SyncRequest) (*pb.SyncResponse, error) {
	userID, err := h.userIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	var lastSync time.Time
	if req.GetLastSyncAt() > 0 {
		lastSync = time.Unix(req.GetLastSyncAt(), 0).UTC()
	}

	secrets, syncAt, err := h.svc.Sync(ctx, userID, lastSync)
	if err != nil {
		return nil, mapError(err)
	}

	resp := &pb.SyncResponse{SyncAt: syncAt.Unix()}
	for _, s := range secrets {
		resp.Secrets = append(resp.Secrets, toSecretResponse(s))
	}
	return resp, nil
}

// AuthUnaryInterceptor validates JWT tokens for protected RPC methods.
func AuthUnaryInterceptor(jwt *auth.Manager) grpc.UnaryServerInterceptor {
	public := map[string]bool{
		"/gophkeeper.v1.GophKeeper/Register":     true,
		"/gophkeeper.v1.GophKeeper/Login":        true,
		"/gophkeeper.v1.GophKeeper/RefreshToken": true,
	}

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if public[info.FullMethod] {
			return handler(ctx, req)
		}

		userID, err := userIDFromMetadata(ctx, jwt)
		if err != nil {
			return nil, err
		}
		ctx = ContextWithUserID(ctx, userID)
		return handler(ctx, req)
	}
}

type userIDKey struct{}

// ContextWithUserID returns a context carrying the authenticated user ID.
func ContextWithUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, userIDKey{}, userID)
}

func (h *Handler) userIDFromContext(ctx context.Context) (uuid.UUID, error) {
	userID, ok := ctx.Value(userIDKey{}).(uuid.UUID)
	if ok && userID != uuid.Nil {
		return userID, nil
	}

	// grpc-gateway invokes handlers in-process without gRPC interceptors.
	userID, err := userIDFromMetadata(ctx, h.jwt)
	if err != nil {
		return uuid.Nil, err
	}
	return userID, nil
}

func userIDFromMetadata(ctx context.Context, jwt *auth.Manager) (uuid.UUID, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return uuid.Nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		return uuid.Nil, status.Error(codes.Unauthenticated, "missing authorization header")
	}

	token := strings.TrimPrefix(values[0], "Bearer ")
	userID, err := jwt.ValidateAccessToken(token)
	if err != nil {
		return uuid.Nil, status.Error(codes.Unauthenticated, "invalid token")
	}
	return userID, nil
}

func toAuthResponse(tokens *models.TokenPair) *pb.AuthResponse {
	return &pb.AuthResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		UserId:       tokens.UserID.String(),
	}
}

func toSecretResponse(secret *models.Secret) *pb.SecretResponse {
	return &pb.SecretResponse{
		Id:            secret.ID.String(),
		Type:          toProtoSecretType(secret.Type),
		Name:          secret.Name,
		EncryptedData: secret.EncryptedData,
		Metadata:      secret.Metadata,
		Version:       secret.Version,
		CreatedAt:     secret.CreatedAt.Unix(),
		UpdatedAt:     secret.UpdatedAt.Unix(),
		Deleted:       secret.IsDeleted(),
	}
}

func toProtoSecretType(t models.SecretType) pb.SecretType {
	switch t {
	case models.SecretTypeCredential:
		return pb.SecretType_SECRET_TYPE_CREDENTIAL
	case models.SecretTypeText:
		return pb.SecretType_SECRET_TYPE_TEXT
	case models.SecretTypeBinary:
		return pb.SecretType_SECRET_TYPE_BINARY
	case models.SecretTypeCard:
		return pb.SecretType_SECRET_TYPE_CARD
	case models.SecretTypeOTP:
		return pb.SecretType_SECRET_TYPE_OTP
	default:
		return pb.SecretType_SECRET_TYPE_UNSPECIFIED
	}
}

func fromProtoSecretType(t pb.SecretType) models.SecretType {
	switch t {
	case pb.SecretType_SECRET_TYPE_CREDENTIAL:
		return models.SecretTypeCredential
	case pb.SecretType_SECRET_TYPE_TEXT:
		return models.SecretTypeText
	case pb.SecretType_SECRET_TYPE_BINARY:
		return models.SecretTypeBinary
	case pb.SecretType_SECRET_TYPE_CARD:
		return models.SecretTypeCard
	case pb.SecretType_SECRET_TYPE_OTP:
		return models.SecretTypeOTP
	default:
		return models.SecretTypeText
	}
}

func mapError(err error) error {
	switch {
	case errors.Is(err, domainerrors.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domainerrors.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domainerrors.ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, domainerrors.ErrUnauthorized):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, domainerrors.ErrConflict):
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, domainerrors.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
