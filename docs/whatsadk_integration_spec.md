# WhatsaDK Storage Backend Integration Specification (`sqlite-pear-p2p`)

## 1. Overview
This specification provides complete, step-by-step instructions for a coding agent to integrate `crm-sqlite-pear-p2p` as a first-class storage backend in **WhatsaDK** (`/home/innomon/B204-zone/orez/adk/whatsadk`).

By embedding `crm-sqlite-pear-p2p/pkg/whatsadk`, WhatsaDK gains a serverless, local-first, peer-to-peer replicated storage engine powered by pure Go SQLite (`modernc.org/sqlite` with zero CGO) and Hypercore/Autobase decentralized consensus.

---

## 2. Architecture & Design

### 2.1 Backend Contract
The package `crm-sqlite-pear-p2p/pkg/whatsadk` implements the complete WhatsaDK `storeBackend` interface:
```go
type storeBackend interface {
	Close() error
	EnqueueCommand(ctx context.Context, cmd string, payload interface{}) (int64, error)
	UpdateCommandStatus(ctx context.Context, id int64, status string, result interface{}) error
	PollPendingCommands(ctx context.Context) ([]Command, error)
	WaitForCommand(ctx context.Context, id int64, timeout time.Duration) (*Command, error)
	PutFile(ctx context.Context, path string, metadata interface{}, content []byte, timestamp time.Time) error
	IsBlacklisted(ctx context.Context, phone string) (bool, error)
	AddBlacklist(ctx context.Context, phone, reason string) error
	RemoveBlacklist(ctx context.Context, phone string) error
	ListBlacklist(ctx context.Context) ([]BlacklistedNumber, error)
	ListContacts(ctx context.Context, query string) ([]Contact, error)
	GetFilesysLogs(ctx context.Context, phone string, limit int) ([]FileEntry, error)
	GetLatestGlobalMessages(ctx context.Context, limit int) ([]FileEntry, error)
	QueryFilesys(ctx context.Context, query string, args ...interface{}) ([]map[string]interface{}, error)
	GetFile(ctx context.Context, path string) (*FileEntry, error)
	DeleteFile(ctx context.Context, path string) error
	ListFiles(ctx context.Context, prefix string, limit int) ([]FileEntry, error)
	GetAllContacts(ctx context.Context) ([]Contact, error)
	PutContact(ctx context.Context, contact Contact) error
	GetAllCommands(ctx context.Context) ([]Command, error)
	PutCommand(ctx context.Context, cmd Command) error
	GetAllFiles(ctx context.Context) ([]FileEntry, error)
	ResetSequence(ctx context.Context) error
}
```

### 2.2 Storage & View Mapping
All entities are persisted within a unified SQLite table `crm_store (key TEXT PRIMARY KEY, metadata TEXT, data BLOB) WITHOUT ROWID`:
- **Filesys**: `whatsadk:filesys:<path>`
- **Contacts**: `whatsadk:contact:<our_jid>:<their_jid>`
- **Commands**: `whatsadk:command:<id>`
- **Blacklist**: `whatsadk:blacklist:<phone>`

Standard SQLite views (`filesys`, `whatsmeow_contacts`, `whatsmeow_commands`, `blacklisted_numbers`) project this unified store into SQL tables, ensuring raw SQL queries executed via `QueryFilesys` or admin tools run natively with parameter normalization (`$1, $2` converted to `?`).

### 2.3 Media Handling (Images, Video, Voice)
- **Binary Media Payloads**: Images (JPEG, PNG, WebP), audio/voice notes (OGG/Opus, WAV), and video (MP4) are persisted directly as raw binary BLOBs in `crm_store.data`.
- **Zero-Bloat Querying**: In `GetFilesysLogs` and `GetLatestGlobalMessages`, conditional projection (`CASE WHEN (metadata->>'mime_type' = 'text/plain') THEN content ELSE NULL END`) returns metadata while omitting heavy binary content during message browsing. Full binary content is retrieved on demand via `GetFile(ctx, path)`.
- **P2P Streaming & Crypto-Shredding**: Media BLOBs are replicated over peer swarms using Autobase append-only feeds. When per-contact payload encryption is enabled, deleting the customer key cryptographically shreds all media across all swarm peers.

---

## 3. Step-by-Step Integration in WhatsaDK

### Step 1: Update `go.mod` in WhatsaDK
In the WhatsaDK repository root (`/home/innomon/B204-zone/orez/adk/whatsadk/go.mod`), add the module dependency and local replace directive:
```bash
go mod edit -require=crm-sqlite-pear-p2p@v0.0.0
go mod edit -replace=crm-sqlite-pear-p2p=/home/innomon/B204-zone/owly-sewa/crm-sqlite-pear-p2p
go mod tidy
```

