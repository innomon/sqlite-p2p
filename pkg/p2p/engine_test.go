package p2p_test

import (
	"context"
	"encoding/json"
	"testing"

	"sqlite-p2p/internal/store"
	"sqlite-p2p/pkg/p2p"

	"go-pear/pkg/policy"
	"go-pear/pkg/secretstream"
)

func TestEngine_Lifecycle(t *testing.T) {
	ctx := context.Background()

	opts := p2p.EngineOptions{
		DBPath:    ":memory:",
		EnableWAL: false,
	}

	engine, err := p2p.OpenEngine(opts)
	if err != nil {
		t.Fatalf("OpenEngine failed: %v", err)
	}
	defer engine.Close()

	if engine.DB() == nil {
		t.Fatal("expected non-nil DB")
	}
	if engine.Repository() == nil {
		t.Fatal("expected non-nil Repository")
	}
	if engine.ChangesetTracker() == nil {
		t.Fatal("expected non-nil ChangesetTracker")
	}

	// Track changesets
	var emitted []*store.Changeset
	engine.ChangesetTracker().Subscribe(func(cs *store.Changeset, raw []byte) {
		emitted = append(emitted, cs)
	})

	// Put record
	key := "test:item:1"
	meta := json.RawMessage(`{"type":"test"}`)
	data := []byte("payload data")
	if err := engine.Put(ctx, key, meta, data); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get record
	rec, err := engine.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if rec.Key != key || string(rec.Data) != "payload data" {
		t.Fatalf("unexpected record: %+v", rec)
	}

	// Delete record
	if err := engine.Delete(ctx, key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = engine.Get(ctx, key)
	if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	if len(emitted) != 2 { // 1 insert, 1 delete
		t.Fatalf("expected 2 emitted changesets, got %d", len(emitted))
	}
}

func TestEngine_WithExistingDB(t *testing.T) {
	db, err := store.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	opts := p2p.EngineOptions{
		DB: db,
	}

	engine, err := p2p.OpenEngine(opts)
	if err != nil {
		t.Fatalf("OpenEngine with existing db failed: %v", err)
	}

	if engine.DB() != db {
		t.Fatal("expected engine to use provided DB")
	}

	if err := engine.Close(); err != nil {
		t.Fatalf("engine.Close failed: %v", err)
	}
}

func TestEngine_WithCrypto(t *testing.T) {
	ctx := context.Background()

	opts := p2p.EngineOptions{
		DBPath:       ":memory:",
		EnableCrypto: true,
	}

	engine, err := p2p.OpenEngine(opts)
	if err != nil {
		t.Fatalf("OpenEngine with crypto failed: %v", err)
	}
	defer engine.Close()

	if engine.KeyRegistry() == nil {
		t.Fatal("expected non-nil KeyRegistry")
	}

	key := "customer:123"
	if err := engine.Put(ctx, key, json.RawMessage(`{}`), []byte("sensitive profile")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify plaintext in Get
	rec, err := engine.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(rec.Data) != "sensitive profile" {
		t.Fatalf("unexpected decrypted data: %s", string(rec.Data))
	}

	// Verify raw ciphertext in DB
	rawRec, err := engine.Repository().GetRaw(ctx, key)
	if err != nil {
		t.Fatalf("GetRaw failed: %v", err)
	}
	if string(rawRec.Data) == "sensitive profile" {
		t.Fatal("expected data to be encrypted on disk")
	}

	// Crypto shredding: purge key from registry
	if err := engine.KeyRegistry().PurgeKey(key); err != nil {
		t.Fatalf("PurgeKey failed: %v", err)
	}
	_, err = engine.Get(ctx, key)
	if err == nil {
		t.Fatal("expected decryption error after key deletion (crypto-shredded)")
	}
}

func TestEngine_Validation(t *testing.T) {
	_, err := p2p.OpenEngine(p2p.EngineOptions{})
	if err == nil {
		t.Fatal("expected error when DBPath and DB are both empty")
	}
}

func TestEngine_WithSwarmTopic(t *testing.T) {
	ctx := context.Background()

	var topic [32]byte
	copy(topic[:], "test-cluster-topic-123456789012")

	opts := p2p.EngineOptions{
		DBPath:     ":memory:",
		SwarmTopic: topic,
	}

	engine, err := p2p.OpenEngine(opts)
	if err != nil {
		t.Fatalf("OpenEngine with topic failed: %v", err)
	}
	defer engine.Close()

	if engine.ReplicationEngine() == nil {
		t.Fatal("expected non-nil ReplicationEngine")
	}

	if err := engine.Put(ctx, "replicated:key", json.RawMessage(`{}`), []byte("sync data")); err != nil {
		t.Fatalf("Put with replication failed: %v", err)
	}

	rec, err := engine.Get(ctx, "replicated:key")
	if err != nil || string(rec.Data) != "sync data" {
		t.Fatalf("unexpected record: %v (err: %v)", rec, err)
	}

	if err := engine.Delete(ctx, "replicated:key"); err != nil {
		t.Fatalf("Delete with replication failed: %v", err)
	}
}

func TestEngine_ReplicationPolicy(t *testing.T) {
	var topic [32]byte
	copy(topic[:], "test-engine-policy-topic-1234567")

	kp, _ := secretstream.GenerateKeyPair()
	kpAllowed, _ := secretstream.GenerateKeyPair()
	kpDenied, _ := secretstream.GenerateKeyPair()

	pol := policy.New(policy.ModeWhitelist)
	pol.AddWhitelist(kpAllowed.Public)

	opts := p2p.EngineOptions{
		DBPath:     ":memory:",
		SwarmTopic: topic,
		KeyPair:    kp,
		Policy:     pol,
	}

	engine, err := p2p.OpenEngine(opts)
	if err != nil {
		t.Fatalf("OpenEngine failed: %v", err)
	}
	defer engine.Close()

	if engine.Policy() == nil {
		t.Fatal("expected non-nil policy from Engine")
	}

	if engine.Policy().Mode() != policy.ModeWhitelist {
		t.Fatalf("expected ModeWhitelist, got %s", engine.Policy().Mode())
	}

	if !engine.Policy().IsAllowed(kpAllowed.Public) {
		t.Errorf("expected allowed peer to be permitted")
	}
	if engine.Policy().IsAllowed(kpDenied.Public) {
		t.Errorf("expected denied peer to be rejected")
	}

	// Dynamic policy mutation
	newPol := policy.New(policy.ModeAllAllowed)
	engine.SetPolicy(newPol)

	if engine.Policy().Mode() != policy.ModeAllAllowed {
		t.Fatalf("expected ModeAllAllowed after SetPolicy, got %s", engine.Policy().Mode())
	}

	if !engine.Policy().IsAllowed(kpDenied.Public) {
		t.Errorf("expected denied peer to be permitted in ModeAllAllowed")
	}

	// DisconnectPeer and EvictDisallowed smoke tests
	_ = engine.DisconnectPeer(kpDenied.Public)
	evicted := engine.EvictDisallowed()
	if evicted != 0 {
		t.Fatalf("expected 0 evicted peers, got %d", evicted)
	}
}

