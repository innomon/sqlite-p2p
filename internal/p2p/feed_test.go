package p2p_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sqlite-p2p/internal/p2p"
	"sqlite-p2p/internal/store"
)

func TestChangesetFeedAppendAndGet(t *testing.T) {
	tempDir := t.TempDir()
	feedDir := filepath.Join(tempDir, "feed")

	feed, err := p2p.NewChangesetFeed(feedDir)
	if err != nil {
		t.Fatalf("failed to create changeset feed: %v", err)
	}

	cs := &store.Changeset{
		Timestamp: time.Now().UnixNano(),
		Sequence:  1,
		Operation: store.OpInsert,
		Key:       "in.qzip.crm.customer:TESTKEY",
		Metadata:  json.RawMessage(`{"tier":"gold"}`),
		Data:      []byte("customer encrypted payload"),
	}

	seq, err := feed.Append(cs)
	if err != nil {
		t.Fatalf("failed to append changeset to feed: %v", err)
	}
	if seq != 0 {
		t.Fatalf("expected first sequence index 0, got %d", seq)
	}

	if feed.Len() != 1 {
		t.Fatalf("expected feed length 1, got %d", feed.Len())
	}

	// Verify Key and DiscoveryKey accessors
	pubKey := feed.Key()
	discKey := feed.DiscoveryKey()
	if pubKey == [32]byte{} || discKey == [32]byte{} {
		t.Fatalf("keys should not be zero values")
	}

	retrieved, err := feed.Get(0)
	if err != nil {
		t.Fatalf("failed to get changeset at index 0: %v", err)
	}

	if retrieved.Key != cs.Key {
		t.Fatalf("expected key %s, got %s", cs.Key, retrieved.Key)
	}
	if retrieved.Operation != cs.Operation {
		t.Fatalf("expected operation %v, got %v", cs.Operation, retrieved.Operation)
	}
	if string(retrieved.Data) != string(cs.Data) {
		t.Fatalf("expected data %s, got %s", string(cs.Data), string(retrieved.Data))
	}

	// Test Get out of range
	_, err = feed.Get(999)
	if err == nil {
		t.Fatalf("expected error on out of bounds Get")
	}

	// Reopen feed from disk to verify persistence
	feed2, err := p2p.NewChangesetFeed(feedDir)
	if err != nil {
		t.Fatalf("failed to reopen feed: %v", err)
	}
	if feed2.Len() != 1 {
		t.Fatalf("expected reopened feed to have length 1, got %d", feed2.Len())
	}
	retrieved2, err := feed2.Get(0)
	if err != nil {
		t.Fatalf("failed to get changeset from reopened feed: %v", err)
	}
	if retrieved2.Key != cs.Key {
		t.Fatalf("expected key %s from reopened feed, got %s", cs.Key, retrieved2.Key)
	}
}

func TestChangesetFeedStreamReplay(t *testing.T) {
	tempDir := t.TempDir()
	feed, err := p2p.NewChangesetFeed(filepath.Join(tempDir, "replay_feed"))
	if err != nil {
		t.Fatalf("failed to create feed: %v", err)
	}

	count := 5
	for i := 0; i < count; i++ {
		_, err := feed.Append(&store.Changeset{
			Timestamp: time.Now().UnixNano(),
			Sequence:  uint64(i + 1),
			Operation: store.OpInsert,
			Key:       "in.qzip.crm.customer:KEY" + string(rune('A'+i)),
		})
		if err != nil {
			t.Fatalf("append %d failed: %v", i, err)
		}
	}

	var replayed []*store.Changeset
	err = feed.Replay(0, func(cs *store.Changeset) error {
		replayed = append(replayed, cs)
		return nil
	})
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}

	if len(replayed) != count {
		t.Fatalf("expected %d replayed items, got %d", count, len(replayed))
	}

	// Replay with visitor error
	err = feed.Replay(0, func(cs *store.Changeset) error {
		return os.ErrInvalid
	})
	if err != os.ErrInvalid {
		t.Fatalf("expected visitor error propagation, got %v", err)
	}
}

func TestChangesetFeedInvalidDir(t *testing.T) {
	// Trying to create a feed under a file path that cannot be a directory
	tmpFile, err := os.CreateTemp("", "bad_dir_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	_, err = p2p.NewChangesetFeed(filepath.Join(tmpFile.Name(), "feed"))
	if err == nil {
		t.Fatalf("expected error creating feed in invalid path")
	}
}

func TestChangesetFeedConcurrentReplayAndAppend(t *testing.T) {
	tempDir := t.TempDir()
	feed, err := p2p.NewChangesetFeed(filepath.Join(tempDir, "concurrent_feed"))
	if err != nil {
		t.Fatalf("NewChangesetFeed error: %v", err)
	}

	// Seed initial items
	for i := 0; i < 5; i++ {
		_, err := feed.Append(&store.Changeset{
			Timestamp: int64(1000 + i),
			Sequence:  uint64(i + 1),
			Operation: store.OpInsert,
			Key:       "in.qzip.crm.customer:INIT",
		})
		if err != nil {
			t.Fatalf("append %d error: %v", i, err)
		}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			_ = feed.Replay(0, func(cs *store.Changeset) error {
				_ = cs.Key
				return nil
			})
		}
	}()

	for i := 0; i < 20; i++ {
		_, _ = feed.Append(&store.Changeset{
			Timestamp: int64(2000 + i),
			Sequence:  uint64(10 + i),
			Operation: store.OpUpdate,
			Key:       "in.qzip.crm.customer:CONCURRENT",
		})
	}

	<-done
}

