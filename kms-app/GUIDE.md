# RaftVault KMS — Faculty Demo Guide

This guide walks you through a complete, step-by-step demonstration of every cryptographic feature in the system. Each section explains *what* is happening, *why* it matters, and *what to show* on screen.

---

## Pre-Demo Setup (5 minutes before)

**Terminal 1 — Start the 3-node Raft cluster:**
```bash
./start_cluster.sh
```
Wait until you see `[RAFT] Elected leader` in one of the node logs.

**Terminal 2 — Start the KMS dashboard:**
```bash
cd kms-app && npm run dev
```
Open `http://localhost:5173` in your browser.

**Terminal 3 — (Optional) Start the ML analysis service:**
```bash
python3 crypto/server.py
```

**Login credentials:**
- Username: `admin`
- Password / API Key: `admin-secret-key`

---

## Demo Flow

---

### Step 1 — Overview: The Crypto Stack at a Glance

**Navigate to: Overview (home page)**

Point out the "Cryptographic Stack" panel at the bottom. It shows all four crypto primitives active in the system:

| Primitive | What it does |
|-----------|-------------|
| Envelope Encryption | KEK/DEK separation — master key never touches data |
| HKDF-SHA256 | Derives the KEK from master secret at runtime |
| HMAC Chain | Every audit entry is cryptographically linked |
| RSA-OAEP | Key export — wrapped for a specific recipient |

**What to say:** "This isn't just AES encryption bolted on. Every layer has a specific cryptographic purpose, and together they replicate what AWS KMS and Google Cloud KMS do internally."

---

### Step 2 — Key Registry: HKDF Key Derivation

**Navigate to: Key Registry**

1. Create a key — type `demo-key-001` and click Create.
2. Expand the key row to see the version and master secret (truncated).

**What to say:** "What's stored in Raft is a 256-bit *master secret* — raw random bytes. The actual encryption key (KEK) is *never stored*. It's derived fresh every time using HKDF-SHA256 with a context string that includes the key ID, version number, and creation timestamp. This means the same master secret produces a different KEK for every key version."

**Show the HKDF context:** When you encrypt something in the next step, the Envelope Breakdown will show the exact context string used.

3. Click **Rotate** on the key. A new version appears.

**What to say:** "Rotating a key generates a new master secret. The HKDF context changes, so the new KEK is completely different. Old ciphertext still decrypts because the version number is embedded in the ciphertext — the system automatically picks the right version."

---

### Step 3 — Encrypt / Decrypt: Envelope Encryption Live

**Navigate to: Encrypt / Decrypt**

1. Select `demo-key-001`, type any plaintext (e.g. `"patient_id: 12345, diagnosis: confidential"`), click **Encrypt with Envelope Encryption**.

2. The ciphertext appears. Click **Show Envelope Encryption Breakdown**.

**Walk through each step with the faculty:**

**Step 1 — HKDF-SHA256:**
- The master secret + context string → HKDF → KEK (256-bit)
- The KEK is shown truncated (first 8 bytes in hex)
- "This key is derived in memory and immediately used. It is never written to disk or stored anywhere."

**Step 2 — DEK Generation + Wrapping:**
- A fresh random 256-bit DEK is generated using `crypto/rand` (OS entropy)
- The DEK is encrypted by the KEK using AES-256-GCM → Wrapped DEK
- "Every single encryption operation gets a unique DEK. If an attacker somehow gets one DEK, they can only decrypt one message."

**Step 3 — Data Encryption:**
- The plaintext is encrypted by the DEK using AES-256-GCM
- "The master key never touches your data. The KEK never touches your data. Only the DEK does — and the DEK is itself encrypted."

**Final output format:**
```
version(4B) | wrappedDEKLen(2B) | wrappedDEK(AES-GCM) | ciphertext(AES-GCM)
```
"The version prefix means you can rotate keys freely — old ciphertext self-identifies which version to use for decryption."

3. Copy the ciphertext, switch to the Decrypt tab, paste it, select the same key, decrypt.

**What to say:** "Decryption reverses the process: parse version → derive KEK via HKDF → unwrap DEK → decrypt data. The master key is never used directly."

---

### Step 4 — Audit Log: HMAC-SHA256 Chained Trail

**Navigate to: Audit Log**

You should see entries for the ENCRYPT and DECRYPT operations just performed.

1. Click the **expand arrow** (▼) on any entry to reveal the hash chain details.

**Show the faculty:**
- `previous_hash` — the hash of the entry before this one
- `current_hash` — HMAC-SHA256 of `(prev_hash | timestamp | username | action | key_id)`
- The formula is shown inline: `HMAC-SHA256(key, "prev|timestamp|user|action|keyid")`

**What to say:** "This is a cryptographic chain — like a mini blockchain. Each entry's hash depends on the previous entry's hash. If anyone modifies entry 3 — changes the action, the timestamp, anything — entry 3's hash changes, which means entry 4's `previous_hash` no longer matches, and the chain breaks at entry 4. You can't silently tamper with history."

2. Click **Verify Chain**.

