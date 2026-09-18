# Technical Specification: Distributed Multimodal Agentic CRM (SQLite + Pure Go Pear/P2P)

## 1. Overview & System Goal
This document specifies the architecture, data structures, and implementation guidelines for a distributed, multi-writer, agentic CRM scaling to 100K+ customer records.

The system operates over a peer-to-peer (P2P) network using a **pure Go implementation of the Pear/Holepunch protocol stack** (Hypercore, Hyperswarm, HyperDHT, Autobase). State is decentralized and synchronized across distributed human and agent nodes without central cloud infrastructure or centralized message brokers (e.g., NATS).

---

## 2. Core Identifiers, Privacy & Security

### 2.1 Customer Primary Key (Deterministic Pseudonym)
* **Format**: `BASE32(Hash(<Customer Primary Mobile Number>))`
* **Hashing Algorithm**: SHA-256 (or BLAKE3) strictly evaluated prior to encoding.
* **Rule**: No cleartext Personal Identifiable Information (PII) such as phone numbers, Aadhaar numbers, or PANs may be stored in cleartext in logs, index keys, or file paths.

### 2.2 Privacy Compliance & Data Purging (Crypto-Shredding)
* **Payload Encryption**: All customer interactions and PII attributes written to Autobase logs MUST be encrypted with a per-customer symmetric key (`AES-256-GCM`).
* **Data Expiry / Erasure**: Upon TTL expiry or explicit deletion request, the per-customer symmetric key is deleted/shredded.
* **Log Retention**: Nodes execute local Hypercore block clearing (`core.Clear(start, end)`) to reclaim disk space while preserving cryptographic hash verification.

---

## 3. Storage Architecture: CQRS Pattern

The system uses Command Query Responsibility Segregation (CQRS) to achieve multi-writer consensus while maintaining fast lookups ($O(\log N)$) across 100K+ customer records.

```
┌─────────────────────────────────────────────────────────────────┐
│                      Agent / Human Node                         │
│                                                                 │
│  ┌──────────────────────┐         ┌──────────────────────────┐  │
│  │   SQLite Read/Write  │         │  Markdown / YAML Files   │  │
│  │  (Local Operational) │         │  (Shared File System)    │  │
│  └──────────┬───────────┘         └────────────┬─────────────┘  │
│             │ Capture Changesets               │ Log File Events│
│             ▼                                  ▼                │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │            Pure Go Autobase Replication Engine            │  │
│  └──────────────────────────┬────────────────────────────────┘  │
└─────────────────────────────┼───────────────────────────────────┘
                              │
                    Hyperswarm (P2P Mesh)
```

### 3.1 Write Model: Pure Go Autobase Log
* Each node appends SQLite Session `changesets` or interaction events to its local Go `hypercore`.
* **Go Autobase** linearizes inputs from all connected agent cores into a single, deterministically ordered virtual stream using vector clocks.

### 3.2 Read Model: Local SQLite Database
* Every node continuously consumes the linearized Autobase stream and applies changesets locally via `sqlite3changeset_apply`.
* **Pragmas Required**:
  ```sql
  PRAGMA journal_mode = WAL;
  PRAGMA synchronous = NORMAL;
  PRAGMA foreign_keys = ON;
  ```

### 3.3 Shared File System & Ontology Graph
* Interaction logs and unstructured context are saved as Markdown files containing YAML headers.
* Local nodes parse YAML metadata into a localized SQLite graph table (or in-memory index) using CTEs to support fast graph traversals.

### 2.1 Write Side (P2P Log Engine)
- Every active node maintains a local, append-only **Hypercore** log.
- Mutations (INSERT, UPDATE, DELETE) are captured as binary changesets via SQLite's Session API (`sqlite3_session`).
- **Autobase** linearizes changesets across all writing peers into a deterministic, causally-ordered virtual stream.

### 2.2 Read Side (Local Materialization)
- Each node consumes the linearized Autobase stream and applies changesets locally to its embedded `crm.db` using `sqlite3changeset_apply()`.
- Reads, queries, full-text searches, and graph traversals execute locally against SQLite with $O(1)$ / $O(\log N)$ performance.


