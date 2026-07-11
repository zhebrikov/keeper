// Package server contains the GophKeeper API server bootstrap.
//
// Run loads configuration, connects to PostgreSQL, registers gRPC handlers,
// and serves until the context is cancelled. Protected RPC methods require
// a Bearer JWT in the authorization metadata header.
package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pb "github.com/zhebrikov/gophkeeper/api/gen/gophkeeper/v1"
	"github.com/zhebrikov/gophkeeper/internal/auth"
	"github.com/zhebrikov/gophkeeper/internal/config"
	"github.com/zhebrikov/gophkeeper/internal/gateway"
	grpchandler "github.com/zhebrikov/gophkeeper/internal/handler/grpc"
	"github.com/zhebrikov/gophkeeper/internal/repository/postgres"
	"github.com/zhebrikov/gophkeeper/internal/service"
	"github.com/zhebrikov/gophkeeper/internal/swagger"
)

// Run starts the GophKeeper gRPC API server and blocks until ctx is cancelled.
func Run(ctx context.Context) error {
	cfg, err := config.LoadServer()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	repo, err := postgres.New(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer repo.Close()

	jwtManager := auth.NewManager(cfg.JWTSecret, cfg.AccessTokenTTL)
	svc := service.New(repo, jwtManager, cfg.RefreshTokenTTL)
	handler := grpchandler.NewHandler(svc, jwtManager)

	serverOpts := []grpc.ServerOption{
		grpc.UnaryInterceptor(grpchandler.AuthUnaryInterceptor(jwtManager)),
	}
	if cfg.TLSCertFile != "" {
		creds, err := credentials.NewServerTLSFromFile(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return fmt.Errorf("load TLS credentials: %w", err)
		}
		serverOpts = append(serverOpts, grpc.Creds(creds))
	}

	grpcServer := grpc.NewServer(serverOpts...)
	pb.RegisterGophKeeperServer(grpcServer, handler)

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.GRPCAddr, err)
	}

	httpServer, err := newHTTPServer(ctx, cfg.HTTPAddr, handler)
	if err != nil {
		return fmt.Errorf("init http server: %w", err)
	}

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if cfg.TLSCertFile != "" {
			log.Printf("gophkeeper server listening on %s (TLS)", cfg.GRPCAddr)
		} else {
			log.Printf("gophkeeper server listening on %s (insecure)", cfg.GRPCAddr)
		}
		return grpcServer.Serve(lis)
	})

	g.Go(func() error {
		log.Printf("swagger UI available at http://localhost%s/swagger/", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	g.Go(func() error {
		<-gctx.Done()
		log.Println("shutting down server...")
		grpcServer.GracefulStop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown http server: %w", err)
		}
		return nil
	})

	return g.Wait()
}

func newHTTPServer(ctx context.Context, httpAddr string, handler pb.GophKeeperServer) (*http.Server, error) {
	gw, err := gateway.NewHandler(ctx, handler)
	if err != nil {
		return nil, err
	}

	swaggerHandler, err := swagger.Handler()
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/openapi.json", swaggerHandler)
	mux.Handle("/swagger/", http.StripPrefix("/swagger", swaggerHandler))
	mux.Handle("/", gw)

	return &http.Server{
		Addr:    httpAddr,
		Handler: mux,
	}, nil
}
