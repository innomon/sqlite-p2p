package mcp

import (
	"context"
	"testing"
)

func TestP2PTools_Lifecycle(t *testing.T) {
	repo, _, _, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	opts := ServerOptions{
		Version:   "1.0.0",
		DBPath:    "/tmp/test_crm.db",
		EnableWAL: true,
		Repo:      repo,
	}

	handler := NewP2PToolHandler(opts, nil, nil)

	// 1. P2PStatus (with nil swarm/engine)
	res, statusOut, err := handler.P2PStatus(ctx, nil, P2PStatusInput{})
	if err != nil {
		t.Fatalf("P2PStatus failed: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("P2PStatus returned error: %v", res.Content)
	}
	if statusOut.ConnectedPeers != 0 {
		t.Errorf("expected 0 connected peers, got %d", statusOut.ConnectedPeers)
	}

	// 2. P2PPeers
	res, peersOut, err := handler.P2PPeers(ctx, nil, P2PPeersInput{})
	if err != nil {
		t.Fatalf("P2PPeers failed: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("P2PPeers returned error")
	}
	if peersOut.Count != 0 {
		t.Errorf("expected 0 peers, got %d", peersOut.Count)
	}

	// 3. P2PSync
	res, syncOut, err := handler.P2PSync(ctx, nil, P2PSyncInput{})
	if err != nil {
		t.Fatalf("P2PSync failed: %v", err)
	}
	if res != nil && res.IsError {
		t.Fatalf("P2PSync returned error")
	}
	if syncOut.BroadcastCount != 0 {
		t.Errorf("expected 0 broadcast changesets, got %d", syncOut.BroadcastCount)
	}
}
