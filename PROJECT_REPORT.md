# RaftKMS — Project Report

**Full Title**: A Cryptographically Hardened, Fault-Tolerant, Peer-to-Peer Key Management System with Envelope Encryption, HKDF Key Derivation, Tamper-Evident Audit Chains, RSA-OAEP Key Transport, and ML-Based Randomness Auditing  
**Domain**: Network and Information Security · Applied Cryptography · Distributed Systems

---

## 1. Abstract

RaftKMS is an enterprise-grade Key Management System built entirely from scratch, without external databases or cloud dependencies. It combines two independently complex engineering domains: applied cryptography and distributed consensus.

On the cryptographic side, the system implements envelope encryption (KEK/DEK separation), HKDF-SHA256 key derivation with context binding, AES-256-GCM authenticated encryption, HMAC-SHA256 tamper-evident audit chains, and RSA-OAEP asymmetric key wrapping for secure export. These are not textbook exercises — they are the exact cryptographic patterns used by AWS KMS, Google Cloud KMS, and HashiCorp Vault.

On the distributed systems side, the system implements the Raft consensus algorithm in pure Go, guaranteeing that all cryptographic state is replicated across a majority quorum before any operation is acknowledged. The system survives node crashes, network partitions, and leader failures with zero data loss and automatic recovery.

A companion Python subsystem (RandSentinel) validates the statistical quality of generated key material using a trained Random Forest classifier, outperforming the NIST SP 800-22 test suite at all practical key lengths.

---

## 2. Problem Statement

### 2.1 Why Existing KMS Solutions Are Insufficient

**Centralized cloud KMS (AWS KMS, Azure Key Vault)**:
- Require internet connectivity — unusable in air-gapped or restricted networks
- Introduce a hard external dependency — if the cloud endpoint is unreachable, applications cannot encrypt or decrypt
- Provide no visibility into the cryptographic implementation — the system is a black box

**Self-hosted single-server KMS (HashiCorp Vault, standalone)**:
- Single point of failure — one server crash takes down all cryptographic operations
- Susceptible to split-brain if deployed naively in a cluster without proper consensus
- Audit logs stored locally — a privileged insider can modify or delete them

**Academic distributed systems projects**:
- Typically implement consensus without real cryptography — keys are raw random bytes, no envelope encryption, no KDF
- Audit trails are flat in-memory lists with no tamper-evidence
- No key export mechanism — keys cannot be securely transferred to external parties

RaftKMS addresses all of these gaps.

### 2.2 Design Goals

1. **Cryptographic authenticity**: Implement the same cryptographic patterns used in production KMS systems — envelope encryption, HKDF, AEAD, RSA-OAEP.
2. **Operational resilience**: Survive node failures and network partitions without data loss or service interruption.
3. **Tamper-evident auditing**: Make the audit trail cryptographically verifiable — any modification breaks the chain.
4. **Zero external dependencies**: No external databases, no cloud APIs, no third-party consensus libraries.
5. **Randomness validation**: Provide ML-based verification that generated key material meets cryptographic quality standards.

---

## 3. Technology Stack

### 3.1 Backend: Go (Golang 1.21+)

Go was chosen for three reasons specific to this system:

- **Concurrency model**: Raft requires managing multiple concurrent goroutines — election timers, heartbeat senders, RPC handlers, log replication loops. Go's goroutines and channels make this natural and safe.
- **Standard library cryptography**: Go's `crypto/aes`, `crypto/cipher`, `crypto/rand`, `crypto/rsa`, `crypto/hmac`, and `crypto/sha256` packages provide all required primitives without external dependencies. The implementations are audited and constant-time where required.
- **Simplicity**: The entire system — Raft engine, KMS state machine, API server, storage layer — is implemented in approximately 2,000 lines of Go with zero third-party dependencies beyond the standard library.

### 3.2 Python ML Subsystem

The randomness auditing subsystem is implemented in Python using scikit-learn, NumPy, SciPy, and PyCryptodome. Python was chosen for this component because the ML ecosystem (scikit-learn, XGBoost, joblib) is mature and the feature extraction code benefits from NumPy's vectorized operations.

The subsystem exposes a Flask HTTP API on port 7777, accepting base64-encoded key material and returning a classification result (SECURE / WEAK) with a confidence score.

### 3.3 Frontend: React 18 + Vite

The dashboard provides real-time visualization of cluster state and a full cryptographic workspace. React's virtual DOM handles the high-frequency SSE updates (node state changes, packet animations) without layout thrashing. Vite provides fast development builds and optimized production output.

