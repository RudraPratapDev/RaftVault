# Formal Technical Report: Design and Implementation of a Cryptographically Hardened, Fault-Tolerant Key Management System (RaftKMS)

**Course Domain**: Network and Information Security / Applied Cryptography  
**System**: RaftKMS — Peer-to-Peer Distributed Key Management System with Envelope Encryption, HKDF Key Derivation, HMAC-SHA256 Audit Chains, RSA-OAEP Key Transport, and ML-Based Randomness Auditing

---

## 1. Abstract

Modern Key Management Systems face a dual challenge: they must be cryptographically rigorous and operationally resilient. Centralized solutions such as AWS KMS or HashiCorp Vault solve the cryptographic problem but introduce a single point of failure — if the key server is unreachable, the entire system loses the ability to encrypt or decrypt data. This report presents **RaftKMS**, a system that addresses both dimensions simultaneously.

On the cryptographic side, RaftKMS implements a full envelope encryption pipeline using HKDF-derived Key Encryption Keys (KEK) and per-operation random Data Encryption Keys (DEK), both protected by AES-256-GCM authenticated encryption. Key material is never derived naively — it passes through an RFC 5869-compliant HKDF construction with context binding. The audit trail is tamper-evident through HMAC-SHA256 chain linking. Key export uses RSA-OAEP, ensuring key material never travels in plaintext. A companion Python ML subsystem (RandSentinel) validates the cryptographic quality of generated key material using a trained Random Forest classifier evaluated against NIST SP 800-22 baselines.

On the distributed systems side, RaftKMS implements the Raft consensus algorithm from scratch in Go, guaranteeing that all cryptographic state — keys, users, audit entries — is replicated across a majority quorum of nodes before any operation is acknowledged. The system tolerates node failures, network partitions, and leader crashes without data loss or service interruption, provided a strict majority `(N/2)+1` of nodes remains reachable.

---

## 2. Problem Motivation

### 2.1 The Cryptographic Weakness of Naive Key Management

The most common failure mode in deployed KMS implementations is not algorithmic — it is architectural. Systems that encrypt data directly with a long-lived master key create a catastrophic exposure surface: if the master key is ever observed, all past and future ciphertexts are compromised. Real-world KMS systems (AWS KMS, Google Cloud KMS) solve this with envelope encryption, a two-layer scheme where the master key never touches plaintext data directly.

A second common failure is poor key derivation. Generating key material as raw random bytes without a formal Key Derivation Function (KDF) means the key has no cryptographic binding to its context — the same master secret could produce the same key material for different key IDs or versions, creating subtle collision risks. RFC 5869 HKDF was designed specifically to address this.

A third failure is audit trail integrity. Flat log files on a single server can be modified by a privileged insider. A tamper-evident audit chain — where each entry's hash depends on all previous entries — makes retroactive modification detectable.

### 2.2 The Operational Weakness of Centralized KMS

Even a cryptographically perfect KMS is operationally useless if it is unavailable. A single-server KMS creates a hard dependency: any application that needs to encrypt or decrypt data is blocked the moment the key server goes down. In air-gapped environments, defense networks, or high-availability financial systems, this is unacceptable.

The Raft consensus algorithm provides a mathematically proven solution: as long as a majority of nodes are reachable, the system continues to serve requests. Minority partitions are locked out of writes, preventing the "split-brain" scenario where two isolated halves of the cluster diverge into inconsistent cryptographic states.

---

## 3. System Architecture

RaftKMS is composed of four integrated layers:

```
┌─────────────────────────────────────────────────────────┐
│              React Dashboard (Port 5173)                 │
│   Network Topology · Crypto Workspace · Audit Ledger    │
└────────────────────────┬────────────────────────────────┘
                         │ HTTP REST  Bearer <API-Key>
┌────────────────────────▼────────────────────────────────┐
│              API Gateway Layer  (server.go)              │
│   Auth Middleware · Leader Redirect · CORS · SSE        │
└────────────────────────┬────────────────────────────────┘
                         │ Validated Commands
┌────────────────────────▼────────────────────────────────┐
│           Raft Consensus Engine  (raft.go)               │
│   Leader Election · Log Replication · Commit Index      │
└────────────────────────┬────────────────────────────────┘
                         │ Committed Log Entries
┌────────────────────────▼────────────────────────────────┐
│        Cryptographic KMS State Machine  (kms.go)         │
│  Envelope Encrypt · HKDF · HMAC Audit · RSA-OAEP Export │
└─────────────────────────────────────────────────────────┘
```

