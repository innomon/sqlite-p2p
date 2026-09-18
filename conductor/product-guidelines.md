# Product Guidelines: Distributed Multimodal Agentic CRM

## 1. Architectural & Design Philosophy
- **Decentralized First**: No single point of failure; zero reliance on central servers, cloud APIs, or brokers like NATS.
- **Offline & Local First**: All operations read and write locally to embedded SQLite first (`WAL` mode). Network propagation happens asynchronously and opportunistically over peer-to-peer Hyperswarm.
- **Zero-CGO & Minimal Dependencies**: Build with pure Go standard library and pure Go Pear protocols to maintain cross-compilation and containerless deployment simplicity.
- **Handcrafted Command Architecture**: CLI interactions use a handcrafted command and subcommand registry without heavy third-party CLI frameworks (strictly no `cobra` / `pflag`).

## 2. Privacy, Security & Data Custody
- **Zero Cleartext PII**: Customer primary identifiers (phone numbers, Aadhaar, PAN) must NEVER be persisted or indexed in cleartext. Deterministic pseudonymization (`BASE32(SHA256(Primary Phone))`) must be used as primary keys.
- **Crypto-Shredding by Design**: Encrypt entity payloads using per-record symmetric keys (`AES-256-GCM`). Deletion/forgetting of records purges the key, permanently rendering append-only log blocks indecipherable.
- **Auditable Append-Only Logs**: State mutations are recorded as cryptographic changesets in local Hypercore logs, verified with Merkle tree hash integrity.

## 3. Communication, CLI & UX Standards
- **CLI Simplicity & Predictability**: Single binary distribution (`crm-peer`) with intuitive subcommands (`start`, `status`, `query`, `peer`, `keygen`).
- **Configuration Hierarchy**: Check configuration in the following order: (1) CLI argument `--config <path>`, (2) executable directory `./config.yaml`, (3) current working directory `config.yaml`.
- **Structured JSON Logging**: All logs must follow the structured logging pattern (`structured_logging_pattern.md`) outputting machine-parseable JSON lines with standardized fields (`timestamp`, `level`, `service`, `event`, `trace_id`).

## 4. Multimodal & Ontology Guidelines
- **Human-Readable Context**: Markdown files with YAML frontmatter act as the primary format for agent reasoning, interaction history, and contextual notes.
- **Graph Traversal via SQLite**: Metadata frontmatter is parsed into local relational graph structures (`ontology_nodes`, `ontology_edges`) with recursive Common Table Expressions (CTEs) for fast traversal.