---

## 4. Repository Structure

```
raft-kms/
├── cmd/
│   └── main.go                    # Entry point — wires config, Raft, KMS, API
├── configs/network/
│   ├── node1.json                 # Node address and peer list
│   ├── node2.json
│   └── node3.json
├── internal/
│   ├── api/
│   │   └── server.go              # HTTP handlers, auth middleware, SSE, RSA-OAEP export
│   ├── chaos/
│   │   └── chaos.go               # Latency injection, partition simulation, kill switch
│   ├── config/
│   │   └── config.go              # JSON config parsing
│   ├── kms/
│   │   └── kms.go                 # Envelope encryption, HKDF, HMAC audit chain, RBAC
│   ├── raft/
│   │   ├── raft.go                # Leader election, log replication, commit loop
│   │   ├── rpc.go                 # HTTP client wrappers for inter-node RPCs
│   │   ├── types.go               # LogEntry, AppendEntries, RequestVote structs
│   │   └── events.go              # Thread-safe ring buffer for SSE telemetry
│   └── storage/
│       └── storage.go             # Atomic JSON persistence
├── crypto/
│   ├── server.py                  # Flask API serving trained ML models
│   ├── backend.py                 # RandSentinel feature extraction and inference
│   ├── feature_script.py          # 18-feature extraction pipeline
│   ├── comparision.py             # Model evaluation vs NIST baseline
│   ├── RandomForest_len256.pkl    # Trained model for 256-bit sequences
│   └── RandomForest_len4096.pkl   # Trained model for 4096-bit sequences
├── dashboard/src/
│   ├── App.jsx                    # Main dashboard — topology, crypto workspace, audit ledger
│   └── index.css                  # Deep-space glassmorphism UI
├── kms-app/src/
│   └── components/                # Standalone KMS client application
├── start_with_dashboard.sh        # Starts all 3 nodes + React dashboard
├── reset.sh                       # Wipes all Raft state and audit records
└── test_demo.sh                   # End-to-end integration test
```

---

## 5. Cryptographic Implementation

### 5.1 Envelope Encryption

Envelope encryption is the defining cryptographic pattern of this system. It separates the key hierarchy into two layers:

**Key Encryption Key (KEK)**: Derived from the master secret using HKDF. Never stored directly — recomputed on demand. Never used to encrypt user data.

**Data Encryption Key (DEK)**: Generated fresh for every single encryption operation using `crypto/rand`. Used once to encrypt the plaintext, then discarded. The DEK is stored only in its wrapped (KEK-encrypted) form alongside the ciphertext.

**Why this matters**:
- The master key never touches plaintext. Even if an attacker observes the master key, they cannot decrypt past ciphertexts without also obtaining the wrapped DEK for each one.
- Key rotation is cheap. Rotating the master key only requires re-wrapping the DEKs — the ciphertexts themselves do not need to be re-encrypted.
- Each ciphertext is independently protected. Compromising one DEK does not compromise any other ciphertext.

**Wire format**:
```
[ Version: 4 bytes Big-Endian uint32 ]
[ WrappedDEKLen: 2 bytes Big-Endian uint16 ]
[ WrappedDEK: variable length ]
[ Ciphertext: variable length ]
→ entire payload base64-encoded for transport
```

The version prefix enables backward compatibility: a ciphertext encrypted under key version 1 can be decrypted after the key has been rotated to version 5, because the version field tells the decryption routine exactly which key version to use for KEK derivation.

### 5.2 HKDF Key Derivation

HKDF (HMAC-based Key Derivation Function, RFC 5869) is the standard for deriving cryptographic keys from a master secret. RaftKMS implements it from scratch using Go's `crypto/hmac` and `crypto/sha256`.

**Two-phase construction**:

*Extract phase* — converts the master secret into a uniformly distributed pseudorandom key:
```
PRK = HMAC-SHA256(salt, master_secret)
```

*Expand phase* — derives output key material of the desired length:
```
T(i) = HMAC-SHA256(PRK, T(i-1) || info || counter_byte)
OKM  = T(1) || T(2) || ... truncated to desired_length
```

**Context binding**: The `info` parameter encodes `"keyID:version:createdAt"`. This means the KEK is uniquely bound to a specific key at a specific version created at a specific time. The same master secret with different context produces a completely different KEK — there is no way to use a KEK derived for `key-A:v1` to unwrap a DEK that was wrapped for `key-B:v2`.

This is the same construction used in TLS 1.3's key schedule and in NIST SP 800-56C key derivation.

### 5.3 AES-256-GCM

