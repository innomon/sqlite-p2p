package whatsadk

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"crm-sqlite-pear-p2p/internal/p2p"
	"crm-sqlite-pear-p2p/internal/store"
)

// DDL for SQLite views projecting unified crm_store into whatsadk compatible schemas.
const viewsDDL = `
CREATE VIEW IF NOT EXISTS filesys AS
SELECT
    substr(key, 18) AS path,
    CASE 
        WHEN json_extract(metadata, '$._is_null') = 1 THEN NULL
        ELSE json_remove(metadata, '$._tmstamp')
    END AS metadata,
    data AS content,
    json_extract(metadata, '$._tmstamp') AS tmstamp
FROM crm_store
WHERE key LIKE 'whatsadk:filesys:%';

CREATE VIEW IF NOT EXISTS whatsmeow_contacts AS
SELECT
    json_extract(metadata, '$.our_jid') AS our_jid,
    json_extract(metadata, '$.their_jid') AS their_jid,
    json_extract(metadata, '$.full_name') AS full_name,
    json_extract(metadata, '$.short_name') AS short_name,
    json_extract(metadata, '$.push_name') AS push_name,
    json_extract(metadata, '$.business_name') AS business_name
FROM crm_store
WHERE key LIKE 'whatsadk:contact:%';

CREATE VIEW IF NOT EXISTS whatsmeow_commands AS
SELECT
    CAST(json_extract(metadata, '$.id') AS INTEGER) AS id,
    json_extract(metadata, '$.command') AS command,
    json_extract(metadata, '$.payload') AS payload,
    json_extract(metadata, '$.status') AS status,
    json_extract(metadata, '$.result') AS result,
    json_extract(metadata, '$.created_at') AS created_at,
    json_extract(metadata, '$.updated_at') AS updated_at
FROM crm_store
WHERE key LIKE 'whatsadk:command:%';

CREATE VIEW IF NOT EXISTS blacklisted_numbers AS
SELECT
    substr(key, 20) AS phone,
    json_extract(metadata, '$.reason') AS reason,
    json_extract(metadata, '$.created_at') AS created_at
FROM crm_store
WHERE key LIKE 'whatsadk:blacklist:%';
`

// Options configures the whatsadk storage backend.
type Options struct {
	DBPath    string
	DB        *sql.DB
	EnableWAL bool
	Repo      *store.Repository
	Tracker   *store.ChangesetTracker
	Engine    *p2p.ReplicationEngine
}

// Backend implements StoreBackend using SQLite unified crm_store and Autobase replication.
type Backend struct {
	db      *sql.DB
	ownsDB  bool
	repo    *store.Repository
	tracker *store.ChangesetTracker
	engine  *p2p.ReplicationEngine
	mu      sync.RWMutex
	cmdSeq  atomic.Int64
}

// Compile-time check that Backend implements StoreBackend.
var _ StoreBackend = (*Backend)(nil)

// NewBackend creates and initializes a Backend instance.
func NewBackend(opts Options) (*Backend, error) {
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
			return nil, fmt.Errorf("failed to open sqlite database: %w", err)
		}
		ownsDB = true
	}

	// Initialize views for whatsadk SQL compatibility
	if _, err := db.Exec(viewsDDL); err != nil {
		if ownsDB {
			_ = db.Close()
		}
		return nil, fmt.Errorf("failed to create whatsadk views: %w", err)
	}

	repo := opts.Repo
	if repo == nil {
		repo = store.NewRepository(db)
	}

	tracker := opts.Tracker
	if tracker == nil {
		tracker = store.NewChangesetTracker(repo)
	}

	backend := &Backend{
		db:      db,
		ownsDB:  ownsDB,
		repo:    repo,
		tracker: tracker,
		engine:  opts.Engine,
	}

	// Initialize command sequence from existing commands in DB
	var maxID sql.NullInt64
	err = db.QueryRow("SELECT MAX(CAST(json_extract(metadata, '$.id') AS INTEGER)) FROM crm_store WHERE key LIKE 'whatsadk:command:%'").Scan(&maxID)
	if err == nil && maxID.Valid {
		backend.cmdSeq.Store(maxID.Int64)
	}

	return backend, nil
}

// DB returns the underlying sql.DB instance.
func (b *Backend) DB() *sql.DB {
	return b.db
}

// Repo returns the underlying store.Repository instance.
func (b *Backend) Repo() *store.Repository {
	return b.repo
}

// Tracker returns the underlying store.ChangesetTracker instance.
func (b *Backend) Tracker() *store.ChangesetTracker {
	return b.tracker
}

// Close closes the underlying SQLite database if owned by this Backend.
func (b *Backend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.ownsDB && b.db != nil {
		return b.db.Close()
	}
	return nil
}

// --- Placeholder methods for StoreBackend interface (to be implemented in following phases) ---

func (b *Backend) EnqueueCommand(ctx context.Context, cmd string, payload interface{}) (int64, error) {
	return 0, errors.New("not implemented")
}

func (b *Backend) UpdateCommandStatus(ctx context.Context, id int64, status string, result interface{}) error {
	return errors.New("not implemented")
}

func (b *Backend) PollPendingCommands(ctx context.Context) ([]Command, error) {
	return nil, errors.New("not implemented")
}

func (b *Backend) WaitForCommand(ctx context.Context, id int64, timeout time.Duration) (*Command, error) {
	return nil, errors.New("not implemented")
}


func (b *Backend) IsBlacklisted(ctx context.Context, phone string) (bool, error) {
	return false, errors.New("not implemented")
}

func (b *Backend) AddBlacklist(ctx context.Context, phone, reason string) error {
	return errors.New("not implemented")
}

func (b *Backend) RemoveBlacklist(ctx context.Context, phone string) error {
	return errors.New("not implemented")
}

func (b *Backend) ListBlacklist(ctx context.Context) ([]BlacklistedNumber, error) {
	return nil, errors.New("not implemented")
}

func (b *Backend) ListContacts(ctx context.Context, query string) ([]Contact, error) {
	return nil, errors.New("not implemented")
}

func (b *Backend) GetFilesysLogs(ctx context.Context, phone string, limit int) ([]FileEntry, error) {
	return nil, errors.New("not implemented")
}

func (b *Backend) GetLatestGlobalMessages(ctx context.Context, limit int) ([]FileEntry, error) {
	return nil, errors.New("not implemented")
}

func (b *Backend) QueryFilesys(ctx context.Context, query string, args ...interface{}) ([]map[string]interface{}, error) {
	return nil, errors.New("not implemented")
}

func (b *Backend) GetAllContacts(ctx context.Context) ([]Contact, error) {
	return nil, errors.New("not implemented")
}

func (b *Backend) PutContact(ctx context.Context, contact Contact) error {
	return errors.New("not implemented")
}

func (b *Backend) GetAllCommands(ctx context.Context) ([]Command, error) {
	return nil, errors.New("not implemented")
}

func (b *Backend) PutCommand(ctx context.Context, cmd Command) error {
	return errors.New("not implemented")
}

func (b *Backend) ResetSequence(ctx context.Context) error {
	return errors.New("not implemented")
}
