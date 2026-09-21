package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"crm-sqlite-pear-p2p/internal/crypto"
	"crm-sqlite-pear-p2p/internal/store"
)

func TestChangesetEncodeDecode(t *testing.T) {
	cs := &store.Changeset{
		Timestamp: time.Now().UnixNano(),
		Sequence:  101,
		Operation: store.OpInsert,
		Key:       "in.qzip.crm.customer:XYZ123",
		Metadata:  json.RawMessage(`{"schemas":[{"$ID":"https://qzip.in/schemas/crm/customer-v1.json"}]}`),
		Data:      []byte("payload-bytes"),
		PrevHash:  []byte("sha256-dummy-hash"),
	}

	encoded, err := cs.Encode()
	if err != nil {
		t.Fatalf("failed to encode changeset: %v", err)
	}
	if len(encoded) == 0 {
		t.Fatalf("expected non-empty encoded bytes")
	}

	decoded, err := store.DecodeChangeset(encoded)
	if err != nil {
		t.Fatalf("failed to decode changeset: %v", err)
	}

	if decoded.Key != cs.Key {
		t.Errorf("expected key %s, got %s", cs.Key, decoded.Key)
	}
	if decoded.Sequence != cs.Sequence {
		t.Errorf("expected sequence %d, got %d", cs.Sequence, decoded.Sequence)
	}
	if decoded.Operation != cs.Operation {
		t.Errorf("expected op %d, got %d", cs.Operation, decoded.Operation)
	}
	if string(decoded.Data) != string(cs.Data) {
		t.Errorf("expected data %s, got %s", string(cs.Data), string(decoded.Data))
	}
	if string(decoded.Metadata) != string(cs.Metadata) {
		t.Errorf("expected metadata %s, got %s", string(cs.Metadata), string(decoded.Metadata))
	}
}

func TestChangesetTrackerAndApply(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	tracker := store.NewChangesetTracker(repo)
	ctx := context.Background()

	var captured []*store.Changeset
	tracker.Subscribe(func(cs *store.Changeset, raw []byte) {
		captured = append(captured, cs)
	})

	key, err := store.FormatCustomerKey("9123456780")
	if err != nil {
		t.Fatalf("format key failed: %v", err)
	}

	meta := json.RawMessage(`{"schemas":[{"$ID":"https://qzip.in/schemas/crm/customer-v1.json","status":"new"}]}`)
	data := []byte("secret-customer-data")

	// 1. Put through tracker
	err = tracker.Put(ctx, key, meta, data)
	if err != nil {
		t.Fatalf("tracker put failed: %v", err)
	}

	if len(captured) != 1 {
		t.Fatalf("expected 1 captured changeset, got %d", len(captured))
	}
	if captured[0].Operation != store.OpInsert && captured[0].Operation != store.OpUpdate {
		t.Errorf("expected insert or update op, got %d", captured[0].Operation)
	}

	// 2. Delete through tracker
	err = tracker.Delete(ctx, key)
	if err != nil {
		t.Fatalf("tracker delete failed: %v", err)
	}

	if len(captured) != 2 {
		t.Fatalf("expected 2 captured changesets, got %d", len(captured))
	}
	if captured[1].Operation != store.OpDelete {
		t.Errorf("expected delete op, got %d", captured[1].Operation)
	}

	// 3. Apply remote changeset to a second DB
	db2, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open db2: %v", err)
	}
	defer db2.Close()

	repo2 := store.NewRepository(db2)
	// Apply the first changeset (insert) to repo2
	err = store.ApplyChangeset(ctx, repo2, captured[0])
	if err != nil {
		t.Fatalf("apply changeset failed: %v", err)
	}

	rec, err := repo2.Get(ctx, key)
	if err != nil {
		t.Fatalf("expected record in repo2: %v", err)
	}
	if rec.Key != key {
		t.Errorf("expected key %s, got %s", key, rec.Key)
	}
}

func TestChangesetTracker_EncryptionAndCryptoShredding(t *testing.T) {
	db1, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open db1: %v", err)
	}
	defer db1.Close()

	keys1, err := crypto.NewKeyRegistry(db1)
	if err != nil {
		t.Fatalf("failed to init keys1: %v", err)
	}

	repo1 := store.NewRepository(db1)
	repo1.SetKeyRegistry(keys1)
	tracker1 := store.NewChangesetTracker(repo1)
	ctx := context.Background()

	var captured []*store.Changeset
	tracker1.Subscribe(func(cs *store.Changeset, raw []byte) {
		captured = append(captured, cs)
	})

	key, _ := store.FormatCustomerKey("9998887776")
	meta := json.RawMessage(`{"schemas":[{"$ID":"https://qzip.in/schemas/crm/customer-v1.json"}]}`)
	plaintext := []byte("Highly confidential PII data")

	// 1. Put through tracker
	if err := tracker1.Put(ctx, key, meta, plaintext); err != nil {
		t.Fatalf("tracker put failed: %v", err)
	}

	if len(captured) != 1 {
		t.Fatalf("expected 1 captured changeset, got %d", len(captured))
	}

	// Changeset Data must be ciphertext, NOT plaintext
	if string(captured[0].Data) == string(plaintext) {
		t.Fatal("captured changeset data must be ciphertext, found plaintext")
	}

	// 2. Replicate to node 2
	db2, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("failed to open db2: %v", err)
	}
	defer db2.Close()

	keys2, err := crypto.NewKeyRegistry(db2)
	if err != nil {
		t.Fatalf("failed to init keys2: %v", err)
	}

	// Share/provision symmetric key to node 2
	symKey, err := keys1.GetKey(key)
	if err != nil {
		t.Fatalf("failed to get sym key: %v", err)
	}
	if err := keys2.SetKey(key, symKey); err != nil {
		t.Fatalf("failed to set sym key on node 2: %v", err)
	}

	repo2 := store.NewRepository(db2)
	repo2.SetKeyRegistry(keys2)

	// Apply changeset to node 2
	if err := store.ApplyChangeset(ctx, repo2, captured[0]); err != nil {
		t.Fatalf("failed to apply changeset on node 2: %v", err)
	}

	// Node 2 should be able to get and decrypt
	rec2, err := repo2.Get(ctx, key)
	if err != nil {
		t.Fatalf("failed to get and decrypt on node 2: %v", err)
	}
	if string(rec2.Data) != string(plaintext) {
		t.Fatalf("expected %s, got %s", string(plaintext), string(rec2.Data))
	}

	// 3. Delete through tracker
	if err := tracker1.Delete(ctx, key); err != nil {
		t.Fatalf("tracker delete failed: %v", err)
	}

	if len(captured) != 2 {
		t.Fatalf("expected 2 captured changesets, got %d", len(captured))
	}

	// Node 1 key is crypto-shredded
	hasKey1, _ := keys1.HasKey(key)
	if hasKey1 {
		t.Fatal("expected key1 to be purged on node 1")
	}

	// Apply delete changeset on node 2
	if err := store.ApplyChangeset(ctx, repo2, captured[1]); err != nil {
		t.Fatalf("failed to apply delete changeset on node 2: %v", err)
	}

	// Node 2 key is crypto-shredded
	hasKey2, _ := keys2.HasKey(key)
	if hasKey2 {
		t.Fatal("expected key2 to be purged on node 2")
	}
}
