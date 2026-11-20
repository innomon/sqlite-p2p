package p2p_test

import (
	"context"
	"testing"

	"sqlite-p2p/pkg/p2p"
)

func TestStoreExports(t *testing.T) {
	db, err := p2p.OpenDB(":memory:", false)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	repo := p2p.NewRepository(db)
	if repo == nil {
		t.Fatal("NewRepository returned nil")
	}

	tracker := p2p.NewChangesetTracker(repo)
	if tracker == nil {
		t.Fatal("NewChangesetTracker returned nil")
	}

	keys, err := p2p.NewKeyRegistry(db)
	if err != nil {
		t.Fatalf("NewKeyRegistry failed: %v", err)
	}
	if keys == nil {
		t.Fatal("NewKeyRegistry returned nil")
	}

	ctx := context.Background()
	_, err = repo.Get(ctx, "nonexistent")
	if err != p2p.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	feedDir := t.TempDir()
	feed, err := p2p.NewChangesetFeed(feedDir)
	if err != nil {
		t.Fatalf("NewChangesetFeed failed: %v", err)
	}

	rep := p2p.NewReplicator(feed, nil)
	if rep == nil {
		t.Fatal("NewReplicator returned nil")
	}

	replEngine := p2p.NewReplicationEngine(repo, feed, rep)
	if replEngine == nil {
		t.Fatal("NewReplicationEngine returned nil")
	}

	swarm, err := p2p.NewSwarmManager(p2p.SwarmManagerOptions{Port: 0})
	if err != nil {
		t.Fatalf("NewSwarmManager failed: %v", err)
	}
	defer swarm.Close()

	customRepo := p2p.NewRepositoryWithTable(db, "crm_store")
	if customRepo == nil || customRepo.TableName() != "crm_store" {
		t.Fatalf("expected customRepo targeting crm_store")
	}

	cs := &p2p.Changeset{
		Operation: p2p.OpInsert,
		Key:       "k1",
		Metadata:  []byte(`{"ok":true}`),
		Data:      []byte("v1"),
	}
	raw, _ := cs.Encode()
	dec, err := p2p.DecodeChangeset(raw)
	if err != nil {
		t.Fatalf("DecodeChangeset failed: %v", err)
	}
	if err := p2p.ApplyChangeset(ctx, customRepo, dec); err != nil {
		t.Fatalf("ApplyChangeset failed: %v", err)
	}
	_ = p2p.NormalizeDHTAddr("0.0.0.0:4000")
}

