# RaftKMS — Localhost Demo Guide

Everything runs on one machine. You simulate the distributed system by running 3 processes on different ports. Leader failure is simulated via the chaos API (not by physically disconnecting anything).

---

## Prerequisites

- Go 1.21+
- Node.js 18+ (for the React dashboard)
- A terminal multiplexer like iTerm2 tabs, or just 4 terminal windows
- Postman (optional, for the journal scenario section)

---

## Step 1 — Fresh Start

Always begin a demo from a clean state so the audience sees a real election from scratch.

```bash
./reset.sh
```

This wipes all Raft state, logs, and persisted data. You'll see:
```
[1/3] Stopping any running nodes...
[2/3] Wiping Raft state (data/)...
[3/3] Wiping logs (logs/)...
✅ Reset complete.
```

---

## Step 2 — Start the Cluster

```bash
./start_cluster.sh
```

This builds the binary, starts 3 nodes on ports 5001/5002/5003, waits 3 seconds, then prints the status of each node.

**What to point out to your teacher:**
- All 3 nodes boot as FOLLOWER
- Within ~1.5–3 seconds one node times out waiting for a heartbeat and starts an election
- It increments its term, votes for itself, and sends RequestVote to the other two
- The other two grant their votes → majority reached → that node becomes LEADER

You'll see something like:
```
--- Node on port 5001 ---
{ "node_id": "node1", "role": "FOLLOWER", "current_term": 1, "leader_id": "node3" }

--- Node on port 5002 ---
{ "node_id": "node2", "role": "FOLLOWER", "current_term": 1, "leader_id": "node3" }

--- Node on port 5003 ---
{ "node_id": "node3", "role": "LEADER", "current_term": 1, "leader_id": "node3" }
```

---

## Step 3 — Open the Dashboard

In a new terminal:

```bash
cd dashboard
npm run dev
```

Open `http://localhost:5173` in your browser.

Log in with:
- API Key: `admin-secret-key`

**What to show:**
- The 3 node orbs in the topology — one glowing green (LEADER), two blue (FOLLOWER)
- The term number on each node
- The live event stream on the right showing heartbeats and replication events

---

## Step 4 — Open the Test Demo Page

Open a second browser tab:

```
http://localhost:5001/test/demo
```

This page requires no login. It's your quick-action panel for the demo. You can:
- Create keys without auth
- Encrypt/decrypt text
- Kill and revive nodes
- Watch the live event stream

---

## Step 5 — Create a Key (Journal User Setup)

In the test demo page, under "Create Key":

- Key ID: `alice`
- Click **Create Key via Raft**

Watch what happens in the event stream:
1. The request hits the leader
2. Leader appends a `CREATE_KEY` entry to its log
3. Leader sends AppendEntries to both followers
4. Both followers acknowledge → majority → entry committed
5. All 3 nodes apply the entry to their state machine

In the logs (`tail -f logs/node3.log` if node3 is leader):
```
[RAFT] Leader node3: Appended entry Index=1 Term=1 Action=CREATE_KEY
[REPLICATION] Leader node3 → node1: Replicated 1 entries | matchIndex=1
[REPLICATION] Leader node3 → node2: Replicated 1 entries | matchIndex=1
[COMMIT] Leader node3: Entry Index=1 replicated on 3/3 nodes — COMMITTED ✓
[APPLY] Node node3: Applying entry Index=1 Term=1 Action=CREATE_KEY
```

Create a second key too:
- Key ID: `bob`

---

## Step 6 — Encrypt a Journal Entry

In the test demo page, under "Encrypt / Decrypt":

- Key ID: `alice`
- Plaintext: `Today I finally finished my distributed systems project. It was hard but worth it.`
- Click **Encrypt**

You get back a base64 ciphertext. This is what would be stored in MongoDB Atlas — the actual text never touches the database.

Copy the ciphertext. Now decrypt it:
- Paste it into the Ciphertext field
- Key ID: `alice`
- Click **Decrypt**

You get the original text back. **This is the core KMS demo** — encrypt before store, decrypt on read.

---

## Step 7 — Watch the Logs Live

Open 3 terminal tabs and run:

```bash
tail -f logs/node1.log
tail -f logs/node2.log
tail -f logs/node3.log
```

You'll see heartbeats every 500ms from the leader to followers, and the follower logs showing they're receiving them. Point out:
- `[SYNC]` lines showing followers staying in sync
- `[APPLY]` lines showing the state machine being updated on every node
- The term number staying consistent across all 3

---

## Step 8 — Simulate Leader Failure (The Key Demo Moment)

This is what you're showing your teacher. The system survives a leader crash.

**Before killing the leader, do one more encrypt** so there's a pending operation in the audience's mind.

Now in the test demo page, under "Chaos / Failover Simulation":

1. Select the leader node from the dropdown (check which one is LEADER in the cluster status card)
2. Click **💀 Kill Node**

**What happens (watch the event stream and logs):**
- The killed node stops responding to heartbeats
- The other 2 nodes wait ~1.5–3 seconds (election timeout)
- One of them times out first, increments the term, becomes CANDIDATE
- It sends RequestVote to the other follower
- The other follower grants its vote → new LEADER elected

In the logs you'll see:
```
[ELECTION] ⚡ Node node1 timed out waiting for heartbeat — starting election | Term=2
[ELECTION] Node node1: Voted for self (1/2). Requesting votes from peers...
[ELECTION] Node node1 ← node2: Vote GRANTED ✓ | Votes=2/2 (need 2)
[LEADER] 🎉 Node node1 became LEADER for Term 2
```

