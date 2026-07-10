// Package gateway exposes the gRPC API over HTTP using grpc-gateway.
package gateway

import (
	"context"
	"fmt"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	pb "github.com/zhebrikov/gophkeeper/api/gen/gophkeeper/v1"
)

// NewHandler registers REST routes that invoke gRPC handlers in-process.
func NewHandler(ctx context.Context, srv pb.GophKeeperServer) (http.Handler, error) {
	mux := runtime.NewServeMux()
	if err := pb.RegisterGophKeeperHandlerServer(ctx, mux, srv); err != nil {
		return nil, fmt.Errorf("register gateway handlers: %w", err)
	}
	return mux, nil
}
