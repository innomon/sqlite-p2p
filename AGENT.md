# Agent Integration Guide: sqlite-p2p

This guide provides instructions for AI coding agents and autonomous developer harnesses (Cursor, Claude, Goose, Antigravity) integrating or working with the **`sqlite-p2p`** decentralized storage engine.

**Rule**: always use project relative paths, avoid absolute path, if needed create an environment variable, example, `$ROOT=~/projects` and save to `.env`  

> [!NOTE]
> If you are looking for the **Multimodal CRM Agent & MCP Server**, that functionality has been factored into the standalone [`crm-lite`]($PROJECT_ROOT/crm-lite) repository. See `crm-lite/AGENT.md` for CRM MCP tools.

---


## 1. Library Overview

`sqlite-p2p` is a pure Go (`CGO_ENABLED=0`) decentralized SQLite database engine that provides:

- **Local SQLite Store**: Powered by `modernc.org/sqlite` with WAL mode and `busy_timeout` enabled.
- **Universal Multi-Table Auto-Discovery & Trigger CDC**: Automatically discovers all user tables (`crm_store`, `ecom_store`, and arbitrary relational schemas), attaches `AFTER INSERT/UPDATE/DELETE` triggers into `_p2p_cdc_outbox`, guards remote changeset application via `_p2p_sync_state.applying_remote = 1`, and replicates `OpDDL` + row mutations across peers.
- **Peer-to-Peer Replication**: Powered by pure Go Holepunch/Pear protocols (Hypercore append-only feeds, Autobase changeset linearization, table-scoped LWW reconciliation, and Hyperswarm DHT discovery).
- **Cryptographic Shredding**: Per-record AES-256-GCM encryption with key purging for mathematical right-to-be-forgotten on append-only feeds.
- **Clean Public Embedding API**: Exported via `sqlite-p2p/pkg/p2p`.

---

## 2. Embedding in Go Projects

To embed `sqlite-p2p` in your Go service or application:

### Step 1: Add Dependency

In your `go.mod`:

```go
require sqlite-p2p v0.0.0

replace sqlite-p2p => $PROJECT_ROOT/sqlite-p2p
```

### Step 2: Initialize Engine

```go
package main

import (
    "context"
    "crypto/sha256"
    "encoding/json"
    "log"
    "time"

    "sqlite-p2p/pkg/p2p"
)

func main() {
    ctx := context.Background()
    topic := sha256.Sum256([]byte("my.app.cluster.v1"))

    engine, err := p2p.OpenEngine(p2p.EngineOptions{
        DBPath:        "my_node.db",
        DefaultTable:  "crm_store", // Or "ecom_store" for e-commerce KV workloads
        IncludeTables: nil,         // Empty = auto-discover & replicate all user tables
        ExcludeTables: nil,
        CDCInterval:   150 * time.Millisecond,
        EnableWAL:     true,
        EnableCrypto:  false,
        SwarmTopic:    topic,
    })
    if err != nil {
        log.Fatalf("failed to init engine: %v", err)
    }
    defer engine.Close()

    // 1. KV API Write (writes to DefaultTable and broadcasts immediately)
    err = engine.Put(ctx, "user:123", json.RawMessage(`{"name":"Alice"}`), []byte("confidential"))
    if err != nil {
        log.Fatalf("failed to put: %v", err)
    }

    // 2. Direct SQL Write on any table (captured automatically by SQLite CDC triggers)
    _, err = engine.DB().ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS orders (id TEXT PRIMARY KEY, status TEXT, total REAL);
        INSERT INTO orders (id, status, total) VALUES ('ord:1', 'PAID', 99.50);
    `)
    if err != nil {
        log.Fatalf("failed to exec sql: %v", err)
    }
    // Optional: trigger immediate CDC outbox drain without waiting for background poller
    _, _ = engine.DrainCDCOutbox(ctx)
}
```

---

## 3. Architecture & Reference

- **[ARCHITECTURE.md](/ARCHITECTURE.md)**: Deep dive into the CQRS replication pipeline, encryption at rest, and DHT topic discovery.
- **[README.md](/README.md)**: Build instructions, testing guidelines, and quality gates.
