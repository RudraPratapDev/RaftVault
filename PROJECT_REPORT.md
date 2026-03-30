# Enterprise RaftKMS: A Comprehensive Technical Project Report

**Title**: Decentralized, Fault-Tolerant, Peer-to-Peer Key Management System (KMS) governed by the Raft Consensus Algorithm.  
**Domain**: Distributed Systems, Applied Cryptography, Network Security, Node Orchestration.

---

## 1. Abstract
As organizations scale heavily into geographically separated or localized air-gapped networks, the reliance on centralized cryptographic infrastructure (e.g., AWS KMS, Azure Key Vault, HashiCorp Vault) introduces severe critical vulnerabilities. If the centralized key controller experiences an outage or a network partition severs connectivity, applications immediately lose the ability to encrypt and decrypt sensitive data, stalling entire operational pipelines.

**RaftKMS** addresses these catastrophic single-points-of-failure by marrying modern authenticated cryptography (AES-256-GCM) with the mathematically proven **Raft Consensus Algorithm**. This project distributes key generation, IAM policies, and cryptographic operations across a peer-to-peer (P2P) mesh network of standard machines. Built entirely from scratch in Go (Golang) with zero external database dependencies, RaftKMS guarantees strong consistency, high availability, and tamper-proof cryptographic audit tracking even when extreme chaos (hardware failures, split-brain networks, extreme latency) is injected into the environment. 

This document provides a highly technical, deep-dive architectural post-mortem and report on the system's construction, operations, and verified capabilities.

---

## 2. Problem Statement & Motivation

### 2.1 The Flaw in Centralized Cloud KMS
Modern Key Management Systems are functionally centralized. Even when deployed in "Highly Available" cloud configurations, they rely on a localized quorum (like a single primary database cluster). 
1. **Single Point of Failure (SPOF):** If the primary key-server crashes, dependent microservices cannot perform symmetric or asymmetric operations.
2. **Network Partitions (Split-Brain):** If a network fractures, standard systems struggle to decide which half of the network holds the "truth" without causing data corruption or divergent cryptographic states.
3. **Audit Vulnerability:** In traditional systems, server administrators have theoretical access to tamper with localized log files to hide unauthorized cryptographic access or key rotations.
4. **Air-gapped / Localized mesh restrictions:** In defense, maritime, or highly secure localized environments (e.g., a shared organizational Wi-Fi node), establishing connections to cloud providers is impossible.

### 2.2 Objective
Design and implement a completely decentralized, enterprise-grade Key Management System capable of surviving multiple instantaneous machine crashes, healing network partitions, and providing absolute cryptographic uptime to organizations.

---

## 3. Technology Stack & Design Philosophy

The project adheres to a strict "Zero External Magic" philosophy. Instead of wrapping heavy framework dependencies, the core algorithms are written in pure idiomatic code to prove deep mastery of distributed systems.

### 3.1 Backend: Go (Golang 1.21+)
- **Why Go?** Distributed systems require massive parallel I/O handling (heartbeats, RPCs, state transitions). Go's native `goroutines` and `channels` allow the Raft engine to manage asynchronous leader selections without thread exhaustion.
- **Zero Third-Party Database Dependencies**: RaftKMS uses standard library `encoding/json` and `os` file operations to build its own atomic persistent data store. It doesn't rely on etcd, PostgreSQL, or Redis.
- **Cryptography**: Native `crypto/aes` and `crypto/rand` for unbreakable cryptographic entropy and symmetric encryption.

### 3.2 Frontend: React 18 & Vite
- **Why React?** The dashboard requires sub-millisecond DOM updates to render live HTTP packet visualizations. React's virtual DOM prevents layout thrashing during heavy network activity.
- **Vite Bundler**: Offers instantaneous Hot Module Replacement (HMR) and highly optimized production builds over legacy Webpack instances.
- **Vanilla CSS3 Glassmorphism**: For authentic UI/UX representation, the application relies on native CSS backdrop-filters and hardware-accelerated SVG animations, negating the need for bloated libraries like Tailwind or Bootstrap. 

### 3.3 Communication Protocols
- **HTTP/1.1 REST**: Native HTTP handlers manage the Inter-Process Communication (IPC). The Raft nodes use serialized JSON payloads for `RequestVote` and `AppendEntries` RPCs.
- **Server-Sent Events (SSE)**: Standard WebSockets are overkill and require two-way tunneling. SSE pushes binary state updates unidirectionally from the server to the browser immediately upon state mutation (e.g., a leader crash).

---

## 4. Repository Project Structure

The codebase is highly modularized, adopting a Domain-Driven Design layout.

