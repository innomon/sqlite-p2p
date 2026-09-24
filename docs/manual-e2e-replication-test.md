# Manual End-to-End Replication Test Guide

This guide describes how to perform manual end-to-end (E2E) replication testing between two separate physical or virtual computers (for example, a **Linux ARM64** device such as a Raspberry Pi 5 or AWS Graviton, and a **macOS Apple Silicon M4 / M3 / M2 / M1** workstation).

---

## 1. Overview & Architecture

`sqlite-p2p` synchronizes SQLite databases across decentralized peer nodes using:

- **Pure Go SQLite Engine**: Zero CGo overhead (`modernc.org/sqlite`).
- **Hypercore Changeset Feed**: Append-only binary log capturing all insertions, updates, and deletions (`internal/p2p/feed.go`).
- **P2P Replicator**: Streaming bidirectional length-prefixed protocol over TCP connections (`internal/p2p/replicator.go`).
- **Last-Writer-Wins (LWW) Conflict Resolution**: Convergent state synchronization based on nanosecond timestamps (`internal/p2p/engine.go`).

```
 +-------------------------+                  +-------------------------+
 |       Computer 1        |                  |       Computer 2        |
 |      (Linux ARM64)      |                  |       (macOS M4)        |
 |                         |                  |                         |
 | +---------------------+ |   TCP Replication| +---------------------+ |
 | | SQLite (node1.db)   | | <=============>  | | SQLite (node2.db)   | |
 | +---------------------+ |     Port: 9001   | +---------------------+ |
 | | Hypercore Feed      | |                  | | Hypercore Feed      | |
 | +---------------------+ |                  | +---------------------+ |
 | | e2e-replication CLI | |                  | | e2e-replication CLI | |
 +-------------------------+                  +-------------------------+
```

---

## 2. Included Artifacts & Directory Layout

All required configuration files, runners, and precompiled binaries are included in the repository:

- `/scripts/run-e2e-test.sh` (or symlink `/run-e2e-test.sh`): Universal shell runner that inspects `uname -s` and `uname -m`, detects the platform, and launches the appropriate executable.
- `/bin/e2e-replication-linux-arm64`: Precompiled standalone binary for Linux ARM64 / AArch64.
- `/bin/e2e-replication-darwin-arm64`: Precompiled standalone binary for macOS Apple Silicon (ARM64 / M1–M4).
- `/config/node1.json`: Default configuration for Computer 1 (Linux ARM64).
- `/config/node2.json`: Default configuration for Computer 2 (macOS M4).
- `/config/config.example.json`: Annotated template configuration.
- `/cmd/e2e-replication/main.go`: Pure Go source code implementing a handcrafted command registry (no spf13/Cobra).

---

## 3. Network Prerequisites

Ensure both computers are connected to the same local area network (LAN), Wi-Fi, or overlay network (e.g., Tailscale / WireGuard):

1. **Find IP Address on Computer 1 (Linux):**

   ```bash
   ip -brief address show
   # Or: hostname -I
   # Example output: 192.168.1.100
   ```

2. **Find IP Address on Computer 2 (macOS):**

   ```bash
   ipconfig getifaddr en0
   # Example output: 192.168.1.105
   ```

3. **Verify Connectivity:**
   From Computer 2, verify you can ping Computer 1:

   ```bash
   ping -c 3 192.168.1.100
   ```

4. **Firewall / Port Permissions:**
   Ensure TCP port `9001` (or whichever port you specify in the config) is allowed through the host firewall:
   - On Linux (`ufw`): `sudo ufw allow 9001/tcp`
   - On macOS: Allow incoming network connections if prompted by macOS gatekeeper.

---

## 4. Configuration Walkthrough

### Computer 1 (Linux ARM64) Configuration

Inspect or edit `/config/node1.json`:

```json
{
  "node_id": "node-linux-arm64",
  "listen_addr": "0.0.0.0:9001",
  "peer_addrs": [],
  "db_path": "data/node1.db",
  "swarm_topic": "sqlite-p2p-e2e-cluster",
  "enable_wal": true,
  "enable_crypto": false,
  "auto_sync": true
}
```

*Note: Node 1 acts as the initial listener; `peer_addrs` can remain empty.*

### Computer 2 (macOS M4) Configuration

Edit `/config/node2.json` and replace `127.0.0.1` with Computer 1's actual IP address:

```json
{
  "node_id": "node-macos-m4",
  "listen_addr": "0.0.0.0:9001",
  "peer_addrs": [
    "192.168.1.100:9001"
  ],
  "db_path": "data/node2.db",
  "swarm_topic": "sqlite-p2p-e2e-cluster",
  "enable_wal": true,
  "enable_crypto": false,
  "auto_sync": true
}
```

