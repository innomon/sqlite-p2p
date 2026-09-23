package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"sqlite-p2p/internal/store"
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

func TestOntologyGraphTraversal(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	ctx := context.Background()

	// Build network: Alice (agent) -> Ticket T1 -> Bob (customer) -> Charlie (customer)
	nodes := []store.Node{
		{ID: "agent:alice", Label: "Alice Agent", Type: "agent"},
		{ID: "ticket:T1", Label: "Billing Inquiry", Type: "ticket"},
		{ID: "customer:bob", Label: "Bob Customer", Type: "customer"},
		{ID: "customer:charlie", Label: "Charlie Customer", Type: "customer"},
	}
	for _, n := range nodes {
		if err := repo.UpsertNode(ctx, n); err != nil {
			t.Fatalf("UpsertNode %s failed: %v", n.ID, err)
		}
	}

	edges := []store.Edge{
		{Source: "agent:alice", Target: "ticket:T1", Relationship: "assigned_to", Weight: 1.0},
		{Source: "ticket:T1", Target: "customer:bob", Relationship: "opened_by", Weight: 1.0},
		{Source: "customer:bob", Target: "customer:charlie", Relationship: "referred", Weight: 2.0},
	}
	for _, e := range edges {
		if err := repo.UpsertEdge(ctx, e); err != nil {
			t.Fatalf("UpsertEdge failed: %v", err)
		}
	}

	// 1. Depth 1 from Alice: only Ticket T1
	steps1, err := repo.TraverseNeighbors(ctx, "agent:alice", 1)
	if err != nil {
		t.Fatalf("TraverseNeighbors depth 1 failed: %v", err)
	}
	if len(steps1) != 1 || steps1[0].NodeID != "ticket:T1" {
		t.Fatalf("expected [ticket:T1], got %+v", steps1)
	}

	// 2. Depth 2 from Alice: Ticket T1 and Bob
	steps2, err := repo.TraverseNeighbors(ctx, "agent:alice", 2)
	if err != nil {
		t.Fatalf("TraverseNeighbors depth 2 failed: %v", err)
	}
	if len(steps2) != 2 {
		t.Fatalf("expected 2 steps for depth 2, got %d", len(steps2))
	}
	if steps2[0].NodeID != "ticket:T1" || steps2[1].NodeID != "customer:bob" {
		t.Fatalf("unexpected depth 2 traversal steps: %+v", steps2)
	}

	// 3. Depth 3 from Alice: reaches Charlie
	steps3, err := repo.TraverseNeighbors(ctx, "agent:alice", 3)
	if err != nil {
		t.Fatalf("TraverseNeighbors depth 3 failed: %v", err)
	}
	if len(steps3) != 3 {
		t.Fatalf("expected 3 steps for depth 3, got %d", len(steps3))
	}
	if steps3[2].NodeID != "customer:charlie" {
		t.Fatalf("expected 3rd step to be customer:charlie, got %s", steps3[2].NodeID)
	}

	// 4. FindPath Alice -> Charlie
	path, err := repo.FindPath(ctx, "agent:alice", "customer:charlie", 5)
	if err != nil {
		t.Fatalf("FindPath failed: %v", err)
	}
	if len(path) == 0 {
		t.Fatal("expected path between Alice and Charlie, got empty")
	}
	lastStep := path[len(path)-1]
	if lastStep.NodeID != "customer:charlie" || lastStep.Depth != 3 {
		t.Fatalf("expected path ending at Charlie at depth 3, got %+v", lastStep)
	}
}

func TestOntologyCycleHandling(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	ctx := context.Background()

	// Form cycle: NodeA -> NodeB -> NodeC -> NodeA
	_ = repo.UpsertNode(ctx, store.Node{ID: "node:A", Label: "A", Type: "test"})
	_ = repo.UpsertNode(ctx, store.Node{ID: "node:B", Label: "B", Type: "test"})
	_ = repo.UpsertNode(ctx, store.Node{ID: "node:C", Label: "C", Type: "test"})

	_ = repo.UpsertEdge(ctx, store.Edge{Source: "node:A", Target: "node:B", Relationship: "next"})
	_ = repo.UpsertEdge(ctx, store.Edge{Source: "node:B", Target: "node:C", Relationship: "next"})
	_ = repo.UpsertEdge(ctx, store.Edge{Source: "node:C", Target: "node:A", Relationship: "loop"})

	// Must terminate cleanly without infinite recursion
	steps, err := repo.TraverseNeighbors(ctx, "node:A", 10)
	if err != nil {
		t.Fatalf("TraverseNeighbors with cycle failed: %v", err)
	}
	// Total steps traversed without visiting already visited nodes in the current path
	if len(steps) != 2 {
		t.Fatalf("expected 2 unique reachable steps (B and C) before cycle prevention stops, got %d: %+v", len(steps), steps)
	}
}
