# Implementation Plan: Configurable Cross-LAN Discovery via DHT Swarm & Secure HTTPS Registry API

## Phase 1: Configuration & Cross-LAN Mode Modeling [checkpoint: 49a8181]
- [x] Task: Extend `pkg/config` with `CrossLANMode` and `HTTPSAPIConfig` 360cecd
    - [x] Write failing unit tests in `pkg/config/config_test.go` for parsing, defaulting, and validating `cross_lan_mode` (`"dht"`, `"https"`, `"both"`, `"none"`) and `https_api` (`url`, `api_key`, `poll_interval_sec`, `timeout_sec`, `advertise_addr`, `tls_skip_verify`) in `NodeConfig`.
    - [x] Implement `CrossLANMode` constants, `HTTPSAPIConfig` struct, helper methods (`EffectiveCrossLANMode()`, `Validate()`), and fields in `pkg/config/config.go`.
    - [x] Run tests and verify >80% coverage for `pkg/config`.
- [x] Task: Update `conductor/tech-stack.md` & `conductor/product.md` a531089
    - [x] Document the Configurable Cross-LAN Discovery architecture (always-on LAN UDP beacon + configurable DHT / Secure HTTPS Registry API) in `conductor/tech-stack.md` and `conductor/product.md`.
- [x] Task: Conductor - User Manual Verification 'Phase 1: Configuration & Cross-LAN Mode Modeling' (Protocol in workflow.md) 49a8181

## Phase 2: Secure HTTPS Discovery Client & Cryptographic Request Signing (`internal/p2p`) [checkpoint: 7e295e2]
- [x] Task: Implement Ed25519 Request Signing and Canonical Verification Helpers 2dd66f2
    - [x] Write failing unit tests in `internal/p2p/https_registry_test.go` covering canonical payload construction (`METHOD\nPATH_AND_QUERY\nTIMESTAMP\nSHA256_HEX`), Ed25519 signing using `secretstream.KeyPair`, header attachment (`Authorization`, `X-Peer-Public-Key`, `X-Peer-Timestamp`, `X-Peer-Signature`), and signature verification.
    - [x] Implement request signing and verification utilities in `internal/p2p/https_registry.go`.
- [x] Task: Implement Stateless REST Discovery Client (`HTTPSRegistryClient`) & Background Poller 2dd66f2
    - [x] Write failing unit tests in `internal/p2p/https_registry_test.go` using `httptest.Server` for `Announce`, `LookupPeers`, `Unannounce`, `Health`, `ReplicationPolicy` filtering, self-PK skipping, and periodic poll/announce worker lifecycle.
    - [x] Implement `HTTPSRegistryClient` and background discovery worker in `internal/p2p/https_registry.go`, connecting discovered peers via a dial callback (`SwarmManager.ConnectDirect`).
    - [x] Run `go test -v -cover ./internal/p2p/...` and confirm all tests pass with >80% coverage.
- [x] Task: Conductor - User Manual Verification 'Phase 2: Secure HTTPS Discovery Client & Cryptographic Request Signing (`internal/p2p`)' (Protocol in workflow.md) 7e295e2

## Phase 3: Engine Integration & End-to-End Cross-LAN Replication (`pkg/p2p`)
- [x] Task: Integrate `CrossLANMode` and `HTTPSRegistryClient` into `pkg/p2p.Engine` 2b2d5be
    - [x] Write failing unit and integration tests in `pkg/p2p/engine_test.go` verifying:
        1. Intra-LAN UDP beacon discovery remains active across all `CrossLANMode` values (`dht`, `https`, `both`, `none`).
        2. Two `Engine` instances configured with `CrossLANMode: "https"` (and no DHT bootstrap) discover each other via an `httptest.Server` mock registry, establish a direct Noise connection, and replicate SQLite changesets end-to-end.
        3. Disallowed peers are rejected according to `ReplicationPolicy`.
        4. `Engine.Close()` cleanly unannounces the peer from the HTTPS registry.
    - [x] Update `EngineOptions`, `OpenEngine`, `OpenEngineFromNodeConfig`, `Close` in `pkg/p2p/engine.go` and export public types/aliases in `pkg/p2p/store.go`.
    - [x] Run full test suite `go test -v -cover ./...` and verify all tests pass.
- [~] Task: Conductor - User Manual Verification 'Phase 3: Engine Integration & End-to-End Cross-LAN Replication (`pkg/p2p`)' (Protocol in workflow.md)

## Phase 4: OpenAPI 3.1 Specification & Coding Agent Specification (`docs/`)
- [ ] Task: Author OpenAPI 3.1 Specification (`docs/https-swarm-openapi.yaml`)
    - [ ] Write a structural validation test in `internal/p2p/https_specs_test.go` verifying that `docs/https-swarm-openapi.yaml` and `docs/https-swarm-agent-spec.md` exist, contain all required endpoints/schemas/headers, and use clean workspace-relative paths.
    - [ ] Create `docs/https-swarm-openapi.yaml` defining `/v1/health`, `/v1/topics/{topic}/announce`, `/v1/topics/{topic}/peers`, and `/v1/topics/{topic}/peers/{peer_pk}` with Bearer auth, Ed25519 signature headers, request/response schemas, and error payloads.
- [ ] Task: Author Coding Agent Specification (`docs/https-swarm-agent-spec.md`)
    - [ ] Create `docs/https-swarm-agent-spec.md` providing a complete blueprint for a coding agent to build the Secure HTTPS Swarm Discovery Server (system overview, endpoint contracts, canonical Ed25519 signature verification algorithm + test vectors, TTL store schema, NAT/IP inference rules, security/rate-limiting requirements, and verification test suite).
    - [ ] Run `go test -v ./...` to verify all specification checks and unit/integration tests pass.
- [ ] Task: Conductor - User Manual Verification 'Phase 4: OpenAPI 3.1 Specification & Coding Agent Specification (`docs/`)' (Protocol in workflow.md)
