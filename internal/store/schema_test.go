package store_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"sqlite-p2p/internal/store"
)

func setupTestSchemaDB(t *testing.T) (*store.Repository, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "schema-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(tmpDir, "crm.db")
	db, err := store.OpenDB(dbPath, true)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to open test db: %v", err)
	}

	repo := store.NewRepository(db)
	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}
	return repo, cleanup
}

func TestSchemaCRUD(t *testing.T) {
	repo, cleanup := setupTestSchemaDB(t)
	defer cleanup()

	ctx := context.Background()
	uri := "https://crm.local/schemas/customer.json"
	schemaJSON := json.RawMessage(`{"$id":"https://crm.local/schemas/customer.json","type":"object","properties":{"name":{"type":"string"}}}`)
	docMD := "# Customer Schema\nThis schema defines customer attributes."

	// 1. PutSchema
	saved, err := repo.PutSchema(ctx, uri, schemaJSON, docMD)
	if err != nil {
		t.Fatalf("PutSchema failed: %v", err)
	}
	if saved.URI != uri {
		t.Errorf("expected URI %s, got %s", uri, saved.URI)
	}
	expectedKey := "org.schema:" + uri
	if saved.Key != expectedKey {
		t.Errorf("expected key %s, got %s", expectedKey, saved.Key)
	}

	// 2. GetSchema
	fetched, err := repo.GetSchema(ctx, uri)
	if err != nil {
		t.Fatalf("GetSchema failed: %v", err)
	}
	if fetched.URI != uri {
		t.Errorf("expected URI %s, got %s", uri, fetched.URI)
	}
	if string(fetched.JSONSchema) != string(schemaJSON) {
		t.Errorf("expected schema %s, got %s", string(schemaJSON), string(fetched.JSONSchema))
	}
	if fetched.DocMarkdown != docMD {
		t.Errorf("expected doc %s, got %s", docMD, fetched.DocMarkdown)
	}

	// 3. ListSchemas
	schemas, err := repo.ListSchemas(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListSchemas failed: %v", err)
	}
	if len(schemas) != 1 {
		t.Fatalf("expected 1 schema, got %d", len(schemas))
	}
	if schemas[0].URI != uri {
		t.Errorf("expected listed URI %s, got %s", uri, schemas[0].URI)
	}

	// 4. DeleteSchema
	err = repo.DeleteSchema(ctx, uri)
	if err != nil {
		t.Fatalf("DeleteSchema failed: %v", err)
	}

	_, err = repo.GetSchema(ctx, uri)
	if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestSchemaInvalidJSON(t *testing.T) {
	repo, cleanup := setupTestSchemaDB(t)
	defer cleanup()

	ctx := context.Background()
	badJSON := json.RawMessage(`{invalid_json}`)
	_, err := repo.PutSchema(ctx, "test_bad", badJSON, "doc")
	if err == nil {
		t.Fatal("expected error for invalid JSON schema")
	}
}
