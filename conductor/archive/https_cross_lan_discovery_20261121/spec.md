# Specification: Configurable Cross-LAN Discovery via DHT Swarm & Buzz Nostr Relay (`wss://`)

## 1. Overview
Currently, `sqlite-p2p` uses `cmd/dht-seed/main.go` (HyperDHT bootstrap node over UDP/TCP) to discover peers across LANs, combined with UDP broadcast beacons (`internal/p2p/beacon.go`) for zero-config intra-LAN discovery. In environments where UDP/DHT traffic across LANs or corporate firewalls is restricted or where operators run a custom domain **Buzz Nostr Relay** (`wss://` over HTTPS), nodes need a standard Nostr cross-LAN discovery mechanism behind a clean `CrossLANDiscoveryClient` interface.

This track implements:
1. **Always-On Intra-LAN Discovery**: Local UDP broadcast beacon discovery (`DiscoverLANBootstrap` and `StartBeaconBroadcaster`) and local Hyperswarm TCP/UDX listening remain active regardless of the cross-LAN mode.
2. **Configurable Cross-LAN Discovery Modes (`cross_lan_mode`)**: Support `"dht"` (existing HyperDHT bootstrap over IP), `"nostr"` (Buzz Nostr Relay over `wss://`), `"both"` (simultaneous DHT + Nostr discovery), and `"none"` (intra-LAN only).
3. **`CrossLANDiscoveryClient` Interface & `NostrDiscoveryClient` (`sqlite-p2p` side)**: A clean Go `CrossLANDiscoveryClient` interface backed by a Nostr `wss://` client (`internal/p2p/nostr_discovery.go`) supporting **NIP-01** (`EVENT`, `REQ`, `CLOSE`, `EOSE`, `OK`), **NIP-33** Parameterized Replaceable Events (`kind: 30078`), **NIP-40** (`expiration` TTL), **NIP-09** (`kind: 5` unannounce deletion), and **NIP-42** (`kind: 22242` `AUTH`), with dual BIP-340 `secp256k1` Schnorr signatures and `ed25519` Swarm `peer_pk` binding.
4. **Protocol Specification (`docs/nostr-buzz-relay-protocol.yaml`)**: Formal AsyncAPI 3.0 & Nostr NIP contract for the Buzz Nostr Relay.
5. **Coding Agent Specification (`docs/nostr-buzz-agent-spec.md`)**: Detailed architectural, cryptographic, storage, and behavioral specification enabling an autonomous coding agent to implement the standalone custom domain **Buzz Nostr Relay** server project.

---

## 2. Functional Requirements

### 2.1 Intra-LAN vs. Cross-LAN Discovery Separation
- **Intra-LAN Discovery (Always Enabled)**:
  - Whenever `SwarmTopic` is non-zero, `Engine` continues to run local UDP broadcast beacon discovery (`DiscoverLANBootstrap`) on startup and periodic LAN UDP broadcasting (`StartBeaconBroadcaster`) on `DefaultBeaconPort` (49736).
- **Cross-LAN Mode Configuration (`cross_lan_mode`)**:
  - Configurable in `pkg/config.NodeConfig` (`cross_lan_mode`) and `pkg/p2p.EngineOptions` (`CrossLANMode`):
    - `"dht"`: Uses `Bootstrap` addresses with HyperDHT (`cmd/dht-seed`) for cross-LAN discovery. (Default when `cross_lan_mode` is omitted, preserving 100% backward compatibility).
    - `"nostr"`: Disables external WAN DHT bootstrap queries (passes only LAN-discovered bootstrap peers to Hyperswarm) and uses `NostrDiscoveryClient` over `wss://` to announce and subscribe to peers across LANs.
    - `"both"`: Uses both HyperDHT `Bootstrap` nodes and `NostrDiscoveryClient` concurrently.
    - `"none"`: Disables both external DHT bootstrap and Nostr discovery; only intra-LAN UDP beacon discovery and static `peer_addrs` are used.

