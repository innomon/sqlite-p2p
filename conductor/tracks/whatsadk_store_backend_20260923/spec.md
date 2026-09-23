# Specification: whatsadk storeBackend Implementation & Integration (`whatsadk_store_backend`)

## 1. Overview
Implement the whatsadk `storeBackend` interface in a public, embeddable package (`pkg/whatsadk`) backed by the Pure Go SQLite `crm_store` table and Autobase P2P replication engine. This allows `whatsadk` (`/home/innomon/B204-zone/orez/adk/whatsadk`) to embed `sqlite-p2p` as a serverless, local-first, peer-to-peer storage backend alternative to PostgreSQL and SurrealDB. In addition, create a detailed integration specification in `docs/whatsadk_integration_spec.md` for coding agents working in the `whatsadk` workspace.

## 2. Functional Requirements
### 2.1 Package Architecture (`pkg/whatsadk`)
- Exported package `sqlite-p2p/pkg/whatsadk`.
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

### 2.3 Media Handling (Images, Video, Voice)
- **Binary Media Payloads (`content []byte`)**: Images (JPEG, PNG, WebP), audio/voice notes (OGG/Opus, WAV), and video (MP4) are persisted directly as raw binary BLOBs in the SQLite `crm_store.data` column. Modern SQLite natively handles BLOBs up to 2GB with high performance.
- **MIME & Metadata Attributes (`metadata`)**: Media ingestion (`PutFile`) stores a JSON descriptor in `crm_store.metadata` containing `mime_type` (e.g. `image/jpeg`, `audio/ogg`, `video/mp4`), timestamps, and contextual headers (`is_from_me`, message references, sender JID).
- **Zero-Bloat Chat Log Querying**: In `GetFilesysLogs` and `GetLatestGlobalMessages`, a conditional SQL projection (`CASE WHEN (metadata->>'mime_type' = 'text/plain') THEN content ELSE NULL END`) omits large binary media payloads during routine message queries. Media content is loaded on demand when consumers invoke `GetFile(ctx, path)`.
- **P2P Media Streaming & Crypto-Shredding**: Media BLOBs are replicated over peer swarms using Hypercore append-only feeds. When per-user cryptographic shredding is configured, media payloads are AES-256-GCM encrypted, allowing instant irrevocable shredding of all media associated with deleted contacts across all swarm peers.

### 2.4 Decentralized P2P Replication
- Mutations on whatsadk entities trigger SQLite Session API changeset tracking via `store.ChangesetTracker`.
- Changesets are replicated across peer nodes over the 32-byte Hyperswarm cluster topic via pure Go Autobase and Hypercore feeds.

### 2.5 Integration Specification (`docs/whatsadk_integration_spec.md`)
- Create a comprehensive implementation spec for the `whatsadk` workspace:
  - Module import: replace directive or module require for `sqlite-p2p`.
  - URI / DSN schema: `pear://<path>?swarm_topic=<hex>&wal=true` or `sqlite-p2p://<path>`.
  - Step-by-step code modifications for `whatsadk/internal/store/store.go` to register and instantiate the backend.

## 3. Acceptance Criteria
- [ ] `pkg/whatsadk` compiles and fully satisfies the `storeBackend` interface.
- [ ] Unit tests in `pkg/whatsadk` verify 100% of interface methods (commands, filesys, contacts, blacklist) with >80% test coverage.
- [ ] Changeset emission and P2P reconciliation are verified for whatsadk entity writes.
- [ ] `docs/whatsadk_integration_spec.md` is complete, accurate, and ready for a coding agent in whatsadk.

## 4. Out of Scope
- Direct file modification in the `whatsadk` repository (covered by the separate integration spec document).
