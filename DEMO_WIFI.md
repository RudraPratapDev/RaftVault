# RaftKMS — WiFi (Multi-Laptop) Demo Guide (UI & Crypto Edition)

3 laptops. Same WiFi network. Each physically runs one component node. Leader failure is simulated by *literally turning off your laptop's WiFi*. 
This is the ultimate, impressive real-world demo setup!

---

## Setup Overview

| Machine | Node ID | Port | Role at start |
|---------|---------|------|---------------|
| Laptop A | `node1` | 5001 | Follower → *Runs the `dashboard` and `python` server too* |
| Laptop B | `node2` | 5002 | Follower |
| Laptop C | `node3` | 5003 | Follower |

**Note**: You only need to run the `dashboard` and `crypto/server.py` on ONE laptop (e.g. Laptop A), but the teacher will watch the browser on Laptop A while you physically unplug Laptop B and C from the WiFi!

---

## Before the Demo — One-Time Setup on Each Laptop

### 1. Find each laptop's local IP address
On each macOS laptop run:
```bash
ipconfig getifaddr en0
```
Write them down! Let's pretend they are:
- Laptop A: `192.168.1.101`
- Laptop B: `192.168.1.102`
- Laptop C: `192.168.1.103`

### 2. Update the config files across all computers
Inside `configs/node1.json` (on A), `configs/node2.json` (on B), etc:
Replace the dummy IPs with the exact IP map. Example for `node1`:
```json
{
  "node_id": "node1",
  "address": "192.168.1.101:5001",
  "peers": ["192.168.1.102:5002", "192.168.1.103:5003"],
  "data_dir": "./data/node1"
}
```

### 3. Point the Dashboard to the physical Nodes
Only on Laptop A (the presenter's screen), open `dashboard/src/App.jsx` and edit line 4:
```js
const DEFAULT_NODES = ['192.168.1.101:5001', '192.168.1.102:5002', '192.168.1.103:5003']
```

---

## Step 1 — Fresh Start (Before Teacher Arrives)

On EVERY laptop, make sure the system is completely reset:
```bash
./reset.sh
```

---

## Step 2 — Start The Engine

**On Laptop A (Presenter):**
```bash
# Start your node
./bin/raft-kms --config configs/node1.json --log-dir ./logs

# Open a new terminal tab, start the ML engine
python3 crypto/server.py

# Open a third terminal tab, run the Dashboard UI
cd dashboard && npm run dev
```

**On Laptop B & Laptop C:**
```bash
# Start their respective nodes
./bin/raft-kms --config configs/node2.json --log-dir ./logs
```

*Boom. The nodes instantly detect each other across the WiFi. They timeout, hold an election, and someone wins the LEADER crown.*

---

## Step 3 — The GUI Dashboard

Open `http://localhost:5173` locally on Laptop A's browser.

Log in:
- The node list is already preloaded to `192.168.1.101:5001,...`
- API Key: `admin-secret-key`

**What to point out to your Teacher:**
- Show them the 3 nodes visualized perfectly on the UI showing the topology.
- Point dramatically to the 3 laptops and state: *"There is no central database. That is Laptop A, B, and C. They are talking securely peer-to-peer over your WiFi."*

---

## Step 4 — Envelope Encoding & ML Analysis

Under the right-side **🔒 Cryptographic Workspace**:
1. Click **Generate** key.
2. Put "My Secret Document" in the Encrypt field and hit **Encrypt & Audit**.

**What to state:**
> "Unlike classic projects, we don't just use standard AES. We integrated extreme **Envelope Encryption** alongside an **HKDF Key Generator**. Behind the scenes, the cluster spins up a randomized, volatile DEK (Data Encryption Key) strictly for this string payload. It's masking it dynamically, meaning the master key never travels or encrypts data directly."

3. Click **Analyze (ML)** on your key in the dashboard.

> "A major vulnerability in crypto is bad Random Number Generators. Our frontend just pinged a Python Machine Learning server running a Random Forest classifier. It statistically tore apart our Go Raft nodes' byte generation, passed NIST checks, and mathematically proved we aren't vulnerable to backwards LCG engineering. It flagged our system as highly SECURE."

---

## Step 5 — Tamper-Evident Hash Chains

Go to the **🛡️ Cryptographic Audit Ledger** tab.
> "For ultimate logging, every action we take constructs a localized blockchain constraint via HMAC-SHA256. Notice every log entry calculates a `previous_hash` and a `current_hash`. If an insider modifies the node records to hide their tracks, the hash chain breaks completely."

---

## Step 6 — RSA-OAEP Secure Export

Provide a mock RSA key over terminal to securely wrap the KMS transmission.
1. Open up a terminal. Make an RSA cert: `openssl genrsa -out test.key 2048 && openssl rsa -in test.key -pubout -out test.pub`
2. Paste the `test.pub` string text into your Dashboard's "RSA Export" field. Hit export.

> "If Enterprise X needs our key, we don't send it raw over this public WiFi. We natively wrapped it against their RSA Public key. Only the enterprise's private key can decrypt this massive Base64 payload."

---

## Step 7 — The Heartbeat Wi-Fi Failover

The main event. Tell the teacher you're going to simulate a full server building crashing.

1. Find out which laptop is the **LEADER** (Look at the green node on the UI Dashboard).
2. Look at the teacher.
3. Physically reach over to that laptop, go to its Network settings, and **Turn off the WiFi / Disconnect.**

**What the audience sees natively on Laptop A's Dashboard (within ~3 seconds):**
1. The Leader orb instantly turns red/offline as heartbeats fail.
2. The packet animations stop in their tracks.
3. Automatically, one of the remaining two nodes realizes the leader is dead — its orb turns yellow (CANDIDATE).
4. Vote granted → new LEADER elected — orb turns green.
5. Heartbeat animations resume between the 2 surviving laptops!

**Now immediately encrypt another entry** via the UI — it perfectly succeeds!

**Conclusion Narration:**
> "We just lost an entire node server abruptly. Raft detected the timeout, executed a fault-tolerant multi-consensus vote, transferred leadership, and resumed military-grade cryptographic operation with strictly 2 out of 3 servers. All zero-trust, completely peer-to-peer."