All symmetric encryption uses AES-256-GCM. GCM (Galois/Counter Mode) is an AEAD mode — it provides both encryption and authentication in a single pass.

**Properties**:
- 256-bit key — 2²⁵⁶ possible keys, computationally infeasible to brute-force
- 12-byte random nonce per operation — encrypting the same plaintext twice produces different ciphertexts
- 128-bit authentication tag — any modification to the ciphertext causes decryption to fail with an explicit error, not silently produce wrong output

The authentication tag is what makes GCM suitable for a KMS. If an attacker modifies a stored ciphertext (e.g., in a database breach), the GCM tag verification fails on decryption. The system rejects the tampered data rather than decrypting it to garbage.

### 5.4 HMAC-SHA256 Tamper-Evident Audit Chain

Every cryptographic operation — key creation, encryption, decryption, key rotation, user management — generates an audit entry. These entries are not stored as a flat list. They form a hash chain where each entry's hash depends on all previous entries.

**Chain structure**:
```
Entry 0: PreviousHash = "0000...0000" (genesis)
         CurrentHash  = HMAC-SHA256(key, "0000...0000|timestamp|user|action|keyID")

Entry 1: PreviousHash = Entry 0's CurrentHash
         CurrentHash  = HMAC-SHA256(key, Entry0Hash|timestamp|user|action|keyID)

Entry N: PreviousHash = Entry N-1's CurrentHash
         CurrentHash  = HMAC-SHA256(key, EntryN-1Hash|timestamp|user|action|keyID)
```

**Tamper detection**: If an attacker modifies entry 3 (e.g., to hide an unauthorized decryption), the HMAC of entry 3 changes. This invalidates the `PreviousHash` field of entry 4, which changes entry 4's HMAC, which invalidates entry 5, and so on. The entire chain from the modified entry forward is broken. Any audit verification tool will immediately detect the inconsistency.

**Distributed reinforcement**: Because audit entries are Raft commands, they are replicated to all nodes. A rogue administrator cannot silently modify their local audit log — the cluster majority will overwrite it on the next heartbeat synchronization.

### 5.5 RSA-OAEP Key Export

When an external party needs a copy of a symmetric key — for example, to decrypt archived data or to establish a shared secret — the key must be transported without ever appearing in plaintext on the network.

RSA-OAEP (Optimal Asymmetric Encryption Padding) is the standard for this. The recipient generates an RSA-2048 key pair, sends their public key to the KMS, and receives back the AES key material encrypted under their public key. Only the holder of the corresponding private key can recover the AES key.

**Endpoint**: `POST /kms/exportKey`  
**Request**: `{ "key_id": "...", "public_key": "-----BEGIN PUBLIC KEY-----\n..." }`  
**Response**: `{ "wrapped_key": "<base64>" }`

The server accepts both PKIX (`BEGIN PUBLIC KEY`) and PKCS1 (`BEGIN RSA PUBLIC KEY`) PEM formats. The hash function used in OAEP is SHA-256. The operation is logged to the audit chain.

This is the KEM (Key Encapsulation Mechanism) pattern — the symmetric key is encapsulated inside an asymmetric envelope for transport, then decapsulated by the recipient.

---

## 6. Distributed Consensus: Raft

### 6.1 The Core Problem

Without consensus, a distributed KMS is dangerous. If two nodes independently accept write requests during a network partition, they can generate different keys under the same ID, creating a split-brain state where different nodes have different answers to "what is the current key material for key X?" This silently corrupts the cryptographic state.

Raft solves this by ensuring that a write is only acknowledged after it has been durably replicated to a majority of nodes. A minority partition cannot reach quorum, so it cannot process writes. There is always exactly one consistent view of the cryptographic state.

### 6.2 Leader Election

Nodes start as Followers. Each node sets a randomized election timeout (1500–3000ms). If no heartbeat arrives before the timeout, the node becomes a Candidate:

1. Increments `CurrentTerm`
2. Votes for itself
3. Sends `RequestVote` RPCs to all peers

A peer grants its vote if:
- The candidate's term ≥ the peer's current term
- The peer has not voted in this term
- The candidate's log is at least as up-to-date as the peer's (prevents electing a node with stale data)

A candidate that receives `(N/2)+1` votes becomes Leader and immediately sends heartbeats to suppress new elections.

The randomized timeout window is the key to avoiding split votes. If two nodes time out simultaneously and split the vote, they both reset to new random timeouts. The probability of repeated splits decreases exponentially with each round.

### 6.3 Log Replication

