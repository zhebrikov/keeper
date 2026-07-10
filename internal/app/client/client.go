// Package client contains the GophKeeper CLI application bootstrap.
//
// Commands:
//   - version — print build version and date
//   - register, login, logout — account management
//   - add, get, list, update, delete — secret CRUD
//   - sync — incremental synchronization with the server
//
// Secrets are encrypted locally with a master password before upload.
// Session tokens are persisted in session.json under the config directory.
package client

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zhebrikov/gophkeeper/internal/config"
	"github.com/zhebrikov/gophkeeper/internal/version"
)

// Run starts the CLI client and blocks until the root command completes.
func Run(ctx context.Context) error {
	cfg := config.LoadClient()

	root := &cobra.Command{
		Use:   "gophkeeper",
		Short: "GophKeeper password manager CLI",
	}

	root.AddCommand(
		newVersionCmd(),
		newRegisterCmd(ctx, cfg),
		newLoginCmd(ctx, cfg),
		newLogoutCmd(cfg),
		newTUICmd(ctx, cfg),
		newSyncCmd(ctx, cfg),
		newListCmd(ctx, cfg),
		newGetCmd(ctx, cfg),
		newAddCmd(ctx, cfg),
		newUpdateCmd(ctx, cfg),
		newDeleteCmd(ctx, cfg),
	)

	return root.Execute()
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print client version and build date",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("gophkeeper-client %s (built %s)\n", version.Version, version.BuildDate)
		},
	}
}

func sessionPath(cfg config.ClientConfig) string {
	return filepath.Join(cfg.ConfigPath, "session.json")
}

func requireAuth(cfg config.ClientConfig) (*gophkeeperClient, error) {
	api, err := newGophkeeperClient(cfg)
	if err != nil {
		return nil, err
	}
	if !api.IsAuthenticated() {
		_ = api.Close()
		return nil, fmt.Errorf("not authenticated: run 'login' first")
	}
	return api, nil
}

func readPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	var password string
	if _, err := fmt.Scanln(&password); err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return password, nil
}

func readLine(prompt string) (string, error) {
	fmt.Print(prompt)
	var line string
	if _, err := fmt.Scanln(&line); err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}
	return line, nil
}

func readMasterPassword() (string, error) {
	return readPassword("Master password: ")
}

var exitFunc = os.Exit

func exitOnError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		exitFunc(1)
	}
}
