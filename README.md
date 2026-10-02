# sqlite-p2p

A decentralized, local-first, multi-master SQLite replication engine in Pure Go (`CGO_ENABLED=0`) powered by the Pear/Holepunch P2P protocol stack (Hypercore, Hyperswarm, Autobase).

`sqlite-p2p` enables distributed applications to read and write against an embedded SQLite database with local-first speed, automatic binary changeset capture, Last-Write-Wins (LWW) conflict reconciliation, per-record AES-256-GCM cryptographic shredding, and decentralized peer-to-peer synchronization over 32-byte swarm topics.

---

## Key Features

- **Pure Go & Zero-CGO**: Built with `modernc.org/sqlite` and `CGO_ENABLED=0`. Cross-compiles instantly for Linux, macOS, and Windows without dynamic C library dependencies.
- **Generic Key-Value Storage**: Core `crm_store` table configured with WAL mode, foreign keys, and normal synchronization:

  ```sql
  CREATE TABLE IF NOT EXISTS crm_store (
      key TEXT PRIMARY KEY,
      metadata TEXT CHECK(json_valid(metadata)),
      data BLOB
  ) WITHOUT ROWID;
  ```

- **Automatic Changeset Capture**: Intercepts mutations and generates binary changesets suitable for streaming replication over append-only feeds.
- **Pear/Holepunch P2P Mesh**: Append-only Hypercore feeds, peer discovery over Hyperswarm DHT topics, and Autobase multi-writer consensus.
- **Privacy & Crypto-Shredding**: Integrated AES-256-GCM symmetric cipher with per-key registry. Deleting a key executes instant cryptographic shredding while retaining replication causality.
- **Clean Public Embedding API (`pkg/p2p`)**: Designed to be embedded into any Go application (e.g. `crm-lite`, `whatsadk`).

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

### P2P Swarm Synchronization

```go
// Derive 32-byte swarm topic
topic := [32]byte{ /* 32-byte swarm cluster key */ }

engine, err := p2p.OpenEngine(p2p.EngineOptions{
    DBPath:     "nodeA.db",
    SwarmTopic: topic,
})
```

---

## Project Structure

```
sqlite-p2p/
├── pkg/p2p/           # Public embedding API (OpenEngine, OpenDB, Repository, Tracker)
├── internal/
│   ├── crypto/        # AES-256-GCM cipher and per-key registry
│   ├── logger/        # Structured slog logger with rotation
│   ├── p2p/           # Hypercore feeds, Autobase reconciliation, Hyperswarm coordinator
│   └── store/         # Pure Go SQLite connection, WAL mode, changeset tracking
├── docs/              # Architectural documentation and decoupling specifications
└── tests/             # End-to-end multi-node synchronization and crypto-shredding tests
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

