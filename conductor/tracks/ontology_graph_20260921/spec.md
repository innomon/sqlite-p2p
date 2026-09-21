# Specification: Markdown Interaction Watcher & SQLite Ontology Graph Traversal

## 1. Overview
Implement an asynchronous filesystem watcher and parser for human/agent Markdown interaction logs containing YAML frontmatter headers. The engine ingests relationship metadata and schema nodes/edges into relational SQLite graph tables (`ontology_nodes` and `ontology_edges`), enabling recursive Common Table Expression (CTE) graph traversals and CLI query tools.

## 2. Functional Requirements
- **FR-1: Relational Graph Schema & Traversal CTEs (`internal/store/ontology.go`)**:
  - Tables:
    ```sql
    CREATE TABLE IF NOT EXISTS ontology_nodes (
        id TEXT PRIMARY KEY,
        label TEXT NOT NULL,
        type TEXT NOT NULL,
        metadata TEXT
    );
    CREATE TABLE IF NOT EXISTS ontology_edges (
        source TEXT NOT NULL,
        target TEXT NOT NULL,
        relationship TEXT NOT NULL,
        weight REAL DEFAULT 1.0,
        metadata TEXT,
        PRIMARY KEY (source, target, relationship)
    );
    ```
  - Implement recursive CTE queries to traverse entity-agent networks, find paths between customers and agents, and query multi-hop relationships.
- **FR-2: YAML Frontmatter Parser (`internal/ontology/parser.go`)**:
  - Extract YAML frontmatter from `.md` files matching compound schema `https://qzip.in/schemas/crm/ontology-graph-v1.json`.
  - Parse `nodes` (`id`, `label`, `type`) and `edges` (`source`, `target`, `relationship`) into typed Go structs.
- **FR-3: Filesystem Watcher (`internal/ontology/watcher.go`)**:
  - Monitor configured `markdown_dir` (e.g. `records/`) using `github.com/fsnotify/fsnotify`.
  - Automatically parse newly created or updated `.md` files and upsert graph entities into SQLite in the background.
- **FR-4: CLI Query Subcommand (`internal/cli/commands.go`)**:
  - `crm-peer query graph <node_id> [--depth N]`: Display connected entities, relationships, and traversal paths.
  - `crm-peer query nodes [type]`: List registered ontology nodes by type.

## 3. Non-Functional Requirements & Constraints
- **Zero CGO**: Pure Go runtime (`CGO_ENABLED=0`).
- **Standard Formatting**: Clean workspace-root relative paths and structured logging on file watcher events.
- **Test Coverage**: ≥ 80% coverage on new packages.

## 4. Acceptance Criteria
1. Writing a Markdown file with YAML frontmatter into `records/` automatically populates `ontology_nodes` and `ontology_edges`.
2. Recursive CTE queries correctly traverse multi-hop relationships between agents and customers.
3. CLI `crm-peer query graph <id>` outputs graph paths and neighbor associations.
4. All tests pass with `CGO_ENABLED=0`.
