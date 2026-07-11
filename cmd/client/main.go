// Command client runs the GophKeeper CLI password manager.
//
// See internal/app/client for available subcommands and flags.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/zhebrikov/gophkeeper/internal/app/client"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	if err := client.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