Every write operation is a `LogEntry`:
```go
type LogEntry struct {
    Term    int
    Index   int
    Command storage.Command  // serialized []byte payload
}
```

The Leader appends the entry locally and sends `AppendEntries` RPCs to all Followers concurrently. Each RPC includes a `prevLogIndex` and `prevLogTerm` that the Follower uses to verify log consistency (the Log Matching Property). Upon receiving a majority of acknowledgements, the Leader commits the entry and executes it against the KMS state machine.

### 6.4 Persistence

Each node persists its Raft state (current term, voted-for, log entries) to a JSON file on disk. On restart, the node reloads this state and resumes from where it left off. This means a crashed node that restarts will catch up to the current cluster state by receiving missing log entries from the Leader.

### 6.5 Dynamic Cluster Membership

The system supports adding new nodes at runtime via `POST /cluster/addNode`. The `ADD_NODE` command is processed through the Raft pipeline — it is replicated to all nodes before taking effect. Upon commitment, each node updates its peer list and recalculates the quorum threshold. No restart is required.

---

## 7. ML Randomness Auditing (RandSentinel)

### 7.1 The Problem with NIST SP 800-22

The NIST SP 800-22 test suite is the industry standard for randomness testing. It includes 15 statistical hypothesis tests. The fundamental problem: most tests require sequences of at least 100,000 bits to achieve meaningful statistical power. Cryptographic keys are 256 to 4096 bits. At these lengths, NIST tests achieve FPR=1.0 — they flag every sequence, including those from cryptographically secure generators, as potentially weak.

### 7.2 The ML Approach

RandSentinel trains a supervised classifier on labeled sequences from 8 known generators (5 weak, 3 secure) across 4 sequence lengths. The classifier learns statistical signatures that distinguish weak from secure output, even at short lengths where hypothesis tests fail.

**Dataset**: 320,000 sequences total — 10,000 per generator per length (256, 1024, 4096, 16384 bits).

**Weak generators**: LCG, LFSR (16-bit), MT19937, RC4 with 3-byte key, C stdlib rand()  
**Secure generators**: /dev/urandom, AES-128-CTR DRBG, ChaCha20

### 7.3 Feature Engineering

18 statistical features are extracted per sequence:

**Frequency**: `bit_freq` — proportion of 1-bits. Secure generators produce values close to 0.5.

**Run-length**: `num_runs`, `longest_run_0`, `longest_run_1` — LCGs with poor low-bit behavior produce anomalously long runs. LFSRs with short periods produce runs that repeat.

**Autocorrelation** (lags 1–5): `autocorr_lag1` through `autocorr_lag5` — LCGs have strong positive autocorrelation at lag 1 due to the linear recurrence `Xₙ₊₁ = (aXₙ + c) mod m`. Secure generators have autocorrelations near zero.

**Entropy**: `approx_entropy` (Approximate Entropy, m=1) measures sequential regularity. `byte_entropy` (Shannon entropy over byte values) measures marginal distribution. RC4 with weak keys exhibits known byte-value biases that reduce byte entropy.

**Spectral**: `spectral_mean`, `spectral_std`, `spectral_max` — FFT magnitudes. Periodic generators (LCG, LFSR) produce spectral peaks at their fundamental frequency. Secure generators produce flat (white noise) spectra.

**Complexity**: `compression_ratio` (zlib level 9) — random data is incompressible (ratio ≈ 1.0). Structured output compresses significantly. `ngram2_chi2` and `ngram3_chi2` — chi-squared deviation of 2-gram and 3-gram frequencies from uniform. `linear_complexity` (Berlekamp-Massey on first 128 bits) — LFSR output has linear complexity exactly equal to the register length (16). Secure output has linear complexity ≈ n/2.

### 7.4 Model Architecture

**Random Forest** (primary model): 300 trees, max_depth=15, min_samples_leaf=5, trained with 5-fold stratified cross-validation.

**XGBoost** (comparison): 300 estimators, max_depth=6, learning_rate=0.05, subsample=0.8.

Random Forest consistently outperforms XGBoost across all metrics and sequence lengths. The combination of bootstrap aggregating (bagging) and leaf-count regularization makes it better suited to this low-dimensional, well-structured feature space.

### 7.5 Results

| Sequence Length | RF ROC-AUC | RF F1 | RF FPR | NIST FPR |
|---|---|---|---|---|
| 256 bits | 0.958 | 0.784 | 0.918 | 1.000 |
| 1024 bits | 0.940 | 0.792 | 0.877 | 1.000 |
| 4096 bits | 0.947 | 0.905 | 0.222 | 1.000 |
| 16384 bits | **0.975** | **0.930** | **0.123** | 1.000 |

