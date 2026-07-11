// Package storage provides local client-side persistence for session data.
//
// [Store] reads and writes a JSON session file with restrictive file
// permissions (0600 for the file, 0700 for the directory).
package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Session holds locally stored authentication and sync state.
type Session struct {
	// AccessToken is the current JWT used for authenticated RPC calls.
	AccessToken string `json:"access_token"`
	// RefreshToken is stored to renew expired access tokens.
	RefreshToken string `json:"refresh_token"`
	// UserID is the authenticated account identifier.
	UserID string `json:"user_id"`
	// ServerAddr is the gRPC server the session was created against.
	ServerAddr string `json:"server_addr"`
	// LastSyncAt is the timestamp of the last successful sync.
	LastSyncAt time.Time `json:"last_sync_at"`
}

// Store manages reading and writing the local session file.
type Store struct {
	path string
}

// New creates a Store that persists data at the given path.
func New(path string) *Store {
	return &Store{path: path}
}

// Load reads the session from disk. Returns nil session if the file does not exist.
func (s *Store) Load() (*Session, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("parse session: %w", err)
	}
	return &session, nil
}

// Save writes the session to disk with restrictive permissions.
func (s *Store) Save(session *Session) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("write session: %w", err)
	}
	return nil
}

// Clear removes the stored session file.
func (s *Store) Clear() error {
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove session: %w", err)
	}
	return nil
}