```text
distProject/
├── cmd/
│   └── main.go                  # The primary injection point. Wires configurations to the engine.
├── configs/
│   └── network/                 # JSON configurations mapping physical IPs and node roles.
├── dashboard/
│   ├── src/
│   │   ├── App.jsx              # React Main View (SVG Mesh, Forms, Layout)
│   │   └── index.css            # Enterprise Deep Space UI stylesheets
│   └── package.json             # Frontend dependency tracking
├── internal/
│   ├── api/
│   │   └── server.go            # Multiplexer, REST endpoints, SSE Flusher, CORS headers.
│   ├── chaos/
│   │   └── chaos.go             # Injects latency, packet drops, kills, and split-brain testing.
│   ├── config/
│   │   └── config.go            # Parses `nodeX.json` into typed structs.
│   ├── kms/
│   │   └── kms.go               # The Cryptographic State Machine (AES-GCM, RBAC mapping).
│   ├── raft/
│   │   ├── events.go            # Thread-safe ring buffer capturing cluster mutations for SSE.
│   │   ├── raft.go              # The Consensus Brain (Leader election, log replication, commit loops).
│   │   ├── rpc.go               # HTTP client wrappers for inter-node consensus.
│   │   └── types.go             # Core Data Structures (Role enums, AppendEntries payloads).
│   └── storage/
│       └── storage.go           # Atomic JSON disk persistence.
├── generate_configs.sh          # Bash tool to map Local Area Network (LAN) IPs for live deployments.
├── start_cluster.sh             # Bash tool to simulate 3 nodes on port 5001, 5002, 5003 natively.
├── start_with_dashboard.sh      # Bash tool to bootstrap Go clusters and Vite UI simultaneously.
└── test_demo.sh                 # End-to-End integration test validating encryption survival on crashes.
```

---

## 5. System Architecture & Lifecycle

The architecture is loosely coupled. The Raft Core acts as a secure, serialized pipe to the KMS Engine.

```text
+-------------------------------------------------------------+
|               Enterprise React Dashboard UI                 |
+-----------------------------+-------------------------------+
                              | HTTP REST Header: Bearer <Key>
+-----------------------------v-------------------------------+
|                      API & Gateway Layer                    |
|                (Validates API Key via KMS Store)            |
+-----------------------------+-------------------------------+
                              | If Valid
+-----------------------------v-------------------------------+
|         Chaos Engine        |        Raft Consensus         |
| (Checks if Node is Killed)  |        Engine Core            |
+-----------------------------+---------------+---------------+
                                              |
+-----------------------------+               | Log Commits
|  Dynamic Membership Module  | <-------------+
|    (ADD_NODE / REMOVE_NODE) |               |
+-----------------------------+---------------v---------------+
|              Cryptographic KMS State Machine                |
+-----------------------------+-------------------------------+
                              | Serialization
+-----------------------------v-------------------------------+
|                    Disk Persistence Layer                   |
+-------------------------------------------------------------+
```

### 5.1 The Lifecycle of an Encryption Request
To understand the system, trace an HTTP `POST /kms/encrypt`:
1. **API Perimeter**: Client connects to Node 1 via `POST`. The API intercepts the request, maps the `Authorization: Bearer` token, and verifies the identity with the `kmsStore`.
2. **Leader Redirect**: If Node 1 is a Follower, it rejects the request, instantly mapping back the known Leader's address (`HTTP 307 Redirect` strategy).
3. **Consensus Pipeline**: The client reaches the Leader. The Leader generates an `AUDIT_LOG` and an `ENCRYPT` action, packaging them into `[]byte` payloads.
4. **Log Replication**: To prevent data loss if the leader crashes *right now*, the Leader issues concurrent `AppendEntries` RPCs to all peers.
5. **Commitment**: The peers write to their JSON persistent storages and reply `Success`. 
6. **State Machine Execution**: The Leader hits Quorum, moves the `commitIndex`, and passes the payload physically to the `KMS Engine`.
7. **Execution**: AES-GCM encryption occurs in RAM, and the ciphertext is returned to the user under a fraction of a second.

---

## 6. Deep Dive: The Raft Consensus Algorithm Implementation

Raft is mathematically optimized for safety and understandability.

### 6.1 Leader Election Mechanics & Splitting Votes
Every node boots into the `FOLLOWER` state.
- **Randomized Timers**: To prevent infinite "split vote" loops where multiple nodes endlessly tie during elections, every node implements a dynamically shifting countdown window (`1500ms` to `3000ms`). 
- **Elections**: The first timer to expire increments `CurrentTerm` and broadcasts `RequestVote`. A node requires `(N+1)/2+1` majority quorum to transition to `LEADER`.

### 6.2 The Safety Properties
RaftKMS enforces absolute mathematical safety:
1. **Election Safety**: Only one leader can exist per Term.
2. **Leader Append-Only**: Replicated logs are never deleted or rewritten by the Leader.
3. **Log Matching**: If two logs contain an entry with the same index and term, then the logs are completely identical in all preceding entries.

### 6.3 Resolving Split-Brain Networks
Using our custom `ChaosModule`, we simulated Network Partitions mapping isolated nodes in the environment. 
- **The Minority**: Attempts elections, inflating its `Term`, but fails to reach quorum. It fundamentally cannot process write requests, preventing the dual-truth "Brain Split" corruption.
- **The Majority**: Elects a new leader and scales flawlessly.
- **Healing via Appending**: Once the network is surgically restored, the Minority node broadcasts its inflated `Term`. The active Leader instantly steps down to investigate. It realizes the Minority node's log index is deeply behind, denies its vote, and forces the network back to reality by rewriting the Minority node's corrupted ledger. 