The FPR improvement from 256 to 16384 bits is 7.5× for Random Forest (0.918 → 0.123). The jump between 1024 and 4096 bits is the largest single improvement, identifying 4096 bits as the practical minimum for reliable classification with this feature set.

The NIST baseline achieves FPR=1.0 at all lengths — it flags every secure sequence as potentially weak, making it operationally useless at practical key lengths.

### 7.6 Integration

The trained models are served by `crypto/server.py` (Flask, port 7777). The dashboard's "Analyze (ML)" button sends the key material to this endpoint and displays the result. The server loads the appropriate model based on the key material length.

---

## 8. API Reference

All endpoints require `Authorization: Bearer <API-Key>`.

### Cryptographic Operations

| Method | Endpoint | Role | Description |
|---|---|---|---|
| `POST` | `/kms/createKey` | admin | Generate a new 256-bit master secret, replicate via Raft |
| `POST` | `/kms/encrypt` | service/admin | Envelope-encrypt plaintext, log to audit chain |
| `POST` | `/kms/decrypt` | service/admin | Unwrap DEK, decrypt ciphertext, log to audit chain |
| `POST` | `/kms/rotateKey` | admin | Append new key version, preserve backward compatibility |
| `POST` | `/kms/deleteKey` | admin | Mark key as deleted (soft delete) |
| `GET` | `/kms/keys` | any | List all keys and their versions |
| `GET` | `/kms/keyMaterial` | any | Retrieve raw key material for a specific version |
| `POST` | `/kms/exportKey` | any | RSA-OAEP wrap key material with provided public key |

### Identity Management

| Method | Endpoint | Role | Description |
|---|---|---|---|
| `POST` | `/kms/createUser` | admin | Create user with role and API key, replicate via Raft |
| `POST` | `/kms/deleteUser` | admin | Remove user from cluster |
| `GET` | `/kms/users` | admin | List all users |

### Audit and Telemetry

| Method | Endpoint | Role | Description |
|---|---|---|---|
| `GET` | `/kms/auditLog` | any | Return full HMAC-chained audit trail |
| `GET` | `/events` | any | SSE stream of real-time cluster state events |
| `GET` | `/status` | any | Node role, term, leader address, log index |

### Cluster Management

| Method | Endpoint | Role | Description |
|---|---|---|---|
| `POST` | `/cluster/addNode` | admin | Add a new peer to the cluster dynamically |
| `POST` | `/chaos/partition` | admin | Simulate network partition for testing |
| `POST` | `/chaos/kill` | admin | Simulate node crash |

---

## 9. Security Properties Summary

| Property | Mechanism | Standard |
|---|---|---|
| Key confidentiality | Envelope encryption (KEK/DEK) | AWS KMS, Google Cloud KMS pattern |
| Key derivation | HKDF-SHA256 with context binding | RFC 5869 |
| Data encryption | AES-256-GCM (AEAD) | NIST FIPS 197 + SP 800-38D |
| Integrity protection | GCM authentication tag | Detects any ciphertext modification |
| Audit tamper-evidence | HMAC-SHA256 hash chain | Blockchain-style ledger |
| Key transport | RSA-2048 OAEP-SHA256 | PKCS#1 v2.2 |
| Randomness validation | Random Forest classifier | Outperforms NIST SP 800-22 at ≥4096 bits |
| Distributed consistency | Raft consensus (majority quorum) | Ongaro & Ousterhout 2014 |
| Access control | RBAC with API key authentication | Role-based endpoint restriction |
| Audit distribution | Raft-replicated audit commands | Prevents local log tampering |

---

## 10. Conclusion

RaftKMS is not a distributed systems project with some cryptography bolted on. It is a cryptography project where the distributed systems layer exists to make the cryptography operationally reliable.

Every cryptographic decision was made to match production standards: envelope encryption because direct encryption with a master key is architecturally wrong; HKDF because raw random bytes are not proper key material; GCM because unauthenticated encryption is not encryption; HMAC chains because flat logs are not audit trails; RSA-OAEP because raw key transport is not key transport.

The Raft layer ensures that these cryptographic guarantees hold even when machines crash, networks partition, and leaders fail. The ML layer ensures that the randomness feeding into the cryptographic primitives is statistically sound.

The result is a system that a security engineer would recognize as architecturally correct — not a simulation of security, but a genuine implementation of the patterns that underpin real-world key management infrastructure.
