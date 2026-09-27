# Track Plan: Replication Gating & Swarm Access Control (`p2p_replication_gating`)

## Phase 1: SwarmManager Policy Integration
- [x] Task 1.1: Integrate `go-pear/pkg/policy.ReplicationPolicy` into `SwarmManagerOptions` and wire `Authorizer` into `hyperswarm.New()` in `internal/p2p/swarm.go`.
- [x] Task 1.2: Add `Policy()`, `SetPolicy()`, `DisconnectPeer()`, and `EvictDisallowed()` methods to `*SwarmManager`.
- [x] Task 1.3: Add unit tests in `internal/p2p/swarm_test.go` verifying whitelist, blacklist, and dynamic eviction.

## Phase 2: Engine Policy Integration
- [x] Task 2.1: Extend `EngineOptions` with `Policy *policy.ReplicationPolicy` and forward to `SwarmManager` in `pkg/p2p/engine.go`.
- [x] Task 2.2: Expose `Policy()`, `SetPolicy()`, `DisconnectPeer()`, and `EvictDisallowed()` on `*Engine`.
- [x] Task 2.3: Add unit tests in `pkg/p2p/engine_test.go` verifying replication gating on the high-level Engine.

## Phase 3: E2E CLI Flags & Verification
- [x] Task 3.1: Wire `--replication-mode`, `--allow-peer`, `--deny-peer`, and `--gate-config` flags into `cmd/e2e-replication/main.go`.
- [x] Task 3.2: Run full test suite (`go test ./...`) and verify clean build.
