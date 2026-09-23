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
	"crm-sqlite-pear-p2p/internal/store"
)

func TestMCPSubcommand_DryRun(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "mcp_cli.db")

	cfg := config.DefaultConfig()
	cfg.DBPath = dbPath

	root := cli.BuildRootCommand("0.1.0", cfg)
	var out bytes.Buffer
	root.Stdout = &out

	// 1. Dry run with default stdio
	err := root.Dispatch(context.Background(), []string{"mcp", "--dry-run"})
	if err != nil {
		t.Fatalf("mcp --dry-run failed: %v", err)
	}
	if !strings.Contains(out.String(), "transport=stdio") {
		t.Errorf("expected output to contain 'transport=stdio', got: %s", out.String())
	}

	// 2. Dry run with SSE custom params
	out.Reset()
	err = root.Dispatch(context.Background(), []string{"mcp", "--transport=sse", "--host=0.0.0.0", "--port=9099", "--path=/custom-mcp", "--dry-run"})
	if err != nil {
		t.Fatalf("mcp custom flags failed: %v", err)
	}
	if !strings.Contains(out.String(), "transport=sse") || !strings.Contains(out.String(), "9099") || !strings.Contains(out.String(), "/custom-mcp") {
		t.Errorf("expected output to contain sse params, got: %s", out.String())
	}
}

func TestMCPSubcommand_ExecutionWithContext(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "mcp_cli_run.db")

	db, err := store.OpenDB(dbPath, true)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	tracker := store.NewChangesetTracker(repo)

	cfg := config.DefaultConfig()
	cfg.DBPath = dbPath

	root := cli.BuildRootCommandWithTracker("0.1.0", cfg, tracker)
	var out bytes.Buffer
	root.Stdout = &out

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Runs SSE server and context cancels cleanly
	err = root.Dispatch(ctx, []string{"mcp", "--transport=sse", "--host=127.0.0.1", "--port=0", "--path=/mcp"})
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("unexpected error from mcp run: %v", err)
	}
}
