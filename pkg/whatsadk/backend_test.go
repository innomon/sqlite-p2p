package whatsadk

import (
	"context"
	"testing"

	"sqlite-p2p/internal/store"
)

func TestNewBackend_Memory(t *testing.T) {
	ctx := context.Background()

	opts := Options{
		DBPath: ":memory:",
	}
	backend, err := NewBackend(opts)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Close()

	if backend.DB() == nil {
		t.Fatal("expected non-nil DB")
	}
	if backend.Repo() == nil {
		t.Fatal("expected non-nil Repo")
	}
	if backend.Tracker() == nil {
		t.Fatal("expected non-nil Tracker")
	}

	// Verify crm_store table exists
	var tableName string
	err = backend.DB().QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='crm_store'").Scan(&tableName)
	if err != nil {
		t.Fatalf("crm_store table missing: %v", err)
	}

	// Verify views exist
	expectedViews := []string{"filesys", "whatsmeow_contacts", "whatsmeow_commands", "blacklisted_numbers"}
	for _, view := range expectedViews {
		var viewName string
		err = backend.DB().QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='view' AND name=?", view).Scan(&viewName)
		if err != nil {
			t.Fatalf("view %s missing: %v", view, err)
		}
		if viewName != view {
			t.Fatalf("expected view %s, got %s", view, viewName)
		}
	}

	if err := backend.Close(); err != nil {
		t.Fatalf("backend.Close failed: %v", err)
	}
}

func TestNewBackend_WithExistingDB(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open store DB: %v", err)
	}
	defer db.Close()

	opts := Options{
		DB: db,
	}
	backend, err := NewBackend(opts)
	if err != nil {
		t.Fatalf("failed to create backend with existing db: %v", err)
	}

	if backend.DB() != db {
		t.Fatalf("expected backend to use provided db")
	}

	// Close backend; since it doesn't own db, it shouldn't error
	if err := backend.Close(); err != nil {
		t.Fatalf("failed to close backend: %v", err)
	}
}

func TestNewBackend_InvalidPath(t *testing.T) {
	opts := Options{
		DBPath: "/nonexistent_dir/impossible/path/db.sqlite",
	}
	// SQLite OpenDB creates parent directories unless it has no permission
	// Using a path under a read-only root or invalid device
	opts.DBPath = "/sys/kernel/debug/invalid/db.sqlite"
	_, err := NewBackend(opts)
	if err == nil {
		// If running as root, this might succeed; test empty DBPath
		opts.DBPath = ""
		_, err = NewBackend(opts)
		if err == nil {
			t.Fatal("expected error for empty DB path without DB")
		}
	}
}
