# Implementation Plan: Configurable Cross-LAN Discovery via DHT Swarm & Buzz Nostr Relay (`wss://`)

## Phase 1: Configuration & Cross-LAN Mode Modeling
- [x] Task: Extend `pkg/config` with `CrossLANMode` (`"dht"`, `"nostr"`, `"both"`, `"none"`) and `NostrRelayConfig` (`nostr_relay`)
    - [x] Write unit tests in `pkg/config/config_test.go` for parsing, defaulting, and validating `cross_lan_mode` and `nostr_relay` (`url`, `relays`, `api_key`, `announce_interval_sec`, `timeout_sec`, `advertise_addr`, `tls_skip_verify`) in `NodeConfig`.
    - [x] Implement `CrossLANMode` constants, `NostrRelayConfig` struct, helper methods (`EffectiveCrossLANMode()`, `EffectiveRelayURLs()`, `Validate()`), and fields in `pkg/config/config.go`.
    - [x] Document the Configurable Cross-LAN Discovery architecture (always-on LAN UDP beacon + configurable DHT / Buzz Nostr `wss://` Relay) in `conductor/tech-stack.md` and `conductor/product.md`.

## Phase 2: `CrossLANDiscoveryClient` Interface & Nostr `wss://` Client (`internal/p2p`)
- [x] Task: Implement BIP-340 Schnorr Signing, Ed25519 Peer Identity Binding, and NIP-01/09/33/40/42 Helpers
    - [x] Write unit tests in `internal/p2p/nostr_discovery_test.go` covering `DeriveNostrKeyPair`, `SignNostrEvent`, `VerifyNostrEvent`, `BuildAnnounceEvent` (Kind 30078 + NIP-40 expiration), `ParseAnnounceEvent` (with Ed25519 `VerifyPeerBinding`), `BuildUnannounceEvent` (Kind 5), and `BuildAuthEvent` (Kind 22242).
    - [x] Implement `CrossLANDiscoveryClient` interface and `NostrDiscoveryClient` in `internal/p2p/nostr_discovery.go`.
- [x] Task: Implement Live WebSocket Subscription & Periodic Announcer (`NostrDiscoveryClient`)
    - [x] Write unit tests in `internal/p2p/nostr_discovery_test.go` using an `httptest.Server` WebSocket Nostr relay for `Announce`, `LookupPeers`, `Unannounce`, NIP-42 `AUTH` challenge-response, `ReplicationPolicy` filtering, self-PK skipping, and background worker lifecycle.

## Phase 3: Engine Integration & End-to-End Cross-LAN Replication (`pkg/p2p`)
- [x] Task: Integrate `CrossLANMode`, `CrossLANDiscoveryClient`, and `NostrDiscoveryClient` into `pkg/p2p.Engine`
    - [x] Write unit and integration tests in `pkg/p2p/engine_test.go` verifying:
        1. Intra-LAN UDP beacon discovery remains active across all `CrossLANMode` values (`dht`, `nostr`, `both`, `none`).
        2. Two `Engine` instances configured with `CrossLANMode: "nostr"` (and no DHT bootstrap) discover each other via a mock Nostr `wss://` relay, establish a direct Noise connection, and replicate SQLite changesets end-to-end.
        3. `Engine.Close()` cleanly publishes a NIP-09 Kind 5 unannounce event to the Nostr relay.
    - [x] Update `EngineOptions`, `OpenEngine`, `OpenEngineFromNodeConfig`, `Close` in `pkg/p2p/engine.go` and export public types/aliases in `pkg/p2p/store.go`.

## Phase 4: Buzz Nostr Relay Protocol & Coding Agent Specifications (`docs/`)
- [x] Task: Author AsyncAPI 3.0 / Nostr Protocol Specification (`docs/nostr-buzz-relay-protocol.yaml`)
    - [x] Write structural validation test in `internal/p2p/nostr_specs_test.go` verifying that `docs/nostr-buzz-relay-protocol.yaml` and `docs/nostr-buzz-agent-spec.md` exist, contain all required NIPs/schemas/kinds, and use clean workspace-relative paths.
    - [x] Create `docs/nostr-buzz-relay-protocol.yaml` and `docs/nostr-buzz-agent-spec.md`.