The Raft engine is the source of truth. Every cryptographic operation — key creation, encryption, decryption, user management, audit logging — is serialized as a `[]byte` command payload and submitted to the Raft pipeline. The KMS state machine only executes a command after it has been committed to a majority quorum. This guarantees that no cryptographic state exists on any node that has not been durably replicated.

### 3.1 Request Lifecycle: Encryption

Tracing a `POST /kms/encrypt` request through the full stack:

1. The API layer validates the `Authorization: Bearer <key>` header against the in-memory user registry.
2. If the receiving node is a Follower, it returns an HTTP 307 redirect to the current Leader's address.
3. The Leader packages an `ENCRYPT` command and an `AUDIT_LOG` command as `[]byte` payloads.
4. The Leader issues concurrent `AppendEntries` RPCs to all peer nodes.
5. Peers write the entries to their persistent JSON stores and acknowledge.
6. Upon receiving acknowledgements from a majority, the Leader advances the `commitIndex`.
7. The KMS state machine executes the envelope encryption pipeline and returns the ciphertext.

---

## 4. Cryptographic Protocol Design

### 4.1 Envelope Encryption

Envelope encryption is the foundational cryptographic pattern of every production KMS. RaftKMS implements it fully.

**The problem with direct encryption**: If a single AES key encrypts all data, key rotation requires re-encrypting every existing ciphertext. More critically, the key is used repeatedly, increasing the statistical surface for cryptanalysis.

**The envelope solution**: Each encryption operation generates a fresh, random 256-bit Data Encryption Key (DEK). The DEK encrypts the plaintext. The DEK is then itself encrypted (wrapped) by a Key Encryption Key (KEK) that is derived from the master key material. The output is the wrapped DEK concatenated with the ciphertext — the master key never touches the plaintext.

**Implementation** (`kms.go`, `Encrypt()`):

```
Master Secret (256-bit random, stored in cluster)
        │
        ▼  HKDF(master, salt=nil, info="keyID:version:timestamp", len=32)
       KEK  (256-bit, derived per key version)
        │
        ├──► AES-256-GCM encrypt(DEK) ──► WrappedDEK  (stored alongside ciphertext)
        │
DEK ◄──┘   (256-bit, freshly generated per operation via crypto/rand)
 │
 ▼  AES-256-GCM encrypt(plaintext)
Ciphertext

Output wire format:
[ Version: 4 bytes ] [ WrappedDEKLen: 2 bytes ] [ WrappedDEK ] [ Ciphertext ]
→ base64-encoded for transport
```

Both the KEK wrapping and the DEK encryption use AES-256-GCM, which provides authenticated encryption — any tampering with the wrapped DEK or the ciphertext causes decryption to fail with an authentication error, not silently produce garbage.

**Decryption** (`Decrypt()`): The version prefix is read first, the correct key version is located, the KEK is re-derived via HKDF with the same context, the DEK is unwrapped, and the plaintext is recovered. This design supports backward compatibility — a ciphertext encrypted under key version 1 can still be decrypted after the key has been rotated to version 3.

### 4.2 HKDF Key Derivation (RFC 5869)

Raw random bytes are not suitable as direct key material in a multi-version, multi-context system. HKDF (HMAC-based Key Derivation Function) provides a formal, standardized way to derive cryptographically strong keys from a master secret with context binding.

RaftKMS implements HKDF-SHA256 from scratch following RFC 5869:

**HKDF-Extract** — converts the master secret into a pseudorandom key (PRK):
```
PRK = HMAC-SHA256(salt, IKM)
```
If no salt is provided, a zero-filled byte array of SHA-256 output length is used.

**HKDF-Expand** — expands the PRK into output key material of the desired length:
```
T(1) = HMAC-SHA256(PRK, "" || info || 0x01)
T(2) = HMAC-SHA256(PRK, T(1) || info || 0x02)
OKM  = T(1) || T(2) || ... (truncated to desired length)
```

**Context binding** (`kms.go`, line ~408):
```go
info := []byte(fmt.Sprintf("%s:%d:%s", key.KeyID, latestVersion.Version, latestVersion.CreatedAt))
kek  := HKDF(masterBytes, nil, info, 32)
```

The `info` string encodes the key ID, version number, and creation timestamp. This means:
- Two different keys with the same master secret produce different KEKs.
- Two different versions of the same key produce different KEKs.
- The KEK is cryptographically bound to its context — it cannot be reused in a different context without detection.

This is precisely how AWS KMS and Google Cloud KMS derive their internal key material.

### 4.3 AES-256-GCM Authenticated Encryption