### Step 2: Add SQLite P2P DSN Detection in `internal/store/store.go`
In `internal/store/store.go`, add DSN inspection logic to distinguish P2P SQLite from SurrealDB and PostgreSQL:

```go
func IsSQLiteP2P(dsn string) bool {
	return strings.HasPrefix(dsn, "sqlite-p2p://") ||
		strings.HasPrefix(dsn, "pear://") ||
		strings.HasPrefix(dsn, "sqlite://") ||
		strings.HasSuffix(dsn, ".db") ||
		strings.HasSuffix(dsn, ".sqlite") ||
		dsn == ":memory:"
}
```

### Step 3: Implement Backend Instantiation in `internal/store/store.go`
Import `crm-sqlite-pear-p2p/pkg/whatsadk`:
```go
import (
	// ... existing imports ...
	peerstore "crm-sqlite-pear-p2p/pkg/whatsadk"
)
```

Add the `openSQLiteP2P` factory function:
```go
func openSQLiteP2P(dsn string) (storeBackend, error) {
	// Parse DSN path and query parameters
	cleanPath := dsn
	enableWAL := true

	if strings.HasPrefix(cleanPath, "sqlite-p2p://") {
		cleanPath = strings.TrimPrefix(cleanPath, "sqlite-p2p://")
	} else if strings.HasPrefix(cleanPath, "pear://") {
		cleanPath = strings.TrimPrefix(cleanPath, "pear://")
	} else if strings.HasPrefix(cleanPath, "sqlite://") {
		cleanPath = strings.TrimPrefix(cleanPath, "sqlite://")
	}

	// Strip URL query parameters if present (e.g. ?wal=false)
	if idx := strings.Index(cleanPath, "?"); idx != -1 {
		query := cleanPath[idx+1:]
		cleanPath = cleanPath[:idx]
		if strings.Contains(query, "wal=false") {
			enableWAL = false
		}
	}

	opts := peerstore.Options{
		DBPath:    cleanPath,
		EnableWAL: enableWAL,
	}

	backend, err := peerstore.NewBackend(opts)
	if err != nil {
		return nil, fmt.Errorf("open sqlite-pear-p2p backend: %w", err)
	}

	return backend, nil
}
```

### Step 4: Dispatch in `Open(dsn string)`
Update `Open(dsn string)` in `internal/store/store.go`:
```go
func Open(dsn string) (*Store, error) {
	if IsSQLiteP2P(dsn) {
		backend, err := openSQLiteP2P(dsn)
		if err != nil {
			return nil, err
		}
		return &Store{backend: backend}, nil
	}

	if IsSurrealDB(dsn) {
		backend, err := openSurrealDB(dsn)
		if err != nil {
			return nil, err
		}
		return &Store{backend: backend}, nil
	}

	// Default fallback: PostgreSQL
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open store db: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping store db: %w", err)
	}

	s := &sqlStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate store db: %w", err)
	}

	return &Store{backend: s}, nil
}
```

### Step 5: Update `DatabaseType()`
Update `DatabaseType()` in `internal/store/store.go`:
```go
func (s *Store) DatabaseType() string {
	switch s.backend.(type) {
	case *sqlStore:
		return "postgres"
	case *surrealStore:
		return "surrealdb"
	case *peerstore.Backend:
		return "sqlite-pear-p2p"
	default:
		return "unknown"
	}
}
```

---

## 4. Verification & Testing

### 4.1 Running Existing WhatsaDK Store Tests
Run the WhatsaDK store unit tests targeting SQLite P2P:
```bash
cd /home/innomon/B204-zone/orez/adk/whatsadk
WHATSADK_STORE_DSN="sqlite-p2p://:memory:" go test -v ./internal/store/...
```

### 4.2 Multi-Node Peer Synchronization Test
To test replication between two peer WhatsaDK instances in tests:
1. Initialize Store 1 with `sqlite-p2p:///tmp/node1.db`.
2. Initialize Store 2 with `sqlite-p2p:///tmp/node2.db`.
3. Intercept changesets from Store 1 and apply to Store 2:
```go
backend1 := s1.backend.(*peerstore.Backend)
backend2 := s2.backend.(*peerstore.Backend)

backend1.Tracker().Subscribe(func(cs *store.Changeset, raw []byte) {
    _ = store.ApplyChangeset(context.Background(), backend2.Repo(), cs)
})
```
4. Verify files, contacts, blacklist entries, and commands written to Store 1 appear on Store 2.
