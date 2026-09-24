# Implementation Plan - Track: Split Project into 3 Decoupled Repositories (`split_three_projects_20260924`)

## Phase 1: Core Engine Refactoring & Public API (`sqlite-p2p`) [checkpoint: 676b143]
- [x] Task: Export clean public API in `pkg/p2p` (TDD) (cbd732d)
    - [x] Write unit tests for public API initialization, DB opening, repository CRUD, and replication engine in `pkg/p2p/engine_test.go`
    - [x] Implement exported API in `pkg/p2p/engine.go` wrapping store, crypto, and P2P engine
- [x] Task: Decouple CRM-specific tables from core DB initialization (bd622cd)
    - [x] Make `ontology_nodes` and `ontology_edges` table creation an opt-in extension rather than core `OpenDB` DDL
    - [x] Update core store tests and verify >80% coverage
- [x] Task: Conductor - User Manual Verification 'Core Engine Refactoring & Public API' (Protocol in workflow.md) (676b143)

## Phase 2: Scaffold & Extract `crm-lite` Application
- [x] Task: Scaffold `crm-lite` repository (6721d7b)
    - [x] Create `/home/innomon/B204-zone/owly-sewa/crm-lite` with `go.mod` requiring `sqlite-p2p`
    - [x] Setup initial build and test verification scripts
- [x] Task: Migrate CRM components to `crm-lite` (c501da4)
    - [x] Move `cmd/crm-peer`, `internal/cli`, `internal/mcp`, `internal/ontology`, `internal/config`, `schema.go`, and `ontology.go` into `crm-lite`
    - [x] Update imports to `crm-lite/...` and `sqlite-p2p/...`
    - [x] Verify `crm-peer` binary compilation, all tests, and >80% test coverage in `crm-lite`
- [ ] Task: Conductor - User Manual Verification 'Scaffold & Extract crm-lite Application' (Protocol in workflow.md)

## Phase 3: Integrate WhatsaDK Storage Backend into WhatsaDK
- [ ] Task: Configure WhatsaDK dependencies
    - [ ] Add `sqlite-p2p` requirement and replace directive to `/home/innomon/B204-zone/orez/adk/whatsadk/go.mod`
- [ ] Task: Implement native SQLite P2P backend in WhatsaDK (TDD)
    - [ ] Move `pkg/whatsadk` code into `whatsadk/internal/store/p2p/`
    - [ ] Wire `openSQLiteP2P` and `IsSQLiteP2P` in `whatsadk/internal/store/store.go`
    - [ ] Run WhatsaDK store unit tests targeting `sqlite-p2p://:memory:`
- [ ] Task: Conductor - User Manual Verification 'Integrate WhatsaDK Storage Backend into WhatsaDK' (Protocol in workflow.md)

## Phase 4: Final Decoupling, Repository Renaming & Documentation
- [ ] Task: Purge extracted application code from `sqlite-p2p`
    - [ ] Remove `internal/cli`, `internal/mcp`, `internal/ontology`, `cmd/crm-peer`, and `pkg/whatsadk` from `sqlite-p2p`
    - [ ] Ensure `sqlite-p2p` tests pass with >80% coverage across all remaining packages
- [ ] Task: Synchronize Documentation and Quality Gates
    - [ ] Author distinct `README.md` and `ARCHITECTURE.md` for `sqlite-p2p`, `crm-lite`, and WhatsaDK
    - [ ] Run full test suites across all 3 independent projects
- [ ] Task: Conductor - User Manual Verification 'Final Decoupling, Repository Renaming & Documentation' (Protocol in workflow.md)
