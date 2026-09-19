# Implementation Plan: Core SQLite crm_store Engine, Changeset Capture & CLI Foundation

## Phase 1: Foundation, Configuration & Structured Logging [checkpoint: 927d793]

- [x] Task: Initialize pure Go module and project directory structure (79ef80b)
    - [x] Write Tests: Test directory layout verification and build target validation
    - [x] Implement: Initialize `go.mod`, setup package folders (`cmd/crm-peer`, `internal/config`, `internal/logger`, `internal/store`, `internal/cli`)

- [x] Task: Implement 3-tier hierarchical YAML configuration loader (5ae663e)
    - [x] Write Tests: Unit tests for CLI flag override, binary directory lookup, cwd fallback, and default values
    - [x] Implement: Configuration structs and loader in `internal/config/config.go` with YAML parsing

- [x] Task: Implement zero-dependency structured JSON logger (ae1e27c)
    - [x] Write Tests: Unit tests verifying RFC3339Nano timestamps, standard field structure, log levels, and error serialization
    - [x] Implement: Logger package in `internal/logger/logger.go` adhering to `structured_logging_pattern.md`

- [x] Task: Conductor - User Manual Verification 'Phase 1: Foundation, Configuration & Structured Logging' (Protocol in workflow.md) (927d793)

---

## Phase 2: Handcrafted CLI Command Registry [checkpoint: 7b48377]

- [x] Task: Implement handcrafted command and subcommand router (064e2d1)
    - [x] Write Tests: Unit tests verifying command registration, path resolution, subcommand dispatch, and error handling
    - [x] Implement: Command registry in `internal/cli/registry.go` without third-party CLI dependencies

- [x] Task: Wire CLI application entry point and baseline subcommands (ed9ee4e)
    - [x] Write Tests: Unit tests for `help`, `status`, and version flag execution
    - [x] Implement: Main entry point in `cmd/crm-peer/main.go` and core handlers in `internal/cli/commands.go`

- [x] Task: Conductor - User Manual Verification 'Phase 2: Handcrafted CLI Command Registry' (Protocol in workflow.md) (7b48377)

---

## Phase 3: Pure Go SQLite Engine & `crm_store` Single-Table Storage [checkpoint: 1088015]

- [x] Task: Implement deterministic customer key generator (7cb41d3)
    - [x] Write Tests: Unit tests for phone number sanitization, SHA-256 hashing, Base32 encoding, and URI formatting
    - [x] Implement: Key generator in `internal/store/key.go` returning `in.qzip.crm.customer:<BASE32(SHA256(phone))>`

- [x] Task: Implement Pure Go SQLite database initialization with WAL pragmas (c60a296)
    - [x] Write Tests: Unit tests verifying database opening, WAL pragma enforcement, and `crm_store` WITHOUT ROWID table creation
    - [x] Implement: DB manager in `internal/store/db.go` using pure Go SQLite driver (`modernc.org/sqlite`)

- [x] Task: Implement CRUD operations with JSON metadata validation (b80fd9b)
    - [x] Write Tests: Unit tests for record insert, upsert, query by key, and invalid JSON metadata rejection
    - [x] Implement: Storage repository methods in `internal/store/repository.go`

- [x] Task: Conductor - User Manual Verification 'Phase 3: Pure Go SQLite Engine & crm_store Single-Table Storage' (Protocol in workflow.md) (1088015)

---

## Phase 4: SQLite Session Changeset Capture Foundation

- [x] Task: Implement changeset capture abstraction and hooks (43b4ef3)
    - [x] Write Tests: Unit tests verifying mutation session tracking and changeset diff binary generation on `crm_store`
    - [x] Implement: Changeset capture engine in `internal/store/changeset.go`

- [x] Task: Integrate changeset generation into CLI customer commands (39594cd)
    - [x] Write Tests: Integration tests verifying customer record creation emits a valid binary changeset
    - [x] Implement: CLI `customer put` and `customer get` handlers utilizing the store and changeset capture

- [ ] Task: Conductor - User Manual Verification 'Phase 4: SQLite Session Changeset Capture Foundation' (Protocol in workflow.md)
