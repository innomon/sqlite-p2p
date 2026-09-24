package cli_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"sqlite-p2p/internal/cli"
	"sqlite-p2p/internal/config"
	"sqlite-p2p/internal/store"
)

func TestQueryNodesCLI(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "query_nodes.db")

	db, err := store.OpenDB(dbPath, true)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	if err := store.InitOntologySchema(db); err != nil {
		t.Fatalf("InitOntologySchema failed: %v", err)
	}

	repo := store.NewRepository(db)
	ctx := context.Background()

	_ = repo.UpsertNode(ctx, store.Node{ID: "agent:alex", Label: "Alex Agent", Type: "agent"})
	_ = repo.UpsertNode(ctx, store.Node{ID: "customer:corp1", Label: "Acme Corp", Type: "customer"})

	cfg := config.DefaultConfig()
	cfg.DBPath = dbPath
	root := cli.BuildRootCommand("0.1.0", cfg)

	// 1. List all nodes
	var out bytes.Buffer
	root.Stdout = &out
	err = root.Dispatch(ctx, []string{"query", "nodes"})
	if err != nil {
		t.Fatalf("query nodes failed: %v", err)
	}
	if !strings.Contains(out.String(), "agent:alex") || !strings.Contains(out.String(), "customer:corp1") {
		t.Errorf("expected both nodes in output: %s", out.String())
	}

	// 2. Filter by type
	out.Reset()
	err = root.Dispatch(ctx, []string{"query", "nodes", "agent"})
	if err != nil {
		t.Fatalf("query nodes agent failed: %v", err)
	}
	if !strings.Contains(out.String(), "agent:alex") || strings.Contains(out.String(), "customer:corp1") {
		t.Errorf("expected only agent node: %s", out.String())
	}
}

func TestQueryGraphCLI(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "query_graph.db")

	db, err := store.OpenDB(dbPath, true)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	if err := store.InitOntologySchema(db); err != nil {
		t.Fatalf("InitOntologySchema failed: %v", err)
	}

	repo := store.NewRepository(db)
	ctx := context.Background()

	_ = repo.UpsertNode(ctx, store.Node{ID: "agent:support", Label: "Support Lead", Type: "agent"})
	_ = repo.UpsertNode(ctx, store.Node{ID: "ticket:101", Label: "Login Bug", Type: "ticket"})
	_ = repo.UpsertNode(ctx, store.Node{ID: "customer:john", Label: "John Doe", Type: "customer"})

	_ = repo.UpsertEdge(ctx, store.Edge{Source: "agent:support", Target: "ticket:101", Relationship: "assigned_to"})
	_ = repo.UpsertEdge(ctx, store.Edge{Source: "ticket:101", Target: "customer:john", Relationship: "reported_by"})

	cfg := config.DefaultConfig()
	cfg.DBPath = dbPath
	root := cli.BuildRootCommand("0.1.0", cfg)

	var out bytes.Buffer
	root.Stdout = &out

	// 1. Traverse graph
	err = root.Dispatch(ctx, []string{"query", "graph", "agent:support", "--depth", "2"})
	if err != nil {
		t.Fatalf("query graph failed: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "ticket:101") || !strings.Contains(output, "customer:john") {
		t.Errorf("expected traversed entities in output, got: %s", output)
	}
	if !strings.Contains(output, "assigned_to") || !strings.Contains(output, "reported_by") {
		t.Errorf("expected relationships in output, got: %s", output)
	}

	// 2. Missing args
	out.Reset()
	err = root.Dispatch(ctx, []string{"query", "graph"})
	if err == nil {
		t.Fatal("expected error on query graph without node_id, got nil")
	}
}
