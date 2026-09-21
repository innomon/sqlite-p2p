package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"crm-sqlite-pear-p2p/internal/store"
)

func TestOntologyNodeCRUD(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	ctx := context.Background()

	node := store.Node{
		ID:       "agent:support-bot-01",
		Label:    "Tier 1 Support Bot",
		Type:     "agent",
		Metadata: json.RawMessage(`{"model":"gemini-flash","version":"2.0"}`),
	}

	// 1. Upsert (Insert)
	if err := repo.UpsertNode(ctx, node); err != nil {
		t.Fatalf("UpsertNode failed: %v", err)
	}

	// 2. Get
	fetched, err := repo.GetNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("GetNode failed: %v", err)
	}
	if fetched.ID != node.ID || fetched.Label != node.Label || fetched.Type != node.Type {
		t.Fatalf("node mismatch: expected %+v, got %+v", node, fetched)
	}
	if string(fetched.Metadata) != string(node.Metadata) {
		t.Fatalf("metadata mismatch: expected %s, got %s", string(node.Metadata), string(fetched.Metadata))
	}

	// 3. Upsert (Update)
	node.Label = "Tier 1 Senior Support Bot"
	node.Metadata = json.RawMessage(`{"model":"gemini-flash","version":"2.1"}`)
	if err := repo.UpsertNode(ctx, node); err != nil {
		t.Fatalf("UpsertNode update failed: %v", err)
	}

	updated, err := repo.GetNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("GetNode after update failed: %v", err)
	}
	if updated.Label != "Tier 1 Senior Support Bot" {
		t.Fatalf("expected updated label, got %s", updated.Label)
	}

	// 4. List by type
	custNode := store.Node{
		ID:    "customer:cust-100",
		Label: "Acme Corp",
		Type:  "customer",
	}
	if err := repo.UpsertNode(ctx, custNode); err != nil {
		t.Fatalf("UpsertNode custNode failed: %v", err)
	}

	agents, err := repo.ListNodes(ctx, "agent")
	if err != nil {
		t.Fatalf("ListNodes agent failed: %v", err)
	}
	if len(agents) != 1 || agents[0].ID != node.ID {
		t.Fatalf("expected 1 agent, got %+v", agents)
	}

	allNodes, err := repo.ListNodes(ctx, "")
	if err != nil {
		t.Fatalf("ListNodes all failed: %v", err)
	}
	if len(allNodes) != 2 {
		t.Fatalf("expected 2 total nodes, got %d", len(allNodes))
	}

	// 5. Delete
	if err := repo.DeleteNode(ctx, node.ID); err != nil {
		t.Fatalf("DeleteNode failed: %v", err)
	}

	_, err = repo.GetNode(ctx, node.ID)
	if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound after deletion, got %v", err)
	}
}

func TestOntologyEdgeCRUD(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	ctx := context.Background()

	n1 := store.Node{ID: "agent:alice", Label: "Alice Agent", Type: "agent"}
	n2 := store.Node{ID: "customer:bob", Label: "Bob Customer", Type: "customer"}
	_ = repo.UpsertNode(ctx, n1)
	_ = repo.UpsertNode(ctx, n2)

	edge := store.Edge{
		Source:       n1.ID,
		Target:       n2.ID,
		Relationship: "assigned_to",
		Weight:       1.5,
		Metadata:     json.RawMessage(`{"channel":"whatsapp"}`),
	}

	// 1. Upsert (Insert)
	if err := repo.UpsertEdge(ctx, edge); err != nil {
		t.Fatalf("UpsertEdge failed: %v", err)
	}

	// 2. Get
	fetched, err := repo.GetEdge(ctx, edge.Source, edge.Target, edge.Relationship)
	if err != nil {
		t.Fatalf("GetEdge failed: %v", err)
	}
	if fetched.Weight != 1.5 || fetched.Relationship != "assigned_to" {
		t.Fatalf("edge mismatch: %+v", fetched)
	}

	// 3. Upsert (Update weight and metadata)
	edge.Weight = 2.0
	edge.Metadata = json.RawMessage(`{"channel":"voice"}`)
	if err := repo.UpsertEdge(ctx, edge); err != nil {
		t.Fatalf("UpsertEdge update failed: %v", err)
	}

	updated, err := repo.GetEdge(ctx, edge.Source, edge.Target, edge.Relationship)
	if err != nil {
		t.Fatalf("GetEdge after update failed: %v", err)
	}
	if updated.Weight != 2.0 {
		t.Fatalf("expected weight 2.0, got %f", updated.Weight)
	}

	// 4. List by source
	edges, err := repo.ListEdges(ctx, n1.ID)
	if err != nil {
		t.Fatalf("ListEdges failed: %v", err)
	}
	if len(edges) != 1 || edges[0].Target != n2.ID {
		t.Fatalf("expected 1 edge for %s, got %+v", n1.ID, edges)
	}

	// 5. Delete edge
	if err := repo.DeleteEdge(ctx, edge.Source, edge.Target, edge.Relationship); err != nil {
		t.Fatalf("DeleteEdge failed: %v", err)
	}

	_, err = repo.GetEdge(ctx, edge.Source, edge.Target, edge.Relationship)
	if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound after edge deletion, got %v", err)
	}
}
