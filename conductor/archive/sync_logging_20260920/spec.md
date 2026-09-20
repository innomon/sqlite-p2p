# Specification: P2P Feed Sync Broadcast & Structured JSON Logging

## 1. Overview
Address findings from the P2P engine review by making the `crm-peer sync` command functionally broadcast local changesets to all connected peers, introducing standardized structured JSON logging using Go standard library `log/slog` across the entire P2P and node lifecycle, and adding graceful daemon shutdown cleanup.

## 2. Functional Requirements
- **FR-1: Peer Feed Broadcast on Sync (`internal/cli/commands.go`, `internal/p2p/engine.go`)**:
  - Implement full feed replay broadcast in `crm-peer sync`: replay local `ChangesetFeed` from sequence `0` to the end and broadcast each changeset to all active peers via `Replicator.BroadcastChangeset`.
  - Provide a dedicated engine method (e.g. `engine.SyncPeers(ctx) error` or `BroadcastFeed(ctx) error`) to encapsulate reconciliation and report broadcast count.
- **FR-2: Standardized Structured JSON Logging (`internal/logger`, `internal/p2p/`, `internal/cli/`)**:
  - Implement structured logger initialization using standard library `log/slog` with `slog.NewJSONHandler`, writing machine-parseable JSON lines.
  - Standardize log fields: `timestamp`, `level`, `service="crm-peer"`, `event`, `trace_id` (or component context).
  - Instrument P2P Swarm lifecycle: topic joined, topic left, peer connection established, peer disconnected.
  - Instrument Replicator events: changeset broadcast, changeset received, decode failure, connection termination.
  - Instrument Engine conflict resolution: remote changeset applied, obsolete changeset skipped (LWW drop).
- **FR-3: Graceful Daemon Shutdown (`internal/cli/commands.go`)**:
  - Ensure `crm-peer start` properly closes `SwarmManager` and `Replicator` upon context cancellation (`ctx.Done()`), logging termination events.
- **FR-4: Code Polish & Hygiene**:
  - Clean up dead code (`_ = i` in `internal/p2p/autobase.go:74`).
  - Extract unexported `get()` helper in `internal/p2p/feed.go` to eliminate nested lock pattern in `Replay()`.

## 3. Non-Functional Requirements & Constraints
- **Zero CGO**: Pure Go builds (`CGO_ENABLED=0`).
- **Standard Library First**: Zero external logging dependencies; use `log/slog`.
- **Test Coverage**: Maintain ≥ 80% coverage on `internal/p2p` and `internal/cli`.

## 4. Acceptance Criteria
1. Running `crm-peer sync` broadcasts all local feed changesets to connected peers and reports the count of broadcasted changesets.
2. An end-to-end or integration test verifies that a lagged peer catches up upon receiving changesets broadcasted during sync.
3. Swarm, Replicator, Engine, and CLI output valid JSON log records containing `timestamp`, `level`, `service`, and `event`.
4. Graceful shutdown in `start` properly invokes resource cleanup on cancellation.
5. `go test -v -cover ./...` passes with CGO_ENABLED=0.