---

## 5. Step-by-Step Test Procedure

### Step 1: Start Node 1 on Computer 1 (Linux ARM64)

Open a terminal on Computer 1 and execute:

```bash
./run-e2e-test.sh -config config/node1.json
```

**Expected Output:**

```text
=================================================================
  sqlite-p2p E2E Replication Runner
  OS:           linux
  Architecture: aarch64
  Executable:   /workspace/bin/e2e-replication-linux-arm64
=================================================================
=================================================================
  sqlite-p2p Node [node-linux-arm64] Online
  Listening: 0.0.0.0:9001
  Database:  data/node1.db
  Cluster:   sqlite-p2p-e2e-cluster
=================================================================

Type 'help' for commands, or 'exit' to quit.
>
```

---

### Step 2: Start Node 2 on Computer 2 (macOS M4)

Open a terminal on Computer 2 and execute:

```bash
./run-e2e-test.sh -config config/node2.json
```

**Expected Output on Computer 2:**

```text
=================================================================
  sqlite-p2p E2E Replication Runner
  OS:           darwin
  Architecture: arm64
  Executable:   /workspace/bin/e2e-replication-darwin-arm64
=================================================================
=================================================================
  sqlite-p2p Node [node-macos-m4] Online
  Listening: 0.0.0.0:9001
  Database:  data/node2.db
  Cluster:   sqlite-p2p-e2e-cluster
=================================================================

[P2P] Connected to remote peer: 192.168.1.100:9001
Type 'help' for commands, or 'exit' to quit.
>
```

**Expected Notice on Computer 1:**

```text
[P2P] Inbound peer connection from: 192.168.1.105:51234
```

Verify connection status on either node by typing:

```text
> peers
Connected peers (1):
  - 192.168.1.100:9001
```

---

### Step 3: Test Real-Time Bidirectional Replication

#### A. Write on Computer 1 (Linux) -> Read on Computer 2 (macOS)

1. On **Computer 1 (Linux)**, insert a customer profile:

   ```text
   > put in.qzip.crm.customer:+91-9988776655 {"tier":"platinum","status":"active"}
   ```

   *Output:*

   ```text
   OK: Put "in.qzip.crm.customer:+91-9988776655" (38 bytes) in 1.2ms. Changeset broadcasted to peers.
   ```

2. Watch **Computer 2 (macOS)** terminal. You will immediately see:

   ```text
   time=... level=INFO msg="changeset received over replication" key=in.qzip.crm.customer:+91-9988776655 operation=1
   time=... level=INFO msg="remote changeset applied to local state" key=in.qzip.crm.customer:+91-9988776655 operation=1
   ```

3. Query the record on **Computer 2 (macOS)**:

   ```text
   > get in.qzip.crm.customer:+91-9988776655
   ```

   *Output:*

   ```text
   Record "in.qzip.crm.customer:+91-9988776655":
     Metadata: {"node":"node-linux-arm64","updated_at":...}
     Data:     {"tier":"platinum","status":"active"}
   ```

#### B. Write on Computer 2 (macOS) -> Read on Computer 1 (Linux)

1. On **Computer 2 (macOS)**, insert a new record:

   ```text
   > put in.qzip.crm.deal:DEAL-2026-001 {"title":"Enterprise License","value":50000}
   ```

2. On **Computer 1 (Linux)**, inspect the store:

   ```text
   > get in.qzip.crm.deal:DEAL-2026-001
   ```

   *Output:*

   ```text
   Record "in.qzip.crm.deal:DEAL-2026-001":
     Metadata: {"node":"node-macos-m4","updated_at":...}
     Data:     {"title":"Enterprise License","value":50000}
   ```

3. View all records replicated across both computers:

   ```text
   > list
   Found 2 record(s):
     [1] Key: in.qzip.crm.customer:+91-9988776655 | Data: {"tier":"platinum","status":"active"}
     [2] Key: in.qzip.crm.deal:DEAL-2026-001       | Data: {"title":"Enterprise License","value":50000}
   ```

---

### Step 4: Test Offline / Lagged Catch-up Reconciliation

This scenario verifies that a peer that was offline during mutations catches up completely as soon as it reconnects:

1. **Take Computer 2 (macOS) Offline:**
   Type `exit` in Computer 2's terminal to terminate the process.

2. **Perform mutations on Computer 1 (Linux) while Node 2 is offline:**

   ```text
   > put in.qzip.crm.lead:LEAD-001 {"source":"webinar","score":95}
   > put in.qzip.crm.lead:LEAD-002 {"source":"referral","score":80}
   > del in.qzip.crm.deal:DEAL-2026-001
   ```

