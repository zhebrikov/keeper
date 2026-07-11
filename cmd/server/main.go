// Command server runs the GophKeeper gRPC API server.
//
// Configuration is loaded from environment variables (see internal/config).
// The process shuts down gracefully on SIGINT, SIGTERM, or SIGQUIT.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	_ "github.com/lib/pq"

	"github.com/zhebrikov/gophkeeper/internal/app/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	if err := server.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
