package p2p_test

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"crm-sqlite-pear-p2p/internal/p2p"
	"crm-sqlite-pear-p2p/internal/store"
)

func TestBidirectionalChangesetReplication(t *testing.T) {
	tempDir := t.TempDir()

	feed1, err := p2p.NewChangesetFeed(filepath.Join(tempDir, "node1_feed"))
	if err != nil {
		t.Fatalf("feed1 init: %v", err)
	}
	feed2, err := p2p.NewChangesetFeed(filepath.Join(tempDir, "node2_feed"))
	if err != nil {
		t.Fatalf("feed2 init: %v", err)
	}

	conn1, conn2 := net.Pipe()
	defer conn1.Close()
	defer conn2.Close()

	recvedOnNode2 := make(chan *store.Changeset, 5)
	rep2 := p2p.NewReplicator(feed2, func(cs *store.Changeset) error {
		recvedOnNode2 <- cs
		return nil
	})

	recvedOnNode1 := make(chan *store.Changeset, 5)
	rep1 := p2p.NewReplicator(feed1, func(cs *store.Changeset) error {
		recvedOnNode1 <- cs
		return nil
	})

	rep1.AddPeer(conn1)
	rep2.AddPeer(conn2)
	go rep1.HandleConnection(conn1)
	go rep2.HandleConnection(conn2)

	if rep1.PeerCount() != 1 || rep2.PeerCount() != 1 {
		t.Fatalf("expected peer count 1 on both replicators")
	}

	// Send changeset from Node 1 to Node 2
	cs1 := &store.Changeset{
		Timestamp: time.Now().UnixNano(),
		Sequence:  1,
		Operation: store.OpInsert,
		Key:       "in.qzip.crm.customer:KEY1",
		Metadata:  json.RawMessage(`{"tier":"platinum"}`),
		Data:      []byte("payload-1"),
	}

	err = rep1.BroadcastChangeset(cs1)
	if err != nil {
		t.Fatalf("rep1 broadcast error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	select {
	case received := <-recvedOnNode2:
		if received.Key != cs1.Key {
			t.Fatalf("expected key %s, got %s", cs1.Key, received.Key)
		}
		if string(received.Data) != string(cs1.Data) {
			t.Fatalf("expected data %s, got %s", string(cs1.Data), string(received.Data))
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for changeset on Node 2")
	}

	// Send changeset from Node 2 to Node 1
	cs2 := &store.Changeset{
		Timestamp: time.Now().UnixNano(),
		Sequence:  2,
		Operation: store.OpUpdate,
		Key:       "in.qzip.crm.customer:KEY2",
		Metadata:  json.RawMessage(`{"tier":"silver"}`),
		Data:      []byte("payload-2"),
	}

	err = rep2.BroadcastChangeset(cs2)
	if err != nil {
		t.Fatalf("rep2 broadcast error: %v", err)
	}

	select {
	case received := <-recvedOnNode1:
		if received.Key != cs2.Key {
			t.Fatalf("expected key %s, got %s", cs2.Key, received.Key)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for changeset on Node 1")
	}

	// Close replicators
	_ = rep1.Close()
	_ = rep2.Close()
	if rep1.PeerCount() != 0 {
		t.Fatalf("expected 0 peers after close")
	}
}

func TestReplicatorInvalidPayload(t *testing.T) {
	conn1, conn2 := net.Pipe()
	defer conn1.Close()
	defer conn2.Close()

	rep := p2p.NewReplicator(nil, nil)
	go rep.HandleConnection(conn1)

	// Send an oversized length header
	badHeader := []byte{0xff, 0xff, 0xff, 0xff}
	_, _ = conn2.Write(badHeader)

	time.Sleep(20 * time.Millisecond)
	_ = rep.Close()
}