```

┌───────────────────────────────────────────────────────────┐
│                    LOCAL CLIENT NODE                      │
│                                                           │
│  1. Client App executes SQL write                         │
│     │                                                     │
│     ▼                                                     │
│  2. Local SQLite DB (crm_store)                           │
│     │                                                     │
│     ▼ (SQLite Session API Hooks)                          │
│  3. Binary Changeset Generated                            │
│     │                                                     │
│     ▼                                                     │
│  4. Append to Local Pure Go Hypercore Log                 │
└─────────────────────────────┬─────────────────────────────┘
│
│ 5. P2P Replication (Hyperswarm / DHT)
▼
┌───────────────────────────────────────────────────────────┐
│                   REMOTE PEER NODES                       │
│                                                           │
│  6. Pure Go Autobase Orders / Linearizes Streams          │
│     │                                                     │
│     ▼                                                     │
│  7. Conflict Resolution (LWW / Custom Interceptor)         │
│     │                                                     │
│     ▼                                                     │
│  8. `sqlite3changeset_apply()` to Local `crm_store`       │
└───────────────────────────────────────────────────────────┘

```

---

---

## 4. Database Schema & Single-Table Key-Value Architecture

### 3.1 Unified Storage Table (`crm_store`)
All entity records, interactions, and ontology structures are stored in a single table without `ROWID`:

```sql
CREATE TABLE IF NOT EXISTS crm_store (
    key TEXT PRIMARY KEY,
    metadata TEXT CHECK(json_valid(metadata)),
    data BLOB
) WITHOUT ROWID;

```

### 4.2 Key Formatting Standard

Keys follow a strict namespaced URI format: `string <namespace>:<unique_key>`

* **Customer Records:**
`in.qzip.crm.customer:<BASE32(Hash(Primary Mobile))>`
* **Privacy Guarantee:** Primary identifiers (mobile number, Aadhaar, PAN) are never stored in cleartext.

### 4.3 Metadata Schema & Compound Objects

The `metadata` column stores a JSON object containing an array of independent compound objects under `schemas`. Each compound object MUST specify a `$ID` schema URI:

```json
{
  "schemas": [
    {
      "$ID": "[https://qzip.in/schemas/crm/customer-v1.json](https://qzip.in/schemas/crm/customer-v1.json)",
      "tier": "enterprise",
      "status": "active",
      "last_interaction": "2026-09-18T20:30:00Z"
    },
    {
      "$ID": "[https://qzip.in/schemas/crm/ontology-graph-v1.json](https://qzip.in/schemas/crm/ontology-graph-v1.json)",
      "nodes": [
        {"id": "c1", "label": "Customer", "type": "Entity"},
        {"id": "a1", "label": "Agent_42", "type": "Agent"}
      ],
      "edges": [
        {"source": "a1", "target": "c1", "relationship": "MANAGES"}
      ]
    }
  ]
}

```
## 4. Client Application Mutation & Write Pipeline

### 4.1 Write Lifecycle Rules

1. **No Direct Remote Writes:** Client apps never write to a remote node directly.
2. **Local First:** All SQL mutations run against the local embedded `crm_store` table in WAL mode (`_journal_mode=WAL`).
3. **Changeset Capture:** SQLite Session API records row modifications on `crm_store`.
4. **Log Propagation:** The resulting binary changeset is appended to the local `hypercore` and replicated over Hyperswarm.
5. **Autobase Ingestion:** Remote nodes receive changesets, pass them through the Autobase consensus layer, and apply them locally via `sqlite3changeset_apply()`.

### 4.2 SQLite Session Attachment (Go)

```go
// Begin Session tracking on crm_store
session, err := sqlite3.NewSession(db, "main")
if err != nil {
    return err
}
defer session.Delete()

// Attach the single crm_store table
err = session.Attach("crm_store")
if err != nil {
    return err
}

// Execute local mutation
_, err = db.Exec(`
    INSERT INTO crm_store (key, metadata, data) 
    VALUES (?, ?, ?) 
    ON CONFLICT(key) DO UPDATE SET 
        metadata = excluded.metadata,
        data = excluded.data
`, key, metadataJSON, binaryData)

// Capture changeset diff
changeset, err := session.Changeset()
if err != nil {
    return err
}

// Append changeset to local pure Go Hypercore
_, err = localHypercore.Append(changeset)

```

---

## 5. Conflict Resolution & Data Governance

### 5.1 Causal Ordering via Autobase

