package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"crm-sqlite-pear-p2p/internal/store"
)

// ReplicationEngine integrates SQLite storage, Hypercore changeset persistence,
// Autobase consensus, and P2P network replication with LWW conflict resolution.
type ReplicationEngine struct {
	mu         sync.RWMutex
	repo       *store.Repository
	feed       *ChangesetFeed
	replicator *Replicator
	autobase   *AutobaseManager
	timestamps map[string]int64 // Track latest known timestamp per key for LWW resolution
}

// NewReplicationEngine creates and initializes a ReplicationEngine.
func NewReplicationEngine(repo *store.Repository, feed *ChangesetFeed, replicator *Replicator) *ReplicationEngine {
	var ab *AutobaseManager
	if feed != nil {
		ab = NewAutobaseManager(feed)
	}

	return &ReplicationEngine{
		repo:       repo,
		feed:       feed,
		replicator: replicator,
		autobase:   ab,
		timestamps: make(map[string]int64),
	}
}

// PutLocal performs a local write to SQLite and appends the resulting changeset to Hypercore and broadcasts it.
func (e *ReplicationEngine) PutLocal(ctx context.Context, key string, metadata json.RawMessage, data []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Check insert vs update
	_, err := e.repo.Get(ctx, key)
	op := store.OpUpdate
	if err == store.ErrNotFound {
		op = store.OpInsert
	}

	if err := e.repo.Put(ctx, key, metadata, data); err != nil {
		return fmt.Errorf("local put failed: %w", err)
	}

	now := time.Now().UnixNano()
	e.timestamps[key] = now

	cs := &store.Changeset{
		Timestamp: now,
		Sequence:  uint64(now),
		Operation: op,
		Key:       key,
		Metadata:  metadata,
		Data:      data,
	}

	if e.autobase != nil {
		_, _ = e.autobase.Append(cs)
	} else if e.feed != nil {
		_, _ = e.feed.Append(cs)
	}
	if e.replicator != nil {
		_ = e.replicator.BroadcastChangeset(cs)
	}

	return nil
}

// DeleteLocal removes a record locally and broadcasts an OpDelete changeset.
func (e *ReplicationEngine) DeleteLocal(ctx context.Context, key string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.repo.Delete(ctx, key); err != nil {
		return fmt.Errorf("local delete failed: %w", err)
	}

	now := time.Now().UnixNano()
	e.timestamps[key] = now

	cs := &store.Changeset{
		Timestamp: now,
		Sequence:  uint64(now),
		Operation: store.OpDelete,
		Key:       key,
	}

	if e.autobase != nil {
		_, _ = e.autobase.Append(cs)
	} else if e.feed != nil {
		_, _ = e.feed.Append(cs)
	}
	if e.replicator != nil {
		_ = e.replicator.BroadcastChangeset(cs)
	}

	return nil
}

// ApplyRemoteChangeset applies a remote changeset using Last-Writer-Wins (LWW) conflict resolution.
func (e *ReplicationEngine) ApplyRemoteChangeset(ctx context.Context, cs *store.Changeset) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Check if local timestamp is newer (LWW conflict resolution)
	if existingTS, ok := e.timestamps[cs.Key]; ok {
		if existingTS >= cs.Timestamp {
			// Local state is newer or equal; skip applying obsolete changeset
			return nil
		}
	}

	// Apply mutation to SQLite repository
	switch cs.Operation {
	case store.OpInsert, store.OpUpdate:
		if err := e.repo.Put(ctx, cs.Key, cs.Metadata, cs.Data); err != nil {
			return fmt.Errorf("apply remote changeset failed: %w", err)
		}
	case store.OpDelete:
		if err := e.repo.Delete(ctx, cs.Key); err != nil && err != store.ErrNotFound {
			return fmt.Errorf("apply remote delete failed: %w", err)
		}
	default:
		return fmt.Errorf("unknown changeset operation: %v", cs.Operation)
	}

	// Update latest known timestamp
	e.timestamps[cs.Key] = cs.Timestamp

	// Append to local feed history
	if e.feed != nil {
		_, _ = e.feed.Append(cs)
	}

	return nil
}

// Feed returns the underlying ChangesetFeed.
func (e *ReplicationEngine) Feed() *ChangesetFeed {
	return e.feed
}

// Autobase returns the AutobaseManager instance.
func (e *ReplicationEngine) Autobase() *AutobaseManager {
	return e.autobase
}
