# Manual End-to-End Replication Test Guide (Pear P2P)

This guide describes how to perform manual end-to-end (E2E) replication testing between two separate computers (for example, a **Linux ARM64** device such as a Raspberry Pi 5 or AWS Graviton, and a **macOS Apple Silicon M4 / M3 / M2 / M1** workstation) using the **Pear / Holepunch P2P protocol**.

---

## 1. Why No Static TCP Port Needs to Be Exposed

Unlike traditional client-server databases or HTTP APIs, `sqlite-p2p` runs natively on the **Pear / Holepunch P2P stack** (`go-pear`):

1. **Zero Open / Forwarded Static Ports**:
   The engine defaults to `swarm_port: 0`. The operating system kernel automatically assigns a random dynamic ephemeral UDP & TCP port on launch. You **do not** need to expose, hardcode, or port-forward static ports (e.g. `9001`) in your router or firewall.
2. **Topic-Based Peer Rendezvous**:
   Peers find each other by announcing and discovering a shared 32-byte cryptographic **Swarm Topic** (`swarm_topic: "sqlite-p2p-e2e-cluster"`).
3. **HyperDHT & UDX Hole Punching**:
   Peer discovery and NAT traversal occur via HyperDHT and UDX.
4. **Encrypted Noise SecretStream**:
   All peer connections are authenticated and encrypted using Noise SecretStream sessions before any changesets are transmitted.

```
 +-----------------------------+                    +-----------------------------+
 |         Computer 1          |                    |         Computer 2          |
 |        (Linux ARM64)        |                    |         (macOS M4)          |
 |                             |                    |                             |
 | +-------------------------+ |    Pear Swarm Topic| +-------------------------+ |
 | | SQLite (node1.db)       | |   HyperDHT / Noise | | SQLite (node2.db)       | |
 | +-------------------------+ | <================> | +-------------------------+ |
 | | Hypercore Changeset Feed| |   (Dynamic Port)   | | Hypercore Changeset Feed| |
 | +-------------------------+ |                    | +-------------------------+ |
 | | Hyperswarm Manager      | |                    | | Hyperswarm Manager      | |
 +-----------------------------+                    +-----------------------------+
```

---

## 2. Included Artifacts & Directory Layout

- `/scripts/run-e2e-test.sh` (or symlink `/run-e2e-test.sh`): Universal shell runner that inspects `uname -s` and `uname -m`, detects the platform, and launches the appropriate executable.
- `/bin/e2e-replication-linux-arm64`: Precompiled standalone binary for Linux ARM64 / AArch64.
- `/bin/e2e-replication-darwin-arm64`: Precompiled standalone binary for macOS Apple Silicon (ARM64 / M1–M4).
- `/bin/dht-seed-linux-arm64` & `/bin/dht-seed-darwin-arm64`: Standalone HyperDHT bootstrap seed server binaries.
- `/config/node1.json`: Configuration for Computer 1 (Linux ARM64).
- `/config/node2.json`: Configuration for Computer 2 (macOS M4).
- `/config/config.example.json`: Annotated template configuration.
- `/cmd/e2e-replication/main.go`: Pure Go CLI entrypoint with a handcrafted command registry (no spf13/Cobra).
- `/cmd/dht-seed/main.go`: Standalone HyperDHT bootstrap server entrypoint.

---

## 3. DHT Bootstrapping & Public Server Architecture

### 3.1 Does it use a public server if "bootstrap" is not provided?

**No.** If `"bootstrap"` is empty (`[]` or omitted), `sqlite-p2p` does **not** connect to any public server on the internet:

1. **Air-Gapped & Offline by Default**: In `/internal/p2p/swarm.go` and `go-pear/pkg/hyperdht/dht.go`, an empty bootstrap array tells the HyperDHT subsystem to initialize with zero external bootstrap contacts. This guarantees complete network isolation, zero telemetry, zero data leakage, and total independence from third-party cloud infrastructure.
2. **Local Root / Seed Mode**: A node running with `bootstrap: []` operates as an independent root DHT node. It maintains its own local routing table and serves incoming DHT queries from other peers that connect to it.

### 3.2 Implemented Zero-Config Solutions

To solve peer discovery seamlessly across both local and internet environments without relying on third-party servers, three unified solutions are implemented:

#### Solution A: Automatic LAN UDP Broadcast Discovery (Zero Manual Config on Wi-Fi/LAN)

