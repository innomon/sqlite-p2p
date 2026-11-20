# Implementation Plan: Multi-Table Schema Auto-Discovery, Trigger-Based CDC Outbox & Universal Hypercore Swarm Replication

## Phase 1: Multi-Table Changeset Envelope & Schema Discovery Engine (`internal/store`) [checkpoint: 8bd7ce9]

- [x] Task: Write Unit Tests for Multi-Table `Changeset` & Table Discovery (TDD Red Phase) 319c652
    - [x] Test encoding/decoding `Changeset` with `Table`, `Columns`, `DDL`, and `OpDDL`
    - [x] Verify backward compatibility: decoding legacy `Changeset` with empty `Table` defaults to `"crm_store"`
    - [x] Test `DiscoverTables(db, include, exclude)` querying `sqlite_master` and `PRAGMA table_info`, verifying exclusion of `sqlite_%`, `_p2p_%`, and `crm_keys`
    - [x] Test detection of KV-schema tables (`ecom_store`, `crm_store`) vs general relational tables (single PK, composite PK, `rowid`)
- [x] Task: Implement Multi-Table `Changeset` & Schema Inspector in `internal/store` (TDD Green Phase) 319c652
    - [x] Extend `OperationType` (`OpDDL`) and `Changeset` struct in `internal/store/changeset.go`
    - [x] Implement `TableInfo`, `ColumnInfo`, and `DiscoverTables` in `internal/store/tables.go`
    - [x] Parameterize `Repository` table name (`NewRepositoryWithTable(db, tableName)`) while keeping `NewRepository(db)` defaulting to `"crm_store"`
- [x] Task: Conductor - User Manual Verification 'Phase 1: Multi-Table Changeset Envelope & Schema Discovery Engine' (Protocol in workflow.md) 8bd7ce9

## Phase 2: SQLite Trigger-Based CDC Outbox & Loop Prevention (`internal/store`)

- [x] Task: Write Unit Tests for CDC Triggers & Outbox Draining (TDD Red Phase) 858bc5a
    - [x] Test initialization of `_p2p_sync_state` and `_p2p_cdc_outbox` tables
    - [x] Test automatic installation of `AFTER INSERT`, `AFTER UPDATE`, and `AFTER DELETE` triggers on `ecom_store` (`WITHOUT ROWID` KV table) and a relational `orders` table
    - [x] Test that direct `db.Exec("INSERT/UPDATE/DELETE ...")` populates `_p2p_cdc_outbox` with accurate operation, PK key, metadata/data or column JSON
    - [x] Test that mutations executed inside `WithRemoteApply(tx, ...)` (`applying_remote = 1`) do NOT generate `_p2p_cdc_outbox` rows
- [x] Task: Implement Trigger Generator, Outbox Reader & Remote-Apply Guard (TDD Green Phase) 858bc5a
    - [x] Implement `InitCDC(db)` and `InstallTableTriggers(db, tableInfo)` in `internal/store/cdc.go`
    - [x] Implement `ReadAndClearOutbox(ctx, db, limit)` converting outbox records into `[]*Changeset`
    - [x] Implement `ApplyChangesetToTable(ctx, db, cs)` with `applying_remote = 1` transactional guard, auto-table creation for KV/DDL changesets, and parameterized `INSERT ... ON CONFLICT DO UPDATE` / `DELETE`
- [~] Task: Conductor - User Manual Verification 'Phase 2: SQLite Trigger-Based CDC Outbox & Loop Prevention' (Protocol in workflow.md)

## Phase 3: Multi-Table ReplicationEngine, LWW Scoping & Outbox Poller (`internal/p2p` & `pkg/p2p`)

- [ ] Task: Write Unit Tests for Multi-Table `ReplicationEngine` & Outbox Drainer (TDD Red Phase)
    - [ ] Test LWW timestamp scoping by `table + "\x00" + key` so identical keys in `crm_store` and `ecom_store` do not collide
    - [ ] Test `SeedFromRepository` emitting `OpDDL` and `OpInsert` changesets for all discovered user tables (`crm_store`, `ecom_store`, custom tables)
    - [ ] Test `DrainCDCOutbox` polling `_p2p_cdc_outbox`, appending to `ChangesetFeed`/`AutobaseManager`, and broadcasting to connected peers
    - [ ] Test `ApplyRemoteChangeset` handling `OpDDL`, KV tables (`ecom_store`), and relational tables end-to-end
- [ ] Task: Upgrade `ReplicationEngine` and `pkg/p2p.Engine` for Multi-Table CDC Sync (TDD Green Phase)
    - [ ] Update `ReplicationEngine` in `internal/p2p/engine.go` to key `timestamps` by `tableKey(cs.Table, cs.Key)` and delegate remote application to `store.ApplyChangesetToTable`
    - [ ] Implement `DrainCDCOutbox(ctx)` and background outbox poller lifecycle in `ReplicationEngine` and `pkg/p2p.Engine`
    - [ ] Add `DefaultTable`, `IncludeTables`, `ExcludeTables`, and `CDCInterval` options to `pkg/p2p.EngineOptions` and `pkg/config.NodeConfig`
- [ ] Task: Conductor - User Manual Verification 'Phase 3: Multi-Table ReplicationEngine, LWW Scoping & Outbox Poller' (Protocol in workflow.md)

## Phase 4: End-to-End Multi-Node Swarm Replication Verification (`tests/` & `cmd/e2e-replication`)

- [ ] Task: Write End-to-End Multi-Table Swarm Replication Tests (TDD Red Phase)
    - [ ] Create E2E test with Node A initialized with a `kheti-badi-ecom` `ecom_store` table and a custom `orders` table, replicating over Hyperswarm to Node B (which starts with an empty DB)
    - [ ] Verify that direct SQL `INSERT`, `UPDATE`, and `DELETE` via `sql` CLI / `*sql.DB` on Node A replicate to Node B and vice versa without replication loops
- [ ] Task: Wire `sql` CLI Trigger Refresh & Pass Full Quality Gates (TDD Green Phase)
    - [ ] Trigger immediate `DrainCDCOutbox` and schema discovery refresh after `sql` / `/sql` command execution in `cmd/e2e-replication/main.go`
    - [ ] Run full test suite (`go test -v -cover ./...`) and verify >80% coverage across all packages
- [ ] Task: Conductor - User Manual Verification 'Phase 4: End-to-End Multi-Node Swarm Replication Verification' (Protocol in workflow.md)
