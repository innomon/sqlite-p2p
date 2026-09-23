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

func TestCustomerCLIIntegration(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "customer_cli.db")

	db, err := store.OpenDB(dbPath, true)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	tracker := store.NewChangesetTracker(repo)

	var emittedChangesets []*store.Changeset
	tracker.Subscribe(func(cs *store.Changeset, raw []byte) {
		emittedChangesets = append(emittedChangesets, cs)
	})

	cfg := config.DefaultConfig()
	cfg.DBPath = dbPath
	root := cli.BuildRootCommandWithTracker("0.1.0", cfg, tracker)

	var out bytes.Buffer
	root.Stdout = &out

	// 1. customer put
	phone := "+91-98765-43210"
	metadata := `{"schemas":[{"$ID":"https://qzip.in/schemas/crm/customer-v1.json","tier":"enterprise"}]}`
	err = root.Dispatch(context.Background(), []string{"customer", "put", phone, metadata})
	if err != nil {
		t.Fatalf("customer put failed: %v", err)
	}

	if !strings.Contains(out.String(), "Saved customer record") {
		t.Errorf("expected output to contain 'Saved customer record', got: %s", out.String())
	}
	if len(emittedChangesets) != 1 {
		t.Fatalf("expected 1 emitted changeset, got %d", len(emittedChangesets))
	}
	if emittedChangesets[0].Operation != store.OpInsert {
		t.Errorf("expected OpInsert, got %d", emittedChangesets[0].Operation)
	}

	// 2. customer get by phone
	out.Reset()
	err = root.Dispatch(context.Background(), []string{"customer", "get", phone})
	if err != nil {
		t.Fatalf("customer get failed: %v", err)
	}
	if !strings.Contains(out.String(), "enterprise") {
		t.Errorf("expected customer metadata in get output, got: %s", out.String())
	}

	// 3. customer list
	out.Reset()
	err = root.Dispatch(context.Background(), []string{"customer", "list"})
	if err != nil {
		t.Fatalf("customer list failed: %v", err)
	}
	if !strings.Contains(out.String(), "Total Customers: 1") {
		t.Errorf("expected list count 1, got: %s", out.String())
	}
}

func TestKeygenCLI(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "keygen_cli.db")

	cfg := config.DefaultConfig()
	cfg.DBPath = dbPath
	root := cli.BuildRootCommand("0.1.0", cfg)

	var out bytes.Buffer
	root.Stdout = &out

	phone := "+1-555-010-9999"
	err := root.Dispatch(context.Background(), []string{"keygen", phone})
	if err != nil {
		t.Fatalf("keygen failed: %v", err)
	}

	if !strings.Contains(out.String(), "Generated symmetric key for:") {
		t.Errorf("unexpected output from keygen: %s", out.String())
	}

	// Missing args
	err = root.Dispatch(context.Background(), []string{"keygen"})
	if err == nil {
		t.Fatal("expected error on keygen without args, got nil")
	}
}

func TestCustomerDeleteCLI(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "delete_cli.db")

	db, err := store.OpenDB(dbPath, true)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	tracker := store.NewChangesetTracker(repo)

	var emittedChangesets []*store.Changeset
	tracker.Subscribe(func(cs *store.Changeset, raw []byte) {
		emittedChangesets = append(emittedChangesets, cs)
	})

	cfg := config.DefaultConfig()
	cfg.DBPath = dbPath
	root := cli.BuildRootCommandWithTracker("0.1.0", cfg, tracker)

	var out bytes.Buffer
	root.Stdout = &out

	phone := "+1-555-444-3322"
	metadata := `{"schemas":[{"$ID":"https://qzip.in/schemas/crm/customer-v1.json","tier":"vip"}]}`

	// Put customer
	if err := root.Dispatch(context.Background(), []string{"customer", "put", phone, metadata, "sensitive data"}); err != nil {
		t.Fatalf("customer put failed: %v", err)
	}

	// Delete customer
	out.Reset()
	if err := root.Dispatch(context.Background(), []string{"customer", "delete", phone}); err != nil {
		t.Fatalf("customer delete failed: %v", err)
	}

	if !strings.Contains(out.String(), "Deleted customer record") {
		t.Errorf("unexpected output from customer delete: %s", out.String())
	}

	if len(emittedChangesets) != 2 {
		t.Fatalf("expected 2 emitted changesets (insert + delete), got %d", len(emittedChangesets))
	}
	if emittedChangesets[1].Operation != store.OpDelete {
		t.Errorf("expected OpDelete, got %d", emittedChangesets[1].Operation)
	}

	// Verify get fails now
	out.Reset()
	err = root.Dispatch(context.Background(), []string{"customer", "get", phone})
	if err == nil {
		t.Fatal("expected error getting deleted customer, got nil")
	}

	// Missing args
	err = root.Dispatch(context.Background(), []string{"customer", "delete"})
	if err == nil {
		t.Fatal("expected error on delete without args, got nil")
	}
}

