package mcp

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"crm-sqlite-pear-p2p/internal/crypto"
	"crm-sqlite-pear-p2p/internal/store"
)

func setupTestDB(t *testing.T) (*store.Repository, *crypto.KeyRegistry, *store.ChangesetTracker, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "mcp-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(tmpDir, "crm.db")
	db, err := store.OpenDB(dbPath, true)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to open test db: %v", err)
	}

	keys, err := crypto.NewKeyRegistry(db)
	if err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create key registry: %v", err)
	}

	repo := store.NewRepository(db)
	repo.SetKeyRegistry(keys)
	tracker := store.NewChangesetTracker(repo)

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}
	return repo, keys, tracker, cleanup
}

func TestNewServer_Defaults(t *testing.T) {
	repo, keys, tracker, cleanup := setupTestDB(t)
	defer cleanup()

	opts := ServerOptions{
		Version: "1.0.0",
		Repo:    repo,
		KeyReg:  keys,
		Tracker: tracker,
	}

	srv, err := NewServer(opts)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	if srv == nil {
		t.Fatal("expected non-nil server")
	}

	if srv.opts.Transport != "stdio" {
		t.Errorf("expected default transport 'stdio', got '%s'", srv.opts.Transport)
	}
	if srv.opts.Host != "127.0.0.1" {
		t.Errorf("expected default host '127.0.0.1', got '%s'", srv.opts.Host)
	}
	if srv.opts.Port != "8083" {
		t.Errorf("expected default port '8083', got '%s'", srv.opts.Port)
	}
	if srv.opts.Path != "/mcp" {
		t.Errorf("expected default path '/mcp', got '%s'", srv.opts.Path)
	}
}

func TestServer_SSERunAndShutdown(t *testing.T) {
	repo, keys, tracker, cleanup := setupTestDB(t)
	defer cleanup()

	// Find free port for testing
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on random port: %v", err)
	}
	_, portStr, err := net.SplitHostPort(l.Addr().String())
	l.Close()
	if err != nil {
		t.Fatalf("failed to get port: %v", err)
	}

	opts := ServerOptions{
		Version:   "1.0.0",
		Transport: "sse",
		Host:      "127.0.0.1",
		Port:      portStr,
		Path:      "/mcp",
		Repo:      repo,
		KeyReg:    keys,
		Tracker:   tracker,
	}

	srv, err := NewServer(opts)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx)
	}()

	// Wait briefly for server to bind
	time.Sleep(150 * time.Millisecond)

	// Ping HTTP endpoint
	resp, err := http.Get("http://127.0.0.1:" + portStr + "/mcp")
	if err != nil {
		t.Fatalf("failed to connect to sse endpoint: %v", err)
	}
	resp.Body.Close()

	// Cancel context and assert clean shutdown
	cancel()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("srv.Run returned error on shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server shutdown timed out")
	}
}

func TestNewServer_WithDBPathAndUnsupportedTransport(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-dbpath-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	dbPath := filepath.Join(tmpDir, "crm.db")

	opts := ServerOptions{
		DBPath:    dbPath,
		EnableWAL: true,
		Transport: "invalid_proto",
	}

	srv, err := NewServer(opts)
	if err != nil {
		t.Fatalf("NewServer with DBPath failed: %v", err)
	}
	if srv.opts.Repo == nil {
		t.Fatal("expected non-nil repo initialized from DBPath")
	}

	err = srv.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for unsupported transport")
	}

	// Test invalid DB path
	badOpts := ServerOptions{
		DBPath: "/non_existent_dir_xyz/crm.db",
	}
	_, err = NewServer(badOpts)
	if err == nil {
		t.Fatal("expected error for invalid DBPath")
	}
}

