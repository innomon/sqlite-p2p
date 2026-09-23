package tests_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"sqlite-p2p/internal/p2p"
	"sqlite-p2p/internal/store"
)

func TestMultiNodeDualWriteConvergence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	baseDir := t.TempDir()

	// --- Node 1 Setup ---
	node1Dir := filepath.Join(baseDir, "node1")
	db1, err := store.OpenDB(filepath.Join(node1Dir, "crm.db"), true)
	if err != nil {
		t.Fatalf("db1 open error: %v", err)
	}
	defer db1.Close()
	repo1 := store.NewRepository(db1)

	feed1, err := p2p.NewChangesetFeed(filepath.Join(node1Dir, "feed"))
	if err != nil {
		t.Fatalf("feed1 init error: %v", err)
	}

	var engine1 *p2p.ReplicationEngine
	rep1 := p2p.NewReplicator(feed1, func(cs *store.Changeset) error {
		return engine1.ApplyRemoteChangeset(context.Background(), cs)
	})
	defer rep1.Close()
	engine1 = p2p.NewReplicationEngine(repo1, feed1, rep1)

	// --- Node 2 Setup ---
	node2Dir := filepath.Join(baseDir, "node2")
	db2, err := store.OpenDB(filepath.Join(node2Dir, "crm.db"), true)
	if err != nil {
		t.Fatalf("db2 open error: %v", err)
	}
	defer db2.Close()
	repo2 := store.NewRepository(db2)

	feed2, err := p2p.NewChangesetFeed(filepath.Join(node2Dir, "feed"))
	if err != nil {
		t.Fatalf("feed2 init error: %v", err)
	}

	var engine2 *p2p.ReplicationEngine
	rep2 := p2p.NewReplicator(feed2, func(cs *store.Changeset) error {
		return engine2.ApplyRemoteChangeset(context.Background(), cs)
	})
	defer rep2.Close()
	engine2 = p2p.NewReplicationEngine(repo2, feed2, rep2)

	// --- Connect Node 1 & Node 2 ---
	conn1, conn2 := net.Pipe()
	defer conn1.Close()
	defer conn2.Close()

	rep1.AddPeer(conn1)
	rep2.AddPeer(conn2)
	go rep1.HandleConnection(conn1)
	go rep2.HandleConnection(conn2)

	// --- Action 1: Node 1 writes Customer A ---
	phoneA := "+91-9988776655"
	keyA, err := store.FormatCustomerKey(phoneA)
	if err != nil {
		t.Fatalf("FormatCustomerKey A: %v", err)
	}
	metaA := json.RawMessage(`{"tier":"platinum","status":"active"}`)
	payloadA := []byte("payload-customer-A")

	err = engine1.PutLocal(ctx, keyA, metaA, payloadA)
	if err != nil {
		t.Fatalf("engine1 PutLocal: %v", err)
	}

	// --- Action 2: Node 2 writes Customer B ---
	phoneB := "+91-9123456789"
	keyB, err := store.FormatCustomerKey(phoneB)
	if err != nil {
		t.Fatalf("FormatCustomerKey B: %v", err)
	}
	metaB := json.RawMessage(`{"tier":"gold","status":"pending"}`)
	payloadB := []byte("payload-customer-B")

	err = engine2.PutLocal(ctx, keyB, metaB, payloadB)
	if err != nil {
		t.Fatalf("engine2 PutLocal: %v", err)
	}

	// Allow propagation
	time.Sleep(100 * time.Millisecond)

	// --- Verification 1: Both nodes must have Customer A and Customer B ---
	recAOnNode2, err := repo2.Get(ctx, keyA)
	if err != nil {
		t.Fatalf("Node 2 missing Customer A: %v", err)
	}
	if string(recAOnNode2.Data) != string(payloadA) {
		t.Errorf("Customer A data mismatch on Node 2: %s vs %s", recAOnNode2.Data, payloadA)
	}

	recBOnNode1, err := repo1.Get(ctx, keyB)
	if err != nil {
		t.Fatalf("Node 1 missing Customer B: %v", err)
	}
	if string(recBOnNode1.Data) != string(payloadB) {
		t.Errorf("Customer B data mismatch on Node 1: %s vs %s", recBOnNode1.Data, payloadB)
	}

	// --- Action 3: Concurrent Conflict Resolution Test on Same Key ---
	// Both nodes update Customer A simultaneously with different timestamps
	timeOlder := time.Now().UnixNano()
	timeNewer := timeOlder + int64(time.Hour)

	csOlder := &store.Changeset{
		Timestamp: timeOlder,
		Sequence:  uint64(timeOlder),
		Operation: store.OpUpdate,
		Key:       keyA,
		Metadata:  json.RawMessage(`{"tier":"bronze"}`),
		Data:      []byte("bronze-payload"),
	}
	csNewer := &store.Changeset{
		Timestamp: timeNewer,
		Sequence:  uint64(timeNewer),
		Operation: store.OpUpdate,
		Key:       keyA,
		Metadata:  json.RawMessage(`{"tier":"vip"}`),
		Data:      []byte("vip-payload"),
	}

	// Apply newer first, then older on Node 1
	_ = engine1.ApplyRemoteChangeset(ctx, csNewer)
	_ = engine1.ApplyRemoteChangeset(ctx, csOlder)

	// Apply older first, then newer on Node 2
	_ = engine2.ApplyRemoteChangeset(ctx, csOlder)
	_ = engine2.ApplyRemoteChangeset(ctx, csNewer)

	finalOn1, err := repo1.Get(ctx, keyA)
	if err != nil {
		t.Fatalf("Get keyA on Node 1: %v", err)
	}
	finalOn2, err := repo2.Get(ctx, keyA)
	if err != nil {
		t.Fatalf("Get keyA on Node 2: %v", err)
	}

	// Both must converge to "vip-payload"
	if string(finalOn1.Data) != "vip-payload" || string(finalOn2.Data) != "vip-payload" {
		t.Fatalf("LWW state divergence: Node 1 has %s, Node 2 has %s", finalOn1.Data, finalOn2.Data)
	}
}

