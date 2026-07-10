package client

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/zhebrikov/gophkeeper/internal/config"
	"github.com/zhebrikov/gophkeeper/internal/tui"
)

func newTUICmd(ctx context.Context, cfg config.ClientConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch interactive terminal UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.Run(ctx, cfg)
		},
	}
}