- When any node starts on a local network, it launches a background UDP broadcast beacon on port `49736` announcing its cluster topic and routable LAN IP (`NormalizeDHTAddr`).
- When a second node starts on the same Wi-Fi/LAN with `"bootstrap": []`, it performs a brief 600ms passive listen on `49736`. If an active cluster seed is heard, it **automatically bootstraps without requiring any manual IP or port entry**.

#### Solution B: Dedicated Standalone HyperDHT Seed Server (For Internet / WAN)

- For multi-office or public internet deployments where devices are behind distinct NATs, you can run the standalone seed daemon:

  ```bash
  ./bin/dht-seed-linux-arm64 -port 49737
  ```

- Deploy this to any cloud VM (AWS, DigitalOcean, Hetzner) and point your client nodes to it:

  ```json
  "bootstrap": ["dht.yourdomain.com:49737"]
  ```

#### Solution C: Dart (`dap`) Host IP Resolution & Unspecified Address Fix

- In `/home/innomon/B204-zone/dap/pear`:
  - `HyperDhtNode.resolveRoutableAddress()` now actively inspects non-loopback host network interfaces (`NetworkInterface.list()`).
  - Announcements no longer store `0.0.0.0` in `_topicStorage`.
  - `HolePunchCoordinator` candidate fallbacks now use routable host IP addresses instead of unspecified loopbacks.

### 3.3 Comparison with the Official Node.js Holepunch Ecosystem

| Dimension | Node.js Holepunch (`hyperswarm`) | Pure Go `sqlite-p2p` (`go-pear`) | Dart (`dap_pear`) |
| :--- | :--- | :--- | :--- |
| **Default Bootstrap** | Hardcoded public servers (`bootstrap1.hyperdht.org`) | **Empty by default (`[]`)** | **Empty by default (`[]`)** |
| **LAN Auto-Discovery** | Manual / Local subnet | **Automatic UDP broadcast beacon (port 49736)** | Resolved routable host IPs |
| **Internet Requirement** | Requires public internet | **Zero internet required** (LAN/air-gapped ready) | Zero internet required |
| **Dedicated Seed Node** | `hyperdht --bootstrap` | Standalone `./bin/dht-seed-*` binary | Supported via `HyperDhtNode.bind()` |

### 3.4 Deploying Global Discovery (Optional)

If you want devices to discover each other over the public internet without exchanging IP addresses, you can run an instance of `sqlite-p2p` (or a lightweight DHT seed node) on a public VPS with a static IP and list it in every client node's configuration:

```json
"bootstrap": [
  "203.0.113.10:49737"
]
```

### 3.5 Multi-Cluster, Multi-Topic Isolation & Shared Infrastructure

When running multiple independent decentralized applications on the same network (for example, `dap` for Keet chat/agents alongside `sqlite-p2p` for CRM database replication):

#### 1. Do You Need Multiple Beacon Servers?

**No.** The LAN discovery beacon is not a centralized server; it is an embedded peer-to-peer worker built directly into each node.

- Every node on the same local network transmits and listens on the same shared UDP broadcast port (`49736`).
- Discovery packets carry both a `cluster` tag and a `topic` hash.
- Nodes automatically filter out packets belonging to different clusters or topics. A `dap` node listening on `49736` silently discards `sqlite-p2p` beacons, preventing cross-talk.

#### 2. Can Multiple Clusters Share the Same DHT Bootstrap Server?

**Yes.** HyperDHT is completely **topic-agnostic**:

- A single HyperDHT bootstrap node (running on LAN or on a cloud VPS at `port 49737`) can serve as the routing rendezvous for hundreds of distinct clusters simultaneously.
- The DHT routing table indexes announcements purely by their 32-byte target topic hash (`targetId`).
- A node querying for `topic: "crm-topic"` only receives peer endpoints that announced `topic: "crm-topic"`. Peers querying `topic: "dap-chat"` only receive `dap-chat` contacts.

#### 3. Topic Separation as the Cryptographic Boundary

Always assign unique, meaningful topic strings to each cluster:

- **`sqlite-p2p` (CRM Database)**: `"sqlite-p2p-crm-cluster"`
- **`dap` (Keet Chat / Media)**: `"dap-agent-swarm-v1"`

Even when sharing the exact same bootstrap node or local Wi-Fi, peers on different topics remain cryptographically isolated and will never exchange changesets or establish connection sessions.

---

## 4. Configuration

### Computer 1 (Linux ARM64) Configuration

