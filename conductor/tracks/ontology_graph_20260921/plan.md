# Implementation Plan: Markdown Interaction Watcher & SQLite Ontology Graph Traversal

## Phase 1: SQLite Ontology Schema & Recursive CTE Graph Traversal [checkpoint: 1d89bff]
- [x] Task: Create ontology schema tables and Repository methods [b18c32a]
    - [x] Write Tests: Unit tests for inserting, updating, and querying `ontology_nodes` and `ontology_edges`
    - [x] Implement: Tables migration and CRUD queries in `internal/store/ontology.go`
- [x] Task: Implement Recursive CTE graph queries [5a46194]
    - [x] Write Tests: Unit tests verifying multi-hop pathfinding, neighbor lookups, and cycle handling using SQLite recursive CTEs
    - [x] Implement: Graph traversal methods in `internal/store/ontology.go`
- [x] Task: Conductor - User Manual Verification 'Phase 1: SQLite Ontology Schema & Recursive CTE Graph Traversal' (Protocol in workflow.md)

---

## Phase 2: YAML Frontmatter Parser & Filesystem Watcher [checkpoint: b881489]
- [x] Task: Implement YAML frontmatter extractor and schema parser [42783db]
    - [x] Write Tests: Unit tests parsing Markdown files with ontology-graph schemas into typed nodes and edges
    - [x] Implement: `internal/ontology/parser.go`
- [x] Task: Implement background filesystem watcher [bc0175e]
    - [x] Write Tests: Unit tests verifying `fsnotify` file creation and modification triggers automatic graph upsert
    - [x] Implement: `internal/ontology/watcher.go`
- [x] Task: Conductor - User Manual Verification 'Phase 2: YAML Frontmatter Parser & Filesystem Watcher' (Protocol in workflow.md)

---

## Phase 3: CLI Query Subcommands & End-to-End Verification
- [x] Task: Implement CLI `query` command group [87f4588]
    - [x] Write Tests: CLI tests for `crm-peer query graph` and `crm-peer query nodes`
    - [x] Implement: Subcommand handlers in `internal/cli/commands.go`
- [x] Task: End-to-end Markdown ingestion and graph traversal verification [a77c634]
    - [x] Write Tests: Integration test creating Markdown logs, observing background ingestion, and querying traversals via CLI
    - [x] Implement: Test suite in `tests/ontology_graph_test.go`
- [ ] Task: Conductor - User Manual Verification 'Phase 3: CLI Query Subcommands & End-to-End Verification' (Protocol in workflow.md)
