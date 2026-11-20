package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"sqlite-p2p/internal/store"
)

// ReplicationEngine integrates SQLite storage, Hypercore changeset persistence,
// Autobase consensus, and P2P network replication with LWW conflict resolution.
type ReplicationEngine struct {
	mu            sync.RWMutex
	repo          *store.Repository
	feed          *ChangesetFeed
	replicator    *Replicator
	autobase      *AutobaseManager
	timestamps    map[string]int64 // Track latest known timestamp per (table, key) for LWW resolution
	includeTables []string
	excludeTables []string
	knownTables   map[string]bool
	pollerCancel  context.CancelFunc
	logger        *slog.Logger
}

func tableKey(table, key string) string {
	if table == "" {
		table = store.DefaultTableName
	}
	return table + "\x00" + key
}

// SetLogger attaches a structured logger to the ReplicationEngine.
func (e *ReplicationEngine) SetLogger(l *slog.Logger) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.logger = l
}

// SetTableFilters configures optional table inclusion and exclusion lists for multi-table discovery.
func (e *ReplicationEngine) SetTableFilters(include, exclude []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.includeTables = append([]string(nil), include...)
	e.excludeTables = append([]string(nil), exclude...)
}

// NewReplicationEngine creates and initializes a ReplicationEngine.
func NewReplicationEngine(repo *store.Repository, feed *ChangesetFeed, replicator *Replicator) *ReplicationEngine {
	var ab *AutobaseManager
	if feed != nil {
		ab = NewAutobaseManager(feed)
	}

	eng := &ReplicationEngine{
		repo:        repo,
		feed:        feed,
		replicator:  replicator,
		autobase:    ab,
		timestamps:  make(map[string]int64),
		knownTables: make(map[string]bool),
	}

	if repo != nil && repo.DB() != nil {
		if tables, err := store.EnsureAllTriggers(context.Background(), repo.DB(), nil, nil); err == nil {
			for _, tbl := range tables {
				eng.knownTables[tbl.Name] = true
			}
		}
	}

	return eng
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
	if e.repo.DB() != nil {
		_, _ = store.ReadAndClearOutbox(ctx, e.repo.DB(), 100)
	}

	// Capture the stored raw data (ciphertext if repository is encryption-aware)
	storedData := data
	if rawRec, err := e.repo.GetRaw(ctx, key); err == nil {
		storedData = rawRec.Data
	}

	tbl := e.repo.TableName()
	now := time.Now().UnixNano()
	e.timestamps[tableKey(tbl, key)] = now

	cs := &store.Changeset{
		Timestamp: now,
		Sequence:  uint64(now),
		Operation: op,
		Table:     tbl,
		Key:       key,
		Metadata:  metadata,
		Data:      storedData,
	}

	if e.autobase != nil {
		_, _ = e.autobase.Append(cs)
	} else if e.feed != nil {
		_, _ = e.feed.Append(cs)
	}
	if e.replicator != nil {
		_ = e.replicator.BroadcastChangeset(cs)
	}

	if e.logger != nil {
		e.logger.Info("local record stored and changeset appended",
			"event", "local_mutation_applied",
			"table", tbl,
			"key", key,
			"operation", op,
			"timestamp", now,
		)
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
	if e.repo.DB() != nil {
		_, _ = store.ReadAndClearOutbox(ctx, e.repo.DB(), 100)
	}

	tbl := e.repo.TableName()
	now := time.Now().UnixNano()
	e.timestamps[tableKey(tbl, key)] = now

	cs := &store.Changeset{
		Timestamp: now,
		Sequence:  uint64(now),
		Operation: store.OpDelete,
		Table:     tbl,
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

	if e.logger != nil {
		e.logger.Info("local record deleted and changeset appended",
			"event", "local_delete_applied",
			"table", tbl,
			"key", key,
			"timestamp", now,
		)
	}

	return nil
}

// ApplyRemoteChangeset applies a remote changeset using table-scoped Last-Writer-Wins (LWW) conflict resolution.
func (e *ReplicationEngine) ApplyRemoteChangeset(ctx context.Context, cs *store.Changeset) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	tbl := cs.EffectiveTable()
	tk := tableKey(tbl, cs.Key)

	// Check if local timestamp is newer (LWW conflict resolution) for non-DDL mutations
	if cs.Operation != store.OpDDL {
		if existingTS, ok := e.timestamps[tk]; ok {
			if existingTS >= cs.Timestamp {
				// Local state is newer or equal; skip applying obsolete changeset
				if e.logger != nil {
					e.logger.Info("obsolete changeset dropped by LWW conflict resolution",
						"event", "lww_changeset_skipped",
						"table", tbl,
						"key", cs.Key,
						"local_timestamp", existingTS,
						"remote_timestamp", cs.Timestamp,
					)
				}
				return nil
			}
		}
	}

	if e.repo != nil && e.repo.DB() != nil {
		if err := store.ApplyChangesetToTable(ctx, e.repo.DB(), cs); err != nil {
			return fmt.Errorf("apply remote changeset failed: %w", err)
		}
		if cs.Operation == store.OpDDL && tbl != "" {
			e.knownTables[tbl] = true
		}
		if cs.Operation == store.OpDelete && tbl == e.repo.TableName() && e.repo.KeyRegistry() != nil {
			_ = e.repo.KeyRegistry().PurgeKey(cs.Key)
		}
	} else if e.repo != nil {
		switch cs.Operation {
		case store.OpInsert, store.OpUpdate:
			if err := e.repo.PutRaw(ctx, cs.Key, cs.Metadata, cs.Data); err != nil {
				return fmt.Errorf("apply remote changeset failed: %w", err)
			}
		case store.OpDelete:
			if err := e.repo.Delete(ctx, cs.Key); err != nil && err != store.ErrNotFound {
				return fmt.Errorf("apply remote delete failed: %w", err)
			}
		default:
			return fmt.Errorf("unknown changeset operation: %v", cs.Operation)
		}
	}

	// Update latest known timestamp
	e.timestamps[tk] = cs.Timestamp

	// Append to local feed history
	if e.feed != nil {
		_, _ = e.feed.Append(cs)
	}

	if e.logger != nil {
		e.logger.Info("remote changeset applied to local state",
			"event", "remote_changeset_applied",
			"table", tbl,
			"key", cs.Key,
			"operation", cs.Operation,
			"timestamp", cs.Timestamp,
		)
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

// SyncPeers replays the entire local feed from sequence 0 and broadcasts each changeset
// to all active peer connections. Returns the count of changesets broadcasted.
func (e *ReplicationEngine) SyncPeers(ctx context.Context) (int, error) {
	if e.feed == nil || e.replicator == nil {
		return 0, nil
	}

	count := 0
	err := e.feed.Replay(0, func(cs *store.Changeset) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := e.replicator.BroadcastChangeset(cs); err != nil {
			return err
		}
		count++
		return nil
	})

	if err != nil {
		if e.logger != nil {
			e.logger.Error("peer sync failed during feed replay broadcast",
				"event", "sync_peers_failed",
				"error", err,
				"broadcast_count", count,
			)
		}
		return count, err
	}

	if e.logger != nil {
		e.logger.Info("peer sync sweep completed",
			"event", "sync_peers_completed",
			"broadcast_count", count,
			"peer_count", e.replicator.PeerCount(),
		)
	}

	return count, nil
}

// SeedFromRepository discovers all user tables in SQLite, installs CDC triggers on them,
// imports existing records into LWW timestamps, and seeds the ChangesetFeed if empty.
func (e *ReplicationEngine) SeedFromRepository(ctx context.Context) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.repo == nil || e.repo.DB() == nil {
		return 0, nil
	}
	db := e.repo.DB()

	tables, err := store.EnsureAllTriggers(ctx, db, e.includeTables, e.excludeTables)
	if err != nil {
		return 0, fmt.Errorf("seed from repository discover tables: %w", err)
	}

	var allSeeded []*store.Changeset
	for _, tbl := range tables {
		e.knownTables[tbl.Name] = true
		csList, err := store.SeedTableChangesets(ctx, db, tbl)
		if err != nil {
			return 0, fmt.Errorf("seed table %s: %w", tbl.Name, err)
		}
		for _, cs := range csList {
			if cs.Operation != store.OpDDL {
				tk := tableKey(cs.EffectiveTable(), cs.Key)
				if existing, ok := e.timestamps[tk]; !ok || cs.Timestamp > existing {
					e.timestamps[tk] = cs.Timestamp
				}
			}
			allSeeded = append(allSeeded, cs)
		}
	}

	if e.feed == nil || e.feed.Len() > 0 {
		return 0, nil
	}

	count := 0
	for _, cs := range allSeeded {
		if e.autobase != nil {
			_, _ = e.autobase.Append(cs)
		} else {
			_, _ = e.feed.Append(cs)
		}
		count++
	}

	if e.logger != nil && count > 0 {
		e.logger.Info("seeded feed from repository", "count", count, "tables", len(tables))
	}
	return count, nil
}

// DrainCDCOutbox inspects for newly created tables, installs triggers, drains pending mutations
// from _p2p_cdc_outbox, appends them to ChangesetFeed/Autobase, and broadcasts them to peers.
func (e *ReplicationEngine) DrainCDCOutbox(ctx context.Context) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.repo == nil || e.repo.DB() == nil {
		return 0, nil
	}
	db := e.repo.DB()

	totalEmitted := 0

	// 1. Check if any new user tables were created since last check
	tables, err := store.DiscoverTables(ctx, db, e.includeTables, e.excludeTables)
	if err == nil {
		for _, tbl := range tables {
			if !e.knownTables[tbl.Name] {
				e.knownTables[tbl.Name] = true
				_ = store.InstallTableTriggers(ctx, db, tbl)
				// Seed DDL + any initial rows created before trigger installation
				if seeded, sErr := store.SeedTableChangesets(ctx, db, tbl); sErr == nil {
					for _, cs := range seeded {
						e.recordAndBroadcastLocked(cs)
						totalEmitted++
					}
				}
			}
		}
	}

	// 2. Drain pending CDC outbox rows
	outbox, err := store.ReadAndClearOutbox(ctx, db, 1000)
	if err != nil {
		return totalEmitted, err
	}

	for _, cs := range outbox {
		e.recordAndBroadcastLocked(cs)
		totalEmitted++
	}

	return totalEmitted, nil
}

func (e *ReplicationEngine) recordAndBroadcastLocked(cs *store.Changeset) {
	if cs.Operation != store.OpDDL {
		tk := tableKey(cs.EffectiveTable(), cs.Key)
		e.timestamps[tk] = cs.Timestamp
	}
	if e.autobase != nil {
		_, _ = e.autobase.Append(cs)
	} else if e.feed != nil {
		_, _ = e.feed.Append(cs)
	}
	if e.replicator != nil {
		_ = e.replicator.BroadcastChangeset(cs)
	}
}

// StartCDCPoller starts a background goroutine that periodically drains the SQLite CDC outbox.
func (e *ReplicationEngine) StartCDCPoller(interval time.Duration) {
	e.mu.Lock()
	if e.pollerCancel != nil {
		e.pollerCancel()
	}
	if interval <= 0 {
		interval = 150 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.pollerCancel = cancel
	e.mu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = e.DrainCDCOutbox(ctx)
			}
		}
	}()
}

// StopCDCPoller stops the background CDC outbox poller if running.
func (e *ReplicationEngine) StopCDCPoller() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pollerCancel != nil {
		e.pollerCancel()
		e.pollerCancel = nil
	}
}


