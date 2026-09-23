package mcp

import (
	"context"
	"testing"

	"crm-sqlite-pear-p2p/internal/store"
)

func TestOntologyTools_Lifecycle(t *testing.T) {
	repo, _, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed nodes and edge
	err := repo.UpsertNode(ctx, store.Node{
		ID:    "customer:cust-101",
		Label: "Acme Corp",
		Type:  "customer",
	})
	if err != nil {
		t.Fatalf("UpsertNode failed: %v", err)
	}

	err = repo.UpsertNode(ctx, store.Node{
		ID:    "agent:support-01",
		Label: "Primary Support Agent",
		Type:  "agent",
	})
	if err != nil {
		t.Fatalf("UpsertNode failed: %v", err)
	}

	err = repo.UpsertEdge(ctx, store.Edge{
		Source:       "customer:cust-101",
		Target:       "agent:support-01",
		Relationship: "assigned_to",
		Weight:       1.0,
	})
	if err != nil {
		t.Fatalf("UpsertEdge failed: %v", err)
	}

	handler := NewOntologyToolHandler(repo)

	// 1. GetNode
	getNodeRes, nodeOut, err := handler.OntologyGetNode(ctx, nil, OntologyGetNodeInput{
		NodeID: "customer:cust-101",
	})
	if err != nil {
		t.Fatalf("OntologyGetNode failed: %v", err)
	}
	if getNodeRes != nil && getNodeRes.IsError {
		t.Fatalf("OntologyGetNode returned error")
	}
	if nodeOut.Label != "Acme Corp" {
		t.Errorf("expected label Acme Corp, got %s", nodeOut.Label)
	}

	// 2. QueryEdges
	edgesRes, edgesOut, err := handler.OntologyQueryEdges(ctx, nil, OntologyQueryEdgesInput{
		SourceID: "customer:cust-101",
	})
	if err != nil {
		t.Fatalf("OntologyQueryEdges failed: %v", err)
	}
	if edgesRes != nil && edgesRes.IsError {
		t.Fatalf("OntologyQueryEdges returned error")
	}
	if edgesOut.Count != 1 {
		t.Fatalf("expected 1 edge, got %d", edgesOut.Count)
	}
	if edgesOut.Edges[0].Target != "agent:support-01" {
		t.Errorf("expected target agent:support-01, got %s", edgesOut.Edges[0].Target)
	}

	// 3. Search Nodes
	searchRes, searchOut, err := handler.OntologySearch(ctx, nil, OntologySearchInput{
		Type: "customer",
	})
	if err != nil {
		t.Fatalf("OntologySearch failed: %v", err)
	}
	if searchRes != nil && searchRes.IsError {
		t.Fatalf("OntologySearch returned error")
	}
	if searchOut.Count != 1 {
		t.Fatalf("expected 1 customer node, got %d", searchOut.Count)
	}
}

func TestOntologyTools_Validation(t *testing.T) {
	repo, _, _, cleanup := setupTestDB(t)
	defer cleanup()

	handler := NewOntologyToolHandler(repo)
	ctx := context.Background()

	// Empty node ID
	res, _, _ := handler.OntologyGetNode(ctx, nil, OntologyGetNodeInput{NodeID: ""})
	if res == nil || !res.IsError {
		t.Fatal("expected error on empty node ID")
	}

	// Non-existent node
	res, _, _ = handler.OntologyGetNode(ctx, nil, OntologyGetNodeInput{NodeID: "nonexistent"})
	if res == nil || !res.IsError {
		t.Fatal("expected error on non-existent node")
	}

	// Empty source ID for edge query
	res, _, _ = handler.OntologyQueryEdges(ctx, nil, OntologyQueryEdgesInput{SourceID: ""})
	if res == nil || !res.IsError {
		t.Fatal("expected error on empty source ID")
	}
}
