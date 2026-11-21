# Specification: Configurable Cross-LAN Discovery via DHT Swarm & Secure HTTPS Registry API

## 1. Overview
Currently, `sqlite-p2p` uses `cmd/dht-seed/main.go` (HyperDHT bootstrap node over UDP/TCP) to discover peers across LANs, combined with UDP broadcast beacons (`internal/p2p/beacon.go`) for zero-config intra-LAN discovery. In environments where UDP/DHT traffic across LANs or corporate firewalls is restricted or where operators prefer an HTTPS-managed rendezvous service, nodes need an alternative cross-LAN discovery mechanism over a stateless Secure Web API (HTTPS).

This track implements:
1. **Always-On Intra-LAN Discovery**: Local UDP broadcast beacon discovery (`DiscoverLANBootstrap` and `StartBeaconBroadcaster`) and local Hyperswarm TCP/UDX listening remain active regardless of the cross-LAN mode.
2. **Configurable Cross-LAN Discovery Modes (`cross_lan_mode`)**: Support `"dht"` (existing HyperDHT bootstrap over IP), `"https"` (Secure HTTPS Peer Discovery Registry), `"both"` (simultaneous DHT + HTTPS discovery), and `"none"` (intra-LAN only).
3. **HTTPS Swarm Discovery Client (`sqlite-p2p` side)**: A pure Go stateless REST client and background announcer/poller that announces local peer endpoints and discovers remote peers per 32-byte topic over HTTPS, authenticates via Bearer Token + Ed25519 request signatures (`secretstream.KeyPair`), checks `ReplicationPolicy`, and establishes peer connections via `SwarmManager.ConnectDirect(ip, port)`.
4. **OpenAPI 3.1 Specification (`docs/https-swarm-openapi.yaml`)**: Formal API contract for the Secure HTTPS Swarm Discovery Registry.
5. **Coding Agent Specification (`docs/https-swarm-agent-spec.md`)**: Detailed architectural, cryptographic, storage, and behavioral specification enabling an autonomous coding agent to implement the server-side HTTPS Discovery Registry.

---

## 2. Functional Requirements

### 2.1 Intra-LAN vs. Cross-LAN Discovery Separation
- **Intra-LAN Discovery (Always Enabled)**:
  - Whenever `SwarmTopic` is non-zero, `Engine` continues to run local UDP broadcast beacon discovery (`DiscoverLANBootstrap`) on startup and periodic LAN UDP broadcasting (`StartBeaconBroadcaster`) on `DefaultBeaconPort` (49736).
- **Cross-LAN Mode Configuration (`cross_lan_mode`)**:
  - Configurable in `pkg/config.NodeConfig` (`cross_lan_mode`) and `pkg/p2p.EngineOptions` (`CrossLANMode`):
    - `"dht"`: Uses `Bootstrap` addresses with HyperDHT (`cmd/dht-seed`) for cross-LAN discovery. (Default when `cross_lan_mode` is omitted and `https_api` is not enabled, preserving 100% backward compatibility).
    - `"https"`: Disables external WAN DHT bootstrap queries (passes only LAN-discovered bootstrap peers to Hyperswarm) and uses the Secure HTTPS Discovery Client to announce and poll peers across LANs.
    - `"both"`: Uses both HyperDHT `Bootstrap` nodes and the Secure HTTPS Discovery Client concurrently.
    - `"none"`: Disables both external DHT bootstrap and HTTPS discovery; only intra-LAN UDP beacon discovery and static `peer_addrs` are used.

### 2.2 HTTPS Configuration (`HTTPSAPIConfig`)
- Added to `pkg/config.NodeConfig` under `https_api` (JSON/YAML) and `pkg/p2p.EngineOptions`:
  - `url` (`string`): Base URL of the HTTPS discovery API (e.g., `https://swarm-api.example.com`).
  - `api_key` (`string`): Bearer token for gateway authentication (`Authorization: Bearer <api_key>`).
  - `poll_interval_sec` (`int`, default `15`): Interval in seconds for periodic re-announcement (heartbeat) and peer list polling.
  - `timeout_sec` (`int`, default `10`): Per-request HTTP timeout in seconds.
  - `advertise_addr` (`string`, optional): Explicit public/routable `host:port` or IP to announce to remote LANs; if omitted, auto-resolves from `SwarmManager` routable IP and listening port.
  - `tls_skip_verify` (`bool`, optional): Allow self-signed certificates in testing/staging environments.

