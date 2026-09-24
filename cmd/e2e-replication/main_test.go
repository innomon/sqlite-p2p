package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"sqlite-p2p/pkg/p2p"
)

func TestCommandRegistry_Basic(t *testing.T) {
	cr := NewCommandRegistry()

	executed := false
	cr.Register(Command{
		Name:        "ping",
		Usage:       "ping",
		Description: "ping command",
		Run: func(ctx context.Context, app *ReplicationApp, args []string) error {
			executed = true
			return nil
		},
	})

	ctx := context.Background()

	// Direct execution
	if err := cr.Execute(ctx, nil, "ping"); err != nil {
		t.Fatalf("execute ping failed: %v", err)
	}
	if !executed {
		t.Fatal("expected command to execute")
	}

	// Slash execution
	executed = false
	if err := cr.Execute(ctx, nil, "/ping"); err != nil {
		t.Fatalf("execute /ping failed: %v", err)
	}
	if !executed {
		t.Fatal("expected slash command to execute")
	}

	// Unknown command
	if err := cr.Execute(ctx, nil, "unknown"); err == nil {
		t.Fatal("expected error on unknown command")
	}

	// Empty string
	if err := cr.Execute(ctx, nil, "   "); err != nil {
		t.Fatalf("expected nil on empty command, got: %v", err)
	}
}

func TestReplicationApp_Commands(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	engine, err := p2p.OpenEngine(p2p.EngineOptions{
		DBPath:    dbPath,
		EnableWAL: true,
	})
	if err != nil {
		t.Fatalf("OpenEngine failed: %v", err)
	}
	defer engine.Close()

	app := &ReplicationApp{
		cfg: NodeConfig{
			NodeID: "test-node",
			DBPath: dbPath,
		},
		engine:   engine,
		registry: NewCommandRegistry(),
		stopChan: make(chan struct{}),
	}
	app.initCommands()

	ctx := context.Background()

	// Test help
	if err := app.registry.Execute(ctx, app, "help"); err != nil {
		t.Fatalf("help failed: %v", err)
	}

	// Test status (listener is nil in this unit test, let's test put/get/list)
	if err := app.registry.Execute(ctx, app, "put user:1 Alice"); err != nil {
		t.Fatalf("put failed: %v", err)
	}

	// Test get
	if err := app.registry.Execute(ctx, app, "get user:1"); err != nil {
		t.Fatalf("get failed: %v", err)
	}

	// Test list
	if err := app.registry.Execute(ctx, app, "list"); err != nil {
		t.Fatalf("list failed: %v", err)
	}

	// Test list with prefix
	if err := app.registry.Execute(ctx, app, "list user:"); err != nil {
		t.Fatalf("list prefix failed: %v", err)
	}

	// Test del
	if err := app.registry.Execute(ctx, app, "del user:1"); err != nil {
		t.Fatalf("del failed: %v", err)
	}

	// Verify not found after del
	if err := app.registry.Execute(ctx, app, "get user:1"); err == nil {
		t.Fatal("expected error getting deleted key")
	}

	// Test validation errors
	if err := app.registry.Execute(ctx, app, "put"); err == nil {
		t.Fatal("expected error on put without args")
	}
	if err := app.registry.Execute(ctx, app, "get"); err == nil {
		t.Fatal("expected error on get without args")
	}
	if err := app.registry.Execute(ctx, app, "del"); err == nil {
		t.Fatal("expected error on del without args")
	}
	if err := app.registry.Execute(ctx, app, "connect"); err == nil {
		t.Fatal("expected error on connect without args")
	}
}

func TestConfigFileLoading(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "node.json")

	cfgJSON := `{
		"node_id": "test-node-load",
		"swarm_port": 9099,
		"bootstrap": ["127.0.0.1:9098"],
		"peer_addrs": ["127.0.0.1:9098"],
		"db_path": "data/test_load.db"
	}`

	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var parsed NodeConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if parsed.NodeID != "test-node-load" {
		t.Fatalf("unexpected node_id: %s", parsed.NodeID)
	}
	if parsed.SwarmPort != 9099 {
		t.Fatalf("unexpected swarm_port: %d", parsed.SwarmPort)
	}
	if len(parsed.Bootstrap) != 1 || parsed.Bootstrap[0] != "127.0.0.1:9098" {
		t.Fatalf("unexpected bootstrap: %+v", parsed.Bootstrap)
	}
	if len(parsed.PeerAddrs) != 1 || parsed.PeerAddrs[0] != "127.0.0.1:9098" {
		t.Fatalf("unexpected peer_addrs: %+v", parsed.PeerAddrs)
	}
}
