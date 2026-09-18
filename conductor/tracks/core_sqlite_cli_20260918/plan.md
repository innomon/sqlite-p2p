# Implementation Plan: Core SQLite crm_store Engine, Changeset Capture & CLI Foundation

## Phase 1: Foundation, Configuration & Structured Logging

- [x] Task: Initialize pure Go module and project directory structure (79ef80b)
    - [x] Write Tests: Test directory layout verification and build target validation
    - [x] Implement: Initialize `go.mod`, setup package folders (`cmd/crm-peer`, `internal/config`, `internal/logger`, `internal/store`, `internal/cli`)

- [x] Task: Implement 3-tier hierarchical YAML configuration loader (5ae663e)
    - [x] Write Tests: Unit tests for CLI flag override, binary directory lookup, cwd fallback, and default values
    - [x] Implement: Configuration structs and loader in `internal/config/config.go` with YAML parsing

- [ ] Task: Implement zero-dependency structured JSON logger
    - [ ] Write Tests: Unit tests verifying RFC3339Nano timestamps, standard field structure, log levels, and error serialization
    - [ ] Implement: Logger package in `internal/logger/logger.go` adhering to `structured_logging_pattern.md`

- [ ] Task: Conductor - User Manual Verification 'Phase 1: Foundation, Configuration & Structured Logging' (Protocol in workflow.md)

---

## Phase 2: Handcrafted CLI Command Registry

- [ ] Task: Implement handcrafted command and subcommand router
    - [ ] Write Tests: Unit tests verifying command registration, path resolution, subcommand dispatch, and error handling
    - [ ] Implement: Command registry in `internal/cli/registry.go` without third-party CLI dependencies

- [ ] Task: Wire CLI application entry point and baseline subcommands
    - [ ] Write Tests: Unit tests for `help`, `status`, and version flag execution
    - [ ] Implement: Main entry point in `cmd/crm-peer/main.go` and core handlers in `internal/cli/commands.go`

- [ ] Task: Conductor - User Manual Verification 'Phase 2: Handcrafted CLI Command Registry' (Protocol in workflow.md)

---

## Phase 3: Pure Go SQLite Engine & `crm_store` Single-Table Storage

- [ ] Task: Implement deterministic customer key generator
    - [ ] Write Tests: Unit tests for phone number sanitization, SHA-256 hashing, Base32 encoding, and URI formatting
    - [ ] Implement: Key generator in `internal/store/key.go` returning `in.qzip.crm.customer:<BASE32(SHA256(phone))>`

- [ ] Task: Implement Pure Go SQLite database initialization with WAL pragmas
    - [ ] Write Tests: Unit tests verifying database opening, WAL pragma enforcement, and `crm_store` WITHOUT ROWID table creation
    - [ ] Implement: DB manager in `internal/store/db.go` using pure Go SQLite driver (`modernc.org/sqlite`)

- [ ] Task: Implement CRUD operations with JSON metadata validation
    - [ ] Write Tests: Unit tests for record insert, upsert, query by key, and invalid JSON metadata rejection
    - [ ] Implement: Storage repository methods in `internal/store/repository.go`

- [ ] Task: Conductor - User Manual Verification 'Phase 3: Pure Go SQLite Engine & crm_store Single-Table Storage' (Protocol in workflow.md)

---

## Phase 4: SQLite Session Changeset Capture Foundation

- [ ] Task: Implement changeset capture abstraction and hooks
    - [ ] Write Tests: Unit tests verifying mutation session tracking and changeset diff binary generation on `crm_store`
    - [ ] Implement: Changeset capture engine in `internal/store/changeset.go`

- [ ] Task: Integrate changeset generation into CLI customer commands
    - [ ] Write Tests: Integration tests verifying customer record creation emits a valid binary changeset
    - [ ] Implement: CLI `customer put` and `customer get` handlers utilizing the store and changeset capture

- [ ] Task: Conductor - User Manual Verification 'Phase 4: SQLite Session Changeset Capture Foundation' (Protocol in workflow.md)
