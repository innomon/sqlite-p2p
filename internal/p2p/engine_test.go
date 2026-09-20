package p2p_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"crm-sqlite-pear-p2p/internal/p2p"
	"crm-sqlite-pear-p2p/internal/store"
)

func TestEngineLWWConflictResolution(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	db, err := store.OpenDB(":memory:", true)
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	feed, err := p2p.NewChangesetFeed(filepath.Join(tempDir, "feed"))
	if err != nil {
		t.Fatalf("feed error: %v", err)
	}

	engine := p2p.NewReplicationEngine(repo, feed, nil)

	testKey := "in.qzip.crm.customer:TESTLWW"
	t1 := time.Now().UnixNano()
	t2 := t1 + int64(time.Second) // strictly newer

	// Write newer record first
	newerChangeset := &store.Changeset{
		Timestamp: t2,
		Sequence:  2,
		Operation: store.OpInsert,
		Key:       testKey,
		Metadata:  json.RawMessage(`{"status":"newer"}`),
		Data:      []byte("newer-data"),
	}

	err = engine.ApplyRemoteChangeset(ctx, newerChangeset)
	if err != nil {
		t.Fatalf("apply newer changeset: %v", err)
	}

	rec, err := repo.Get(ctx, testKey)
	if err != nil {
		t.Fatalf("get record: %v", err)
	}
	if string(rec.Data) != "newer-data" {
		t.Fatalf("expected newer-data, got %s", string(rec.Data))
	}

	// Now try applying an older obsolete changeset with timestamp t1 < t2
	olderChangeset := &store.Changeset{
		Timestamp: t1,
		Sequence:  1,
		Operation: store.OpUpdate,
		Key:       testKey,
		Metadata:  json.RawMessage(`{"status":"older"}`),
		Data:      []byte("older-data"),
	}

	err = engine.ApplyRemoteChangeset(ctx, olderChangeset)
	if err != nil {
		t.Fatalf("apply older changeset: %v", err)
	}

	// Record in SQLite must NOT be overwritten because local record has newer timestamp in metadata/feed
	recAfter, err := repo.Get(ctx, testKey)
	if err != nil {
		t.Fatalf("get record after older changeset: %v", err)
	}
	if string(recAfter.Data) != "newer-data" {
		t.Fatalf("LWW failed: older write overwrote newer record! Got: %s", string(recAfter.Data))
	}
}

func TestEngineDeleteAndAccessors(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	db, err := store.OpenDB(":memory:", true)
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	feed, err := p2p.NewChangesetFeed(filepath.Join(tempDir, "del_feed"))
	if err != nil {
		t.Fatalf("feed error: %v", err)
	}

	engine := p2p.NewReplicationEngine(repo, feed, nil)
	if engine.Feed() == nil || engine.Autobase() == nil {
		t.Fatalf("expected non-nil Feed and Autobase accessors")
	}

	testKey := "in.qzip.crm.customer:DELKEY"
	_ = engine.PutLocal(ctx, testKey, json.RawMessage(`{"status":"active"}`), []byte("to-delete"))

	err = engine.DeleteLocal(ctx, testKey)
	if err != nil {
		t.Fatalf("DeleteLocal failed: %v", err)
	}

	_, err = repo.Get(ctx, testKey)
	if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound after DeleteLocal, got %v", err)
	}

	// Apply remote delete
	tNew := time.Now().UnixNano() + int64(time.Hour)
	delChangeset := &store.Changeset{
		Timestamp: tNew,
		Operation: store.OpDelete,
		Key:       testKey,
	}
	err = engine.ApplyRemoteChangeset(ctx, delChangeset)
	if err != nil {
		t.Fatalf("ApplyRemoteChangeset delete failed: %v", err)
	}
}

func TestEngineLocalMutationCapture(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	db, err := store.OpenDB(":memory:", true)
	if err != nil {
		t.Fatalf("OpenDB error: %v", err)
	}
	defer db.Close()

	repo := store.NewRepository(db)
	feed, err := p2p.NewChangesetFeed(filepath.Join(tempDir, "capture_feed"))
	if err != nil {
		t.Fatalf("feed error: %v", err)
	}

	engine := p2p.NewReplicationEngine(repo, feed, nil)

	testKey := "in.qzip.crm.customer:LOCALWRITE"
	err = engine.PutLocal(ctx, testKey, json.RawMessage(`{"name":"Local Agent"}`), []byte("local-payload"))
	if err != nil {
		t.Fatalf("PutLocal error: %v", err)
	}

	if feed.Len() != 1 {
		t.Fatalf("expected feed length 1 after local write, got %d", feed.Len())
	}

	cs, err := feed.Get(0)
	if err != nil {
		t.Fatalf("feed.Get(0): %v", err)
	}
	if cs.Key != testKey {
		t.Fatalf("expected key %s, got %s", testKey, cs.Key)
	}
}
