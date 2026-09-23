# Distributed Multimodal Agentic CRM (SQLite + Pure Go Pear/P2P)

A distributed, multi-writer, agentic Customer Relationship Management (CRM) platform scaling to 100K+ customer records without central cloud infrastructure or centralized message brokers. The system operates over a peer-to-peer (P2P) mesh using a pure Go implementation of the Pear/Holepunch protocol stack (Hypercore, Hyperswarm, HyperDHT, Autobase) combined with embedded SQLite and CQRS architecture.

- **Agent Integration Guide**: See [AGENT.md](/AGENT.md) for configuring LLM harnesses (Block Goose, Claude Desktop, Cursor, Antigravity).
- **Technical Architecture**: See [ARCHITECTURE.md](/ARCHITECTURE.md) for deep dives into CQRS, cryptographic shredding, Autobase consensus, and graph ontologies.

---

## Key Features

- **Pure Go & Zero-CGO**: Single self-contained binary built with `CGO_ENABLED=0` and `modernc.org/sqlite`. Runs anywhere without dynamic C libraries.
- **Decentralized Multi-Writer CQRS**: Local-first writes against SQLite in WAL mode; mutations are captured as binary changesets via SQLite Session API and linearized across peers using Autobase vector clocks.
- **Privacy-First & Cryptographic Shredding**: Customer primary keys are deterministically pseudonymized via `BASE32(SHA256(phone))`. Payloads are encrypted with per-customer `AES-256-GCM` keys. Key deletion from the key registry executes instant GDPR-compliant crypto-shredding while preserving causal tombstones.
- **Model Context Protocol (MCP) Server**: Native MCP runtime supporting both standard I/O (`stdio`) and streamable HTTP (`sse`) transports to power autonomous AI agent workflows.
- **Record Schema Registry**: Built-in contract registry under the `org.schema:<URI>` namespace storing JSON Schema validation objects and Markdown operational documentation.
- **Multimodal Markdown & Graph Engine**: Directory watcher indexing Markdown YAML frontmatter into localized SQLite graph entities with multi-hop Common Table Expression (CTE) traversals.
- **Handcrafted CLI**: Zero third-party CLI dependencies (no `cobra` or `pflag`), ensuring fast compilation and clean maintainability.

---

## Building Cross-Platform

Because the codebase is 100% Pure Go with zero CGO dependencies, cross-compiling for any supported operating system and architecture requires only setting the `GOOS` and `GOARCH` environment variables.

### Prerequisites

- Go 1.23 or newer (`go version`)

### Build Commands

```bash
# 1. Native Build (current platform)
go build -o bin/crm-peer cmd/crm-peer/main.go

# 2. Linux (x86_64 / amd64)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/crm-peer-linux-amd64 cmd/crm-peer/main.go

# 3. Linux (ARM64 / Raspberry Pi / Graviton)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/crm-peer-linux-arm64 cmd/crm-peer/main.go

# 4. macOS (Apple Silicon / M1 / M2 / M3 / M4)
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o bin/crm-peer-darwin-arm64 cmd/crm-peer/main.go

# 5. macOS (Intel / amd64)
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o bin/crm-peer-darwin-amd64 cmd/crm-peer/main.go

# 6. Windows (x86_64 / amd64)
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/crm-peer-windows-amd64.exe cmd/crm-peer/main.go
```

---

## Executing the CLI

The `crm-peer` binary exposes subcommands for node operations, customer management, key generation, graph querying, and MCP agent hosting:

```bash
# Display help and available commands
./bin/crm-peer --help

# Show node status, storage directory, and database configuration
./bin/crm-peer status

# Generate or inspect a customer's deterministic key and symmetric cipher
./bin/crm-peer keygen "+1-555-123-4567"

# Put a customer record (phone, metadata JSON, optional plaintext payload)
./bin/crm-peer customer put "+1-555-123-4567" '{"tier":"enterprise","schemas":["org.schema:crm.v1"]}' "Confidential Profile"

# Retrieve customer record (automatically decrypts payload)
./bin/crm-peer customer get "+1-555-123-4567"

# List customer records
./bin/crm-peer customer list

# Query ontology nodes and multi-hop graph edges
./bin/crm-peer query nodes customer
./bin/crm-peer query edges "customer:cust-101"
./bin/crm-peer query traverse "customer:cust-101" 3

# Inspect P2P swarm and peer connections
./bin/crm-peer peer

# Trigger immediate peer reconciliation sweep
./bin/crm-peer sync

# Start continuous P2P replication node
./bin/crm-peer start
```

