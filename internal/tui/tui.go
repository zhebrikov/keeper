// Package tui provides an interactive terminal UI for the GophKeeper client.
//
// Run [Run] to start the password manager in full-screen mode with login,
// secret list, CRUD operations, and sync.
package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	gkclient "github.com/zhebrikov/gophkeeper/internal/client"
	"github.com/zhebrikov/gophkeeper/internal/config"
)

// Run starts the TUI and blocks until the user quits.
func Run(ctx context.Context, cfg config.ClientConfig) error {
	api, err := gkclient.New(cfg)
	if err != nil {
		return fmt.Errorf("connect to server: %w", err)
	}

	m := newModel(ctx, api)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		_ = api.Close()
		return fmt.Errorf("run tui: %w", err)
	}

	return api.Close()
}
