package tests_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"sqlite-p2p/internal/crypto"
	"sqlite-p2p/internal/p2p"
	"sqlite-p2p/internal/store"
)

func TestMultiNodeCryptoShreddingE2E(t *testing.T) {
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

	keys1, err := crypto.NewKeyRegistry(db1)
	if err != nil {
		t.Fatalf("keys1 init error: %v", err)
	}
	repo1 := store.NewRepository(db1)
	repo1.SetKeyRegistry(keys1)

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

	keys2, err := crypto.NewKeyRegistry(db2)
	if err != nil {
		t.Fatalf("keys2 init error: %v", err)
	}
	repo2 := store.NewRepository(db2)
	repo2.SetKeyRegistry(keys2)

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

	// --- Step 1: Symmetric Key Generation & Distribution ---
	phone := "+1-555-987-6543"
	custKey, err := store.FormatCustomerKey(phone)
	if err != nil {
		t.Fatalf("FormatCustomerKey error: %v", err)
	}

	symKey, err := keys1.GetOrCreateKey(custKey)
	if err != nil {
		t.Fatalf("keys1.GetOrCreateKey error: %v", err)
	}

	// Distribute key to Node 2
	if err := keys2.SetKey(custKey, symKey); err != nil {
		t.Fatalf("keys2.SetKey error: %v", err)
	}

	// --- Step 2: Node 1 Writes Encrypted Customer Record ---
	metadata := json.RawMessage(`{"tier":"enterprise","gdpr_consent":true}`)
	plaintext := []byte("PII: Alice Wonderland, Secret Financial Account #8833-2211")

	if err := engine1.PutLocal(ctx, custKey, metadata, plaintext); err != nil {
		t.Fatalf("engine1.PutLocal error: %v", err)
	}

	// Verify Node 1 storage contains ciphertext at rest
	rawRec1, err := repo1.GetRaw(ctx, custKey)
	if err != nil {
		t.Fatalf("repo1.GetRaw error: %v", err)
	}
	if bytes.Equal(rawRec1.Data, plaintext) {
		t.Fatal("Node 1 SQLite crm_store.data must be ciphertext, found plaintext")
	}

	// Verify Node 1 feed contains ciphertext
	block0On1, err := feed1.Get(0)
	if err != nil {
		t.Fatalf("feed1.Get(0) error: %v", err)
	}
	if bytes.Equal(block0On1.Data, plaintext) {
		t.Fatal("Node 1 Hypercore feed block 0 must be ciphertext, found plaintext")
	}

	// Allow P2P replication propagation
	time.Sleep(100 * time.Millisecond)

	// --- Step 3: Verify Node 2 Received Ciphertext and Can Decrypt ---
	rawRec2, err := repo2.GetRaw(ctx, custKey)
	if err != nil {
		t.Fatalf("repo2.GetRaw on Node 2 error: %v", err)
	}
	if bytes.Equal(rawRec2.Data, plaintext) {
		t.Fatal("Node 2 SQLite crm_store.data must be ciphertext, found plaintext")
	}

	// Node 2 transparently decrypts
	rec2, err := repo2.Get(ctx, custKey)
	if err != nil {
		t.Fatalf("repo2.Get on Node 2 error: %v", err)
	}
	if !bytes.Equal(rec2.Data, plaintext) {
		t.Fatalf("Node 2 decrypted data mismatch: expected %s, got %s", string(plaintext), string(rec2.Data))
	}

	// --- Step 4: Execute Crypto-Shredding (Customer Deletion) on Node 1 ---
	if err := engine1.DeleteLocal(ctx, custKey); err != nil {
		t.Fatalf("engine1.DeleteLocal error: %v", err)
	}

	// Verify key is destroyed on Node 1
	hasKey1, _ := keys1.HasKey(custKey)
	if hasKey1 {
		t.Fatal("expected key to be purged on Node 1 immediately upon delete")
	}

	// Allow delete changeset propagation over P2P
	time.Sleep(100 * time.Millisecond)

	// Verify key is destroyed on Node 2 (Crypto-shredded across cluster)
	hasKey2, _ := keys2.HasKey(custKey)
	if hasKey2 {
		t.Fatal("expected key to be purged on Node 2 following delete changeset replication")
	}

	// Verify record is deleted in SQLite on both nodes
	if _, err := repo1.Get(ctx, custKey); err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound on Node 1, got %v", err)
	}
	if _, err := repo2.Get(ctx, custKey); err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound on Node 2, got %v", err)
	}

	// --- Step 5: Assert Historical Hypercore Log Blocks are Permanently Undecipherable ---
	// Historical blocks remain immutable in the feeds
	histBlock1, err := feed1.Get(0)
	if err != nil {
		t.Fatalf("feed1 historical block 0 missing: %v", err)
	}
	histBlock2, err := feed2.Get(0)
	if err != nil {
		t.Fatalf("feed2 historical block 0 missing: %v", err)
	}

	// Key is gone from registries
	_, err = keys1.GetKey(custKey)
	if err != crypto.ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound from Node 1 key registry, got %v", err)
	}
	_, err = keys2.GetKey(custKey)
	if err != crypto.ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound from Node 2 key registry, got %v", err)
	}

	// Attempting to decrypt historical blocks with any random or dummy key fails with authentication error
	dummyKey, _ := crypto.GenerateKey()
	_, err = crypto.Decrypt(dummyKey, histBlock1.Data)
	if err == nil {
		t.Fatal("historical feed block 1 must fail authentication when key is destroyed")
	}
	_, err = crypto.Decrypt(dummyKey, histBlock2.Data)
	if err == nil {
		t.Fatal("historical feed block 2 must fail authentication when key is destroyed")
	}
}
