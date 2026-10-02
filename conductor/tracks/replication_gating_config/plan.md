# Track Plan: Replication Gating & Node Access Control Configuration (`replication_gating_config`)

## Phase 1: Central Configuration Package (`pkg/config`)
- [x] Task 1.1: Create `/pkg/config/config.go` with `NodeConfig` struct embedding `*policy.Config` and config loaders (`LoadConfig`, `LoadConfigFile`, `BuildPolicy`). [ce1207a]
- [x] Task 1.2: Add unit tests in `/pkg/config/config_test.go` covering JSON/YAML parsing, missing fields, invalid hex keys, and `BuildPolicy` conversion. [ce1207a]

## Phase 2: Engine Integration & JSON Config Updating
- [x] Task 2.1: Add `OpenEngineFromConfig(path string)` helper or integrate `/pkg/config` in `/pkg/p2p/engine.go`. [cbe01a2]
- [x] Task 2.2: Add unit tests in `/pkg/p2p/engine_test.go` verifying engine initialization with gating configuration. [cbe01a2]
- [x] Task 2.3: Update JSON configuration templates in `/config/config.example.json`, `/config/node1.json`, `/config/node2.json`, `/config/node-mac-m4.json`, `/config/node-rpi4.json`, and `/config/node-rpi5.json` with sample replication gating blocks. [cbe01a2]

## Phase 3: CLI Gating Management & E2E Verification
- [x] Task 3.1: Refactor `/cmd/e2e-replication/main.go` to use `/pkg/config` and add interactive slash commands (`/gating status`, `/gating set-mode`, `/gating allow`, `/gating deny`, `/gating evict`). [a4bf2a7]
- [x] Task 3.2: Add unit/integration tests in `/cmd/e2e-replication/main_test.go` for gating configuration flags and slash commands. [a4bf2a7]
- [x] Task 3.3: Execute full automated test suite (`go test ./...`) and verify clean build. [a4bf2a7]
