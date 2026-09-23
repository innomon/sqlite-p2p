package mcp

import (
	"context"
	"fmt"
	"strings"

	"sqlite-p2p/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OntologyToolHandler manages knowledge graph and ontology inspection operations.
type OntologyToolHandler struct {
	repo *store.Repository
}

// NewOntologyToolHandler creates a new OntologyToolHandler.
func NewOntologyToolHandler(repo *store.Repository) *OntologyToolHandler {
	return &OntologyToolHandler{repo: repo}
}

// RegisterOntologyTools registers graph ontology tools with the MCP server.
func RegisterOntologyTools(server *mcp.Server, repo *store.Repository) {
	if server == nil || repo == nil {
		return
	}
	h := NewOntologyToolHandler(repo)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm_ontology_get_node",
		Description: "Fetch knowledge graph entity node metadata, label, and attributes by node ID.",
	}, h.OntologyGetNode)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm_ontology_query_edges",
		Description: "Query outgoing relationships and edges connected to an entity node.",
	}, h.OntologyQueryEdges)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm_ontology_search",
		Description: "Search ontology entity nodes by type or keyword query.",
	}, h.OntologySearch)
}

// --- Ontology Get Node Tool ---

type OntologyGetNodeInput struct {
	NodeID string `json:"node_id" jsonschema:"The unique entity node ID (e.g. customer:cust-101, agent:support-01)"`
}

type OntologyGetNodeOutput struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Metadata string `json:"metadata,omitempty"`
}

func (h *OntologyToolHandler) OntologyGetNode(ctx context.Context, req *mcp.CallToolRequest, input OntologyGetNodeInput) (*mcp.CallToolResult, OntologyGetNodeOutput, error) {
	if strings.TrimSpace(input.NodeID) == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "node_id cannot be empty"}},
		}, OntologyGetNodeOutput{}, nil
	}

	node, err := h.repo.GetNode(ctx, input.NodeID)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get node %s: %v", input.NodeID, err)}},
		}, OntologyGetNodeOutput{}, nil
	}

	metaStr := ""
	if len(node.Metadata) > 0 {
		metaStr = string(node.Metadata)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Node: %s (%s) [Type: %s]", node.ID, node.Label, node.Type)},
		},
	}, OntologyGetNodeOutput{
		ID:       node.ID,
		Label:    node.Label,
		Type:     node.Type,
		Metadata: metaStr,
	}, nil
}

// --- Ontology Query Edges Tool ---

type OntologyQueryEdgesInput struct {
	SourceID string `json:"source_id" jsonschema:"The source entity node ID whose outgoing relationships are to be queried"`
}

type EdgeSummary struct {
	Source       string  `json:"source"`
	Target       string  `json:"target"`
	Relationship string  `json:"relationship"`
	Weight       float64 `json:"weight"`
}

type OntologyQueryEdgesOutput struct {
	Edges []EdgeSummary `json:"edges"`
	Count int           `json:"count"`
}

func (h *OntologyToolHandler) OntologyQueryEdges(ctx context.Context, req *mcp.CallToolRequest, input OntologyQueryEdgesInput) (*mcp.CallToolResult, OntologyQueryEdgesOutput, error) {
	if strings.TrimSpace(input.SourceID) == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "source_id cannot be empty"}},
		}, OntologyQueryEdgesOutput{}, nil
	}

	edges, err := h.repo.ListEdges(ctx, input.SourceID)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to query edges for %s: %v", input.SourceID, err)}},
		}, OntologyQueryEdgesOutput{}, nil
	}

	summaries := make([]EdgeSummary, len(edges))
	for i, e := range edges {
		summaries[i] = EdgeSummary{
			Source:       e.Source,
			Target:       e.Target,
			Relationship: e.Relationship,
			Weight:       e.Weight,
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Found %d edges from %s", len(summaries), input.SourceID)}},
	}, OntologyQueryEdgesOutput{
		Edges: summaries,
		Count: len(summaries),
	}, nil
}

// --- Ontology Search Tool ---

type OntologySearchInput struct {
	Type  string `json:"type,omitempty" jsonschema:"Filter nodes by entity type (e.g., customer, agent, ticket)"`
	Query string `json:"query,omitempty" jsonschema:"Keyword substring to match in node ID or label"`
}

type NodeSummary struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

type OntologySearchOutput struct {
	Nodes []NodeSummary `json:"nodes"`
	Count int           `json:"count"`
}

func (h *OntologyToolHandler) OntologySearch(ctx context.Context, req *mcp.CallToolRequest, input OntologySearchInput) (*mcp.CallToolResult, OntologySearchOutput, error) {
	nodes, err := h.repo.ListNodes(ctx, input.Type)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to search ontology nodes: %v", err)}},
		}, OntologySearchOutput{}, nil
	}

	q := strings.ToLower(strings.TrimSpace(input.Query))
	var filtered []NodeSummary
	for _, n := range nodes {
		if q != "" {
			if !strings.Contains(strings.ToLower(n.ID), q) && !strings.Contains(strings.ToLower(n.Label), q) {
				continue
			}
		}
		filtered = append(filtered, NodeSummary{
			ID:    n.ID,
			Label: n.Label,
			Type:  n.Type,
		})
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Found %d ontology nodes", len(filtered))}},
	}, OntologySearchOutput{
		Nodes: filtered,
		Count: len(filtered),
	}, nil
}
