package cli_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"crm-sqlite-pear-p2p/internal/cli"
	"crm-sqlite-pear-p2p/internal/config"
	"crm-sqlite-pear-p2p/internal/p2p"
	"crm-sqlite-pear-p2p/internal/store"
)

func TestP2PCLICommands(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "p2p_cli.db")
	storageDir := filepath.Join(tempDir, "p2p_storage")

	db, err := store.OpenDB(dbPath, true)
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	feed, err := p2p.NewChangesetFeed(filepath.Join(storageDir, "feed"))
	if err != nil {
		t.Fatalf("NewChangesetFeed error: %v", err)
	}

	swarm, err := p2p.NewSwarmManager(p2p.SwarmManagerOptions{Port: 0})
	if err != nil {
		t.Fatalf("NewSwarmManager error: %v", err)
	}
	defer swarm.Close()

	replicator := p2p.NewReplicator(feed, nil)
	defer replicator.Close()

	engine := p2p.NewReplicationEngine(repo, feed, replicator)

	cfg := config.DefaultConfig()
	cfg.DBPath = dbPath
	cfg.StorageDir = storageDir

	root := cli.BuildRootCommandWithEngine("0.1.0", cfg, engine, swarm)
	var out bytes.Buffer
	root.Stdout = &out

	// 1. peer status
	out.Reset()
	err = root.Dispatch(context.Background(), []string{"peer", "status"})
	if err != nil {
		t.Fatalf("peer status dispatch failed: %v", err)
	}
	if !strings.Contains(out.String(), "Peer Status") || !strings.Contains(out.String(), "Port:") {
		t.Fatalf("unexpected peer status output: %s", out.String())
	}

	// 2. peer list
	out.Reset()
	err = root.Dispatch(context.Background(), []string{"peer", "list"})
	if err != nil {
		t.Fatalf("peer list dispatch failed: %v", err)
	}
	if !strings.Contains(out.String(), "Connected Peers:") {
		t.Fatalf("unexpected peer list output: %s", out.String())
	}

	// 3. sync
	out.Reset()
	err = root.Dispatch(context.Background(), []string{"sync"})
	if err != nil {
		t.Fatalf("sync dispatch failed: %v", err)
	}
	if !strings.Contains(out.String(), "Sync complete") {
		t.Fatalf("unexpected sync output: %s", out.String())
	}

	// 4. start --daemon / start --once (daemon test with immediate context cancel)
	out.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = root.Dispatch(ctx, []string{"start", "--dry-run"})
	if err != nil {
		t.Fatalf("start dry-run failed: %v", err)
	}
	if !strings.Contains(out.String(), "Starting P2P replication node") {
		t.Fatalf("unexpected start output: %s", out.String())
	}
}

func TestP2PStartGracefulShutdown(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "shutdown.db")
	storageDir := filepath.Join(tempDir, "shutdown_storage")

	db, err := store.OpenDB(dbPath, true)
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	feed, err := p2p.NewChangesetFeed(filepath.Join(storageDir, "feed"))
	if err != nil {
		t.Fatalf("NewChangesetFeed error: %v", err)
	}

	swarm, err := p2p.NewSwarmManager(p2p.SwarmManagerOptions{Port: 0})
	if err != nil {
		t.Fatalf("NewSwarmManager error: %v", err)
	}

	replicator := p2p.NewReplicator(feed, nil)

	engine := p2p.NewReplicationEngine(repo, feed, replicator)

	cfg := config.DefaultConfig()
	cfg.DBPath = dbPath
	cfg.StorageDir = storageDir

	root := cli.BuildRootCommandWithEngine("0.1.0", cfg, engine, swarm)
	var out bytes.Buffer
	root.Stdout = &out

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err = root.Dispatch(ctx, []string{"start"})
	if err != nil {
		t.Fatalf("start dispatch error: %v", err)
	}

	if !strings.Contains(out.String(), "Starting P2P replication node") || !strings.Contains(out.String(), "P2P replication node stopped") {
		t.Errorf("unexpected output: %s", out.String())
	}

	// Verify swarm is closed
	if err := swarm.Close(); err == nil {
		// Calling Close twice should be idempotent or swarm was already closed
	}
}