3. **Bring Computer 2 (macOS) back online:**

   ```bash
   ./run-e2e-test.sh -config config/node2.json
   ```

4. **Verify automatic catch-up on Computer 2:**
   As soon as Node 2 connects, Node 1's automatic replay sync sends all missing feed changesets:

   ```text
   [P2P] Connected to remote peer: 192.168.1.100:9001
   time=... level=INFO msg="remote changeset applied to local state" key=in.qzip.crm.lead:LEAD-001
   time=... level=INFO msg="remote changeset applied to local state" key=in.qzip.crm.lead:LEAD-002
   time=... level=INFO msg="remote changeset applied to local state" key=in.qzip.crm.deal:DEAL-2026-001 operation=3
   ```

5. Confirm locally on Computer 2:

   ```text
   > list
   Found 3 record(s):
     [1] Key: in.qzip.crm.customer:+91-9988776655 | Data: {"tier":"platinum","status":"active"}
     [2] Key: in.qzip.crm.lead:LEAD-001           | Data: {"source":"webinar","score":95}
     [3] Key: in.qzip.crm.lead:LEAD-002           | Data: {"source":"referral","score":80}
   ```

   *Notice `in.qzip.crm.deal:DEAL-2026-001` was deleted, exactly reflecting remote state.*

---

### Step 5: Test Automated High-Throughput Replication

To test batch throughput and latency between the two computers:

On either computer, run:

```text
> auto-test 20
```

*Output:*

```text
Starting auto-test: writing 20 records from node [node-linux-arm64]...
Auto-test completed: 20 records written and replicated in 452.12ms (avg 22.6ms/op)
```

On the receiving computer, run:

```text
> list test:
Found 20 record(s):
  [1] Key: test:node-linux-arm64:item-1 | Data: Auto-generated payload #1...
  ...
  [20] Key: test:node-linux-arm64:item-20 | Data: Auto-generated payload #20...
```

---

## 6. Command Reference

The CLI uses a handcrafted command registry. Both direct commands and slash-command syntax are supported:

| Command | Slash Syntax | Description | Example |
| :--- | :--- | :--- | :--- |
| `put <key> <data>` | `/put` | Inserts/updates record, records changeset & broadcasts | `put user:10 {"name":"Sam"}` |
| `get <key>` | `/get` | Retrieves record from local SQLite database | `get user:10` |
| `del <key>` | `/del` | Deletes record and broadcasts `OpDelete` | `del user:10` |
| `list [prefix]` | `/list` | Lists stored records in local SQLite store | `list in.qzip.crm.` |
| `connect <addr>` | `/connect` | Connects dynamically to another peer | `connect 192.168.1.100:9001` |
| `sync` | `/sync` | Replays local feed changesets to all peers | `sync` |
| `peers` | `/peers` | Displays list of connected peers | `peers` |
| `status` | `/status` | Shows node ID, DB path, active peers, feed length | `status` |
| `auto-test [n]` | `/auto-test` | Runs automated batch write and replication test | `auto-test 50` |
| `help` | `/help` | Shows command reference | `help` |
| `exit` | `/exit` | Gracefully shuts down listener and database | `exit` |

---

## 7. Direct SQLite Inspection

Because `sqlite-p2p` writes standard SQLite databases with WAL mode enabled, you can independently verify the database state on either machine using the standard `sqlite3` CLI:

```bash
sqlite3 data/node1.db "SELECT key, metadata, data FROM crm_store;"
```

Or view SQLite WAL files:

```bash
ls -la data/
# node1.db
# node1.db-wal
# node1.db-shm
# node1.db.hypercore/
```

---

## 8. Troubleshooting

| Issue | Cause | Resolution |
| :--- | :--- | :--- |
| `connection refused` | Node 1 is not running or listening on a different port | Ensure Node 1 is started before Node 2 dials, or use the `connect <ip>:<port>` command once Node 1 is online. |
| `bind: address already in use` | Another process is using port `9001` | Change `"listen_addr": "0.0.0.0:9002"` in the JSON configuration, or specify `-listen 0.0.0.0:9002` on the CLI. |
| Replication changesets not arriving | Host firewall blocking incoming TCP connections | Check `sudo ufw status` on Linux or System Settings -> Network -> Firewall on macOS. |
| Need to rebuild binaries | Modified Go source code | Run `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/e2e-replication-linux-arm64 ./cmd/e2e-replication` and `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o bin/e2e-replication-darwin-arm64 ./cmd/e2e-replication`. |
