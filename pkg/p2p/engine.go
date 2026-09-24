package p2p

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"sqlite-p2p/internal/crypto"
	internalp2p "sqlite-p2p/internal/p2p"
	"sqlite-p2p/internal/store"
)

// EngineOptions configures the decentralized sqlite-p2p engine.
type EngineOptions struct {
	DBPath       string
	DB           *sql.DB
	EnableWAL    bool
	EnableCrypto bool
	SwarmTopic   [32]byte
	KeyRegistry  *crypto.KeyRegistry
	Logger       *slog.Logger
}

// Engine encapsulates the pure Go SQLite storage, cryptographic security,
// and Pear/Holepunch P2P replication mesh.
type Engine struct {
	db          *sql.DB
	ownsDB      bool
	repo        *store.Repository
	tracker     *store.ChangesetTracker
	replEngine  *internalp2p.ReplicationEngine
	feed        *internalp2p.ChangesetFeed
	replicator  *internalp2p.Replicator
	keys        *crypto.KeyRegistry
	logger      *slog.Logger
	mu          sync.RWMutex
}

// OpenEngine initializes and returns an Engine instance.
func OpenEngine(opts EngineOptions) (*Engine, error) {
	var db *sql.DB
	var ownsDB bool
	var err error

	if opts.DB != nil {
		db = opts.DB
		ownsDB = false
	} else {
		if opts.DBPath == "" {
			return nil, errors.New("either DB or DBPath must be provided")
		}
		db, err = store.OpenDB(opts.DBPath, opts.EnableWAL)
		if err != nil {
			return nil, fmt.Errorf("failed to open database: %w", err)
		}
		ownsDB = true
	}

	keys := opts.KeyRegistry
	if keys == nil && opts.EnableCrypto {
		keys, err = crypto.NewKeyRegistry(db)
		if err != nil {
			if ownsDB {
				_ = db.Close()
			}
			return nil, fmt.Errorf("failed to initialize key registry: %w", err)
		}
	}

	repo := store.NewRepository(db)
	if keys != nil {
		repo.SetKeyRegistry(keys)
	}

	tracker := store.NewChangesetTracker(repo)

	engine := &Engine{
		db:      db,
		ownsDB:  ownsDB,
		repo:    repo,
		tracker: tracker,
		keys:    keys,
		logger:  opts.Logger,
	}

	// Initialize P2P replication if a cluster topic is specified
	if opts.SwarmTopic != [32]byte{} {
		feedDir := ""
		if opts.DBPath != ":memory:" && opts.DBPath != "" {
			feedDir = opts.DBPath + ".hypercore"
		}
		feed, err := internalp2p.NewChangesetFeed(feedDir)
		if err != nil {
			if ownsDB {
				_ = db.Close()
			}
			return nil, fmt.Errorf("failed to initialize changeset feed: %w", err)
		}

		replicator := internalp2p.NewReplicator(feed, nil)
		if opts.Logger != nil {
			replicator.SetLogger(opts.Logger)
		}

		replEngine := internalp2p.NewReplicationEngine(repo, feed, replicator)
		if opts.Logger != nil {
			replEngine.SetLogger(opts.Logger)
		}

		replicator.SetHandler(func(cs *store.Changeset) error {
			return replEngine.ApplyRemoteChangeset(context.Background(), cs)
		})

		engine.feed = feed
		engine.replicator = replicator
		engine.replEngine = replEngine
	}

	return engine, nil
}

// DB returns the underlying sql.DB instance.
func (e *Engine) DB() *sql.DB {
	return e.db
}

// Repository returns the underlying store.Repository instance.
func (e *Engine) Repository() *store.Repository {
	return e.repo
}

// ChangesetTracker returns the underlying store.ChangesetTracker instance.
func (e *Engine) ChangesetTracker() *store.ChangesetTracker {
	return e.tracker
}

// KeyRegistry returns the configured crypto.KeyRegistry instance, if any.
func (e *Engine) KeyRegistry() *crypto.KeyRegistry {
	return e.keys
}

// ReplicationEngine returns the internal p2p.ReplicationEngine instance, if initialized.
func (e *Engine) ReplicationEngine() *internalp2p.ReplicationEngine {
	return e.replEngine
}

// Replicator returns the internal p2p.Replicator instance, if initialized.
func (e *Engine) Replicator() *internalp2p.Replicator {
	return e.replicator
}

// ChangesetFeed returns the internal p2p.ChangesetFeed instance, if initialized.
func (e *Engine) ChangesetFeed() *internalp2p.ChangesetFeed {
	return e.feed
}

// Put writes or upserts a record into the repository and triggers changeset tracking.
func (e *Engine) Put(ctx context.Context, key string, metadata json.RawMessage, data []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.replEngine != nil {
		return e.replEngine.PutLocal(ctx, key, metadata, data)
	}
	return e.tracker.Put(ctx, key, metadata, data)
}

// Get retrieves a record by key, decrypting the payload if a KeyRegistry is configured.
func (e *Engine) Get(ctx context.Context, key string) (*store.Record, error) {
	return e.repo.Get(ctx, key)
}

// GetRaw retrieves a record by key without decrypting the payload.
func (e *Engine) GetRaw(ctx context.Context, key string) (*store.Record, error) {
	return e.repo.GetRaw(ctx, key)
}

// List retrieves records matching a key prefix with pagination.
func (e *Engine) List(ctx context.Context, prefix string, limit, offset int) ([]store.Record, error) {
	return e.repo.List(ctx, prefix, limit, offset)
}

// Delete removes a record by key and emits an OpDelete changeset.
func (e *Engine) Delete(ctx context.Context, key string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.replEngine != nil {
		return e.replEngine.DeleteLocal(ctx, key)
	}
	return e.tracker.Delete(ctx, key)
}

// Close closes the engine and releases underlying database and network resources.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.replicator != nil {
		e.replicator.Close()
	}

	if e.ownsDB && e.db != nil {
		return e.db.Close()
	}
	return nil
}