### 2.2 Nostr Relay Configuration (`NostrRelayConfig`)
- Added to `pkg/config.NodeConfig` under `nostr_relay` (JSON/YAML) and `pkg/p2p.EngineOptions` (`NostrRelay`):
  - `url` (`string`): Primary URL of the Buzz Nostr relay (e.g., `wss://buzz.example.com` or `https://buzz.example.com`).
  - `relays` (`[]string`, optional): Additional fallback Nostr relay URLs.
  - `api_key` (`string`, optional): Bearer token for WebSocket upgrade gateway authentication (`Authorization: Bearer <api_key>`).
  - `announce_interval_sec` (`int`, default `15`): Interval in seconds for periodic NIP-33 presence refresh and peer reconciliation.
  - `timeout_sec` (`int`, default `10`): Per-operation WebSocket timeout in seconds.
  - `advertise_addr` (`string`, optional): Explicit public/routable `host:port` or IP to announce to remote LANs; if omitted, auto-resolves from `SwarmManager` routable IP and listening port.
  - `tls_skip_verify` (`bool`, optional): Allow self-signed certificates in testing/staging environments.

### 2.3 Cryptographic Identity & Nostr Event Signing
- **Deterministic `secp256k1` Nostr Key Derivation**:
  - Derived deterministically from the node's 32-byte `secretstream.KeyPair` via `sha256("sqlite-p2p:nostr:v1:" || KeyPair.Private[:])`.
  - Produces a 32-byte x-only BIP-340 public key (`event.pubkey`) and 64-byte BIP-340 Schnorr signature (`event.sig`).
- **Cryptographic Swarm `peer_pk` Binding (`ed25519`)**:
  - To prove that the holder of the Nostr `pubkey` also owns the announced 32-byte Noise/Swarm `peer_pk`, `event.content` includes an Ed25519 signature (`peer_sig`) over:
    `"sqlite-p2p:peer-binding:v1\n" + nostr_pubkey + "\n" + topic_hex + "\n" + peer_pk + "\n" + strings.Join(endpoints, ",") + "\n" + created_at`

### 2.4 Nostr Discovery Protocol & Client Lifecycle (`CrossLANDiscoveryClient`)
- **NIP-33 Announce (`kind: 30078`) + NIP-40 Expiration**:
  - Publishes `["EVENT", ev]` with tags `["d", "sqlite-p2p:" + topicHex]`, `["t", topicHex]`, `["p", peerPKHex]`, and `["expiration", unixSec]`.
- **NIP-01 Real-Time Subscription & Query (`REQ` / `EOSE`)**:
  - Subscribes with `["REQ", subID, {"kinds": [30078], "#t": [topicHex], "limit": 100}]` for both live push discovery and periodic reconciliation.
- **NIP-42 Client Authentication (`kind: 22242`)**:
  - Automatically responds to relay `["AUTH", "<challenge>"]` frames with a signed `kind: 22242` event.
- **NIP-09 Unannounce (`kind: 5`)**:
  - On `Engine.Close()`, publishes a `kind: 5` deletion event referencing `["a", "30078:<nostr_pub>:sqlite-p2p:<topic_hex>"]` and `["e", lastEventID]`.

---

## 3. Acceptance Criteria
1. Intra-LAN UDP beacon discovery (`DiscoverLANBootstrap` / `StartBeaconBroadcaster`) remains active in all `cross_lan_mode` settings (`"dht"`, `"nostr"`, `"both"`, `"none"`).
2. `NodeConfig` and `EngineOptions` parse and validate `cross_lan_mode` and `nostr_relay` settings.
3. `NostrDiscoveryClient` implements `CrossLANDiscoveryClient`, signs events with BIP-340 Schnorr + Ed25519 peer binding, handles NIP-42 `AUTH`, filters via `ReplicationPolicy`, and connects peers via `SwarmManager.ConnectDirect`.
4. End-to-end test verifies two `Engine` nodes with no DHT bootstrap (`cross_lan_mode: "nostr"`) discover each other via a mock Nostr `wss://` relay and replicate SQLite records.
5. `docs/nostr-buzz-relay-protocol.yaml` and `docs/nostr-buzz-agent-spec.md` are created and use clean workspace-relative paths.
