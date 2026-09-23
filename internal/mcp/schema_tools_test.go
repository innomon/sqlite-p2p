package mcp

import (
	"context"
	"testing"
)

func TestSchemaTools_Lifecycle(t *testing.T) {
	repo, _, _, cleanup := setupTestDB(t)
	defer cleanup()

	handler := NewSchemaToolHandler(repo)
	ctx := context.Background()

	// 1. SchemaSet
	setInput := SchemaSetInput{
		URI:         "https://schema.org/Customer",
		SchemaJSON:  `{"type":"object","properties":{"name":{"type":"string"}}}`,
		DocMarkdown: "# Customer Schema\nDocumentation for customer schema.",
	}
	res, setOut, err := handler.SchemaSet(ctx, nil, setInput)
	if err != nil {
		t.Fatalf("SchemaSet failed: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("SchemaSet returned tool error: %v", res.Content)
	}
	if setOut.URI != setInput.URI {
		t.Errorf("expected URI %s, got %s", setInput.URI, setOut.URI)
	}

	// 2. SchemaGet
	getInput := SchemaGetInput{
		URI: "https://schema.org/Customer",
	}
	res, getOut, err := handler.SchemaGet(ctx, nil, getInput)
	if err != nil {
		t.Fatalf("SchemaGet failed: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("SchemaGet returned tool error")
	}
	if getOut.DocMarkdown != setInput.DocMarkdown {
		t.Errorf("expected doc %s, got %s", setInput.DocMarkdown, getOut.DocMarkdown)
	}

	// 3. SchemaList
	listInput := SchemaListInput{Limit: 10}
	res, listOut, err := handler.SchemaList(ctx, nil, listInput)
	if err != nil {
		t.Fatalf("SchemaList failed: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("SchemaList returned tool error")
	}
	if listOut.Count != 1 {
		t.Fatalf("expected count 1, got %d", listOut.Count)
	}

	// 4. SchemaDelete
	delInput := SchemaDeleteInput{URI: "https://schema.org/Customer"}
	res, delOut, err := handler.SchemaDelete(ctx, nil, delInput)
	if err != nil {
		t.Fatalf("SchemaDelete failed: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("SchemaDelete returned tool error")
	}
	if !delOut.Success {
		t.Error("expected delete success")
	}

	// Verify not found after delete
	res, _, err = handler.SchemaGet(ctx, nil, getInput)
	if err != nil {
		t.Fatalf("unexpected error on get after delete: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("expected IsError=true on get of deleted schema")
	}
}

func TestSchemaTools_ValidationErrors(t *testing.T) {
	repo, _, _, cleanup := setupTestDB(t)
	defer cleanup()

	handler := NewSchemaToolHandler(repo)
	ctx := context.Background()

	// Empty URI
	res, _, _ := handler.SchemaSet(ctx, nil, SchemaSetInput{
		URI:        "",
		SchemaJSON: `{"type":"object"}`,
	})
	if res == nil || !res.IsError {
		t.Fatal("expected error for empty URI")
	}

	// Invalid JSON
	res, _, _ = handler.SchemaSet(ctx, nil, SchemaSetInput{
		URI:        "valid/uri",
		SchemaJSON: `{invalid`,
	})
	if res == nil || !res.IsError {
		t.Fatal("expected error for invalid JSON")
	}

	// Delete non-existent
	res, _, _ = handler.SchemaDelete(ctx, nil, SchemaDeleteInput{URI: "nonexistent"})
	if res == nil || !res.IsError {
		t.Fatal("expected error when deleting non-existent schema")
	}
}
