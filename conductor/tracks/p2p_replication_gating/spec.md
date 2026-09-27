# Track Specification: Replication Gating & Swarm Access Control (`p2p_replication_gating`)

## Overview
Integrate `go-pear/pkg/policy` into `sqlite-p2p`'s `SwarmManager` (`internal/p2p/swarm.go`) and high-level `Engine` (`pkg/p2p/engine.go`) to enable configurable peer replication access control across private, enterprise, and sovereign Layer 2 swarms.

Nodes can operate in three distinct access modes:
1. `all`: Open discovery and replication (default permissive).
2. `whitelist`: Default-deny mode; only explicitly whitelisted peer public keys (`[32]byte`) are permitted to connect or replicate.
3. `blacklist`: Default-allow mode; any node can replicate except explicitly blocked public keys.

## Objectives & Requirements

### 1. SwarmManager Policy Integration (`internal/p2p/swarm.go`)
- Extend `SwarmManagerOptions` with `Policy *policy.ReplicationPolicy`.
- When initializing `hyperswarm.New()`, attach `opts.Policy.IsAllowed` to `SwarmOptions.Authorizer`.
- Expose methods on `*SwarmManager`:
  - `Policy() *policy.ReplicationPolicy`
  - `SetPolicy(p *policy.ReplicationPolicy)`
  - `DisconnectPeer(peerPK [32]byte) error`
  - `EvictDisallowed() int`
- When policy is updated or a peer is revoked, actively evict non-compliant active peer connections via `swarm.Disconnect(remotePK)`.
- Unit tests in `internal/p2p/swarm_test.go` verifying whitelist, blacklist, and dynamic eviction.

### 2. High-Level Engine Policy Integration (`pkg/p2p/engine.go`)
- Extend `EngineOptions` with `Policy *policy.ReplicationPolicy`.
- Pass `opts.Policy` through to `SwarmManagerOptions`.
- Expose policy inspection and dynamic mutation methods on `*Engine`:
  - `Policy() *policy.ReplicationPolicy`
  - `SetPolicy(p *policy.ReplicationPolicy)`
  - `DisconnectPeer(peerPK [32]byte) error`
  - `EvictDisallowed() int`

### 3. CLI & E2E Replication Flag Integration (`cmd/e2e-replication/main.go`)
- Support replication gating flags:
  - `--replication-mode=<all|whitelist|blacklist>`
  - `--allow-peer=<hex>` (repeatable / comma-separated 64-char hex)
  - `--deny-peer=<hex>` (repeatable / comma-separated 64-char hex)
  - `--gate-config=<path>` (JSON ACL configuration file)
- Automatically construct and configure `policy.ReplicationPolicy` on the running engine.

### 4. Verification & Non-Functional Requirements
- Zero-allocation hot-path authorizer checks ($O(1)$ zero-copy lookups).
- 100% test pass across all packages without regressions.
- Pure Go (`CGO_ENABLED=0`) compatible builds.
