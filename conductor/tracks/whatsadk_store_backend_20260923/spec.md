# Specification: whatsadk storeBackend Implementation & Integration (`whatsadk_store_backend`)

## 1. Overview
Implement the whatsadk `storeBackend` interface in a public, embeddable package (`pkg/whatsadk`) backed by the Pure Go SQLite `crm_store` table and Autobase P2P replication engine. This allows `whatsadk` (`/home/innomon/B204-zone/orez/adk/whatsadk`) to embed `crm-sqlite-pear-p2p` as a serverless, local-first, peer-to-peer storage backend alternative to PostgreSQL and SurrealDB. In addition, create a detailed integration specification in `docs/whatsadk_integration_spec.md` for coding agents working in the `whatsadk` workspace.

## 2. Functional Requirements
### 2.1 Package Architecture (`pkg/whatsadk`)
- Exported package `crm-sqlite-pear-p2p/pkg/whatsadk`.
- Models mirroring whatsadk contracts:
  - `Command`: ID, Command, Payload, Status, Result, CreatedAt, UpdatedAt.
  - `Contact`: OurJID, TheirJID, FullName, ShortName, PushName, BusinessName.
  - `FileEntry`: Path, Metadata (`sql.NullString`), Content, Timestamp.
  - `BlacklistedNumber`: Phone, Reason, CreatedAt.
- `Backend` struct implementing the full `storeBackend` interface:
  - `Close() error`
  - Command queue: `EnqueueCommand`, `UpdateCommandStatus`, `PollPendingCommands`, `WaitForCommand`, `GetAllCommands`, `PutCommand`, `ResetSequence`
  - Virtual filesys: `PutFile`, `GetFile`, `DeleteFile`, `ListFiles`, `GetAllFiles`, `GetFilesysLogs`, `GetLatestGlobalMessages`, `QueryFilesys`
  - Contacts: `PutContact`, `ListContacts`, `GetAllContacts`
  - Blacklist: `AddBlacklist`, `RemoveBlacklist`, `IsBlacklisted`, `ListBlacklist`

### 2.2 Unified Storage & Key-Value Partitioning (`crm_store`)
- All entities map directly into `crm_store` (`key`, `metadata`, `data` WITHOUT ROWID):
  - **Filesys**: `whatsadk:filesys:<path>`
    - `metadata`: JSON object containing file metadata attributes and timestamp.
    - `data`: Raw byte content.
  - **Contacts**: `whatsadk:contact:<our_jid>:<their_jid>`
    - `metadata`: JSON object with JIDs, full name, short name, push name, business name.
  - **Commands**: `whatsadk:command:<id>`
    - `metadata`: JSON object with command name, payload, status, result, timestamps.
    - Sequence counter stored at `whatsadk:seq:commands`.
  - **Blacklist**: `whatsadk:blacklist:<phone>`
    - `metadata`: JSON object with phone, reason, created_at.
- **SQLite Views for QueryFilesys Compatibility**:
  - Automatically create or initialize a SQLite View `filesys` over `crm_store` so that whatsadk's raw SQL queries (`SELECT ... FROM filesys WHERE ...`) execute natively in SQLite with zero CGO.

### 2.3 Decentralized P2P Replication
- Mutations on whatsadk entities trigger SQLite Session API changeset tracking via `store.ChangesetTracker`.
- Changesets are replicated across peer nodes over the 32-byte Hyperswarm cluster topic via pure Go Autobase and Hypercore feeds.

### 2.4 Integration Specification (`docs/whatsadk_integration_spec.md`)
- Create a comprehensive implementation spec for the `whatsadk` workspace:
  - Module import: replace directive or module require for `crm-sqlite-pear-p2p`.
  - URI / DSN schema: `pear://<path>?swarm_topic=<hex>&wal=true` or `sqlite-p2p://<path>`.
  - Step-by-step code modifications for `whatsadk/internal/store/store.go` to register and instantiate the backend.

## 3. Acceptance Criteria
- [ ] `pkg/whatsadk` compiles and fully satisfies the `storeBackend` interface.
- [ ] Unit tests in `pkg/whatsadk` verify 100% of interface methods (commands, filesys, contacts, blacklist) with >80% test coverage.
- [ ] Changeset emission and P2P reconciliation are verified for whatsadk entity writes.
- [ ] `docs/whatsadk_integration_spec.md` is complete, accurate, and ready for a coding agent in whatsadk.

## 4. Out of Scope
- Direct file modification in the `whatsadk` repository (covered by the separate integration spec document).