**Now immediately try to encrypt something** — it works. The cluster is still serving requests with 2/3 nodes.

---

## Step 9 — Revive the Dead Node

In the test demo page:
1. Select the same node you killed
2. Click **✅ Revive Node**

**What happens:**
- The revived node comes back as FOLLOWER (it can't reclaim leadership)
- The current leader sends it AppendEntries to catch it up
- The revived node syncs its log to match the current state

In the logs:
```
[SYNC] Node node3 ← node1: Appended 2 entries | LogLen now=3 | commitIndex=3
[APPLY] Node node3: Applying entry Index=2 Term=2 Action=CREATE_KEY
```

The revived node is now fully caught up. All 3 nodes have identical state.

---

## Step 10 — Verify Data Survived

Check the cluster status — all 3 nodes should show the same `log_length` and `commit_index`. The keys `alice` and `bob` still exist. Decrypt the ciphertext you saved earlier — it still works because the key survived the failover.

---

## Postman Section — Journal App Scenario

This section simulates a journal app backend calling the KMS. Assume a user "alice" has registered and her journal entries need to be encrypted before going to MongoDB.

### Setup in Postman

Create a new collection called **RaftKMS Journal Demo**.

Set a collection variable:
- `base_url` = `http://localhost:5001`
- `admin_key` = `admin-secret-key`

---

### Request 1 — Create Alice's Encryption Key

This happens when Alice registers in the journal app. The backend calls KMS to provision her personal key.

```
POST {{base_url}}/kms/createKey
Authorization: Bearer {{admin_key}}
Content-Type: application/json

{
  "key_id": "journal-alice"
}
```

Expected response:
```json
{
  "message": "key created",
  "key": {
    "key_id": "journal-alice",
    "status": "active",
    "versions": [{ "version": 1, ... }]
  }
}
```

Save `journal-alice` — this is Alice's personal KMS key. Even if someone dumps MongoDB, they can't read her entries without this key.

---

### Request 2 — Alice Writes a Journal Entry (Encrypt)

Alice types her entry. The journal backend calls KMS before saving to MongoDB.

```
POST {{base_url}}/kms/encrypt
Authorization: Bearer {{admin_key}}
Content-Type: application/json

{
  "key_id": "journal-alice",
  "plaintext": "March 31 — Had a great day. Finished the distributed systems project. Feeling proud."
}
```

Expected response:
```json
{
  "ciphertext": "AQAAAA..."
}
```

**What you'd store in MongoDB:**
```json
{
  "user_id": "alice",
  "date": "2026-03-31",
  "ciphertext": "AQAAAA..."
}
```

The actual journal text never touches MongoDB. Only the encrypted blob does.

---

### Request 3 — Alice Reads Her Journal Entry (Decrypt)

Alice opens her journal. The backend fetches the ciphertext from MongoDB, then calls KMS to decrypt.

```
POST {{base_url}}/kms/decrypt
Authorization: Bearer {{admin_key}}
Content-Type: application/json

{
  "key_id": "journal-alice",
  "ciphertext": "AQAAAA..."
}
```

Expected response:
```json
{
  "plaintext": "March 31 — Had a great day. Finished the distributed systems project. Feeling proud."
}
```

Alice sees her entry normally. She has no idea encryption happened.

---

### Request 4 — Write a Second Entry

```
POST {{base_url}}/kms/encrypt
Authorization: Bearer {{admin_key}}
Content-Type: application/json

{
  "key_id": "journal-alice",
  "plaintext": "April 1 — Presented the project to my teacher. The failover demo worked perfectly."
}
```

Save this ciphertext too.

---

### Request 5 — Kill the Leader Mid-Session

Now simulate the leader going down while Alice is using the app.

```
POST http://localhost:5001/chaos/kill
```

(or whichever port is the current leader — check `/status` first)

Wait 3 seconds. A new leader is elected automatically.

---

### Request 6 — Decrypt After Failover

Now try to decrypt Alice's first entry again, but send the request to a different node (the new leader):

```
POST http://localhost:5002/kms/decrypt
Authorization: Bearer {{admin_key}}
Content-Type: application/json

{
  "key_id": "journal-alice",
  "ciphertext": "AQAAAA..."
}
```

It works. The key survived the failover because it was replicated to all 3 nodes via Raft before the leader died. Alice's journal is accessible even though the original leader is down.

---

### Request 7 — Check the Audit Trail

```
GET {{base_url}}/kms/auditLog
Authorization: Bearer {{admin_key}}
```

You'll see every ENCRYPT and DECRYPT operation logged with timestamp, username, and key ID. This is the immutable cryptographic audit trail replicated through Raft — even if a node crashes, the audit log is preserved.

---

### Request 8 — Revive the Dead Node

```
POST http://localhost:5001/chaos/revive
```

Then check its status:

```
GET http://localhost:5001/status
```

It comes back as FOLLOWER, syncs its log, and is fully operational again.

---

## What to Say to Your Teacher

> "Each node runs an independent Raft state machine. When the leader dies, the remaining nodes detect the missing heartbeat within 1.5–3 seconds and hold an election. The new leader is chosen by majority vote and must have the most up-to-date log. Once elected, it immediately starts serving requests. The journal app never noticed — it just retried on the next available node. The encryption keys are safe because they were committed to a majority of nodes before the leader died."

---

## Cleanup

```bash
pkill -f "raft-kms --config"
```

Or for a full reset before the next demo:

```bash
./reset.sh
```