**What to say:** "This re-computes every HMAC from the genesis hash and checks every link. In a real forensic audit, this is how you prove the log hasn't been tampered with."

The result shows: "All N entries verified. Chain is intact."

**Bonus point:** "These audit entries are committed through Raft consensus — all 3 nodes hold the same chain. An attacker would need to tamper with a majority of nodes simultaneously, which is the Byzantine fault tolerance guarantee."

---

### Step 5 — Key Registry: RSA-OAEP Key Export

**Navigate to: Key Registry**

This demonstrates the KEM (Key Encapsulation Mechanism) pattern.

**First, generate an RSA key pair in Terminal:**
```bash
openssl genrsa -out /tmp/demo.pem 2048
openssl rsa -in /tmp/demo.pem -pubout -out /tmp/demo_pub.pem
cat /tmp/demo_pub.pem
```

1. On the `demo-key-001` row, click **Export**.
2. Paste the contents of `demo_pub.pem` into the public key field.
3. Click **Export Wrapped Key**.

**What to say:** "The server takes the 256-bit key material and encrypts it with your RSA-2048 public key using RSA-OAEP-SHA256. The result is a wrapped key — it can be transmitted over any channel, stored anywhere, and only the holder of the private key can unwrap it. The key material never travels in plaintext."

**Why OAEP and not PKCS#1 v1.5?** "PKCS#1 v1.5 is vulnerable to Bleichenbacher's padding oracle attack — a chosen-ciphertext attack that can recover the plaintext. OAEP uses a hash function and random padding, making it IND-CCA2 secure."

**To verify the export works (optional):**
```bash
echo "<paste wrapped key>" | base64 -d | openssl rsautl -decrypt -oaep -inkey /tmp/demo.pem | xxd
```
The output should be 32 bytes of key material.

---

### Step 6 — Crypto Internals: The Full Picture

**Navigate to: Crypto Internals**

This page is designed specifically for explaining the system to someone technical. Walk through each section:

1. **Envelope Encryption** — the diagram shows the full KEK/DEK flow visually
2. **HKDF-SHA256** — shows the Extract → Expand two-step process with the actual Go code
3. **HMAC-SHA256 Chain** — shows the chain diagram and the exact hash formula
4. **RSA-OAEP** — shows the KEM pattern and why OAEP is used
5. **AES-256-GCM** — explains authenticated encryption, nonce, and auth tag
6. **Crypto Stack Summary table** — every primitive, what it's used for, and the relevant standard (NIST/RFC)

**What to say:** "Every primitive here is a real-world standard. AES-256-GCM is NIST FIPS 197. HKDF is RFC 5869. HMAC is RFC 2104. RSA-OAEP is PKCS#1 v2.2. This isn't academic — this is exactly how production KMS systems are built."

---

### Step 7 — Security Audit: ML Randomness Analysis

**Navigate to: Security Audit** (requires `python3 crypto/server.py` running)

1. Select `demo-key-001`, click **Run Audit**.

**What to say:** "This runs 18 statistical tests on the key's raw bytes — bit frequency, run length, autocorrelation, NIST SP 800-22 tests, and a Random Forest classifier trained on 320,000 bit sequences from 8 different generators. It tells you whether the key material looks like it came from a cryptographically secure source or a weak PRNG."

Show the verdict, NIST test results, and bit heatmap.

---

## Key Talking Points for Q&A

**"Why not just use AES directly with the master key?"**
Envelope encryption means key rotation is cheap (re-wrap the DEK, not re-encrypt all data), and compromise of one DEK only exposes one message, not everything encrypted with that key.

**"Why HKDF instead of using the random bytes directly?"**
HKDF provides domain separation — the same master secret produces different keys for different purposes (different key IDs, versions). It also provides key stretching and ensures the output has the right statistical properties regardless of the input's distribution.

**"How is the audit trail different from a regular database log?"**
A database log can be modified by anyone with DB access. This chain requires an attacker to recompute all subsequent HMACs (which requires the HMAC key) AND update all nodes in the Raft cluster simultaneously. The chain is also replicated — you can't tamper with one node without the others detecting the divergence.

**"What's the threat model for RSA-OAEP export?"**
It solves the key distribution problem. You want to give a service its encryption key, but you can't send it in plaintext. The service generates an RSA key pair, sends you the public key, you wrap the KMS key with it, and only the service can unwrap it. This is exactly how TLS key exchange works.

---

## Crypto Primitives Reference

| Primitive | Standard | Key Size | Security Level |
|-----------|----------|----------|----------------|
| AES-256-GCM | NIST FIPS 197 + SP 800-38D | 256-bit | 128-bit security |
| HKDF-SHA256 | RFC 5869 | Variable | Depends on IKM |
| HMAC-SHA256 | RFC 2104 + FIPS 198 | 256-bit | 128-bit security |
| RSA-OAEP-SHA256 | PKCS#1 v2.2 / RFC 8017 | 2048-bit | ~112-bit security |
| crypto/rand | OS entropy (urandom) | — | CSPRNG |
