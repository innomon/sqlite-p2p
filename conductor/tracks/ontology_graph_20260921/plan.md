# Implementation Plan: Markdown Interaction Watcher & SQLite Ontology Graph Traversal

## Phase 1: SQLite Ontology Schema & Recursive CTE Graph Traversal
- [ ] Task: Create ontology schema tables and Repository methods
    - [ ] Write Tests: Unit tests for inserting, updating, and querying `ontology_nodes` and `ontology_edges`
    - [ ] Implement: Tables migration and CRUD queries in `internal/store/ontology.go`
- [ ] Task: Implement Recursive CTE graph queries
    - [ ] Write Tests: Unit tests verifying multi-hop pathfinding, neighbor lookups, and cycle handling using SQLite recursive CTEs
    - [ ] Implement: Graph traversal methods in `internal/store/ontology.go`
- [ ] Task: Conductor - User Manual Verification 'Phase 1: SQLite Ontology Schema & Recursive CTE Graph Traversal' (Protocol in workflow.md)

---

## Phase 2: YAML Frontmatter Parser & Filesystem Watcher
- [ ] Task: Implement YAML frontmatter extractor and schema parser
    - [ ] Write Tests: Unit tests parsing Markdown files with ontology-graph schemas into typed nodes and edges
    - [ ] Implement: `internal/ontology/parser.go`
- [ ] Task: Implement background filesystem watcher
    - [ ] Write Tests: Unit tests verifying `fsnotify` file creation and modification triggers automatic graph upsert
    - [ ] Implement: `internal/ontology/watcher.go`
- [ ] Task: Conductor - User Manual Verification 'Phase 2: YAML Frontmatter Parser & Filesystem Watcher' (Protocol in workflow.md)

---

## Phase 3: CLI Query Subcommands & End-to-End Verification
- [ ] Task: Implement CLI `query` command group
    - [ ] Write Tests: CLI tests for `crm-peer query graph` and `crm-peer query nodes`
    - [ ] Implement: Subcommand handlers in `internal/cli/commands.go`
- [ ] Task: End-to-end Markdown ingestion and graph traversal verification
    - [ ] Write Tests: Integration test creating Markdown logs, observing background ingestion, and querying traversals via CLI
    - [ ] Implement: Test suite in `tests/ontology_graph_test.go`
- [ ] Task: Conductor - User Manual Verification 'Phase 3: CLI Query Subcommands & End-to-End Verification' (Protocol in workflow.md)
