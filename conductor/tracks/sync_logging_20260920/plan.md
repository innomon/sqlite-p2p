# Implementation Plan: P2P Feed Sync Broadcast & Structured JSON Logging

## Phase 1: Structured JSON Logging Infrastructure & Code Polish [checkpoint: 01c2a9b]
- [x] Task: Clean up dead code and optimize Feed locking (2cfaed6)
    - [x] Write Tests: Unit tests for ChangesetFeed concurrent replay and get access without deadlock
    - [x] Implement: Remove `_ = i` in `autobase.go`, extract internal `get()` in `feed.go` to eliminate nested lock pattern in `Replay()`
- [x] Task: Implement Structured JSON Logger using `log/slog` (1574680)
    - [x] Write Tests: Unit tests verifying logger writes machine-parseable JSON lines with required fields (`timestamp`, `level`, `service`, `event`)
    - [x] Implement: Create `internal/logger/logger.go` wrapping `log/slog.NewJSONHandler` and supporting configured log level
- [x] Task: Conductor - User Manual Verification 'Phase 1: Structured JSON Logging Infrastructure & Code Polish' (Protocol in workflow.md) (01c2a9b)

---

## Phase 2: Instrumentation of P2P Swarm, Replicator, & Engine [checkpoint: 8e69e0e]
- [x] Task: Instrument Swarm and Replicator lifecycle with structured logs (d44f651)
    - [x] Write Tests: Unit tests asserting structured log emission during swarm join/leave and replicator stream handling
    - [x] Implement: Inject logger into `SwarmManager` and `Replicator`, log topic discovery, peer connections/disconnections, and broadcast events
- [x] Task: Instrument ReplicationEngine and LWW conflict resolution (5883be9)
    - [x] Write Tests: Unit tests verifying structured log output for local mutations, remote applications, and LWW dropped obsolete changesets
    - [x] Implement: Update `ReplicationEngine` to emit structured logs on apply and LWW drop
- [x] Task: Conductor - User Manual Verification 'Phase 2: Instrumentation of P2P Swarm, Replicator, & Engine' (Protocol in workflow.md) (8e69e0e)

---

## Phase 3: P2P Feed Broadcast Sync & Graceful Shutdown
- [x] Task: Implement P2P Feed Broadcast Sync (6c87d5a)
    - [x] Write Tests: Unit and integration tests verifying `engine.SyncPeers` replays and broadcasts all feed changesets to connected peers and catches up lagged nodes
    - [x] Implement: Add `SyncPeers(ctx)` method to `ReplicationEngine` and connect to `crm-peer sync` CLI subcommand
- [ ] Task: Implement graceful daemon shutdown in `crm-peer start`
    - [ ] Write Tests: Unit tests verifying `crm-peer start` closes swarm and replicator on context cancellation
    - [ ] Implement: Wire `swarm.Close()` and `replicator.Close()` with structured log logging shutdown lifecycle in `commands.go`
- [ ] Task: Conductor - User Manual Verification 'Phase 3: P2P Feed Broadcast Sync & Graceful Shutdown' (Protocol in workflow.md)
