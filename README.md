# Distributed Multimodal Agentic CRM (SQLite + Pure Go Pear/P2P)

A distributed, multi-writer, agentic Customer Relationship Management (CRM) platform scaling to 100K+ customer records without central cloud infrastructure or centralized message brokers. The system operates over a peer-to-peer (P2P) mesh using a pure Go implementation of the Pear/Holepunch protocol stack (Hypercore, Hyperswarm, HyperDHT, Autobase) combined with embedded SQLite and CQRS architecture.

## Model Context Protocol (MCP) Server

The CRM exposes Model Context Protocol (MCP) server tooling following the monorepo pattern from `mcp-collection` using the official Go MCP SDK (`github.com/modelcontextprotocol/go-sdk`).

### Running the MCP Server

The MCP server runs as a subcommand under `crm-peer`:

```bash
# Stdio transport (default, recommended for Claude Desktop, Antigravity, Cursor)
./bin/crm-peer mcp

# Streamable HTTP / Server-Sent Events (SSE) transport
./bin/crm-peer mcp --transport=sse --host=127.0.0.1 --port=8083 --path=/mcp
```

### Environment Variables

| Variable | Description | Default |
|---|---|---|
| `CRM_MCP_TRANSPORT` | Transport mechanism (`stdio` or `sse`) | `stdio` |
| `CRM_MCP_SSE_HOST` | Host address for SSE server | `127.0.0.1` |
| `CRM_MCP_SSE_PORT` | Port for SSE server | `8083` |
| `CRM_MCP_SSE_PATH` | HTTP endpoint path | `/mcp` |

### MCP Tools Reference

#### 1. Schema Registry Tools (`org.schema:*`)
- `crm_schema_set`: Register or update schema with `$id/uri`, JSON Schema object, and markdown doc.
- `crm_schema_get`: Retrieve schema definition, JSON Schema, and markdown documentation by URI.
- `crm_schema_list`: List registered schemas matching namespace `org.schema:*`.
- `crm_schema_delete`: Remove a schema definition.

#### 2. Customer Management & Privacy Tools
- `crm_customer_put`: Insert or update customer with phone/key, JSON metadata, and optional AES-256-GCM encrypted payload. Captures binary changesets.
- `crm_customer_get`: Retrieve customer by phone or Base32 hashed key, returning decrypted payload and metadata.
- `crm_customer_delete`: Delete customer record.
- `crm_customer_shred`: Cryptographically shred customer's encryption key from `key_registry`, permanently forgetting data while preserving causal tombstone.
- `crm_customer_list`: Query customer keys/metadata with limit/offset.

#### 3. Knowledge Graph & Ontology Tools
- `crm_ontology_get_node`: Inspect ontology entity node by ID, extracting attributes and type.
- `crm_ontology_query_edges`: Query outgoing relationships and edges connected to an entity.
- `crm_ontology_search`: Search indexed frontmatter entities and interactions.

#### 4. P2P Swarm Observability Tools
- `crm_p2p_status`: Node ID, swarm topic key, WAL status, connection count.
- `crm_p2p_peers`: List active connected peers in the mesh.
- `crm_p2p_sync`: Trigger immediate Autobase reconciliation sweep across connected peers.