All symmetric encryption in RaftKMS uses AES-256-GCM (Galois/Counter Mode). GCM is an AEAD (Authenticated Encryption with Associated Data) mode, meaning it provides both:

- **Confidentiality**: The plaintext is encrypted and indistinguishable from random bytes without the key.
- **Integrity and Authenticity**: A 128-bit authentication tag is computed over the ciphertext. Any modification to the ciphertext — even a single bit flip — causes decryption to fail with an explicit authentication error.

Each encryption operation generates a fresh 12-byte nonce via `crypto/rand`. The nonce is prepended to the ciphertext for storage. Using a fresh random nonce per operation ensures that encrypting the same plaintext twice produces different ciphertexts, preventing pattern analysis.

### 4.4 HMAC-SHA256 Tamper-Evident Audit Chain

The audit trail is not a flat log — it is a hash-chained ledger. Each entry is cryptographically linked to all previous entries, making retroactive modification detectable.

**Structure** (`AuditEntry` struct):
```go
type AuditEntry struct {
    Timestamp    string `json:"timestamp"`
    Username     string `json:"username"`
    Action       string `json:"action"`
    KeyID        string `json:"key_id"`
    PreviousHash string `json:"previous_hash"`
    CurrentHash  string `json:"current_hash"`
}
```

**Chain construction** (`applyAuditLog()`):
```go
p.Entry.PreviousHash = s.lastHash
mac := hmac.New(sha256.New, s.auditHMACKey)
data := fmt.Sprintf("%s|%s|%s|%s|%s",
    p.Entry.PreviousHash, p.Entry.Timestamp,
    p.Entry.Username, p.Entry.Action, p.Entry.KeyID)
mac.Write([]byte(data))
p.Entry.CurrentHash = fmt.Sprintf("%x", mac.Sum(nil))
s.lastHash = p.Entry.CurrentHash
```

The canonical data string includes the previous entry's hash, so each `CurrentHash` is a function of the entire history of the chain. If an attacker modifies entry N, the `CurrentHash` of entry N changes, which invalidates the `PreviousHash` field of entry N+1, which cascades through all subsequent entries. The chain is broken and the tampering is immediately detectable.

The chain is initialized with a 64-character zero hash as the genesis `PreviousHash`, analogous to the genesis block in a blockchain.

Because audit entries are submitted as Raft commands, the chain is replicated across all nodes. A rogue administrator cannot silently modify their local copy — the next heartbeat from the cluster majority would overwrite it.

### 4.5 RSA-OAEP Wrapped Key Export

When an external party needs access to a symmetric key — for example, to decrypt archived data — the key must be transported securely. Sending the raw AES key material over a network is never acceptable. RSA-OAEP (Optimal Asymmetric Encryption Padding) solves this with asymmetric key encapsulation.

**Protocol** (`server.go`, `handleExportKey()`):
1. The requesting party generates an RSA-2048 key pair and sends their public key (PEM-encoded) to the `/kms/exportKey` endpoint.
2. The server retrieves the latest key version's master material.
3. The server encrypts the key material using RSA-OAEP with SHA-256 as the hash function.
4. The wrapped key is returned as a base64-encoded string.
5. Only the holder of the corresponding RSA private key can unwrap it.

```
Client RSA Public Key (2048-bit)
        │
        ▼  RSA-OAEP-SHA256 encrypt(AES key material)
  WrappedKey (base64) ──► transmitted over network
        │
        ▼  RSA-OAEP-SHA256 decrypt(WrappedKey) using Client RSA Private Key
  AES Key Material (recovered only by the intended recipient)
```

The server accepts both PKIX and PKCS1 PEM formats for maximum compatibility. The export operation is logged to the audit chain. This pattern is the standard KEM (Key Encapsulation Mechanism) used in enterprise B2B key distribution.

---

## 5. ML-Based Randomness Auditing (RandSentinel)

A cryptographic system is only as strong as its random number generator. RaftKMS integrates a Python ML subsystem that audits the statistical quality of generated key material.

### 5.1 Motivation

The NIST SP 800-22 test suite — the standard tool for randomness auditing — requires sequences of at least 100,000 bits to achieve meaningful statistical power. Cryptographic keys and nonces are typically 256 to 4096 bits. At these lengths, NIST tests are essentially inconclusive: they achieve FPR=1.0 (flagging every sequence, including secure ones, as potentially weak).

RandSentinel takes a supervised learning approach: train a classifier on labeled sequences from known weak and secure generators, then apply it to new key material.

### 5.2 Dataset

