# sqlite-p2p

A decentralized, local-first, multi-master SQLite replication engine in Pure Go (`CGO_ENABLED=0`) powered by the Pear/Holepunch P2P protocol stack (Hypercore, Hyperswarm, Autobase).

`sqlite-p2p` enables distributed applications to read and write against an embedded SQLite database with local-first speed, automatic binary changeset capture, Last-Write-Wins (LWW) conflict reconciliation, per-record AES-256-GCM cryptographic shredding, and decentralized peer-to-peer synchronization over 32-byte swarm topics.

---

## Key Features

- **Pure Go & Zero-CGO**: Built with `modernc.org/sqlite` and `CGO_ENABLED=0`. Cross-compiles instantly for Linux, macOS, and Windows without dynamic C library dependencies.
- **Universal Multi-Table Auto-Discovery & Trigger-Based CDC**:
  - Automatically discovers all user tables in SQLite (`sqlite_master` + `PRAGMA table_info`), excluding internal tables (`sqlite_%`, `_p2p_%`, `crm_keys`).
  - Supports both fast-path Key-Value tables (`crm_store`, `ecom_store` with `(key TEXT PRIMARY KEY, metadata TEXT, data BLOB) WITHOUT ROWID`) and arbitrary relational tables (single-column PK, composite PK, or implicit `rowid`, including `BLOB` columns).
  - Automatically installs `AFTER INSERT`, `AFTER UPDATE`, and `AFTER DELETE` SQLite triggers into `_p2p_cdc_outbox`, guarded by `_p2p_sync_state.applying_remote = 0` so remote changesets never cause infinite replication loops.
  - Emits `OpDDL` (`CREATE TABLE IF NOT EXISTS ...`) changesets to auto-create discovered and newly created tables across peers.
- **Table-Scoped LWW Conflict Resolution**: Tracks Last-Write-Wins (LWW) timestamps per `(table, key)` pair so identical primary keys across different tables (`crm_store`, `ecom_store`, `orders`) never collide.
- **Pear/Holepunch P2P Mesh**: Append-only Hypercore feeds, peer discovery over Hyperswarm DHT topics, LAN UDP broadcast auto-discovery, and Autobase multi-writer consensus.
- **Replication Gating & Access Control**: Dynamic three-tier peer authorization (`all`, `whitelist`, `blacklist`) enforced during the Hyperswarm SecretStream handshake with real-time peer eviction.
- **Privacy & Crypto-Shredding**: Integrated AES-256-GCM symmetric cipher with per-key registry. Deleting a key executes instant cryptographic shredding while retaining replication causality.
- **Interactive & One-Shot CLI (`cmd/e2e-replication`)**: Handcrafted command registry supporting `put`, `get`, `del`, `list`, `sql` / `/sql` (with `json`, `table`, and `csv` output formats), and `gate` / `/gate`.
- **Clean Public Embedding API (`pkg/p2p`)**: Designed to be embedded into any Go application (e.g. `crm-lite`, `kheti-badi-ecom`, `whatsadk`).

---

## Embedding `sqlite-p2p`

Add `sqlite-p2p` to your Go module:

```bash
go get sqlite-p2p
```

### Basic Engine Usage

```go
package main

import (
 "context"
 "fmt"
 "log"

 "sqlite-p2p/pkg/p2p"
)

func main() {
 ctx := context.Background()

 // 1. Open the P2P Engine
 engine, err := p2p.OpenEngine(p2p.EngineOptions{
  DBPath:       "mydata.db",
  EnableWAL:    true,
  EnableCrypto: true,
 })
 if err != nil {
  log.Fatalf("failed to open engine: %v", err)
 }
 defer engine.Close()

 // 2. Put record
 metadata := []byte(`{"type":"document","author":"alice"}`)
 err = engine.Put(ctx, "doc:001", metadata, []byte("Hello, decentralized world!"))
 if err != nil {
  log.Fatalf("failed to put record: %v", err)
 }

 // 3. Get record (automatically decrypted)
 rec, err := engine.Get(ctx, "doc:001")
 if err != nil {
  log.Fatalf("failed to get record: %v", err)
 }
 fmt.Printf("Key: %s, Data: %s\n", rec.Key, string(rec.Data))

 // 4. Access underlying database if needed
 db := engine.DB()
 _ = db.Ping()
}
```

### P2P Swarm Synchronization & Multi-Table CDC

```go
// Derive 32-byte swarm topic
topic := [32]byte{ /* 32-byte swarm cluster key */ }

engine, err := p2p.OpenEngine(p2p.EngineOptions{
    DBPath:        "kheti_badi.db",
    DefaultTable:  "ecom_store",          // Optional: default KV table (defaults to "crm_store")
    IncludeTables: []string{},            // Optional: empty = replicate all discovered user tables
    ExcludeTables: []string{"temp_logs"}, // Optional: exclude specific user tables from CDC
    CDCInterval:   150 * time.Millisecond,
    EnableWAL:     true,
    SwarmTopic:    topic,
})
if err != nil {
    log.Fatal(err)
}
defer engine.Close()

// Any direct SQL DDL or DML executed against engine.DB() is automatically captured
// by SQLite CDC triggers, appended to Hypercore, and replicated across the Swarm:
_, _ = engine.DB().Exec(`
    CREATE TABLE IF NOT EXISTS orders (id TEXT PRIMARY KEY, status TEXT, total REAL);
    INSERT INTO orders (id, status, total) VALUES ('ORD-101', 'PAID', 3200.0);
`)
```

---

## Project Structure

```
sqlite-p2p/
├── cmd/
│   ├── dht-seed/          # Standalone HyperDHT bootstrap & LAN beacon seed server
│   └── e2e-replication/   # Interactive REPL & one-shot CLI (put, get, sql, gate, sync)
├── pkg/
│   ├── config/            # NodeConfig JSON/YAML loader and replication policy builder
│   └── p2p/               # Public embedding API (OpenEngine, OpenDB, Repository, Tracker)
├── internal/
│   ├── crypto/            # AES-256-GCM cipher and per-key registry
│   ├── logger/            # Structured slog logger with rotation
│   ├── p2p/               # Hypercore feeds, Autobase reconciliation, Hyperswarm coordinator
│   ├── sqlexec/           # Transactional SQL executor & JSON/Table/CSV formatter
│   └── store/             # Pure Go SQLite, schema auto-discovery (tables.go), trigger CDC (cdc.go)
├── docs/                  # Architectural documentation and E2E replication test guides
└── tests/                 # End-to-end multi-node synchronization, multi-table CDC, and crypto tests
```

---

## Testing & Quality Gates

All packages enforce >80% test coverage with 100% passing tests:

```bash
# Run all tests with coverage
go test -v -cover ./...
```

---

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