Inspect `/config/node1.json`:

```json
{
  "node_id": "node-linux-arm64",
  "swarm_topic": "sqlite-p2p-e2e-cluster",
  "swarm_port": 0,
  "bootstrap": [],
  "peer_addrs": [],
  "db_path": "data/node1.db",
  "enable_wal": true,
  "enable_crypto": false,
  "auto_sync": true
}
```

*`swarm_port: 0` instructs the OS kernel to select a dynamic ephemeral port.*

### Computer 2 (macOS M4) Configuration

When running on an isolated local network (LAN) without a global public DHT seed node, Node 2 points its `bootstrap` parameter to Node 1's dynamic DHT endpoint:

```json
{
  "node_id": "node-macos-m4",
  "swarm_topic": "sqlite-p2p-e2e-cluster",
  "swarm_port": 0,
  "bootstrap": [
    "<COMPUTER_1_IP>:<COMPUTER_1_DHT_PORT>"
  ],
  "peer_addrs": [],
  "db_path": "data/node2.db",
  "enable_wal": true,
  "enable_crypto": false,
  "auto_sync": true
}
```

*(Node 1 will print its dynamic DHT endpoint immediately upon startup, e.g. `192.168.1.100:43219`).*

---

## 5. Step-by-Step Test Procedure

### Step 1: Start Node 1 on Computer 1 (Linux ARM64)

Open a terminal on Computer 1 and execute:

```bash
./run-e2e-test.sh -config config/node1.json
```

**Expected Startup Output:**

```text
=================================================================
  sqlite-p2p E2E Replication Runner
  OS:           linux
  Architecture: aarch64
  Executable:   /workspace/bin/e2e-replication-linux-arm64
=================================================================
=================================================================
  sqlite-p2p Pear P2P Node [node-linux-arm64] Online
  Pear Protocol: Hyperswarm + HyperDHT + Noise SecretStream
  Swarm Topic:   sqlite-p2p-e2e-cluster
  DHT Address:   192.168.1.100:43219 (Dynamic port 43219 - no static port forward needed)
  Database:      data/node1.db
=================================================================

Type 'help' for commands, or 'exit' to quit.
>
```

Note the displayed **DHT Address** (e.g. `192.168.1.100:43219`).

---

### Step 2: Start Node 2 on Computer 2 (macOS M4)

On Computer 2, start Node 2 with Node 1's DHT address as bootstrap (or configure it in `config/node2.json`):

```bash
./run-e2e-test.sh -config config/node2.json -bootstrap 192.168.1.100:43219
```

**Expected Startup Output on Computer 2:**

```text
=================================================================
  sqlite-p2p E2E Replication Runner
  OS:           darwin
  Architecture: arm64
  Executable:   /workspace/bin/e2e-replication-darwin-arm64
=================================================================
=================================================================
  sqlite-p2p Pear P2P Node [node-macos-m4] Online
  Pear Protocol: Hyperswarm + HyperDHT + Noise SecretStream
  Swarm Topic:   sqlite-p2p-e2e-cluster
  DHT Address:   192.168.1.105:39182 (Dynamic port 39182 - no static port forward needed)
  Database:      data/node2.db
=================================================================

time=... level=INFO msg="peer connected" event=peer_connected
Type 'help' for commands, or 'exit' to quit.
>
```

Verify connection status on either node:

```text
> peers
Active Peer Connections: 1 (Swarm Mesh: 1)
```

---

### Step 3: Test Real-Time Bidirectional Replication

#### A. Write on Computer 1 (Linux) -> Read on Computer 2 (macOS)

1. On **Computer 1 (Linux)**, insert a customer record:

   ```text
   > put in.qzip.crm.customer:+91-9988776655 {"tier":"platinum","status":"active"}
   ```

   *Output:*

   ```text
   OK: Put "in.qzip.crm.customer:+91-9988776655" (38 bytes) in 1.1ms. Changeset broadcasted to peers.
   ```

2. Watch **Computer 2 (macOS)** terminal. The changeset is received over the Pear SecretStream channel:

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

1. On **Computer 2 (macOS)**:

   ```text
   > put in.qzip.crm.deal:DEAL-2026-001 {"title":"Enterprise Cloud","value":100000}
   ```

