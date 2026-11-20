# Specification: Multi-Table Schema Auto-Discovery, Trigger-Based CDC Outbox & Universal Hypercore Swarm Replication

## 1. Overview
Upgrade `sqlite-p2p` from a single-table (`crm_store`), API-only (`engine.Put` / `engine.Delete`) replicator into a **universal multi-table SQLite replication engine** over the Pear/Holepunch P2P stack (Hypercore, Autobase, Hyperswarm). Any user table in a SQLite database (such as `ecom_store`, `crm_store`, or arbitrary relational tables created via DDL/SQL) must be automatically discovered, outfitted with Change Data Capture (CDC) triggers, and replicated across all connected Swarm peers with Last-Write-Wins (LWW) conflict resolution.

## 2. Functional Requirements

### 2.1 Multi-Table & Arbitrary-Schema `Changeset` Format (`internal/store/changeset.go`)
- Extend `OperationType` with `OpDDL OperationType = 4` for schema creation/evolution events.
- Extend `Changeset` struct while maintaining 100% backward compatibility with existing `crm_store` changesets:
  - `Table string` (`json:"table,omitempty"`): Target table name. If empty on decode, defaults to `"crm_store"`.
  - `Key string` (`json:"key"`): Primary key value for single-column PKs, or canonical JSON array string for composite PKs (or `rowid` string if no explicit PK).
  - `Metadata json.RawMessage` (`json:"metadata,omitempty"`): Preserved for fast-path KV tables (`(key, metadata, data)` layout such as `crm_store` and `ecom_store`).
  - `Data []byte` (`json:"data,omitempty"`): Preserved for fast-path KV tables.
  - `Columns map[string]json.RawMessage` (`json:"columns,omitempty"`): Generic column-to-value map (with BLOBs base64 or hex encoded in JSON envelope) for arbitrary relational tables.
  - `DDL string` (`json:"ddl,omitempty"`): Normalized `CREATE TABLE IF NOT EXISTS ...` statement emitted for `OpDDL`.

### 2.2 Table Auto-Discovery & Configurable Table Filtering (`internal/store/`)
- Query `sqlite_master` to discover all user tables (`type = 'table'`), automatically excluding:
  - SQLite internal tables (`sqlite_%`)
  - P2P replication internal tables (`_p2p_%`)
  - Local cryptographic key registry (`crm_keys`)
- Support optional table inclusion/exclusion filters in `EngineOptions` and `NodeConfig` (`IncludeTables []string`, `ExcludeTables []string`).
- Inspect table schema via `PRAGMA table_info(<table>)` and `sqlite_master.sql` to detect:
  1. Whether the table conforms to the canonical KV triple `(key TEXT PRIMARY KEY, metadata TEXT, data BLOB)` (such as `crm_store` or `ecom_store`).
  2. Primary key column(s) (or `rowid` fallback) and column types for general relational tables.
- Allow configuring the default primary KV repository table name (`EngineOptions.DefaultTable`, defaulting to `"crm_store"`).

### 2.3 SQLite Trigger-Based Change Data Capture (`_p2p_cdc_outbox`)
- Create internal book-keeping tables (excluded from replication):
  - `_p2p_sync_state (id INTEGER PRIMARY KEY CHECK (id = 1), applying_remote INTEGER NOT NULL DEFAULT 0)`
  - `_p2p_cdc_outbox (seq INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER NOT NULL, op INTEGER NOT NULL, table_name TEXT NOT NULL, pk_key TEXT NOT NULL, metadata TEXT, data BLOB, row_json TEXT)`
- Automatically install `AFTER INSERT`, `AFTER UPDATE`, and `AFTER DELETE` triggers (`_p2p_trg_<table>_ins`, `_p2p_trg_<table>_upd`, `_p2p_trg_<table>_del`) on every tracked user table.
- Guard all CDC triggers with `WHEN (SELECT applying_remote FROM _p2p_sync_state WHERE id = 1) = 0` so remote changesets applied by `ApplyRemoteChangeset` never recursively trigger local CDC outbox entries.
- Provide an active CDC outbox drainer in `ReplicationEngine` (`DrainCDCOutbox(ctx)`) that runs both on a low-latency ticker (e.g., 100ms), after local `engine.Put`/`Delete` or `sql` CLI execution, and on demand:
  - Reads pending rows from `_p2p_cdc_outbox` in `seq ASC` order.
  - Converts each row into a `store.Changeset`, appends it to the local `ChangesetFeed` / `AutobaseManager`, broadcasts it via `Replicator.BroadcastChangeset`, and deletes the processed rows from `_p2p_cdc_outbox`.
