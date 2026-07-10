package storage_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zhebrikov/gophkeeper/internal/client/storage"
)

func TestStoreSaveLoadClear(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")
	store := storage.New(path)

	session := &storage.Session{
		AccessToken:  "access",
		RefreshToken: "refresh",
		UserID:       "user-1",
		ServerAddr:   "localhost:8080",
		LastSyncAt:   time.Now().UTC(),
	}

	if err := store.Save(session); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected session")
	}
	if loaded.AccessToken != session.AccessToken {
		t.Fatalf("got token %q, want %q", loaded.AccessToken, session.AccessToken)
	}

	if err := store.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	loaded, err = store.Load()
	if err != nil {
		t.Fatalf("Load after clear: %v", err)
	}
	if loaded != nil {
		t.Fatal("expected nil session after clear")
	}
}

func TestStoreLoadMissingFile(t *testing.T) {
	store := storage.New(filepath.Join(t.TempDir(), "missing.json"))
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded != nil {
		t.Fatal("expected nil for missing file")
	}
}
