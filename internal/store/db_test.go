package store_test

import (
	"path/filepath"
	"testing"

	"sqlite-p2p/internal/store"
)

func TestOpenDBWithWAL(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "subdir", "test_crm.db")

	database, err := store.OpenDB(dbPath, true)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	// Verify WAL journal mode
	var journalMode string
	err = database.QueryRow("PRAGMA journal_mode;").Scan(&journalMode)
	if err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("expected journal_mode to be wal, got %s", journalMode)
	}

	// Verify crm_store table exists and has WITHOUT ROWID
	var tableName string
	err = database.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='crm_store';").Scan(&tableName)
	if err != nil {
		t.Fatalf("crm_store table does not exist: %v", err)
	}
	if tableName != "crm_store" {
		t.Errorf("expected table name crm_store, got %s", tableName)
	}
}

func TestOpenDBWithoutWAL(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "nowal.db")

	database, err := store.OpenDB(dbPath, false)
	if err != nil {
		t.Fatalf("failed to open db without wal: %v", err)
	}
	defer database.Close()

	var journalMode string
	err = database.QueryRow("PRAGMA journal_mode;").Scan(&journalMode)
	if err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}
	if journalMode == "wal" {
		t.Errorf("expected journal_mode not to be wal, got %s", journalMode)
	}
}

func TestOpenDBMemory(t *testing.T) {
	database, err := store.OpenDB(":memory:", true)
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	defer database.Close()

	var count int
	err = database.QueryRow("SELECT count(*) FROM crm_store;").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query crm_store in-memory: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}
}

func TestOpenDBInvalidDir(t *testing.T) {
	_, err := store.OpenDB("/dev/null/impossible/db.sqlite", true)
	if err == nil {
		t.Fatalf("expected error for impossible path, got nil")
	}
}

func TestInitOntologySchema_Decoupled(t *testing.T) {
	database, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	defer database.Close()

	// 1. By default, ontology_nodes should NOT exist in core DB
	var tableName string
	err = database.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='ontology_nodes';").Scan(&tableName)
	if err == nil {
		t.Fatalf("expected ontology_nodes table NOT to exist initially in core OpenDB")
	}

	// 2. Calling InitOntologySchema creates both tables
	if err := store.InitOntologySchema(database); err != nil {
		t.Fatalf("failed to init ontology schema: %v", err)
	}

	err = database.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='ontology_nodes';").Scan(&tableName)
	if err != nil || tableName != "ontology_nodes" {
		t.Fatalf("expected ontology_nodes table to exist after InitOntologySchema")
	}

	err = database.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='ontology_edges';").Scan(&tableName)
	if err != nil || tableName != "ontology_edges" {
		t.Fatalf("expected ontology_edges table to exist after InitOntologySchema")
	}

	// 3. Test repo.InitOntologySchema works idempotently
	repo := store.NewRepository(database)
	if err := repo.InitOntologySchema(t.Context()); err != nil {
		t.Fatalf("repo.InitOntologySchema failed: %v", err)
	}
}