- Periodically scan `sqlite_master` (or hook after DDL execution in `sql` command) to attach triggers and emit `OpDDL` for newly created tables.

### 2.4 Multi-Table Seeding & Table-Scoped LWW Reconciliation (`internal/p2p/engine.go`)
- **Table-Scoped LWW Map**: Track latest timestamps per `(table, key)` composite identifier (`table + "\x00" + key`) so identical primary keys across different tables (`crm_store` vs `ecom_store` vs `orders`) never collide.
- **`SeedFromRepository` Across All Tables**:
  - When seeding an empty `.hypercore` feed from an existing SQLite database (e.g., `kheti_badi.db` containing `ecom_store`), first emit `OpDDL` (`CREATE TABLE IF NOT EXISTS ...`) for every discovered user table, then emit `OpInsert` changesets for all existing rows across all tracked tables.
- **`ApplyRemoteChangeset` Multi-Table Execution**:
  - Within a transaction, set `UPDATE _p2p_sync_state SET applying_remote = 1 WHERE id = 1;`.
  - If `cs.Operation == store.OpDDL`: execute the normalized `CREATE TABLE IF NOT EXISTS ...` DDL and attach CDC triggers to the newly created table.
  - If `cs.Operation` is `OpInsert` / `OpUpdate`:
    - If target table does not exist yet and `cs.Columns == nil` (KV fast-path table like `ecom_store`), auto-create the KV table (`CREATE TABLE IF NOT EXISTS <table> (key TEXT PRIMARY KEY, metadata TEXT CHECK(json_valid(metadata)), data BLOB) WITHOUT ROWID;`) and attach CDC triggers.
    - Upsert the row into `cs.Table` using parameterized SQL (`INSERT INTO "<table>" ... ON CONFLICT(...) DO UPDATE SET ...`).
  - If `cs.Operation == store.OpDelete`: delete the row from `cs.Table` by primary key.
  - Reset `UPDATE _p2p_sync_state SET applying_remote = 0 WHERE id = 1;` before committing.

## 3. Non-Functional Requirements
- **100% Pure Go (`CGO_ENABLED=0`)**: Uses standard SQLite triggers and `json_object()` built into `modernc.org/sqlite` without CGo callbacks.
- **Backward Compatibility**: Existing `crm_store` databases, `.hypercore` feeds, and `pkg/p2p` API consumers continue to work without breaking changes.
- **SQL Injection Safety**: All dynamically constructed table and column identifiers must be strictly validated and double-quote escaped (`"table_name"`), with all values bound via `?` parameters.
- **Test Coverage**: >80% unit and integration test coverage across all modified packages.

## 4. Acceptance Criteria
- [ ] A database with an existing `ecom_store` table (like `kheti-badi-ecom`) opened by `sqlite-p2p` automatically seeds `ecom_store` schema and rows into its Hypercore feed and replicates them to a second `sqlite-p2p` peer.
- [ ] Direct SQL mutations (`INSERT`, `UPDATE`, `DELETE`) executed against `ecom_store` or any custom relational table (`CREATE TABLE orders (id TEXT PRIMARY KEY, status TEXT, total REAL)`) via `*sql.DB` or the `sql` CLI command are captured by CDC triggers, appended to Hypercore, and replicated to connected peers.
- [ ] Applying remote changesets sets `applying_remote = 1` and does not cause infinite replication loops in `_p2p_cdc_outbox`.
- [ ] LWW conflict resolution isolates timestamps by `(Table, Key)` so keys shared across multiple tables resolve independently.
- [ ] Legacy changesets with empty `Table` field default to `"crm_store"` and pass all existing tests.
