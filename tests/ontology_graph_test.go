package tests_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"crm-sqlite-pear-p2p/internal/cli"
	"crm-sqlite-pear-p2p/internal/config"
	"crm-sqlite-pear-p2p/internal/ontology"
	"crm-sqlite-pear-p2p/internal/store"
)

func TestEndToEndMarkdownIngestionAndGraphTraversal(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "e2e_ontology.db")
	recordsDir := filepath.Join(tempDir, "records")

	db, err := store.OpenDB(dbPath, true)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)

	watcher, err := ontology.NewWatcher(recordsDir, repo)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("watcher.Start failed: %v", err)
	}
	defer watcher.Stop()

	// 1. Write initial Markdown log into records/
	mdLog1 := `---
schema: https://qzip.in/schemas/crm/ontology-graph-v1.json
nodes:
  - id: agent:gemini-support
    label: Gemini Support AI
    type: agent
    metadata:
      model: gemini-2.0-flash
  - id: customer:enterprise-acme
    label: Acme International
    type: customer
    metadata:
      sla: 24/7
  - id: skill:go-troubleshooting
    label: Go Troubleshooting
    type: skill
  - id: agent:human-tier3
    label: Sarah Jenkins (Tier 3 Lead)
    type: agent
edges:
  - source: agent:gemini-support
    target: skill:go-troubleshooting
    relationship: equipped_with
    weight: 1.0
  - source: agent:gemini-support
    target: customer:enterprise-acme
    relationship: servicing
    weight: 1.0
  - source: customer:enterprise-acme
    target: agent:human-tier3
    relationship: escalated_to
    weight: 2.0
---
# Interaction Summary - Ticket #INC-9021
Customer Acme reported concurrency bottleneck in P2P feed replication.
Gemini Support AI utilized Go Troubleshooting skill to isolate deadlock.
Issue escalated to human specialist Sarah Jenkins for architecture sign-off.
`
	filePath1 := filepath.Join(recordsDir, "2026-09-21-acme-incident.md")
	if err := os.WriteFile(filePath1, []byte(mdLog1), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Allow filesystem watcher event propagation
	time.Sleep(200 * time.Millisecond)

	// Configure CLI
	cfg := config.DefaultConfig()
	cfg.DBPath = dbPath
	root := cli.BuildRootCommand("0.1.0", cfg)

	// 2. Query nodes by type via CLI
	var out bytes.Buffer
	root.Stdout = &out

	if err := root.Dispatch(ctx, []string{"query", "nodes", "agent"}); err != nil {
		t.Fatalf("query nodes agent failed: %v", err)
	}
	agentOut := out.String()
	if !strings.Contains(agentOut, "agent:gemini-support") || !strings.Contains(agentOut, "agent:human-tier3") {
		t.Errorf("expected both agents listed, got: %s", agentOut)
	}

	out.Reset()
	if err := root.Dispatch(ctx, []string{"query", "nodes", "skill"}); err != nil {
		t.Fatalf("query nodes skill failed: %v", err)
	}
	skillOut := out.String()
	if !strings.Contains(skillOut, "skill:go-troubleshooting") {
		t.Errorf("expected skill node listed, got: %s", skillOut)
	}

	// 3. Multi-hop graph traversal via CLI
	out.Reset()
	if err := root.Dispatch(ctx, []string{"query", "graph", "agent:gemini-support", "--depth=2"}); err != nil {
		t.Fatalf("query graph failed: %v", err)
	}
	graphOut := out.String()
	if !strings.Contains(graphOut, "skill:go-troubleshooting") || !strings.Contains(graphOut, "customer:enterprise-acme") {
		t.Errorf("expected 1-hop targets in graph output, got: %s", graphOut)
	}
	if !strings.Contains(graphOut, "agent:human-tier3") {
		t.Errorf("expected 2-hop target (agent:human-tier3) in graph output, got: %s", graphOut)
	}
	if !strings.Contains(graphOut, "escalated_to") {
		t.Errorf("expected multi-hop relationship escalated_to in output, got: %s", graphOut)
	}

	// 4. Update file on disk and verify live update
	mdLog2 := `---
schema: https://qzip.in/schemas/crm/ontology-graph-v1.json
nodes:
  - id: agent:human-tier3
    label: Sarah Jenkins (Tier 3 Lead)
    type: agent
  - id: ticket:resolution-9021
    label: Fix Applied and Verified
    type: resolution
edges:
  - source: agent:human-tier3
    target: ticket:resolution-9021
    relationship: resolved
    weight: 1.0
---
Resolution note: Hotfix deployed.
`
	filePath2 := filepath.Join(recordsDir, "2026-09-21-acme-resolution.md")
	if err := os.WriteFile(filePath2, []byte(mdLog2), 0644); err != nil {
		t.Fatalf("WriteFile resolution failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	out.Reset()
	if err := root.Dispatch(ctx, []string{"query", "graph", "agent:human-tier3", "--depth=1"}); err != nil {
		t.Fatalf("query graph human agent failed: %v", err)
	}
	resOut := out.String()
	if !strings.Contains(resOut, "ticket:resolution-9021") || !strings.Contains(resOut, "resolved") {
		t.Errorf("expected newly ingested resolution in graph traversal, got: %s", resOut)
	}
}
