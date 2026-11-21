# Technology Stack: Distributed Multimodal Agentic CRM

## 1. Programming Language & Runtime
- **Language**: Go (1.23+)
- **Runtime Model**: Pure Go single binary with zero CGO dependency (`CGO_ENABLED=0`).
- **Idiomatic Standard**: Canonical Go idioms, standard library first, strict workspace-root relative paths.

## 2. Storage & Database Architecture
- **Primary Embedded Store**: SQLite compiled in Pure Go (`modernc.org/sqlite`) running in WAL mode (`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA foreign_keys=ON;`).
- **Schema & Query Model**: Universal multi-table storage supporting both fast-path KV tables (`crm_store`, `ecom_store` with `key TEXT PRIMARY KEY, metadata TEXT CHECK(json_valid(metadata)), data BLOB WITHOUT ROWID`) and arbitrary relational SQLite tables (single PK, composite PK, or `rowid`).
- **Change Data Capture**: Pure Go SQLite trigger-based CDC outbox (`_p2p_cdc_outbox`) and transactional remote-apply guard (`_p2p_sync_state`), paired with schema auto-discovery (`sqlite_master` + `PRAGMA table_info`) and table-scoped LWW conflict resolution.
- **Vector Search Extension (`pkg/vect`)**: Pure Go deterministic SQLite scalar extension registered on `modernc.org/sqlite` providing `vec_cosine_similarity`, `vec_cosine_distance`, `vec_l2_distance`, `vec_length`, and `vec_pack_f32` over packed Little-Endian `float32` BLOBs alongside `Repository.PutVector`, `GetVector`, and `SearchSimilar` K-NN helpers.

## 3. Decentralized P2P Protocol Stack (Pear / Holepunch)
- **Local Pure Go Pear Engine**: Referenced from `$PROJECT_ROOT/go-pear`.
  - **Hypercore**: Append-only cryptographic data feeds with Merkle tree verification.
  - **Hyperswarm & HyperDHT**: Distributed peer discovery, holepunching, and P2P connection multiplexing over 32-byte swarm topics.
  - **Configurable Cross-LAN Discovery & Secure HTTPS Swarm Registry**: Always-on intra-LAN UDP broadcast beacon discovery (`DefaultBeaconPort = 49736`) paired with configurable cross-LAN discovery (`cross_lan_mode`: `dht`, `https`, `both`, `none`) supporting both HyperDHT seed nodes (`cmd/dht-seed`) and a stateless Secure HTTPS Peer Discovery Registry API authenticated with Bearer tokens and Ed25519 request signatures (`X-Peer-Public-Key`, `X-Peer-Timestamp`, `X-Peer-Signature`).
  - **Autobase**: Multi-writer consensus, vector clock tracking, and causal linearization of peer changeset streams.
  - **Replication Gating & Access Control (`go-pear/pkg/policy`)**: Thread-safe dynamic peer authorization engine supporting `all`, `whitelist`, and `blacklist` modes with real-time peer eviction and JSON configuration file persistence.

## 4. CLI & Configuration Framework
- **Command Architecture**: Handcrafted command/subcommand dispatch registry (strictly no `spf13/cobra` or `pflag`).
- **SQL Query & Formatting Engine**: Native `internal/sqlexec` module providing multi-statement transactional execution, query vs mutation detection, and zero-dependency JSON, ASCII Table, and RFC 4180 CSV formatting.
- **Configuration**: YAML format (`gopkg.in/yaml.v3`) with 3-tier lookup hierarchy: (1) CLI argument `--config`, (2) executable directory `config.yaml`, (3) current working directory `config.yaml`.

## 5. Security, Privacy & Crypto
- **Identifier Hashing**: `crypto/sha256` with Base32 encoding (`BASE32(SHA256(Mobile))`).
- **Payload Encryption**: `crypto/aes` with `AES-256-GCM` authenticated encryption for per-customer payload privacy.
- **Crypto-Shredding**: Ephemeral key registry allowing permanent data forgetting on key deletion.

## 6. Logging & Observability
- **Structured JSON Logging**: Zero-dependency structured JSON output matching `structured_logging_pattern.md` standards using Go standard library (`log/slog` or custom JSON formatting).

## 7. Multimodal & File System Ingestion
- **File System Watcher**: `github.com/fsnotify/fsnotify` for monitoring markdown interaction logs.
- **Frontmatter Parser**: YAML frontmatter extraction into SQLite graph entities (`ontology_nodes`, `ontology_edges`).

## 8. Model Context Protocol (MCP) Integration
- **MCP Framework**: Official Go MCP SDK (`github.com/modelcontextprotocol/go-sdk/mcp`).
- **Transports**: Dual transport support with standard I/O (`stdio`) as default for subprocess AI agents, and streamable HTTP (`sse`) for network access.
- **Tool Contracts**: Auto-generated JSON Schema parameter bindings for customer CRUD, record schema registry (`org.schema:<URI>`), ontology graph exploration, and P2P swarm metrics.