---

## 7. The Distributed KMS Engine & Cryptography

The Cryptographic engine sits firmly behind the Raft validation wall. 

### 7.1 AEAD (Authenticated Encryption with Associated Data)
The system utilizes **AES-256-GCM** (Galois/Counter Mode).
- **Block Cipher**: AES-256 requires a 256-bit secure string ensuring military-grade physical protection.
- **Initialization Vector (Nonce)**: Protects against identical plaintext matching by applying a unique 12-byte secure random payload.
- **Integrity**: GCM ensures authenticity. If a malicious attacker intercepts the database file and flips a single 0 to a 1 in a ciphertext array, the GCM tag unvalidates during decryption, and the KMS violently rejects the altered data.

### 7.2 Key Versioning & Backward Compatibility
Keys rotate constantly. An enterprise might encrypt a file on Day 1 (V1), rotate the key on Day 2 (V2), and decrypt the file on Day 3. 
- **Prefix Injection**: When encrypting, the KMS queries the latest valid KeyMaterial. The KMS takes the Version Integer (e.g., `2`), converts it to a purely numeric 4-byte `Big-Endian Uint32` array, and prepends it to the ciphertext payload.
- **Decryption Stripping**: Upon decrypt request, the system slices the first 4 bytes natively, dynamically parses the precise KeyVersion from the node's memory map, fetches the legacy Key Material, and decrypts the historic data flawlessly.

---

## 8. Enterprise Integration: IAM, RBAC, and Immutable Audits

A generic simulated consensus engine cannot be deployed to enterprise endpoints. Adding identity completely transformed the system.

### 8.1 Role-Based Access Control (RBAC)
We mapped a real-time tracking engine utilizing a `map[string]*User` structure natively synchronized via Raft across nodes.
- **`ADMIN` Roles**: Can create keys, rotate keys, add new users, and orchestrate underlying IP meshes natively.
- **`SERVICE` Roles**: Mapped exclusively to `/kms/encrypt` and `/kms/decrypt`. Restricting surface impact.

### 8.2 The Cryptographic Audit Ledger
For enterprise compliance (SOC2, ISO 27001), no cryptographic action occurs unmonitored.
- When any user decrypts data, the node inherently builds an `AuditLogPayload`.
- This fires an `AUDIT_LOG` command natively over the Raft TCP/HTTP network.
- As the log propagates to Quorum, an undeniable append-only history is formed tracking `[{Username: admin, Action: ENCRYPT, Timestamp: 12:45pm, KeyID: "MainKey"}]`. A rouge user cannot delete a localized flat file to hide forensic traces—because their local machine's log will immediately be overwritten by the cluster majority during the next Heartbeat.

### 8.3 On-the-Fly Dynamic Clustering
Traditional networks define node counts hardcoded inside Docker Containers. RaftKMS features **Dynamic Membership Changes**.
- An Admin utilizes an HTTP request (`POST /cluster/addNode`) with a completely foreign machine IP.
- The `ADD_NODE` payload intercepts the `ApplyLoop` of the core Consensus engine.
- Instead of encrypting data, the core Raft loops instantly pause, mutate their own underlying `[]string Peers` array, recalculate fractional majority bounds, and dynamically open replication lines allowing true horizontal laptop mesh computing.

---

## 9. Performance Tuning & Optimizations
- **Global `sync.RWMutex` Segregation**: Reading the KMS map does not block Raft Consensus checks. Heavy encryption operations are mapped strictly behind discrete Reader/Writer locks minimizing processor contention.
- **SSE Thread-Safe Ring Buffers**: Realtime visualizers crash if HTTP arrays grow to millions of events. A bounded static integer `maxSize = 250` ring buffer forces native Go garbage collection natively without thrashing server memory.

---

## 10. API Reference

All requests must supply `Authorization: Bearer <Your-API-Key>`.

### KMS Cryptography Endpoints
- `POST /kms/createKey` *(Admin)*: Bootstraps 256-bit symmetric entropy.
- `POST /kms/encrypt` *(Auth)*: Consumes `{key_id, plaintext}` and returns versioned base64 payload.
- `POST /kms/decrypt` *(Auth)*: Maps version prefix and decrypts into utf-8 strings.
- `POST /kms/rotateKey` *(Admin)*: Append-only backwards-compatible key material updates.

### IAM & Audit
- `POST /kms/createUser` *(Admin)*: Defines `<username>` and maps `admin | service` restrictions.
- `GET /kms/auditLog` *(Auth)*: Dumps the entire Immutable Replicated consensus history.

### Cluster Management
- `POST /cluster/addNode` *(Admin)*: Expand the mesh dynamically.
- `POST /chaos/partition` *(Admin)*: Introduce Split-Brain isolation targeting peers.

---

## 11. Concluding Remarks
RaftKMS successfully proves that absolute cryptographic parity natively scales without dependencies. By fundamentally understanding algorithmic timing, `RWMutex` locks, Authenticated Cipher mechanisms, and UI Websocket modeling, this system converts a simple localized Wi-Fi hotspot of 3 laptops into a mission-critical, self-healing, data-secure enterprise vault.
