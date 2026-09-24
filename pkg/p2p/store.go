package p2p

import (
	"database/sql"

	"sqlite-p2p/internal/crypto"
	internalp2p "sqlite-p2p/internal/p2p"
	"sqlite-p2p/internal/store"
)

// ErrNotFound indicates a requested key does not exist.
var ErrNotFound = store.ErrNotFound

// Record aliases the underlying storage record.
type Record = store.Record

// Repository aliases the underlying generic KV repository.
type Repository = store.Repository

// ChangesetTracker aliases the mutation capture tracker.
type ChangesetTracker = store.ChangesetTracker

// KeyRegistry aliases the per-key AES-256-GCM registry.
type KeyRegistry = crypto.KeyRegistry

// ReplicationEngine aliases the P2P synchronization coordinator.
type ReplicationEngine = internalp2p.ReplicationEngine

// OpenDB opens a pure Go SQLite database with WAL and foreign key PRAGMAs.
func OpenDB(dbPath string, enableWAL bool) (*sql.DB, error) {
	return store.OpenDB(dbPath, enableWAL)
}

// NewRepository initializes a Repository on the provided sql.DB.
func NewRepository(db *sql.DB) *Repository {
	return store.NewRepository(db)
}

// NewChangesetTracker initializes a ChangesetTracker for the repository.
func NewChangesetTracker(repo *Repository) *ChangesetTracker {
	return store.NewChangesetTracker(repo)
}

// NewKeyRegistry initializes a cryptographic KeyRegistry on the provided sql.DB.
func NewKeyRegistry(db *sql.DB) (*KeyRegistry, error) {
	return crypto.NewKeyRegistry(db)
}