320,000 binary sequences were generated from 8 PRNG implementations across 4 sequence lengths (256, 1024, 4096, 16384 bits), with 10,000 samples per generator per length.

**Weak generators (label=1)**:
- LCG (Linear Congruential Generator) — Numerical Recipes parameters, exploitable via Marsaglia's lattice theorem
- LFSR (16-bit Linear Feedback Shift Register) — period 65,535, broken by Berlekamp-Massey in 16 bits
- MT19937 (Mersenne Twister) — state fully recoverable after 624 outputs
- RC4 with 3-byte weak key — FMS attack applicable, 2²⁴ key space
- C stdlib rand() — 15-bit output truncation, LCG structure

**Secure generators (label=0)**:
- `/dev/urandom` — OS entropy pool, ChaCha20-backed (Linux ≥5.17)
- AES-128-CTR DRBG — security reduces to AES hardness
- ChaCha20 — 256-bit security, ARX design, no known attacks

### 5.3 Feature Engineering

18 statistical features are extracted per sequence, covering six dimensions:

| Dimension | Features |
|---|---|
| Frequency | `bit_freq` |
| Run-length | `num_runs`, `longest_run_0`, `longest_run_1` |
| Autocorrelation | `autocorr_lag1` through `autocorr_lag5` |
| Entropy | `approx_entropy` (ApEn, m=1), `byte_entropy` (Shannon) |
| Spectral | `spectral_mean`, `spectral_std`, `spectral_max` (FFT magnitudes) |
| Complexity | `compression_ratio` (zlib), `ngram2_chi2`, `ngram3_chi2`, `linear_complexity` (Berlekamp-Massey on 128 bits) |

### 5.4 Model and Results

A Random Forest classifier (300 trees, max_depth=15, min_samples_leaf=5) was trained using 5-fold stratified cross-validation. Results on a held-out 20% test set:

| Sequence Length | ROC-AUC | F1 | FPR |
|---|---|---|---|
| 256 bits | 0.958 | 0.784 | 0.918 |
| 1024 bits | 0.940 | 0.792 | 0.877 |
| 4096 bits | 0.947 | 0.905 | 0.222 |
| 16384 bits | **0.975** | **0.930** | **0.123** |
| NIST SP 800-22 (all lengths) | N/A | 0.667 | 1.000 |

At 16,384 bits, Random Forest achieves ROC-AUC=0.975 and FPR=0.123, compared to NIST's FPR=1.0 at all lengths. The 4096-bit threshold is the inflection point where spectral and n-gram features gain sufficient statistical power to reliably separate the distributions.

The trained models (`RandomForest_len256.pkl`, `RandomForest_len4096.pkl`) are served by a Flask API (`crypto/server.py`) on port 7777. The dashboard sends key material to this endpoint and displays the classification result.

---

## 6. Distributed Consensus: Raft Implementation

### 6.1 Why Raft

Raft was chosen over Paxos for its explicit design goal of understandability. The algorithm partitions the consensus problem into three relatively independent subproblems: leader election, log replication, and safety. Each subproblem has a clean, verifiable solution.

### 6.2 Leader Election

All nodes boot as Followers. Each node initializes a randomized election timeout between 1500ms and 3000ms. If no heartbeat is received before the timeout expires, the node transitions to Candidate, increments its `CurrentTerm`, votes for itself, and broadcasts `RequestVote` RPCs to all peers.

A node grants a vote only if:
- The candidate's term is at least as large as the voter's current term.
- The voter has not already voted in this term.
- The candidate's log is at least as up-to-date as the voter's log (log completeness check).

A candidate that receives votes from a strict majority `(N/2)+1` transitions to Leader and begins sending heartbeat `AppendEntries` RPCs to suppress new elections.

The randomized timeout window prevents split votes: it is statistically unlikely that two nodes time out simultaneously, and even if they do, the next round of timeouts will resolve the tie.

### 6.3 Log Replication

Every write operation is serialized as a `LogEntry` containing a `Term`, an `Index`, and a `[]byte` command payload. The Leader appends the entry to its local log and issues concurrent `AppendEntries` RPCs to all Followers.

A Follower accepts the entry if the `prevLogIndex` and `prevLogTerm` fields match its own log (the Log Matching Property). Upon acceptance, the Follower writes the entry to its persistent JSON store and acknowledges.

Once the Leader receives acknowledgements from a majority, it advances the `commitIndex` and notifies the KMS state machine to execute the command. The committed entry is then included in subsequent heartbeats, allowing Followers to advance their own `commitIndex`.

### 6.4 Safety Guarantees

Raft provides five safety properties:

