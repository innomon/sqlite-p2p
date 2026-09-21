# Specification: Pure Go Pear P2P Swarm & Replication Engine

## 1. Overview
Implement the decentralized, multi-writer peer-to-peer replication engine for the CRM using the pure Go Pear/Holepunch protocol stack (`go-pear` packages: `hypercore`, `hyperswarm`, and `autobase`). Local SQLite mutations captured as changesets will be appended to an append-only Hypercore log, announced across a 32-byte cluster topic over Hyperswarm, causally linearized via Autobase, and applied deterministically to remote SQLite `crm_store` instances with Last-Writer-Wins (LWW) conflict resolution.

## 2. Functional Requirements
- **FR-1: Go Pear Integration**: Configure `go.mod` with a local `replace go-pear => ../go-pear` directive to import pure Go `hypercore`, `hyperswarm`, and `autobase` modules without external network dependencies or CGO.
- **FR-2: 32-Byte Swarm Topic Derivation**: Derive the Hyperswarm topic key as `SHA-256(cfg.SwarmTopic)` (e.g. `in.qzip.crm.cluster.v1`), allowing peers configured with the same topic string to discover and swarm each other.
- **FR-3: Replication Engine (`internal/p2p/engine.go`)**:
  - Connect `ChangesetTracker` events to a local append-only Hypercore feed.
  - Join Hyperswarm topic for automated peer discovery, connection multiplexing, and bidirectional sync.
  - Ingest remote Hypercore feeds into Autobase for causal linearization and vector ordering.
- **FR-4: Conflict Resolution (LWW)**: Apply remote changesets to SQLite `crm_store` using Last-Writer-Wins based on nanosecond timestamps and sequence numbers. Skip obsolete mutations if local record timestamp is strictly newer.
- **FR-5: CLI Commands**:
  - `crm-peer start`: Run foreground peer daemon with background sync worker, graceful shutdown on SIGINT/SIGTERM.
  - `crm-peer peer status` / `crm-peer peer list`: Inspect active swarm connections, peer discovery count, and Hypercore length/storage metrics.
  - `crm-peer sync`: Trigger immediate reconciliation sweep against currently connected peers.

## 3. Non-Functional Requirements & Constraints
- **Zero CGO**: Pure Go builds (`CGO_ENABLED=0`) across all P2P and SQLite components.
- **Privacy & Security**: Payloads remain pseudonymized (primary key format `in.qzip.crm.customer:<BASE32(SHA256(phone))>`).
- **Structured Logging**: All P2P connection lifecycle events, changeset appends, and sync errors logged as structured JSON matching `structured_logging_pattern.md`.
- **Test Coverage**: ≥ 80% unit and integration test coverage across new packages (`internal/p2p`, CLI handlers).

## 4. Acceptance Criteria
1. Multi-peer test: Two independent `crm-peer` nodes with distinct data directories join the same topic and replicate customer records bi-directionally.
2. Concurrent writes to separate keys converge on both nodes.
3. Concurrent writes to the same key resolve deterministically according to LWW timestamps.
4. CLI commands `start`, `peer status`, `peer list`, and `sync` execute with proper output and error codes.
5. `go test -v -cover ./...` passes with CGO_ENABLED=0.

## 5. Out of Scope
- AES-256-GCM crypto-shredding key registry (deferred to dedicated encryption track).
- Markdown/YAML frontmatter file watcher (deferred to ontology track).
