package p2p_test

import (
	"encoding/json"
	"testing"
	"time"

	"crm-sqlite-pear-p2p/internal/p2p"
	"crm-sqlite-pear-p2p/internal/store"
)

func TestAutobaseLinearization(t *testing.T) {
	tempDir := t.TempDir()

	// Local writer feed
	feedLocal, err := p2p.NewChangesetFeed(tempDir + "/local")
	if err != nil {
		t.Fatalf("feedLocal: %v", err)
	}

	// Remote peer feed
	feedRemote, err := p2p.NewChangesetFeed(tempDir + "/remote")
	if err != nil {
		t.Fatalf("feedRemote: %v", err)
	}

	ab := p2p.NewAutobaseManager(feedLocal)
	ab.AddRemoteWriter(feedRemote.Core())

	// Node 1 writes
	cs1 := &store.Changeset{
		Timestamp: time.Now().UnixNano(),
		Sequence:  1,
		Operation: store.OpInsert,
		Key:       "in.qzip.crm.customer:USER1",
		Metadata:  json.RawMessage(`{"tier":"standard"}`),
	}
	_, err = ab.Append(cs1)
	if err != nil {
		t.Fatalf("ab.Append cs1: %v", err)
	}

	// Node 2 writes directly to its feed
	cs2 := &store.Changeset{
		Timestamp: time.Now().UnixNano(),
		Sequence:  2,
		Operation: store.OpInsert,
		Key:       "in.qzip.crm.customer:USER2",
		Metadata:  json.RawMessage(`{"tier":"gold"}`),
	}
	_, err = feedRemote.Append(cs2)
	if err != nil {
		t.Fatalf("feedRemote.Append cs2: %v", err)
	}

	linearized, err := ab.LinearizeChangesets()
	if err != nil {
		t.Fatalf("LinearizeChangesets: %v", err)
	}

	if len(linearized) < 1 {
		t.Fatalf("expected at least 1 linearized changeset, got %d", len(linearized))
	}

	if ab.WriterCount() != 2 {
		t.Fatalf("expected 2 writers, got %d", ab.WriterCount())
	}
}