---

## Model Context Protocol (MCP) Server

The CRM includes a built-in MCP server based on `github.com/modelcontextprotocol/go-sdk` following the design pattern from `/home/innomon/B204-zone/orez/mcp/mcp-collection`.

### Running the MCP Server

```bash
# 1. Standard I/O mode (default, for local LLM harnesses like Claude Desktop, Goose, Cursor)
./bin/crm-peer mcp

# 2. Streamable HTTP / Server-Sent Events (SSE) mode (for remote / network agents)
./bin/crm-peer mcp --transport=sse --host=127.0.0.1 --port=8083 --path=/mcp
```

### Environment Configuration

| Variable | Description | Default |
|---|---|---|
| `CRM_MCP_TRANSPORT` | Transport protocol (`stdio` or `sse`) | `stdio` |
| `CRM_MCP_SSE_HOST` | Host address for SSE server | `127.0.0.1` |
| `CRM_MCP_SSE_PORT` | Port for SSE HTTP listener | `8083` |
| `CRM_MCP_SSE_PATH` | Endpoint path for MCP SSE handler | `/mcp` |

### Available MCP Tools

| Tool Name | Category | Description |
|---|---|---|
| `crm_schema_set` | Schema Registry | Register or update schema definition with `$id/uri`, JSON Schema metadata, and Markdown doc |
| `crm_schema_get` | Schema Registry | Retrieve registered schema, JSON Schema contract, and Markdown doc by URI |
| `crm_schema_list` | Schema Registry | List all registered schema definitions under the `org.schema:*` namespace |
| `crm_schema_delete` | Schema Registry | Delete a schema contract from the store |
| `crm_customer_put` | Customer CRM | Put customer with phone/key, JSON metadata, and optional AES-256-GCM encrypted payload |
| `crm_customer_get` | Customer CRM | Retrieve customer record, metadata, and decrypted payload |
| `crm_customer_delete` | Customer CRM | Delete customer record from `crm_store` |
| `crm_customer_shred` | Privacy / GDPR | Permanently purge customer AES encryption key (crypto-shredding) |
| `crm_customer_list` | Customer CRM | List customer records with pagination |
| `crm_ontology_get_node` | Knowledge Graph | Retrieve entity node metadata, label, and attributes |
| `crm_ontology_query_edges` | Knowledge Graph | Query outgoing directed edges from an entity node |
| `crm_ontology_search` | Knowledge Graph | Search ontology nodes by entity type and text query |
| `crm_p2p_status` | Observability | Inspect node version, DB path, WAL mode, active topics, and peer count |
| `crm_p2p_peers` | Observability | List active connected peers in the Hyperswarm mesh |
| `crm_p2p_sync` | Synchronization | Trigger immediate Autobase peer reconciliation sweep |

For step-by-step agent harness setup instructions (Goose, Claude Desktop, Cursor), see [AGENT.md](/AGENT.md).

---

## WhatsaDK Embeddable Storage Backend (`pkg/whatsadk`)

The repository exports a drop-in storage backend package [`pkg/whatsadk`](/pkg/whatsadk) that implements the WhatsaDK `storeBackend` interface. This allows WhatsaDK instances to run completely serverless with local-first, peer-to-peer replicated SQLite storage:

- **Complete Interface Support**: Virtual filesys (`PutFile`, `GetFile`, `ListFiles`, `QueryFilesys`), contacts (`PutContact`, `ListContacts`), blacklist (`AddBlacklist`, `IsBlacklisted`), and command queue (`EnqueueCommand`, `PollPendingCommands`, `WaitForCommand`).
- **Binary Media & Zero-Bloat Projections**: Media (images, voice, video) is stored as SQLite BLOBs and automatically omitted during chat log scans (`GetFilesysLogs`) to ensure low latency. Content is retrieved on demand via `GetFile`.
- **Peer-to-Peer Replication**: Every mutation is intercepted by `store.ChangesetTracker` and replicated across distributed nodes via Autobase feeds.
- **Integration Guide**: See the detailed coding agent integration specification in [docs/whatsadk_integration_spec.md](/docs/whatsadk_integration_spec.md).

---

## Testing & Quality Gates

Run all automated unit and integration tests with coverage:

```bash
# Run test suite
go test -v ./...

# Verify code coverage (>80% required across all modules)
go test -cover ./...
```
