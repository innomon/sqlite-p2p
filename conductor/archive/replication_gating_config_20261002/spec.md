# Track Specification: Replication Gating & Node Access Control Configuration (`replication_gating_config`)

## Overview
Integrate and standardize Replication Gating & Node Access Control configuration across `sqlite-p2p`. This track extends node configuration parsing, updates node configuration JSON files in `/config/`, provides a dedicated configuration loader package (`/pkg/config`), and wires gating policy settings into the high-level `Engine` (`/pkg/p2p/engine.go`) and E2E CLI (`/cmd/e2e-replication/main.go`).

Nodes can be configured for replication access control using `go-pear/pkg/policy` in three modes:
1. `all`: Open discovery and unrestricted peer replication (default).
2. `whitelist`: Permitting only explicitly whitelisted peer public keys (`[32]byte` hex strings).
3. `blacklist`: Permitting all peers except explicitly blacklisted peer public keys.

## Objectives & Requirements

### 1. Central Node & Gating Configuration Package (`/pkg/config/config.go`)
- Define a unified `NodeConfig` struct supporting node parameters (`node_id`, `swarm_topic`, `swarm_port`, `bootstrap`, `peer_addrs`, `db_path`, `enable_wal`, `enable_crypto`, `auto_sync`) and embedded `Replication` policy configuration (`*policy.Config`).
- Implement `LoadConfig(r io.Reader) (*NodeConfig, error)` and `LoadConfigFile(path string) (*NodeConfig, error)`.
- Implement `(nc *NodeConfig) BuildPolicy() (*policy.ReplicationPolicy, error)` to convert JSON/YAML gating config into an active `*policy.ReplicationPolicy`.

### 2. High-Level Engine Configuration Helper (`/pkg/p2p/engine.go`)
- Support initializing `Engine` directly using `NodeConfig` or `EngineOptions`.
- Ensure `EngineOptions.Policy` is populated from `NodeConfig.BuildPolicy()` when opening an engine from a configuration file.

### 3. Example Node Configurations (`/config/*.json`)
- Update example configuration files in `/config/`:
  - `/config/config.example.json`: Include example `"replication"` block demonstrating `whitelist` and `blacklist` options with commented explanations.
  - `/config/node1.json`, `/config/node2.json`, `/config/node-mac-m4.json`, `/config/node-rpi4.json`, `/config/node-rpi5.json`: Update with optional `"replication"` block for gating configuration.

### 4. CLI Slash Command & CLI Gating Management (`/cmd/e2e-replication/main.go`)
- Update `cmd/e2e-replication/main.go` to use `/pkg/config` for config file parsing.
- Provide slash commands for interactive runtime gating inspection and modification:
  - `/gating status`: Display current policy mode, whitelist count, blacklist count, and active peer access status.
  - `/gating set-mode <all|whitelist|blacklist>`: Dynamically switch replication gating mode and trigger immediate peer eviction.
  - `/gating allow <hex_key>`: Dynamically append a public key to the whitelist.
  - `/gating deny <hex_key>`: Dynamically append a public key to the blacklist.
  - `/gating evict`: Force eviction of currently connected non-compliant peers.

### 5. Verification & Non-Functional Requirements
- 100% test pass rate across all packages with `go test ./...`.
- Code coverage >= 80% for `/pkg/config`.
- Pure Go (`CGO_ENABLED=0`) compatible build.
