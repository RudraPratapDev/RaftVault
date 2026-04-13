# RaftVault — Distributed P2P Key Management System

RaftVault is a **fault-tolerant, fully decentralized Key Management System (KMS)** built from scratch in Go. It implements the [Raft consensus algorithm](https://raft.github.io/) to turn a group of standard laptops into a self-healing cryptographic cluster — no AWS KMS, no HashiCorp Vault, no centralized infrastructure required.

## Table of Contents

- [What It Does](#what-it-does)
- [Architecture](#architecture)
- [Key Features](#key-features)
- [Project Structure](#project-structure)
- [Prerequisites](#prerequisites)
- [Quick Start (Local 3-Node Cluster)](#quick-start-local-3-node-cluster)
- [Running the Dashboard](#running-the-dashboard)
- [Multi-Machine (Wi-Fi) Setup](#multi-machine-wi-fi-setup)
- [API Reference](#api-reference)
- [Configuration](#configuration)
- [Testing](#testing)
- [How Raft Works Here](#how-raft-works-here)

---

## What It Does

RaftVault lets you create a cluster of nodes (one per machine or process) that collectively manage cryptographic keys. Every write operation (create key, rotate key, encrypt data) is first committed to a distributed Raft log that a majority of nodes must confirm before the operation is applied. This means:

- **No single point of failure** — if the leader node dies, the remaining nodes hold a new election and the cluster keeps running.
- **No data loss** — every operation is persisted to disk on a majority of nodes before it is acknowledged.
- **Strong consistency** — all nodes eventually converge to the same key store state.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    HTTP API Server (api)                     │
│  REST endpoints · CORS · API-key auth · SSE event stream    │
└────────────┬────────────────────────────────────────────────┘
             │
┌────────────▼────────────────────────────────────────────────┐
│              Raft Consensus Engine (raft)                    │
│  Leader election · Log replication · Heartbeat (500 ms)     │
└────────────┬────────────────────────────────────────────────┘
             │
┌────────────▼────────────────────────────────────────────────┐
│             KMS State Machine (kms)                          │
│  Key CRUD · AES-256-GCM encryption · RBAC · Audit ledger    │
└────────────┬────────────────────────────────────────────────┘
             │
┌────────────▼────────────────────────────────────────────────┐
│   Storage (storage)              Chaos Module (chaos)        │
│   JSON files on disk             Fault injection for testing │
└─────────────────────────────────────────────────────────────┘
```

**Request lifecycle example** — encrypting a payload:

1. Client `POST /kms/encrypt` → API Server validates the Bearer API key.
2. API Server calls `raft.SubmitCommand("ENCRYPT", …)` — only the **leader** accepts writes.
3. Raft leader appends the entry to its log and replicates it to all followers via `POST /raft/appendEntries`.
4. Once a **majority** confirms, the entry is committed.
5. Raft calls `kms.Apply(entry)` on every node — the state machine updates in-memory state and writes the audit record.
6. The dashboard, subscribed via SSE (`GET /events`), receives a live event notification.

---

## Key Features

| Feature | Details |
|---|---|
| **Raft Consensus** | Leader election with randomized timeouts, log replication, automatic failover |
| **AES-256-GCM Encryption** | Authenticated encryption for every payload; keys stored in-memory, backed by Raft log |
| **Key Versioning** | Keys are rotated (not replaced); old versions are retained for backward-compatible decryption |
| **RBAC** | `admin` role: full cluster management; `service` role: encrypt/decrypt only |
| **Cryptographic Audit Ledger** | Every encrypt/decrypt operation is written to the Raft log as an immutable audit entry |
| **Dynamic Cluster Membership** | Nodes can be added or removed at runtime via the API |
| **Split-Brain Simulator** | Chaos module supports kill, revive, artificial delay, drop rate, and network partition injection |
| **React Dashboard** | Real-time glassmorphism UI with animated SVG network topology and live SSE data packets |
| **Zero external Go dependencies** | Entire backend uses only the Go standard library |

---

## Project Structure

```
RaftVault/
├── cmd/
│   └── main.go                 # Entry point — wires all components together
├── internal/
│   ├── api/
│   │   └── server.go           # HTTP server, all REST handlers, auth middleware
│   ├── raft/
│   │   ├── raft.go             # Core Raft algorithm (election, replication, commit)
│   │   ├── types.go            # RPC message structs (RequestVote, AppendEntries)
│   │   ├── rpc.go              # HTTP-based RPC client
│   │   └── events.go           # Ring-buffer event log + pub/sub for SSE
│   ├── kms/
│   │   └── kms.go              # KMS state machine, key/user management, encryption
│   ├── storage/
│   │   └── storage.go          # Persistent Raft state (JSON files on disk)
│   ├── chaos/
│   │   └── chaos.go            # Fault injection module
│   └── config/
│       └── config.go           # JSON config loader with sensible defaults
├── dashboard/                  # React + Vite frontend
│   └── src/
│       ├── App.jsx             # Main component: network topology, KMS panel
│       └── App.css             # Glassmorphism styling
├── configs/
│   ├── node1.json              # localhost:5001
│   ├── node2.json              # localhost:5002
│   ├── node3.json              # localhost:5003
│   └── network/                # Generated configs for multi-machine setups
├── data/                       # Persistent node state (created at runtime)
├── logs/                       # Per-node log output (created at runtime)
├── bin/                        # Compiled binaries (created by build)
├── generate_configs.sh         # Interactive script to generate network configs
├── start_cluster.sh            # Start a local 3-node cluster
├── start_with_dashboard.sh     # Start cluster + React dashboard
├── test_demo.sh                # Full integration test
└── DEMO_GUIDE.md               # Step-by-step live demo instructions
```

---

## Prerequisites

- **Go 1.25+** (no external modules needed)
- **Node.js 18+** and **npm** (dashboard only)

---

## Quick Start (Local 3-Node Cluster)

```bash
# 1. Build
go build -o bin/raft-kms ./cmd/main.go

# 2. Start a 3-node cluster (each node in its own terminal)
./bin/raft-kms --config configs/node1.json
./bin/raft-kms --config configs/node2.json
./bin/raft-kms --config configs/node3.json

# Or use the helper script:
./start_cluster.sh
```

The nodes will elect a leader within a few seconds. You can verify with:

```bash
curl http://localhost:5001/status
```

### First steps via the API

```bash
# Create a key (admin-secret-key is the default bootstrap admin credential)
curl -X POST http://localhost:5001/kms/createKey \
  -H "Authorization: Bearer admin-secret-key" \
  -H "Content-Type: application/json" \
  -d '{"key_id": "my-key"}'

# Encrypt a message
curl -X POST http://localhost:5001/kms/encrypt \
  -H "Authorization: Bearer admin-secret-key" \
  -H "Content-Type: application/json" \
  -d '{"key_id": "my-key", "plaintext": "hello world"}'

# Decrypt the ciphertext returned above
curl -X POST http://localhost:5001/kms/decrypt \
  -H "Authorization: Bearer admin-secret-key" \
  -H "Content-Type: application/json" \
  -d '{"key_id": "my-key", "ciphertext": "<ciphertext from above>"}'
```

---

## Running the Dashboard

```bash
./start_with_dashboard.sh
# or manually:
cd dashboard && npm install && npm run dev
```

Open `http://localhost:5173`. On the login screen:

- **Nodes:** `localhost:5001, localhost:5002, localhost:5003`
- **API Key:** `admin-secret-key`

The dashboard shows:
- An animated SVG network map with glowing orbs for each node and live data-packet animations representing Raft heartbeats.
- Real-time role labels (LEADER / FOLLOWER) that update automatically during failover.
- A KMS panel for creating keys, encrypting/decrypting data, and inspecting the audit log.

---

## Multi-Machine (Wi-Fi) Setup

To run across multiple laptops on the same network:

```bash
# 1. Generate per-machine configs (prompts for each laptop's LAN IP)
./generate_configs.sh

# 2. Distribute the project folder (including configs/network/) to each laptop.

# 3. Each laptop runs its own node:
./bin/raft-kms --config configs/network/node1.json   # laptop 1
./bin/raft-kms --config configs/network/node2.json   # laptop 2
./bin/raft-kms --config configs/network/node3.json   # laptop 3

# 4. Point the dashboard at the real IPs:
#    Nodes: 192.168.1.10:5001, 192.168.1.11:5001, 192.168.1.12:5001
```

See [DEMO_GUIDE.md](DEMO_GUIDE.md) for a full walkthrough including the live failover demonstration.

---

## API Reference

All endpoints accept and return JSON. Authenticated endpoints require `Authorization: Bearer <api-key>`.

### Raft internals (node-to-node)

| Method | Path | Description |
|---|---|---|
| POST | `/raft/requestVote` | Candidate requests a vote from this node |
| POST | `/raft/appendEntries` | Leader sends heartbeat or log entries to followers |
| GET | `/raft/log` | Returns this node's Raft log (CORS, no auth) |

### Cluster status

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/status` | None | Node status (role, term, leader, commit index) |
| GET | `/cluster/status` | None | Aggregated status of all known nodes |
| POST | `/cluster/addNode` | Admin | Add a node to the cluster |
| POST | `/cluster/removeNode` | Admin | Remove a node from the cluster |

### KMS — Admin operations

| Method | Path | Description |
|---|---|---|
| POST | `/kms/createKey` | Create a new AES-256 key |
| POST | `/kms/deleteKey` | Soft-delete a key |
| POST | `/kms/rotateKey` | Rotate a key (adds a new version, keeps old ones) |
| POST | `/kms/createUser` | Create a user with a role and API key |
| POST | `/kms/deleteUser` | Delete a user |

### KMS — Authenticated operations

| Method | Path | Description |
|---|---|---|
| POST | `/kms/encrypt` | Encrypt plaintext with a named key |
| POST | `/kms/decrypt` | Decrypt ciphertext with a named key |
| GET | `/kms/getKey` | Get metadata for a specific key |
| GET | `/kms/listKeys` | List all keys |
| GET | `/kms/listUsers` | List all users |
| GET | `/kms/auditLog` | View the cryptographic audit ledger |

### Events (Server-Sent Events)

| Method | Path | Description |
|---|---|---|
| GET | `/events` | Live SSE stream of cluster events for the dashboard |
| GET | `/events/history` | Last N events from the ring-buffer |

### Chaos / fault injection (no auth required)

| Method | Path | Description |
|---|---|---|
| POST | `/chaos/kill` | Mark this node as killed (stops processing RPCs) |
| POST | `/chaos/revive` | Revive a killed node |
| POST | `/chaos/delay` | Inject artificial latency into RPC responses |
| POST | `/chaos/drop` | Set a packet drop rate (0.0–1.0) |
| POST | `/chaos/partition` | Simulate a network partition from specific peers |

---

## Configuration

Each node is configured with a JSON file:

```json
{
  "node_id": "node1",
  "address": "localhost:5001",
  "peers": ["localhost:5002", "localhost:5003"],
  "data_dir": "./data/node1",
  "election_timeout_min_ms": 150,
  "election_timeout_max_ms": 300,
  "heartbeat_interval_ms": 50
}
```

| Field | Default | Description |
|---|---|---|
| `node_id` | *(required)* | Unique identifier for this node |
| `address` | *(required)* | `host:port` this node listens on |
| `peers` | `[]` | Addresses of all other nodes in the cluster |
| `data_dir` | *(required)* | Directory for persistent Raft state |
| `election_timeout_min_ms` | `150` | Minimum election timeout (ms) |
| `election_timeout_max_ms` | `300` | Maximum election timeout (ms) |
| `heartbeat_interval_ms` | `50` | Leader heartbeat interval (ms) |

---

## Testing

```bash
# Run the full integration test (starts its own cluster automatically)
./test_demo.sh
```

The test script exercises:
- ✅ Leader election
- ✅ Key creation and log replication across followers
- ✅ AES-256-GCM encryption and decryption
- ✅ Key rotation with backward-compatible decryption
- ✅ Leader kill via the chaos API
- ✅ Automatic re-election by the surviving nodes
- ✅ Write capability after failover
- ✅ Log consistency across all nodes

---

## How Raft Works Here

Raft guarantees that all nodes agree on the same sequence of commands even in the presence of failures.

1. **Leader election** — On startup every node is a *Follower*. If a follower doesn't hear a heartbeat from a leader within its randomized election timeout (150–300 ms by default, configurable per node), it becomes a *Candidate*, increments its term, and sends `RequestVote` RPCs to all peers. The first candidate to collect a majority of votes becomes the new *Leader*.

2. **Log replication** — Only the Leader accepts client writes. It appends the new entry to its log with the current term number, then broadcasts `AppendEntries` RPCs to all followers. Once a majority acknowledges the entry, it is *committed* and the Leader notifies the client.

3. **State machine application** — After an entry is committed, every node calls `kms.Apply(entry)` which updates the in-memory KMS state (creates/deletes/rotates keys, records audit entries, etc.) and persists the Raft state to disk.

4. **Fault tolerance** — If the Leader crashes, followers detect the missing heartbeats and hold a new election. The cluster resumes operation as soon as a new leader is elected (typically within one election timeout window).
