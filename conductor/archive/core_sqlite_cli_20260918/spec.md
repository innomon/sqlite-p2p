# Specification: Core SQLite crm_store Engine, Changeset Capture & CLI Foundation

## 1. Objectives & Overview
This track implements the foundational layer of the Distributed Multimodal Agentic CRM:
1. **Module & Infrastructure**: Pure Go module setup, 3-tier hierarchical YAML configuration loader, and structured JSON logger adhering to `structured_logging_pattern.md`.
2. **Handcrafted CLI Architecture**: A zero-dependency CLI command/subcommand dispatch registry (strictly no `cobra`/`pflag`).
3. **Pure Go SQLite Storage Engine**: Embedded SQLite using pure Go (`modernc.org/sqlite`) configured in WAL mode (`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA foreign_keys=ON;`).
4. **Unified Single-Table Schema (`crm_store`)**: Implementation of `crm_store (key TEXT PRIMARY KEY, metadata TEXT CHECK(json_valid(metadata)), data BLOB) WITHOUT ROWID`, namespaced key generation (`in.qzip.crm.customer:<BASE32(SHA256(phone))>`), and JSON metadata schema validation.
5. **Changeset Capture Interface**: Local mutation hooks and changeset extraction abstraction preparing data for P2P replication via Autobase.

## 2. Technical Architecture & Components

### 2.1 Configuration & Logging
- **Config Loader**: Resolves configuration in order:
  1. CLI argument `--config <path>`
  2. Executable directory `./config.yaml`
  3. Current working directory `./config.yaml`
  Fallback to sensible default configurations if no file is present.
- **Structured Logger**: Emits JSON log lines containing `timestamp` (RFC3339Nano), `level`, `service`, `event`, `trace_id`, and contextual key-value pairs without third-party frameworks.

### 2.2 Handcrafted Command Registry
- Dispatcher pattern mapping command paths (e.g. `customer add`, `customer get`, `status`, `keygen`) to handler functions.
- Pure Go flag parsing using standard library `flag` or custom token scanner.
- Usage help generation and subcommands tree.

### 2.3 Storage Engine & Schema
- Embedded database initialization with WAL pragmas:
  ```sql
  CREATE TABLE IF NOT EXISTS crm_store (
      key TEXT PRIMARY KEY,
      metadata TEXT CHECK(json_valid(metadata)),
      data BLOB
  ) WITHOUT ROWID;
  ```
- **Customer Key Generation**: Function `FormatCustomerKey(phone string) (string, error)` applying SHA256 and RFC4648 Base32 encoding without padding, returning `in.qzip.crm.customer:<BASE32>`.
- **CRUD Operations**: Safe CRUD methods implementing JSON validation and upsert operations (`INSERT ... ON CONFLICT(key) DO UPDATE`).

### 2.4 Changeset Capture & Session API
- Interface `ChangesetCapture` capturing row mutations on `crm_store` as binary changesets.
- Session lifecycle tracking transactions on `crm_store` and emitting changeset byte slices.

## 3. Quality & Acceptance Criteria
- **Pure Go Compliance**: Must build with `CGO_ENABLED=0`.
- **Test-Driven Development**: Every task must have unit tests written first (Red phase) and pass (Green phase).
- **Code Coverage**: Must exceed 80% line coverage.
- **Conformance**: CLI registry must not import `github.com/spf13/cobra` or `github.com/spf13/pflag`.