### 2.3 Authentication & Cryptographic Peer Identity
- Every request from `sqlite-p2p` to the HTTPS API includes:
  - `Authorization: Bearer <api_key>` (when `api_key` is configured).
  - `X-Peer-Public-Key`: 64-character hex-encoded Ed25519 public key from the node's `secretstream.KeyPair`.
  - `X-Peer-Timestamp`: Unix epoch timestamp in seconds (UTC), used for replay protection (server enforces ±300s window).
  - `X-Peer-Signature`: 128-character hex-encoded Ed25519 signature over the canonical string:
    `METHOD + "\n" + REQUEST_PATH_AND_QUERY + "\n" + TIMESTAMP + "\n" + HEX(SHA256(BODY))`

### 2.4 Stateless REST Discovery Protocol & Client Lifecycle
- **Endpoints Emulating `dht-seed`**:
  1. `POST /v1/topics/{topic}/announce`: Announces/refreshes the node's presence on a 64-char hex topic (`peer_pk`, `node_id`, `endpoints` `["ip:port"]`, `ttl_sec`, `policy_mode`). Returns the current list of active peers on that topic so announce + lookup can complete in a single round-trip.
  2. `GET /v1/topics/{topic}/peers`: Queries active peers registered for the 64-char hex topic (supports optional `?exclude_pk=<hex>` and `?limit=<n>`).
  3. `DELETE /v1/topics/{topic}/peers/{peer_pk}`: Unannounces the peer from the topic on clean shutdown (`Close()` / `Leave()`).
  4. `GET /v1/health`: Health and clock-sync check endpoint.
- **Discovery & Direct Connection Flow**:
  - On `Engine` startup (when mode is `"https"` or `"both"`), the HTTPS discovery client starts a background worker for the active topic.
  - Immediately announces the local node and processes the returned peer list, then repeats every `poll_interval_sec`.
  - For each discovered remote peer:
    - Skips self (`peer_pk == local_pk`).
    - Checks `ReplicationPolicy.IsAllowed(peerPK)` before dialing.
    - Dials each candidate endpoint via `SwarmManager.ConnectDirect(host, port)` (which performs the Noise/Ed25519 `secretstream` handshake and attaches `Replicator`).
  - On `Engine.Close()`, sends an unannounce (`DELETE`) request with a short bounded context and stops the background worker.

### 2.5 OpenAPI & Coding Agent Specifications
- **`docs/https-swarm-openapi.yaml`**: Complete OpenAPI 3.1.0 specification defining all paths, request/response schemas, security schemes (`BearerAuth` + Ed25519 signature headers), error envelopes, and status codes.
- **`docs/https-swarm-agent-spec.md`**: Self-contained Coding Agent specification detailing architecture, Ed25519 signature verification algorithm with test vectors, in-memory/SQLite/Redis TTL peer registry data model, IP detection/override rules, rate-limiting, replication gating compatibility, and step-by-step acceptance tests for building the server.

---

## 3. Non-Functional Requirements
- **Zero-CGO & Standard Library HTTP**: Use Go's standard `net/http` and `crypto/ed25519` / `crypto/sha256` with zero external HTTP dependencies.
- **Backward Compatibility**: Existing configurations without `cross_lan_mode` or `https_api` must behave identically to current behavior.
- **Resilience**: Transient HTTPS API errors or timeouts must be logged via structured `slog.Logger` without crashing the engine or interrupting intra-LAN replication.
- **Test Coverage**: >80% unit and integration test coverage using `httptest.Server` to simulate cross-LAN peer discovery and end-to-end SQLite replication between two `Engine` instances.

---

## 4. Acceptance Criteria
1. Intra-LAN UDP beacon discovery (`DiscoverLANBootstrap` / `StartBeaconBroadcaster`) remains active in all `cross_lan_mode` settings (`"dht"`, `"https"`, `"both"`, `"none"`).
2. `NodeConfig` and `EngineOptions` parse and validate `cross_lan_mode` and `https_api` settings.
3. The HTTPS Discovery Client signs requests with Ed25519 headers + Bearer token, announces local endpoints, polls remote peers, filters via `ReplicationPolicy`, and connects peers via `SwarmManager.ConnectDirect`.
4. End-to-end test verifies two `Engine` nodes with no DHT bootstrap (`cross_lan_mode: "https"`) discover each other via a mock HTTPS Registry server and replicate SQLite records.
5. `docs/https-swarm-openapi.yaml` and `docs/https-swarm-agent-spec.md` are created and use clean workspace-relative paths.

---

## 5. Out of Scope
- Deploying or hosting the remote production HTTPS server infrastructure (this track implements the `sqlite-p2p` client/integration, test server harness, OpenAPI spec, and Coding Agent spec for the server project).
- Relaying raw SQLite changeset payloads through the HTTPS API (peers connect directly over TCP/Noise after HTTPS discovery).