func TestLaggedPeerSyncReconciliation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	baseDir := t.TempDir()

	// --- Node 1 Setup ---
	node1Dir := filepath.Join(baseDir, "node1")
	db1, err := store.OpenDB(filepath.Join(node1Dir, "crm.db"), true)
	if err != nil {
		t.Fatalf("db1 open error: %v", err)
	}
	defer db1.Close()
	repo1 := store.NewRepository(db1)

	feed1, err := p2p.NewChangesetFeed(filepath.Join(node1Dir, "feed"))
	if err != nil {
		t.Fatalf("feed1 init error: %v", err)
	}

	var engine1 *p2p.ReplicationEngine
	rep1 := p2p.NewReplicator(feed1, func(cs *store.Changeset) error {
		return engine1.ApplyRemoteChangeset(context.Background(), cs)
	})
	defer rep1.Close()
	engine1 = p2p.NewReplicationEngine(repo1, feed1, rep1)

	// --- Node 2 Setup (initially disconnected / lagged) ---
	node2Dir := filepath.Join(baseDir, "node2")
	db2, err := store.OpenDB(filepath.Join(node2Dir, "crm.db"), true)
	if err != nil {
		t.Fatalf("db2 open error: %v", err)
	}
	defer db2.Close()
	repo2 := store.NewRepository(db2)

	feed2, err := p2p.NewChangesetFeed(filepath.Join(node2Dir, "feed"))
	if err != nil {
		t.Fatalf("feed2 init error: %v", err)
	}

	var engine2 *p2p.ReplicationEngine
	rep2 := p2p.NewReplicator(feed2, func(cs *store.Changeset) error {
		return engine2.ApplyRemoteChangeset(context.Background(), cs)
	})
	defer rep2.Close()
	engine2 = p2p.NewReplicationEngine(repo2, feed2, rep2)

	// Node 1 writes 3 customer records while Node 2 is offline
	for i := 1; i <= 3; i++ {
		key, _ := store.FormatCustomerKey(fmt.Sprintf("+91-900000000%d", i))
		err := engine1.PutLocal(ctx, key, json.RawMessage(`{"status":"offline-written"}`), []byte(fmt.Sprintf("offline-data-%d", i)))
		if err != nil {
			t.Fatalf("PutLocal %d: %v", i, err)
		}
	}

	// Verify Node 2 does not have the records yet
	key1, _ := store.FormatCustomerKey("+91-9000000001")
	_, err = repo2.Get(ctx, key1)
	if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound on lagged node before connection, got: %v", err)
	}

	// Connect Node 1 & Node 2
	conn1, conn2 := net.Pipe()
	defer conn1.Close()
	defer conn2.Close()

	rep1.AddPeer(conn1)
	rep2.AddPeer(conn2)
	go rep1.HandleConnection(conn1)
	go rep2.HandleConnection(conn2)

	// Trigger sync from Node 1 to catch up Node 2
	count, err := engine1.SyncPeers(ctx)
	if err != nil {
		t.Fatalf("SyncPeers error: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 broadcast changesets, got %d", count)
	}

	// Allow propagation
	time.Sleep(100 * time.Millisecond)

	// Verify Node 2 received and applied all 3 records
	for i := 1; i <= 3; i++ {
		key, _ := store.FormatCustomerKey(fmt.Sprintf("+91-900000000%d", i))
		rec, err := repo2.Get(ctx, key)
		if err != nil {
			t.Fatalf("Node 2 missing synced record %d: %v", i, err)
		}
		expected := fmt.Sprintf("offline-data-%d", i)
		if string(rec.Data) != expected {
			t.Errorf("Node 2 record %d mismatch: got %s, want %s", i, string(rec.Data), expected)
		}
	}
}