2. On **Computer 1 (Linux)**:

   ```text
   > get in.qzip.crm.deal:DEAL-2026-001
   ```

   *Output:*

   ```text
   Record "in.qzip.crm.deal:DEAL-2026-001":
     Metadata: {"node":"node-macos-m4","updated_at":...}
     Data:     {"title":"Enterprise Cloud","value":100000}
   ```

3. View all records replicated across both nodes:

   ```text
   > list
   Found 2 record(s):
     [1] Key: in.qzip.crm.customer:+91-9988776655 | Data: {"tier":"platinum","status":"active"}
     [2] Key: in.qzip.crm.deal:DEAL-2026-001       | Data: {"title":"Enterprise Cloud","value":100000}
   ```

---

### Step 4: Test Offline / Lagged Catch-up Reconciliation

1. **Take Computer 2 (macOS) Offline:**
   Type `exit` on Computer 2.

2. **Write records on Computer 1 (Linux) while Node 2 is offline:**

   ```text
   > put in.qzip.crm.lead:LEAD-001 {"source":"inbound","score":90}
   > put in.qzip.crm.lead:LEAD-002 {"source":"referral","score":85}
   > del in.qzip.crm.deal:DEAL-2026-001
   ```

3. **Restart Computer 2 (macOS):**

   ```bash
   ./run-e2e-test.sh -config config/node2.json -bootstrap 192.168.1.100:43219
   ```

4. **Verify automatic catch-up on Computer 2:**
   As soon as the Pear P2P swarm connection reconnects, Node 1's automatic feed replay broadcasts the missing changesets:

   ```text
   time=... level=INFO msg="remote changeset applied to local state" key=in.qzip.crm.lead:LEAD-001
   time=... level=INFO msg="remote changeset applied to local state" key=in.qzip.crm.lead:LEAD-002
   time=... level=INFO msg="remote changeset applied to local state" key=in.qzip.crm.deal:DEAL-2026-001 operation=3
   ```

5. Confirm locally on Computer 2:

   ```text
   > list
   Found 3 record(s):
     [1] Key: in.qzip.crm.customer:+91-9988776655 | Data: {"tier":"platinum","status":"active"}
     [2] Key: in.qzip.crm.lead:LEAD-001           | Data: {"source":"inbound","score":90}
     [3] Key: in.qzip.crm.lead:LEAD-002           | Data: {"source":"referral","score":85}
   ```

---

### Step 5: Automated Stress & Throughput Test

On either computer, run:

```text
> auto-test 50
```

*Output:*

```text
Starting auto-test: writing 50 records from node [node-linux-arm64]...
Auto-test completed: 50 records written and replicated in 1.12s (avg 22.4ms/op)
```

On the other computer, verify all 50 records arrived:

```text
> list test:
Found 50 record(s):
  ...
```

---

## 6. Command Reference

| Command | Slash Syntax | Description |
| :--- | :--- | :--- |
| `put <key> <data>` | `/put` | Inserts/updates record, writes to Hypercore feed, broadcasts to swarm |
| `get <key>` | `/get` | Retrieves record from local SQLite database |
| `del <key>` | `/del` | Deletes record and broadcasts `OpDelete` |
| `list [prefix]` | `/list` | Lists stored records in local SQLite store |
| `connect <addr>` | `/connect` | Establishes a direct encrypted Pear connection to `<ip>:<port>` |
| `sync` | `/sync` | Replays local feed changesets to all connected peers |
| `peers` | `/peers` | Displays active swarm peer connections |
| `status` | `/status` | Shows Pear Swarm topic, dynamic DHT endpoint, peer count, feed length |
| `auto-test [n]` | `/auto-test` | Runs automated batch write and replication benchmark |
| `help` | `/help` | Shows command reference |
| `exit` | `/exit` | Gracefully shuts down swarm and closes database |

---

## 7. Summary: Key Differences from Traditional Client-Server Networking

| Feature | Traditional TCP Server | Pear P2P (`sqlite-p2p`) |
| :--- | :--- | :--- |
| **Port Configuration** | Must open & expose fixed port (e.g. `9001`) | **Zero open ports** (`Port: 0` dynamic ephemeral) |
| **Firewall / NAT** | Requires port forwarding or public IP | Traverses NAT via **UDX hole punching** |
| **Peer Addressing** | Must know remote host IP in advance | Discovers peers via **32-byte Swarm Topic** |
| **Wire Security** | Requires custom TLS certificates | Built-in **Noise SecretStream encryption** |
| **Storage Convergence** | Master-replica replication | **Hypercore + LWW distributed convergence** |
