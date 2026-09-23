package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sqlite-p2p/internal/crypto"
	"sqlite-p2p/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CustomerToolHandler manages customer record lifecycle and crypto-shredding operations.
type CustomerToolHandler struct {
	repo    *store.Repository
	keys    *crypto.KeyRegistry
	tracker *store.ChangesetTracker
}

// NewCustomerToolHandler constructs a new CustomerToolHandler.
func NewCustomerToolHandler(repo *store.Repository, keys *crypto.KeyRegistry, tracker *store.ChangesetTracker) *CustomerToolHandler {
	return &CustomerToolHandler{
		repo:    repo,
		keys:    keys,
		tracker: tracker,
	}
}

// RegisterCustomerTools binds customer management tools to the MCP server.
func RegisterCustomerTools(server *mcp.Server, repo *store.Repository, keys *crypto.KeyRegistry, tracker *store.ChangesetTracker) {
	if server == nil || repo == nil {
		return
	}
	h := NewCustomerToolHandler(repo, keys, tracker)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm_customer_put",
		Description: "Insert or update a customer record with phone/key, JSON metadata, and optional confidential payload encrypted via AES-256-GCM.",
	}, h.CustomerPut)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm_customer_get",
		Description: "Retrieve a customer record by phone or Base32 hashed key, returning metadata and decrypted payload.",
	}, h.CustomerGet)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm_customer_delete",
		Description: "Delete a customer record from the store and purge its encryption key.",
	}, h.CustomerDelete)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm_customer_shred",
		Description: "Permanently shred the AES-256 encryption key for a customer, rendering payload irrevocably unreadable (GDPR crypto-shredding).",
	}, h.CustomerShred)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm_customer_list",
		Description: "List customer records with pagination.",
	}, h.CustomerList)
}

func resolveCustomerKey(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", errors.New("phone_or_key cannot be empty")
	}
	if strings.HasPrefix(trimmed, store.CustomerNamespace+":") {
		return trimmed, nil
	}
	return store.FormatCustomerKey(trimmed)
}

// --- Customer Put Tool ---

type CustomerPutInput struct {
	PhoneOrKey string `json:"phone_or_key" jsonschema:"Customer phone number (e.g., +91-98765-43210) or 52-char Base32 primary key"`
	Metadata   string `json:"metadata" jsonschema:"JSON metadata object describing customer attributes and schema references"`
	Payload    string `json:"payload,omitempty" jsonschema:"Optional private customer profile payload, encrypted at rest"`
}

type CustomerPutOutput struct {
	Key     string `json:"key"`
	Message string `json:"message"`
}

func (h *CustomerToolHandler) CustomerPut(ctx context.Context, req *mcp.CallToolRequest, input CustomerPutInput) (*mcp.CallToolResult, CustomerPutOutput, error) {
	key, err := resolveCustomerKey(input.PhoneOrKey)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid customer identifier: %v", err)}},
		}, CustomerPutOutput{}, nil
	}

	rawMeta := json.RawMessage(input.Metadata)
	if len(rawMeta) == 0 || !json.Valid(rawMeta) {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "metadata must be a valid JSON string"}},
		}, CustomerPutOutput{}, nil
	}

	if h.tracker != nil {
		err = h.tracker.Put(ctx, key, rawMeta, []byte(input.Payload))
	} else {
		err = h.repo.Put(ctx, key, rawMeta, []byte(input.Payload))
	}

	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to save customer %s: %v", key, err)}},
		}, CustomerPutOutput{}, nil
	}

	msg := fmt.Sprintf("saved customer record %s", key)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}, CustomerPutOutput{
		Key:     key,
		Message: msg,
	}, nil
}

// --- Customer Get Tool ---

type CustomerGetInput struct {
	PhoneOrKey string `json:"phone_or_key" jsonschema:"Customer phone number or 52-char Base32 primary key"`
}

type CustomerGetOutput struct {
	Key      string `json:"key"`
	Metadata string `json:"metadata"`
	Payload  string `json:"payload,omitempty"`
}