In multi-writer concurrent updates (e.g., two agents modifying the same record simultaneously), Autobase linearizes updates into a deterministic sequence across all peers.

### 5.2 Conflict Interceptor (Last-Writer-Wins)

When applying changesets via `sqlite3changeset_apply()`, a custom conflict callback evaluates primary key collisions on `crm_store`:

* Compares sequence versions or high-precision timestamps inside the JSON metadata.
* Accepts the latest write or resolves compound schema arrays recursively.

### 5.3 Data Retention & Crypto-Shredding (GDPR/Compliance)

Because append-only P2P logs cannot be altered in-place:

1. Payloads in `data` or sensitive metadata fields are encrypted using per-record symmetric keys.
2. To purge/forget a record upon expiration or TTL, the symmetric decryption key is deleted from the key registry (Crypto-shredding).
3. The historical raw bytes on Autobase logs become permanently unreadable.

---


## 5. Peer-to-Peer Replication Engine Specification

The coding agent MUST implement the synchronization worker using the following Go structure:

```go
package sync

import (
	"context"
	"database/sql"
)

// P2P Engine Configuration
type Config struct {
	SwarmTopic    [32]byte
	DBPath        string
	StorageDir    string
	EnableWAL     bool
}

// ReplicationEngine connects SQLite Session API to Pure Go Autobase
type ReplicationEngine struct {
	db        *sql.DB
	config    Config
	// Pure Go Pear stack interfaces
	autobase  AutobaseCore
	hyperswarm SwarmManager
}

// Initialize and start synchronization loop
func NewReplicationEngine(cfg Config) (*ReplicationEngine, error) {
	// 1. Open SQLite DB with WAL mode
	// 2. Initialize Pure Go Hypercore & Autobase
	// 3. Join Hyperswarm topic
	// 4. Start background change-capture and changeset-apply loops
	return &ReplicationEngine{}, nil
}

// CaptureLocalChange intercepts write transactions and appends changesets to Autobase
func (r *ReplicationEngine) CaptureLocalChange(changeset []byte) error {
	_, err := r.autobase.Append(changeset)
	return err
}

// ProcessRemoteChangesets reads from Autobase stream and applies changesets to SQLite
func (r *ReplicationEngine) ProcessRemoteChangesets(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case change := <-r.autobase.ChangesetStream():
			if err := r.applyChangesetToSQLite(change); err != nil {
				// Handle Last-Writer-Wins (LWW) conflict resolution logic
				r.resolveConflict(change, err)
			}
		}
	}
}

func (r *ReplicationEngine) applyChangesetToSQLite(change []byte) error {
	// Execution of sqlite3changeset_apply using Autobase sequence ordering
	return nil
}

func (r *ReplicationEngine) resolveConflict(change []byte, err error) {
	// Fallback to Last-Writer-Wins using Autobase sequence ID
}
```

---
## 6. Tech 

follow canonical go idioms. Use pure go with no CGO dependency. create a config.yaml for configuration, 1st check if it is provided as arg to main, if not is it in the same dir as 
the excutable else on the current dir.
 
Command subcommand registries should be handcrafted, don't use any their party lib like cobra/spf13 . 

for logging use the guidelines of [logging pattern](structured_logging_pattern.md)

for pure go implementation of [pear-p2p](https://docs.pears.com/p2p) see /home/innomon/B204-zone/owly-sewa/go-pear


## 7. Development & Implementation Roadmap for Coding Agent

1. **Step 1: Database Drivers & Hook Setup**
   * Configure SQLite with WAL mode and attach SQLite Session extension hooks (`sqlite3_session`) to capture binary changesets.
2. **Step 2: Autobase Integration**
   * Wire the pure Go `autobase` library to consume local SQLite changesets and publish them to peer nodes.
3. **Step 3: Network Swarming (Hyperswarm / HyperDHT)**
   * Bind the `autobase` stream to pure Go `hyperswarm` connections using the 32-byte CRM cluster topic key.
4. **Step 4: Ontology Engine & Markdown Watcher**
   * Build a directory watcher for markdown files; parse YAML headers on creation/edit and upsert nodes/edges into `ontology_nodes` and `ontology_edges`.
5. **Step 5: Testing & Validation**
   * Test dual-agent concurrent writes to the same customer key `BASE32(Hash(Mobile))` to verify deterministic state convergence via Autobase vector clock ordering.