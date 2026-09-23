# Initial Concept

Distributed Multimodal Agentic CRM (SQLite + Pure Go Pear/P2P protocol stack) scaling to 100K+ customer records, operating over a peer-to-peer network using a pure Go implementation of the Pear/Holepunch protocol stack (Hypercore, Hyperswarm, HyperDHT, Autobase) with local SQLite and CQRS architecture based on `sqlite_pear_p2p_crm_specification.md`.

# Product Guide: Distributed Multimodal Agentic CRM (SQLite + Pure Go Pear/P2P)

## 1. Vision & Purpose
A distributed, multi-writer, agentic Customer Relationship Management (CRM) platform scaling to 100K+ customer records without central cloud infrastructure or centralized message brokers. The system operates over a peer-to-peer (P2P) mesh using a pure Go implementation of the Pear/Holepunch protocol stack (Hypercore, Hyperswarm, HyperDHT, Autobase) combined with embedded SQLite and CQRS architecture.

## 2. Core Value Propositions
- **Decentralized & Multi-Writer**: Peer-to-peer data replication across human and autonomous agent nodes with vector clock ordering and deterministic state convergence.
- **Privacy-First & Cryptographic Shredding**: Deterministic pseudonymization of customer primary keys (`BASE32(Hash(Mobile))`), AES-256-GCM encrypted payloads, and GDPR-compliant crypto-shredding on key deletion.
- **Zero-CGO & Lightweight Embedded Footprint**: Pure Go single-binary execution with embedded SQLite WAL mode and handcrafted command architecture.
- **Hybrid Data Model & Multimodal Ontology**: Single-table key-value storage (`crm_store`) paired with Markdown/YAML frontmatter watcher indexing entity relationships into localized graph structures.

## 3. Target Users & Stakeholders
- **Autonomous AI Agents**: Multi-agent nodes executing automated customer actions, tracking changesets, and syncing state concurrently.
- **Human CRM Operators & Support Teams**: Field agents and support staff operating offline-first or in distributed P2P meshes without central cloud dependencies.
- **Enterprise System Administrators**: Teams requiring auditable, sovereign, and privacy-compliant data custody with zero external cloud SaaS lock-in.

## 4. Key Features & Capabilities
- **Local-First SQLite CQRS**: Client writes execute locally against `crm_store` table in WAL mode; binary changesets are captured via SQLite Session API and replicated via Autobase.
- **P2P Mesh Swarm**: Autonomous discovery and replication across nodes using pure Go Hyperswarm over a shared 32-byte CRM cluster topic key.
- **Causal Linearization & LWW Conflict Resolution**: Pure Go Autobase linearizes concurrent writes across peers, resolving conflicts using sequence IDs and compound metadata schema evaluation.
- **Markdown & Graph Ontology Engine**: Directory watcher parsing YAML frontmatter into localized ontology nodes and edges for contextual graph traversal.
- **Structured JSON Logging**: Zero-dependency structured JSON logging following standardized enterprise logging schemas.
- **Model Context Protocol (MCP) Server**: Native MCP runtime (`crm-peer mcp`) exposing customer management, schema registry (`org.schema:*`), ontology traversal, and P2P mesh observability tools over `stdio` and streamable HTTP/SSE.
