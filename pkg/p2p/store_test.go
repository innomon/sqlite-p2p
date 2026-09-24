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
}
