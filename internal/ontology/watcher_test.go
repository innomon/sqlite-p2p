package ontology_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sqlite-p2p/internal/ontology"
	"sqlite-p2p/internal/store"
)

func TestWatcher_IngestFile(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	tempDir := t.TempDir()

	watcher, err := ontology.NewWatcher(tempDir, repo)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer watcher.Stop()

	mdContent := `---
schema: https://qzip.in/schemas/crm/ontology-graph-v1.json
nodes:
  - id: agent:bot-101
    label: Ingested Bot
    type: agent
edges:
  - source: agent:bot-101
    target: cust:999
    relationship: manages
---
Notes`

	filePath := filepath.Join(tempDir, "interaction-01.md")
	if err := os.WriteFile(filePath, []byte(mdContent), 0644); err != nil {
		t.Fatalf("failed to write test md file: %v", err)
	}

	parsed, err := watcher.IngestFile(context.Background(), filePath)
	if err != nil {
		t.Fatalf("IngestFile failed: %v", err)
	}
	if len(parsed.Nodes) != 1 || len(parsed.Edges) != 1 {
		t.Fatalf("unexpected parsed entities: %+v", parsed)
	}

	// Verify in database
	node, err := repo.GetNode(context.Background(), "agent:bot-101")
	if err != nil {
		t.Fatalf("repo.GetNode failed: %v", err)
	}
	if node.Label != "Ingested Bot" {
		t.Fatalf("expected Ingested Bot, got %s", node.Label)
	}

	edge, err := repo.GetEdge(context.Background(), "agent:bot-101", "cust:999", "manages")
	if err != nil {
		t.Fatalf("repo.GetEdge failed: %v", err)
	}
	if edge.Relationship != "manages" {
		t.Fatalf("expected manages, got %s", edge.Relationship)
	}
}

func TestWatcher_LiveFileCreationAndModification(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	tempDir := t.TempDir()

	watcher, err := ontology.NewWatcher(tempDir, repo)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("watcher.Start failed: %v", err)
	}
	defer watcher.Stop()

	// 1. Live file creation
	filePath := filepath.Join(tempDir, "live-record.md")
	doc1 := `---
schema: https://qzip.in/schemas/crm/ontology-graph-v1.json
nodes:
  - id: agent:live-agent
    label: Initial Live Agent
    type: agent
---
Content`
	if err := os.WriteFile(filePath, []byte(doc1), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Allow event to process
	time.Sleep(150 * time.Millisecond)

	node, err := repo.GetNode(context.Background(), "agent:live-agent")
	if err != nil {
		t.Fatalf("expected node from live created file: %v", err)
	}
	if node.Label != "Initial Live Agent" {
		t.Fatalf("expected Initial Live Agent, got %s", node.Label)
	}

	// 2. Live file modification
	doc2 := `---
schema: https://qzip.in/schemas/crm/ontology-graph-v1.json
nodes:
  - id: agent:live-agent
    label: Updated Live Agent
    type: agent
---
Updated Content`
	if err := os.WriteFile(filePath, []byte(doc2), 0644); err != nil {
		t.Fatalf("WriteFile update failed: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	nodeUpdated, err := repo.GetNode(context.Background(), "agent:live-agent")
	if err != nil {
		t.Fatalf("expected node after update: %v", err)
	}
	if nodeUpdated.Label != "Updated Live Agent" {
		t.Fatalf("expected Updated Live Agent, got %s", nodeUpdated.Label)
	}
}

func TestWatcher_IngestDirectory(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	tempDir := t.TempDir()

	file1 := filepath.Join(tempDir, "f1.md")
	file2 := filepath.Join(tempDir, "f2.markdown")
	nonMd := filepath.Join(tempDir, "ignore.txt")

	_ = os.WriteFile(file1, []byte(`---
schema: https://qzip.in/schemas/crm/ontology-graph-v1.json
nodes:
  - id: n1
    label: Node One
    type: test
---`), 0644)

	_ = os.WriteFile(file2, []byte(`---
schema: https://qzip.in/schemas/crm/ontology-graph-v1.json
nodes:
  - id: n2
    label: Node Two
    type: test
---`), 0644)

	_ = os.WriteFile(nonMd, []byte(`not markdown`), 0644)

	watcher, err := ontology.NewWatcher(tempDir, repo)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer watcher.Stop()

	count, err := watcher.IngestDirectory(context.Background())
	if err != nil {
		t.Fatalf("IngestDirectory failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 files ingested, got %d", count)
	}

	nodes, err := repo.ListNodes(context.Background(), "test")
	if err != nil || len(nodes) != 2 {
		t.Fatalf("expected 2 test nodes in db, got %d (err: %v)", len(nodes), err)
	}
}

func TestWatcher_EdgeCases(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	// 1. Nil repo
	_, err = ontology.NewWatcher("records", nil)
	if err == nil {
		t.Fatal("expected error on nil repo, got nil")
	}

	repo := store.NewRepository(db)
	tempDir := t.TempDir()

	// 2. Default dir and logger
	watcher, err := ontology.NewWatcher(tempDir, repo)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer watcher.Stop()

	watcher.SetLogger(slog.Default())

	// 3. Ingest non-existent file
	_, err = watcher.IngestFile(context.Background(), filepath.Join(tempDir, "does-not-exist.md"))
	if err == nil {
		t.Fatal("expected error reading non-existent file, got nil")
	}

	// 4. Ingest non-ontology markdown file (should return nil, nil)
	nonOntologyFile := filepath.Join(tempDir, "plain.md")
	_ = os.WriteFile(nonOntologyFile, []byte("# Just a regular markdown file"), 0644)
	parsed, err := watcher.IngestFile(context.Background(), nonOntologyFile)
	if err != nil || parsed != nil {
		t.Fatalf("expected (nil, nil) for non-ontology markdown, got (%+v, %v)", parsed, err)
	}

	// 5. Start idempotency
	ctx := context.Background()
	_ = watcher.Start(ctx)
	err = watcher.Start(ctx) // second start should be no-op
	if err != nil {
		t.Fatalf("expected nil on second Start, got %v", err)
	}
}
