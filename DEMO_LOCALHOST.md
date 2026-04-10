# RaftKMS — Localhost Demo Guide (UI & Crypto Edition)

Everything runs on one machine. You will simulate a distributed KMS system by running 3 Go node processes on different ports, backed by a React web dashboard and a Python Machine Learning server. Leader failure is simulated by manually killing one of the node terminals.

---

## Prerequisites

- Localhost environment setup.
- 3 Terminal Tabs opened.

---

## Step 1 — Fresh Start

Always begin a demo from a clean state so the audience sees reality from scratch.

In **Terminal 1**:
```bash
./reset.sh
```

This wipes all Raft state, datasets, and previous blockchain cryptographic hash records.

---

## Step 2 — Start the Full Stack

**In Terminal 1 (Python ML Server):**
```bash
python3 crypto/server.py
```
*(Leave this running in the background. It listens on port 7777 for PRNG requests).*

**In Terminal 2 (Raft Nodes + React Dashboard):**
```bash
./start_with_dashboard.sh
```
This single script natively starts `node1` (5001), `node2` (5002), and `node3` (5003), and then boots up your Vite React app. The browser will automatically open to *http://localhost:5173*.

---

## Step 3 — The React Dashboard

Log in to the dashboard using the default root key:
- Address: `localhost:5001, localhost:5002, localhost:5003` 
- API Key: `admin-secret-key`

**What to point out to your teacher:**
- The beautiful **Network Topology** visualizer in the center. One Node is glowing green (LEADER), two are blue (FOLLOWER).
- You can explain Raft briefly: *"They just held an election. One won. Every action we take in this UI is instantly replicated to all three."*

---

## Step 4 — The Cryptography Demo

Under the **🔒 Cryptographic Workspace**:
1. Click **Generate** to create a new cluster-wide key (e.g. `mock-key`).
2. Under Data Encryption, type a plaintext like *"Top Secret Evaluation Data"* and hit **Encrypt & Audit**.

**What to say about Envelope Encryption:**
> "Under the hood, we aren't encrypting directly with the master key. To meet industrial standards, the Go backend dynamically generated a random, one-time-use Data Encryption Key (DEK). It then derived a unique Key Encryption Key (KEK) using HKDF, wrapped the random DEK with it, and appended the wrapped byte code. The master key never touched the data directly."

3. Click **Analyze (ML)** right next to your key. 

**What to say about ML validation:**
> "But how do we know our hardware Random Number Generators aren't compromised? The frontend just sent our key material to the Python ML server. It analyzed the entropy and data patterns using a Random Forest model. As you can see by this SECURE popup, it passed NIST validations with high confidence."

---

## Step 5 — Tamper-Evident Hash Chains

Go to the **🛡️ Cryptographic Audit Ledger** tab on the left of your Dashboard.
Look at the logs you just generated.

**What to say:**
> "Every action we take constructs a localized blockchain constraint via HMAC-SHA256. Notice every log entry calculates a `previous_hash` and a `current_hash`. If an insider modifies the MongoDB database to hide their tracks, the hash chain breaks completely."

---

## Step 6 — RSA-OAEP Secure Export (Enterprise B2B Transport)

Under the **RSA-OAEP Key Export** pane in your UI:
1. First, in your empty **Terminal 3**, generate a mock dummy RSA key (acting as a 3rd party enterprise):
   ```bash
   openssl genrsa -out teacher.key 2048
   openssl rsa -in teacher.key -pubout -out teacher.pub
   cat teacher.pub
   ```
2. Copy the entire output of the public key (including the `-----BEGIN...` lines) and paste it into the UI textbox for RSA-OAEP Export.
3. Hit **Wrap & Export**.
4. You will see a long Base64 string return on the screen.

**What to explain:**
> "If an external corporation needs our AES key to decrypt local logs, we NEVER send it over the network raw. We just securely wrapped it completely against their specific RSA public key using OAEP. Only they can unpack it on their side."

---

## Step 7 — Failover Architecture Simulation

This is the main event.

1. Find out which node is currently the **Leader** by looking at the green node orb on your UI Dashboard (e.g., node2).
2. Go to **Terminal 3**, and forcefully kill that specific node process manually (simulating a hard system crash):
   ```bash
   # Say you are killing node2
   lsof -i :5002
   # Note the PID, then kill it:
   kill -9 <PID>
   ```

**What the audience sees on the dashboard (within ~3 seconds):**
1. The Leader orb turns red/offline.
2. The packet animations stop.
3. Automatically, one of the remaining nodes starts an election — its orb turns yellow (CANDIDATE).
4. Vote granted → new LEADER elected — orb turns green.
5. Heartbeat animations resume between the 2 remaining nodes.

**Now immediately encrypt another entry** via the UI — it works!

**Conclusion Narration:**
> "The system continues serving requests perfectly with 2/3 nodes. Raft required 0 human intervention to heal."
