package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"sqlite-p2p/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SchemaToolHandler manages schema registry MCP operations.
type SchemaToolHandler struct {
	repo *store.Repository
}

// NewSchemaToolHandler creates a new SchemaToolHandler.
func NewSchemaToolHandler(repo *store.Repository) *SchemaToolHandler {
	return &SchemaToolHandler{repo: repo}
}

// RegisterSchemaTools registers all schema registry tools with the given MCP server.
func RegisterSchemaTools(server *mcp.Server, repo *store.Repository) {
	if server == nil || repo == nil {
		return
	}
	h := NewSchemaToolHandler(repo)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm_schema_set",
		Description: "Register or update a schema definition with $id/uri, JSON schema metadata, and markdown documentation.",
	}, h.SchemaSet)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm_schema_get",
		Description: "Retrieve a registered schema definition, JSON schema object, and documentation markdown by URI.",
	}, h.SchemaGet)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm_schema_list",
		Description: "List all registered schema contracts under the org.schema:* namespace.",
	}, h.SchemaList)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm_schema_delete",
		Description: "Delete a schema definition from the CRM store by URI.",
	}, h.SchemaDelete)
}

// --- Schema Set Tool ---

type SchemaSetInput struct {
	URI         string `json:"uri" jsonschema:"The schema URI or identifier (e.g., https://schema.org/Customer or customer)"`
	SchemaJSON  string `json:"schema_json" jsonschema:"The JSON schema contract definition as a valid JSON string"`
	DocMarkdown string `json:"doc_markdown" jsonschema:"Markdown documentation describing the schema and its operational use-cases"`
}

type SchemaSetOutput struct {
	URI     string `json:"uri"`
	Key     string `json:"key"`
	Message string `json:"message"`
}

func (h *SchemaToolHandler) SchemaSet(ctx context.Context, req *mcp.CallToolRequest, input SchemaSetInput) (*mcp.CallToolResult, SchemaSetOutput, error) {
	if strings.TrimSpace(input.URI) == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "schema URI cannot be empty"}},
		}, SchemaSetOutput{}, nil
	}

	rawJSON := json.RawMessage(input.SchemaJSON)
	if len(rawJSON) == 0 || !json.Valid(rawJSON) {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "schema_json must be valid JSON"}},
		}, SchemaSetOutput{}, nil
	}

	record, err := h.repo.PutSchema(ctx, input.URI, rawJSON, input.DocMarkdown)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to save schema: %v", err)}},
		}, SchemaSetOutput{}, nil
	}

	msg := fmt.Sprintf("schema '%s' successfully registered under key '%s'", record.URI, record.Key)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}, SchemaSetOutput{
		URI:     record.URI,
		Key:     record.Key,
		Message: msg,
	}, nil
}

// --- Schema Get Tool ---

type SchemaGetInput struct {
	URI string `json:"uri" jsonschema:"The schema URI or canonical key to retrieve"`
}

type SchemaGetOutput struct {
	URI         string `json:"uri"`
	Key         string `json:"key"`
	SchemaJSON  string `json:"schema_json"`
	DocMarkdown string `json:"doc_markdown"`
}

func (h *SchemaToolHandler) SchemaGet(ctx context.Context, req *mcp.CallToolRequest, input SchemaGetInput) (*mcp.CallToolResult, SchemaGetOutput, error) {
	if strings.TrimSpace(input.URI) == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "uri cannot be empty"}},
		}, SchemaGetOutput{}, nil
	}

	record, err := h.repo.GetSchema(ctx, input.URI)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("schema not found: %v", err)}},
		}, SchemaGetOutput{}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Schema: %s\nDocumentation:\n%s", record.URI, record.DocMarkdown)},
		},
	}, SchemaGetOutput{
		URI:         record.URI,
		Key:         record.Key,
		SchemaJSON:  string(record.JSONSchema),
		DocMarkdown: record.DocMarkdown,
	}, nil
}

// --- Schema List Tool ---

type SchemaListInput struct {
	Limit  int `json:"limit,omitempty" jsonschema:"Maximum number of schemas to return (default 50)"`
	Offset int `json:"offset,omitempty" jsonschema:"Pagination offset"`
}

type SchemaSummary struct {
	URI string `json:"uri"`
	Key string `json:"key"`
}

type SchemaListOutput struct {
	Schemas []SchemaSummary `json:"schemas"`
	Count   int             `json:"count"`
}

func (h *SchemaToolHandler) SchemaList(ctx context.Context, req *mcp.CallToolRequest, input SchemaListInput) (*mcp.CallToolResult, SchemaListOutput, error) {
	records, err := h.repo.ListSchemas(ctx, input.Limit, input.Offset)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to list schemas: %v", err)}},
		}, SchemaListOutput{}, nil
	}

	summaries := make([]SchemaSummary, len(records))
	for i, r := range records {
		summaries[i] = SchemaSummary{
			URI: r.URI,
			Key: r.Key,
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Found %d schemas", len(summaries))}},
	}, SchemaListOutput{
		Schemas: summaries,
		Count:   len(summaries),
	}, nil
}

// --- Schema Delete Tool ---

type SchemaDeleteInput struct {
	URI string `json:"uri" jsonschema:"The schema URI or canonical key to delete"`
}

type SchemaDeleteOutput struct {
	URI     string `json:"uri"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (h *SchemaToolHandler) SchemaDelete(ctx context.Context, req *mcp.CallToolRequest, input SchemaDeleteInput) (*mcp.CallToolResult, SchemaDeleteOutput, error) {
	if strings.TrimSpace(input.URI) == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "uri cannot be empty"}},
		}, SchemaDeleteOutput{}, nil
	}

	err := h.repo.DeleteSchema(ctx, input.URI)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to delete schema: %v", err)}},
		}, SchemaDeleteOutput{}, nil
	}

	msg := fmt.Sprintf("schema '%s' successfully deleted", input.URI)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}, SchemaDeleteOutput{
		URI:     input.URI,
		Success: true,
		Message: msg,
	}, nil
}
