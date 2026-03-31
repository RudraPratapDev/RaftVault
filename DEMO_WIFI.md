# RaftKMS — WiFi (Multi-Laptop) Demo Guide

3 laptops on the same WiFi network. Each runs one node. Leader failure is demonstrated by physically disconnecting a laptop from WiFi — no commands needed. This is the live, visible, real-world demo.

---

## Setup Overview

| Machine | Node ID | Port | Role at start |
|---------|---------|------|---------------|
| Laptop A | node1 | 5001 | Follower → possibly Leader |
| Laptop B | node2 | 5002 | Follower → possibly Leader |
| Laptop C | node3 | 5003 | Follower → possibly Leader |

The dashboard runs on any one laptop (or all three — doesn't matter). The audience watches the topology visualization.

---

## Before the Demo — One-Time Setup on Each Laptop

### 1. Find each laptop's local IP

On each laptop:
```bash
ipconfig getifaddr en0
```

You'll get something like:
- Laptop A: `192.168.1.101`
- Laptop B: `192.168.1.102`
- Laptop C: `192.168.1.103`

### 2. Update the config files on each laptop

On **Laptop A**, edit `configs/node1.json`:
```json
{
  "node_id": "node1",
  "address": "192.168.1.101:5001",
  "peers": ["192.168.1.102:5002", "192.168.1.103:5003"],
  "data_dir": "./data/node1",
  "election_timeout_min_ms": 1500,
  "election_timeout_max_ms": 3000,
  "heartbeat_interval_ms": 500
}
```

On **Laptop B**, edit `configs/node2.json`:
```json
{
  "node_id": "node2",
  "address": "192.168.1.102:5002",
  "peers": ["192.168.1.101:5001", "192.168.1.103:5003"],
  "data_dir": "./data/node2",
  "election_timeout_min_ms": 1500,
  "election_timeout_max_ms": 3000,
  "heartbeat_interval_ms": 500
}
```

On **Laptop C**, edit `configs/node3.json`:
```json
{
  "node_id": "node3",
  "address": "192.168.1.103:5003",
  "peers": ["192.168.1.101:5001", "192.168.1.102:5002"],
  "data_dir": "./data/node3",
  "election_timeout_min_ms": 1500,
  "election_timeout_max_ms": 3000,
  "heartbeat_interval_ms": 500
}
```

Replace the IPs with your actual ones.

### 3. Update the dashboard default nodes

On the laptop running the dashboard, edit `dashboard/src/App.jsx`, line 4:

```js
const DEFAULT_NODES = ['192.168.1.101:5001', '192.168.1.102:5002', '192.168.1.103:5003']
```

Then rebuild:
```bash
cd dashboard && npm run build
```

### 4. Allow firewall access on each laptop

macOS may block incoming connections. On each laptop:

```bash
# Allow the node's port through the firewall
# Or just disable the firewall temporarily for the demo:
# System Settings → Network → Firewall → Turn Off
```

Alternatively, test connectivity first:
```bash
# From Laptop A, check if Laptop B is reachable:
curl http://192.168.1.102:5002/status
```

---

## Step 1 — Fresh Start on All 3 Laptops

Run this on each laptop before the demo:

```bash
./reset.sh
```

This wipes all Raft state and logs so you start clean.

---

## Step 2 — Start Each Node

Run on each laptop simultaneously (or one by one — order doesn't matter):

**Laptop A:**
```bash
./bin/raft-kms --config configs/node1.json --log-dir ./logs
```

**Laptop B:**
```bash
./bin/raft-kms --config configs/node2.json --log-dir ./logs
```

**Laptop C:**
```bash
./bin/raft-kms --config configs/node3.json --log-dir ./logs
```

Or use `start_cluster.sh` on each laptop (it will only start the node for that machine's config — you'll need to edit it to only start one node per machine, or just run the binary directly as above).

Within 3 seconds, one node wins the election and becomes LEADER. You'll see it in the terminal output:

```
[LEADER] 🎉 Node node2 became LEADER for Term 1 | LogLen=0 | Peers=[node1, node3]
```

---

## Step 3 — Open the Dashboard

On the laptop running the dashboard (or any laptop):

```bash
cd dashboard && npm run dev
```

Open `http://localhost:5173` in a browser.

In the login screen:
- Cluster Root Node field: enter `192.168.1.101:5001, 192.168.1.102:5002, 192.168.1.103:5003`
- API Key: `admin-secret-key`
- Click Authenticate

**What to show the audience:**
- 3 node orbs connected by lines — this is the actual network topology
- The LEADER orb glowing green
- Packet animations flying between nodes (heartbeats every 500ms)
- The term number on each node

---

## Step 4 — Create Journal Keys

In the dashboard, under "Cryptographic Workspace":

1. Create key: `alice`
2. Create key: `bob`

Watch the packet animations — the leader sends AppendEntries to both followers, they acknowledge, the entry commits. All 3 nodes now have the key in their state machine.

Check the logs on each laptop — they all show the same `[APPLY]` line for `CREATE_KEY`.

---

## Step 5 — Encrypt a Journal Entry

In the dashboard, under "Data Encryption (AES-GCM)":

- Select key: `alice`
- Plaintext: `Today was a great day. I finally understood how Raft consensus works.`
- Click **Encrypt & Audit**

You get back a ciphertext. Point out:
- This ciphertext is what would be stored in MongoDB Atlas
- The plaintext never left the KMS cluster
- The audit trail on the left now shows the ENCRYPT operation

---

## Step 6 — The Failover Demo (The Main Event)

This is the moment. The audience is watching the dashboard.

**Narrate as you do it:**

> "Right now node2 is the leader. It's sending heartbeats to node1 and node3 every 500ms. Watch what happens when I disconnect it from the network."

**Physically disconnect Laptop B (the leader) from WiFi.**

Turn off WiFi on Laptop B. Don't close the terminal — the node is still running, it just can't reach the others.

**What the audience sees on the dashboard (within ~3 seconds):**
1. The node2 orb turns red/offline
2. The packet animations from node2 stop
3. One of the remaining nodes (node1 or node3) starts an election — its orb turns yellow (CANDIDATE)
4. It sends RequestVote to the other node — vote animation fires
5. Vote granted → new LEADER elected — orb turns green
6. Heartbeat animations resume between the 2 remaining nodes

**In the logs on Laptop A or C:**
```
[ELECTION] ⚡ Node node1 timed out waiting for heartbeat — starting election | Term=2
[ELECTION] Node node1: Voted for self (1/2). Requesting votes from peers...
[ELECTION] Node node1 ← node3: Vote GRANTED ✓ | Votes=2/2 (need 2)
[LEADER] 🎉 Node node1 became LEADER for Term 2
```

**Now immediately encrypt another entry** — it works. The cluster is serving requests with 2/3 nodes.

> "The journal app is still running. Alice can still read and write her entries. MongoDB Atlas is unaffected — it's cloud-hosted. The only thing that changed is which node is the leader."

---

## Step 7 — Reconnect the Dead Laptop

Reconnect Laptop B to WiFi.

**What happens automatically:**
- node2 comes back online as FOLLOWER (it can't reclaim leadership — Term 2 > Term 1)
- The current leader detects it and sends AppendEntries to catch it up
- node2 syncs its log to match the current state

**In the logs on Laptop B:**
```
[SYNC] Node node2 ← node1: Appended 1 entries | LogLen now=3 | commitIndex=3
[APPLY] Node node2: Applying entry Index=3 Term=2 Action=AUDIT_LOG
```

**On the dashboard:**
- node2 orb comes back online as FOLLOWER
- Its log_length and commit_index catch up to match the other nodes

> "node2 is back. It's a follower now — the new leader stays in charge. All 3 nodes have identical state. The key alice created before the crash is still there. The ciphertext we encrypted is still decryptable."

---

## Step 8 — Verify Full Recovery

Decrypt the ciphertext you encrypted in Step 5 — it still works. The key survived the failover because it was committed to a majority before the leader died.

Check the audit trail — every operation is logged with timestamp and username. This is replicated through Raft, so even if a node crashes, the audit log is preserved.

---

## Step 9 — Optional: Kill 2 Nodes (Show Quorum Failure)

For extra credit, disconnect 2 laptops from WiFi.

The remaining node cannot form a majority (1/3 < 2). It will:
- Keep trying to start elections
- Never win (can't get 2 votes with only 1 node)
- Refuse to process write requests

Try to create a key — it fails with "no leader available". This is correct behavior — Raft prioritizes consistency over availability. It won't accept writes that can't be safely replicated.

Reconnect one laptop — quorum is restored (2/3), a new leader is elected, and the cluster resumes.

---

## What to Say to Your Teacher

> "Each laptop is a completely independent process. They communicate only over HTTP on the local WiFi network. When I disconnected Laptop B, the other two detected the missing heartbeat within 1.5 to 3 seconds — that's the election timeout. They held an election, one became the new leader by getting a majority vote, and the system continued serving requests. The journal app never went down. The encryption keys are safe because Raft requires a majority to commit any write — so by the time a key is usable, it already exists on at least 2 out of 3 nodes."

---

## Troubleshooting

**Nodes can't see each other:**
- Check that all laptops are on the same WiFi network (not guest vs main)
- Try `ping 192.168.1.102` from Laptop A
- Check macOS firewall settings
- Make sure the port (5001/5002/5003) isn't blocked

**Election never happens:**
- Check the config files — the `peers` list must use the actual IP addresses, not `localhost`
- Verify the node started with the right config: look for the startup banner in the terminal

**Dashboard shows nodes as offline:**
- Make sure the DEFAULT_NODES in App.jsx was updated with real IPs
- Check browser console for CORS errors — the Go backend allows all origins by default

**After reconnecting, node doesn't sync:**
- Give it 2–3 seconds — the leader sends heartbeats every 500ms
- Check the log on the reconnected laptop for `[SYNC]` lines

---

## Cleanup

On each laptop:
```bash
pkill -f "raft-kms --config"
```

For a fresh start next time:
```bash
./reset.sh
```
