# Specification: Split Project into 3 Decoupled Repositories (`split_three_projects_20260924`)

## 1. Overview
This specification details the structural and architectural separation of the current monolithic codebase (`crm-sqlite-pear-p2p` / `sqlite-p2p`) into three independent, modular projects:
1. **`sqlite-p2p`**: Core decentralized database engine providing Pure Go SQLite and P2P changeset replication over the Pear/Holepunch stack (Autobase, Hypercore, Hyperswarm).
2. **`crm-lite`**: Decentralized multimodal CRM application built on top of `sqlite-p2p`, incorporating customer encryption/shredding, schema registry, Markdown ontology graph engine, and the Model Context Protocol (MCP) server with handcrafted CLI.
3. **`whatsadk-litep2p-storage`**: WhatsaDK storage backend implementing WhatsaDK's `storeBackend` interface, merged directly into the `whatsadk` workspace at `/home/innomon/B204-zone/orez/adk/whatsadk`.

---

## 2. Project Boundaries & Responsibilities

### 2.1 Project 1: `sqlite-p2p` (Core Engine)
- **Scope**: Domain-agnostic, decentralized SQLite database engine.
- **Repository Location**: `/home/innomon/B204-zone/owly-sewa/sqlite-p2p` (renamed from `crm-sqlite-pear-p2p`).
- **Module Name**: `sqlite-p2p`
- **Responsibilities**:
  - Pure Go SQLite runtime (`modernc.org/sqlite` with zero CGO) configured with WAL mode, foreign keys, and normal synchronization.
  - Generic key-value table storage (`p2p_store`) with schema `(key TEXT PRIMARY KEY, metadata TEXT CHECK(json_valid(metadata)), data BLOB) WITHOUT ROWID`.
  - Mutation capture and binary changeset serialization via SQLite Session API / Changeset Tracker.
  - Pure Go Autobase consensus, append-only Hypercore feeds, and 32-byte Hyperswarm P2P replication mesh.
  - Per-record symmetric payload encryption (AES-256-GCM) and cryptographic key registry / shredding.
  - Exported public package [`pkg/p2p`](/pkg/p2p) providing programmatic APIs (`OpenDB`, `NewEngine`, `Repository`, `ChangesetTracker`).
- **Zero-Dependency Guardrail**: Zero CRM domain code, zero ontology tables, and zero WhatsApp-specific code.

### 2.2 Project 2: `crm-lite` (CRM Application)
- **Scope**: Multimodal, agentic CRM application.
- **Repository Location**: `/home/innomon/B204-zone/owly-sewa/crm-lite`
- **Module Name**: `crm-lite`
- **Responsibilities**:
  - Deterministic customer key generation (`BASE32(SHA256(phone))`).
  - Customer record CRUD with automatic per-customer key management and GDPR crypto-shredding.
  - JSON Schema Registry under the `org.schema:*` namespace for contract validation.
  - Multimodal Markdown frontmatter parser and filesystem watcher (`fsnotify`) creating localized knowledge graph nodes and edges (`ontology_nodes`, `ontology_edges`) with recursive CTE traversals.
  - Built-in Model Context Protocol (MCP) server supporting `stdio` and streamable HTTP (`sse`) transports.
  - Handcrafted CLI (`crm-peer`) with zero third-party CLI dependencies (no `cobra` or `pflag`).
- **Dependency**: Imports `sqlite-p2p`.

### 2.3 Project 3: `whatsadk-litep2p-storage` (Merged into WhatsaDK)
- **Scope**: Embedded peer-to-peer storage backend for WhatsaDK.
- **Target Location**: Merged directly into `/home/innomon/B204-zone/orez/adk/whatsadk`.
- **Responsibilities**:
  - Complete implementation of WhatsaDK's `storeBackend` interface (`pkg/whatsadk` code).
  - Projection views over `p2p_store`: `filesys`, `whatsmeow_contacts`, `whatsmeow_commands`, `blacklisted_numbers`.
  - High-performance binary media storage (JPEG, PNG, WebP, MP4, OGG, WAV) with zero-bloat chat querying.
  - Full P2P replication of WhatsaDK records across peer nodes.
  - Seamless integration into `whatsadk/internal/store/store.go` via DSN detection (`sqlite-p2p://`, `pear://`, `sqlite://`).
- **Dependency**: Imports `sqlite-p2p`.

---

## 3. Phased Implementation Roadmap
- **Phase 1**: Refine `sqlite-p2p` core engine and provide clean exported public API (`pkg/p2p`).
- **Phase 2**: Scaffold and extract `crm-lite` into `/home/innomon/B204-zone/owly-sewa/crm-lite` and verify CLI, MCP, and tests.
- **Phase 3**: Merge `whatsadk-litep2p-storage` into `/home/innomon/B204-zone/orez/adk/whatsadk` and verify WhatsaDK test suite.
- **Phase 4**: Decouple remaining domain files from `sqlite-p2p`, perform repository renaming, and update documentation across all repositories.

---

## 4. Acceptance Criteria
- [ ] `sqlite-p2p` compiles as an independent module with zero CRM/WhatsaDK dependencies and >80% test coverage.
- [ ] `crm-lite` compiles as an independent project, imports `sqlite-p2p`, runs all CRM CLI subcommands and MCP tools, and achieves >80% test coverage.
- [ ] WhatsaDK integrates the SQLite P2P storage backend, passes its test suite with `sqlite-p2p://:memory:`, and supports multi-node replication.
- [ ] All three projects are cleanly decoupled, version-controlled, and independently buildable.
