# Specification: MCP Server for Distributed CRM (`mcp_server`)

## 1. Overview
Implement a Model Context Protocol (MCP) server for the Distributed Multimodal Agentic CRM following the `mcp-collection` design pattern using `github.com/modelcontextprotocol/go-sdk`. The MCP server exposes CRM tools to autonomous AI agents over standard MCP transports (stdio and streamable HTTP/SSE), integrates into the existing handcrafted CLI as `crm-peer mcp`, and operates directly against the local SQLite CQRS store, crypto registry, ontology graph, and Pear P2P swarm.

## 2. Functional Requirements
### 2.1 Server & Transports
- **CLI Subcommand**: Add `mcp` subcommand to `crm-peer` (`crm-peer mcp [--transport=stdio|sse] [--host=127.0.0.1] [--port=8083] [--path=/mcp]`).
- **Dual Transport Support**:
  - `stdio` (default): Standard I/O JSON-RPC transport for local agent execution (Antigravity, Claude Desktop, Cursor).
  - `sse`: Streamable HTTP server for remote/networked agent access.
- **Stdio Stream Isolation**: All diagnostics and structured logs (`internal/logger`) MUST write to `stderr` or log file so `stdout` remains strictly reserved for MCP JSON-RPC framing.

### 2.2 Customer Management Tools
- `crm_customer_put`: Insert or update customer using phone number (Base32 SHA256 hashed), JSON metadata, and optional payload (encrypted via AES-256-GCM in `crm_store` with key stored in `key_registry`).
- `crm_customer_get`: Fetch customer by phone or hashed key, returning decrypted payload and metadata.
- `crm_customer_delete`: Delete customer record from `crm_store`.
- `crm_customer_shred`: Cryptographically shred customer encryption key from `key_registry`, rendering payload permanently unrecoverable while preserving causal tombstone.
- `crm_customer_list`: List customer records with pagination/limit.

### 2.3 Schema Registry Tools
- Schema storage directly in `crm_store` using key prefix `org.schema:<URI>`.
- `metadata`: JSON Schema object validating record attributes.
- `data`: Markdown (`.md`) documentation describing the schema, fields, and operational use-cases.
- Tools:
  - `crm_schema_set`: Register or update schema with `$id/uri`, JSON schema object, and markdown doc.
  - `crm_schema_get`: Retrieve schema definition, JSON schema, and markdown doc by URI.
  - `crm_schema_list`: List all registered schema definitions under `org.schema:*`.
  - `crm_schema_delete`: Delete schema definition from store.

### 2.4 Knowledge Graph & Ontology Tools
- `crm_ontology_get_node`: Fetch entity node metadata, label, and attributes.
- `crm_ontology_query_edges`: Query inbound/outbound relationships and edges for an entity.
- `crm_ontology_search`: Search entity nodes by type or keyword.

### 2.5 P2P Mesh & Swarm Observability Tools
- `crm_p2p_status`: Node status, DB path, WAL mode, swarm topic, and connected peer count.
- `crm_p2p_peers`: List connected peers.
- `crm_p2p_sync`: Trigger Autobase reconciliation sweep across connected peers.

## 3. Non-Functional Requirements & Architecture
- **Pure Go & Zero-CGO**: Maintain `CGO_ENABLED=0` compatibility using `modernc.org/sqlite`.
- **SDK**: `github.com/modelcontextprotocol/go-sdk/mcp`.
- **Registry Compliance**: Seamlessly register with the handcrafted CLI command tree without third-party CLI packages.
- **Test Coverage**: Comprehensive unit tests covering all MCP tool handlers, schema operations, and transport lifecycles with >80% code coverage.

## 4. Acceptance Criteria
- [ ] `crm-peer mcp` boots successfully in `stdio` mode and handles JSON-RPC initialization and tool discovery.
- [ ] `crm-peer mcp --transport=sse` boots HTTP/SSE server and handles tool invocations.
- [ ] All customer tools (put, get, delete, shred, list) pass functional verification.
- [ ] Schema registry tools store, retrieve, list, and delete schema records with `org.schema:<URI>`, JSON schema metadata, and markdown data blobs.
- [ ] Ontology and P2P tools return accurate graph and swarm state.
- [ ] Unit tests for new MCP package pass with >80% coverage.

## 5. Out of Scope
- External OAuth2 / authentication layers on SSE endpoints.
- Front-end browser UI for MCP visualization.