func (h *CustomerToolHandler) CustomerGet(ctx context.Context, req *mcp.CallToolRequest, input CustomerGetInput) (*mcp.CallToolResult, CustomerGetOutput, error) {
	key, err := resolveCustomerKey(input.PhoneOrKey)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid customer identifier: %v", err)}},
		}, CustomerGetOutput{}, nil
	}

	rec, err := h.repo.Get(ctx, key)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get customer %s: %v", key, err)}},
		}, CustomerGetOutput{}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Customer: %s\nMetadata: %s\nPayload: %s", rec.Key, string(rec.Metadata), string(rec.Data))},
		},
	}, CustomerGetOutput{
		Key:      rec.Key,
		Metadata: string(rec.Metadata),
		Payload:  string(rec.Data),
	}, nil
}

// --- Customer Delete Tool ---

type CustomerDeleteInput struct {
	PhoneOrKey string `json:"phone_or_key" jsonschema:"Customer phone number or 52-char Base32 primary key to delete"`
}

type CustomerDeleteOutput struct {
	Key     string `json:"key"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (h *CustomerToolHandler) CustomerDelete(ctx context.Context, req *mcp.CallToolRequest, input CustomerDeleteInput) (*mcp.CallToolResult, CustomerDeleteOutput, error) {
	key, err := resolveCustomerKey(input.PhoneOrKey)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid customer identifier: %v", err)}},
		}, CustomerDeleteOutput{}, nil
	}

	if h.tracker != nil {
		err = h.tracker.Delete(ctx, key)
	} else {
		err = h.repo.Delete(ctx, key)
	}

	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to delete customer %s: %v", key, err)}},
		}, CustomerDeleteOutput{}, nil
	}

	msg := fmt.Sprintf("deleted customer %s", key)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}, CustomerDeleteOutput{
		Key:     key,
		Success: true,
		Message: msg,
	}, nil
}

// --- Customer Shred Tool ---

type CustomerShredInput struct {
	PhoneOrKey string `json:"phone_or_key" jsonschema:"Customer phone number or 52-char Base32 primary key whose encryption key should be permanently purged"`
}

type CustomerShredOutput struct {
	Key     string `json:"key"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (h *CustomerToolHandler) CustomerShred(ctx context.Context, req *mcp.CallToolRequest, input CustomerShredInput) (*mcp.CallToolResult, CustomerShredOutput, error) {
	key, err := resolveCustomerKey(input.PhoneOrKey)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid customer identifier: %v", err)}},
		}, CustomerShredOutput{}, nil
	}

	if h.keys == nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "key registry not configured"}},
		}, CustomerShredOutput{}, nil
	}

	err = h.keys.PurgeKey(key)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to purge encryption key for %s: %v", key, err)}},
		}, CustomerShredOutput{}, nil
	}

	msg := fmt.Sprintf("cryptographic key permanently purged for customer %s", key)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}, CustomerShredOutput{
		Key:     key,
		Success: true,
		Message: msg,
	}, nil
}

// --- Customer List Tool ---

type CustomerListInput struct {
	Limit  int `json:"limit,omitempty" jsonschema:"Maximum number of records to return (default 50)"`
	Offset int `json:"offset,omitempty" jsonschema:"Pagination offset"`
}

type CustomerSummary struct {
	Key      string `json:"key"`
	Metadata string `json:"metadata"`
}

type CustomerListOutput struct {
	Customers []CustomerSummary `json:"customers"`
	Count     int               `json:"count"`
}

func (h *CustomerToolHandler) CustomerList(ctx context.Context, req *mcp.CallToolRequest, input CustomerListInput) (*mcp.CallToolResult, CustomerListOutput, error) {
	if input.Limit <= 0 {
		input.Limit = 50
	}
	records, err := h.repo.List(ctx, store.CustomerNamespace+":", input.Limit, input.Offset)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to list customers: %v", err)}},
		}, CustomerListOutput{}, nil
	}

	var customers []CustomerSummary
	for _, r := range records {
		customers = append(customers, CustomerSummary{
			Key:      r.Key,
			Metadata: string(r.Metadata),
		})
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Found %d customers", len(customers))}},
	}, CustomerListOutput{
		Customers: customers,
		Count:     len(customers),
	}, nil
}
