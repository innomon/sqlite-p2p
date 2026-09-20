# Implementation Plan: Pure Go Pear P2P Swarm & Replication Engine

## Phase 1: Pure Go Pear Stack Integration & Topic Derivation
- [x] Task: Integrate `go-pear` dependency into `go.mod` (2c364c5)
    - [x] Write Tests: Verify module import and instantiation of core go-pear primitives (Hypercore, Swarm topic hash)
    - [x] Implement: Add `replace go-pear => ../go-pear` and required dependencies in `go.mod`, implement topic derivation helper in `internal/p2p/topic.go`
- [x] Task: Implement Hypercore append-only log adapter for changesets (ae1347f)
    - [x] Write Tests: Unit tests for writing serialized Changeset entries to Hypercore and decoding stream chunks
    - [x] Implement: `internal/p2p/feed.go` wrapping Hypercore storage for append and replay operations
- [ ] Task: Conductor - User Manual Verification 'Phase 1: Pure Go Pear Stack Integration & Topic Derivation' (Protocol in workflow.md)

---

## Phase 2: Hyperswarm P2P Discovery & Connection Manager
- [ ] Task: Implement P2P Swarm connection manager
    - [ ] Write Tests: Unit tests for topic joining, peer discovery events, and connection lifecycle handlers
    - [ ] Implement: `internal/p2p/swarm.go` wrapping pure Go `hyperswarm` and connection multiplexing
- [ ] Task: Implement bidirectional changeset replication protocol
    - [ ] Write Tests: Unit tests for peer handshake, changeset exchange, and feed replication over mock/local connections
    - [ ] Implement: `internal/p2p/replicator.go` streaming changesets across connected peers
- [ ] Task: Conductor - User Manual Verification 'Phase 2: Hyperswarm P2P Discovery & Connection Manager' (Protocol in workflow.md)

---

## Phase 3: Autobase Causal Ordering & LWW State Convergence
- [ ] Task: Wire Autobase multi-writer consensus and causal linearization
    - [ ] Write Tests: Tests verifying multi-writer changesets linearize into a deterministic unified sequence
    - [ ] Implement: `internal/p2p/autobase.go` managing multi-peer Hypercore feeds
- [ ] Task: Implement LWW conflict resolution and SQLite `crm_store` materializer
    - [ ] Write Tests: Unit tests verifying concurrent writes to the same key resolve using timestamp/sequence LWW semantics
    - [ ] Implement: Conflict interceptor in `internal/p2p/engine.go` applying remote changesets to SQLite repository
- [ ] Task: Conductor - User Manual Verification 'Phase 3: Autobase Causal Ordering & LWW State Convergence' (Protocol in workflow.md)

---

## Phase 4: CLI Integration & Multi-Node Verification
- [ ] Task: Implement CLI commands `start`, `peer`, and `sync`
    - [ ] Write Tests: CLI integration tests for `crm-peer start` daemon lifecycle, `peer status`, `peer list`, and `sync`
    - [ ] Implement: CLI subcommand handlers in `internal/cli/commands.go` connected to ReplicationEngine
- [ ] Task: Multi-node dual-write end-to-end integration test
    - [ ] Write Tests: End-to-end test spinning up two peer instances on isolated storage dirs, verifying bi-directional customer sync and state convergence
    - [ ] Implement: End-to-end verification test suite in `tests/p2p_sync_test.go`
- [ ] Task: Conductor - User Manual Verification 'Phase 4: CLI Integration & Multi-Node Verification' (Protocol in workflow.md)
