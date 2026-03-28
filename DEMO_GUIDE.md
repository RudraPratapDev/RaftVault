# Enterprise RaftKMS: A Distributed P2P Security Console

This document explains what RaftKMS is, the engineering decisions behind it, and exactly how to host a live, multi-laptop demonstration with your team over a shared Wi-Fi Hotspot.

---

## 1. What Has Been Built?

We have built a **fault-tolerant, fully decentralized Key Management System (KMS)** from scratch in Go, complete with a React-based **Enterprise Security Console**. 

Instead of relying on AWS KMS or HashiCorp Vault (which require centralized infrastructure), this project turns a group of standard laptops into an unbreakable, secure cryptographic cluster.

### Core Features
- **P2P Cryptography:** Keys are generated, rotated, and used to encrypt/decrypt (AES-256-GCM) entirely over the P2P network.
- **Role-Based Access (RBAC):** Users are assigned `Admin` or `Service` roles. Admins manage the cluster; Services can only encrypt data.
- **Cryptographic Audit Ledger:** Every time a payload is encrypted, an inseparable audit log is written to the decentralized ledger. 
- **Deep Space Glassmorphism UI:** A premium, real-time dashboard featuring an **animated SVG Network Topology** that tracks live data packets shooting between your team's laptops.
- **Split-Brain Simulator:** Allows you to simulate network fractures and watch the cluster heal itself.

---

## 2. Why Raft? (And How It Works)

### The Problem
In standard web applications, if the primary database goes offline, the whole app crashes. In standard cryptography systems, if the key-server goes offline, you cannot decrypt passwords or data.

### The Solution: Raft Consensus
Raft is an industry-standard "Distributed Consensus Algorithm". It forces a group of independent machines to agree on a shared state. 
- **How we used it:** We implemented Raft completely from scratch in Go. 
- **Leader Election:** When the laptops boot up, they vote for a "LEADER". Only the Leader can process write commands (like creating a key). 
- **Log Replication:** If you ask the Leader to encrypt a file, the Leader securely copies (replicates) this audit log to the other laptops (FOLLOWERS). Only when a *majority* of laptops confirm they saved the log does the Leader complete the encryption.
- **Fault Tolerance:** If the Leader's laptop physically dies or loses Wi-Fi connection, the remaining laptops notice the heartbeat stopped. They instantly hold a new election, elect a new Leader, and everything continues functioning with **zero data loss**.

---

## 3. How to Run the "Shared Hotspot" Demo

To demo this perfectly, grab your teammates and connect everyone to the **exact same Wi-Fi network or Mobile Hotspot**. 

### Step 1: Find Everyone's IP Addresses
Have each person open their terminal and find their local Wi-Fi IPv4 address (e.g., `192.168.1.10`).
- **Mac/Linux:** `ifconfig | grep inet`
- **Windows:** `ipconfig`

### Step 2: Generate the Network Configs
1. **On YOUR computer**, open the terminal inside the project folder:
   ```bash
   ./generate_configs.sh
   ```
2. The script will ask for the 3 IP addresses. Type them in carefully. 
3. This creates three heavily customized files in `configs/network/` (`node1.json`, `node2.json`, `node3.json`).

### Step 3: Share the Code
Take the entire `distProject` repository (including the newly generated JSON configs) and ZIP it up. Slack, Airdrop, or USB it to the other two teammates. 

*Everybody must have the exact same folder.*

### Step 4: Boot Up the Distributed Backend
1. **You (Node 1):**
   ```bash
   go build -o bin/raft-kms ./cmd/main.go
   ./bin/raft-kms --config configs/network/node1.json
   ```
2. **Teammate 1 (Node 2):**
   ```bash
   go build -o bin/raft-kms ./cmd/main.go
   ./bin/raft-kms --config configs/network/node2.json
   ```
3. **Teammate 2 (Node 3):**
   ```bash
   go build -o bin/raft-kms ./cmd/main.go
   ./bin/raft-kms --config configs/network/node3.json
   ```

### Step 5: Boot Up the Premium Dashboard
*Any* (or all!) of the laptops can act as the monitoring screen.
1. Open a new terminal tab in the `dashboard/` directory.
2. Run:
   ```bash
   npm install
   npm run dev
   ```
3. Open `http://localhost:5173` in your web browser.
4. **Important:** On the login screen, enter the three IP addresses exactly as they were typed in Step 2. (e.g., `192.168.1.10:5001, 192.168.1.11:5001, 192.168.1.12:5001`).
5. Enter the default API Access Key: `admin-secret-key`.

---

## 4. What Will You See? (The "Wow" Factor)

Once everyone logs into the dashboard, this is what happens:

1. **The Live Network Map:** You will see three floating, glowing orbs representing the three actual laptops in the room. 
2. **Animated Network Packets:** You will visually see neon-colored light streams shooting across the SVGs connecting the laptops. These represent the Raft Heartbeats ensuring the cluster is alive.
3. **Triggering a Failover (The big moment):**
   - Identify the Green "LEADER" orb on the screen (let's say it's Teammate 1's laptop).
   - Tell Teammate 1 to physically press `Ctrl+C` in their terminal to kill their Go server (or just disconnect their Wi-Fi).
   - **Watch the Dashboard:** Everyone will instantly see the glowing packets stop. The Leader orb turns red. After exactly 1.5 seconds, the remaining laptops will flash yellow (Election phase) and one will turn Green. The cluster just healed itself instantly!
4. **Audit Cryptography:** Go to the KMS panel, create a key, and encrypt a message. Because you did this while Teammate 1 was completely offline, the remaining two laptops recorded the encryption. When Teammate 1 reboots their server, watch the data packets swarm their orb as the Raft log brings them entirely up to speed automatically!

This is a true, industry-grade implementation of distributed computing, right in your living room.