1. **Election Safety**: At most one leader can be elected per term.
2. **Leader Append-Only**: A leader never overwrites or deletes entries in its log.
3. **Log Matching**: If two logs contain an entry with the same index and term, the logs are identical in all entries up to that index.
4. **Leader Completeness**: If a log entry is committed in a given term, it will be present in the logs of all leaders for all higher terms.
5. **State Machine Safety**: If a node has applied a log entry at a given index, no other node will ever apply a different entry at that index.

These properties together guarantee that the KMS state machine on every node will eventually reach the same state, regardless of failures or network partitions.

### 6.5 Split-Brain Prevention

In a 3-node cluster, if one node becomes isolated (network partition), it cannot reach quorum and cannot process write requests. It will increment its term indefinitely but fail to elect itself leader. The majority partition continues operating normally.

When the partition heals, the isolated node broadcasts its inflated term. The active leader steps down to investigate. The rejoining node's log is behind, so the leader forces it to synchronize by replaying the missing entries. The cluster converges to a single consistent state without any manual intervention.

---

## 7. Identity and Access Management

### 7.1 Role-Based Access Control

Every API request carries an `Authorization: Bearer <API-Key>` header. The API middleware validates this against the in-memory user registry (replicated via Raft) and enforces role-based permissions:

| Role | Permitted Endpoints |
|---|---|
| `admin` | `/kms/createKey`, `/kms/deleteKey`, `/kms/rotateKey`, `/kms/createUser`, `/kms/deleteUser`, `/cluster/addNode`, `/chaos/*` |
| `service` | `/kms/encrypt`, `/kms/decrypt`, `/kms/exportKey` |
| Both | `/kms/keys`, `/kms/auditLog`, `/kms/keyMaterial` |

The default bootstrap identity (`admin` / `admin-secret-key`) is created at node initialization and replicated to all peers on first startup.

### 7.2 User Management via Raft

User creation and deletion are Raft commands, not local operations. When an admin creates a new service account, the `CREATE_USER` command is replicated to all nodes before the operation is acknowledged. This means every node in the cluster has an identical, consistent view of the user registry at all times.

---

## 8. Real-Time Telemetry

The dashboard receives live cluster state updates via Server-Sent Events (SSE). A thread-safe ring buffer of 250 events captures state transitions (`LEADER_ELECTED`, `VOTE_GRANTED`, `LOG_REPLICATED`, `NODE_CRASHED`) as they occur. The React frontend maintains a persistent SSE connection and updates the network topology visualization in real time — node orbs change color (green=leader, blue=follower, yellow=candidate, red=offline) as the cluster state evolves.

SSE was chosen over WebSockets because the data flow is strictly unidirectional (server to browser), and SSE requires no additional protocol negotiation or library dependencies.

---

## 9. Verification and Testing

### 9.1 Failover Validation

Using the integrated Chaos module and direct SIGKILL signals, the following was verified:

- **Leader crash recovery**: Upon killing the leader process, a new leader is elected within the randomized timeout window (average ~2.2 seconds). The cluster resumes serving encryption requests immediately after election.
- **Minority partition isolation**: An isolated single node cannot process writes. Upon network healing, it resynchronizes from the majority without data loss.
- **Continued operation at N-1**: A 3-node cluster with one node permanently offline continues to serve all requests correctly with 2 nodes.

### 9.2 Cryptographic Correctness

- Ciphertexts produced under key version 1 are correctly decrypted after rotation to version 2, confirming the version-prefix mechanism works.
- Modifying a single byte of a ciphertext causes AES-GCM authentication to fail, confirming integrity protection.
- Modifying any audit entry causes the hash chain to break at that entry, confirming tamper-evidence.
- RSA-OAEP wrapped keys can only be unwrapped with the corresponding private key, confirmed via OpenSSL.

---

## 10. Conclusion

RaftKMS demonstrates that cryptographic rigor and operational resilience are not competing concerns — they can be achieved simultaneously in a system built from first principles. The envelope encryption pipeline with HKDF key derivation matches the architecture of production cloud KMS systems. The HMAC-SHA256 audit chain provides tamper-evidence without external dependencies. RSA-OAEP key export enables secure B2B key transport. The Raft consensus layer ensures that all of this cryptographic state is durably replicated and available even under node failures and network partitions. The ML randomness auditing layer adds a validation dimension that classical statistical tests cannot provide at practical key lengths.

The system proves that a small cluster of commodity machines on a local network can provide enterprise-grade key management without cloud dependencies, without external databases, and without sacrificing cryptographic correctness.
