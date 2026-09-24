# Agent Integration Guide: sqlite-p2p

This guide provides instructions for AI coding agents and autonomous developer harnesses (Cursor, Claude, Goose, Antigravity) integrating or working with the **`sqlite-p2p`** decentralized storage engine.

> [!NOTE]
> If you are looking for the **Multimodal CRM Agent & MCP Server**, that functionality has been factored into the standalone [`crm-lite`](/home/innomon/B204-zone/owly-sewa/crm-lite) repository. See `crm-lite/AGENT.md` for CRM MCP tools.

---

## 1. Library Overview

`sqlite-p2p` is a pure Go (`CGO_ENABLED=0`) decentralized SQLite database engine that provides:
- **Local SQLite Store**: Powered by `modernc.org/sqlite` with WAL mode enabled.
- **Peer-to-Peer Replication**: Powered by pure Go Holepunch/Pear protocols (Autobase changeset linearization and Hyperswarm DHT discovery).
- **Cryptographic Shredding**: Per-record AES-256-GCM encryption with key purging for mathematical right-to-be-forgotten on append-only feeds.
- **Clean Public Embedding API**: Exported via `sqlite-p2p/pkg/p2p`.

---

## 2. Embedding in Go Projects

To embed `sqlite-p2p` in your Go service or application:

### Step 1: Add Dependency

In your `go.mod`:

```go
require sqlite-p2p v0.0.0

replace sqlite-p2p => /home/innomon/B204-zone/owly-sewa/sqlite-p2p
```

### Step 2: Initialize Engine

```go
package main

import (
    "context"
    "log"

    "sqlite-p2p/pkg/p2p"
)

func main() {
    engine, err := p2p.NewEngine(p2p.Config{
        DBPath:     "my_node.db",
        WALMode:    true,
        SwarmTopic: "my.app.cluster.v1",
        StorageDir: ".storage_data",
        EnableP2P:  true,
    })
    if err != nil {
        log.Fatalf("failed to init engine: %v", err)
    }
    defer engine.Close()

    // Access Repository
    repo := engine.Repository()

    // Store record
    ctx := context.Background()
    err = repo.Put(ctx, "user:123", `{"name":"Alice"}`, []byte("confidential"))
    if err != nil {
        log.Fatalf("failed to put: %v", err)
    }
}
```

---

## 3. Architecture & Reference

- **[ARCHITECTURE.md](/ARCHITECTURE.md)**: Deep dive into the CQRS replication pipeline, encryption at rest, and DHT topic discovery.
- **[README.md](/README.md)**: Build instructions, testing guidelines, and quality gates.
