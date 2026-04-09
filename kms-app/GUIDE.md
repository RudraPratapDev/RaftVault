# Glass Foundry KMS — User Guide

A complete guide to using the Glass Foundry Key Management System application.

---

## 1. Prerequisites & Setup

### Install Python Dependencies (first time only)

The Security Audit feature requires Python 3.9+ and a few packages:

```bash
pip3 install flask flask-cors numpy pandas scipy scikit-learn joblib tqdm --break-system-packages
```

### Start the Backend

The KMS app talks to a backend node. Start at least one node from the project root:

```bash
# Build the backend (first time only)
go build -o bin/raft-kms ./cmd/main.go

# Start node 1
./bin/raft-kms --config configs/node1.json
```

For a full 3-node cluster (recommended), open three terminals:

```bash
./bin/raft-kms --config configs/node1.json   # Terminal 1
./bin/raft-kms --config configs/node2.json   # Terminal 2
./bin/raft-kms --config configs/node3.json   # Terminal 3
```

Node 1 listens on `localhost:5001` by default. The frontend proxies all API calls through this address.

### Start the Frontend

```bash
cd kms-app
npm install       # first time only
npm run dev
```

Open your browser at **http://localhost:5173**

### Start the BitSecure Analysis Service

The Security Audit tab requires the BitSecure ML service running separately:

```bash
python3 crypto/server.py
```

This loads the trained Random Forest models and starts a Flask server on **port 7777**. The Vite dev proxy forwards `/bitsecure/*` requests to it automatically.

You should see:
```
[BitSecure] Loaded RF model for len=256
[BitSecure] Loaded RF model for len=4096
[BitSecure] Starting analysis server on port 7777...
```

> The service is optional — all other KMS features work without it. The Security Audit page shows an "offline" indicator if it's not running.

---

## 2. Logging In

On the login screen you'll see two fields:

- **Operator ID** — your username
- **Master Key** — your API key (used as the password)

Default admin credentials:

| Field | Value |
|---|---|
| Operator ID | `admin` |
| Master Key | `admin-secret-key` |

Click **Initialize Session** to enter the dashboard.

Your session is persisted in `localStorage` so you stay logged in across page refreshes. Click **Logout** in the sidebar to end your session.

---

## 3. Navigation

The sidebar has six sections:

| Section | Description |
|---|---|
| Overview | Dashboard with stats and recent activity |
| Key Registry | Create, rotate, and delete cryptographic keys |
| Encrypt / Decrypt | Perform AES-256-GCM crypto operations |
| Audit Log | Immutable record of all crypto operations |
| Security Audit | ML-powered randomness analysis of key material |
| User Management | Create and remove users *(admin only)* |

---

## 4. Overview

The Overview page gives you a live snapshot of the vault:

- **Stat cards** — total active keys, crypto operations count, audit entries, and user count. Click any card to navigate to that section.
- **Key Registry table** — the 5 most recent keys with their status and version count.
- **Recent Activity** — the 5 most recent audit log entries (encrypt/decrypt operations).
- **Cryptographic Health** — shows the percentage of keys that are active vs archived.

Click **Refresh** (top right) to reload all data from the backend.

---

## 5. Key Registry

### Creating a Key

1. Go to **Key Registry**.
2. In the "Create New Key" form, enter a unique Key ID (e.g. `prod-db-key-001`).
3. Click **Create Key**.

A 256-bit AES key is generated server-side and stored securely. You never see the raw key material in full — only a truncated preview in the version history.

### Viewing Key Versions

Click the chevron (▾) next to any key to expand its version history. Each rotation creates a new version. The current (latest) version is used for new encryptions; older versions are retained so existing ciphertext can still be decrypted.

### Rotating a Key

Click **Rotate** next to an active key. A new key version is generated and becomes the active version for future encryptions. Old ciphertext encrypted with previous versions can still be decrypted — the version is embedded in the ciphertext.

### Deleting (Archiving) a Key

Click **Delete** next to an active key, then confirm in the dialog. The key is soft-deleted (status changes to `deleted`). Ciphertext encrypted with a deleted key **cannot be decrypted** afterwards, so archive keys only when you're sure they're no longer needed.

---

## 6. Encrypt / Decrypt

### Encrypting Data

1. Go to **Encrypt / Decrypt** and make sure the **Encrypt** tab is selected.
2. Choose an active key from the dropdown.
3. Enter the plaintext you want to protect.
4. Click **Encrypt Data**.

The output is a base64-encoded ciphertext that includes a version prefix. Copy it using the **Copy** button and store it wherever needed.

### Decrypting Data

1. Switch to the **Decrypt** tab.
2. Select the same key that was used for encryption.
3. Paste the ciphertext.
4. Click **Decrypt Data**.

The original plaintext is displayed. The system automatically uses the correct key version based on the version prefix embedded in the ciphertext.

> Every encrypt and decrypt operation is automatically recorded in the Audit Log.

---

## 7. Audit Log

The Audit Log shows every cryptographic operation performed through the system. It is **immutable** — entries are committed through distributed consensus and cannot be modified or deleted.

### Filtering

- **Search box** — filter by key ID or username.
- **Action filter** — show ALL, ENCRYPT-only, or DECRYPT-only entries.

### Columns

| Column | Description |
|---|---|
| Timestamp | When the operation occurred |
| Action | ENCRYPT or DECRYPT |
| Key ID | Which key was used |
| User | Which operator performed the action |

The log is displayed in reverse chronological order (newest first).

---

## 8. User Management *(Admin only)*

This section is only visible to users with the `admin` role.

### Creating a User

1. Go to **User Management**.
2. Enter a username and select a role:
   - **Service** — can encrypt and decrypt, list keys, and view the audit log.
   - **Admin** — full access including creating/deleting keys and managing users.
3. Click **Create User**.

A unique API key is generated and displayed **once** in a banner at the top of the page. Copy it immediately and share it securely with the user — it will not be shown again (though admins can view masked keys in the table).

### Viewing API Keys

Click the eye icon (👁) next to any user to reveal their API key. Click **Copy** to copy it to the clipboard.

### Removing a User

Click **Remove** next to a user, then confirm in the dialog. The user's API key is immediately invalidated. The default `admin` user cannot be removed.

---

## 9. Authentication

The KMS uses API key authentication. The API key is sent as a Bearer token in every request:

```
Authorization: Bearer <api_key>
```

When you log in, the API key is stored in `localStorage` and automatically attached to all requests by the frontend. If you need to use the API directly (e.g. from a script or another service), use the same header.

---

## 10. Roles & Permissions

| Operation | Service | Admin |
|---|---|---|
| Login | ✓ | ✓ |
| List keys | ✓ | ✓ |
| Get key details | ✓ | ✓ |
| Encrypt / Decrypt | ✓ | ✓ |
| View audit log | ✓ | ✓ |
| List users | ✓ | ✓ |
| Create key | ✗ | ✓ |
| Delete key | ✗ | ✓ |
| Rotate key | ✗ | ✓ |
| Create user | ✗ | ✓ |
| Delete user | ✗ | ✓ |

---

## 11. Troubleshooting

**"Vault: Operational" shows but data doesn't load**
- Make sure the backend node is running on `localhost:5001`.
- Check the browser console for network errors.

**Login fails with "invalid api key"**
- The password field expects the API key, not a traditional password.
- Default: `admin-secret-key`.

**"not leader" errors**
- The frontend automatically retries requests against the cluster leader. If you see this briefly, it's normal during a leader election. Wait a few seconds and try again.

**Bind address error on startup**
- If the node fails to bind, check that `configs/node1.json` uses an address available on your machine (e.g. `localhost:5001`).
- Update `kms-app/vite.config.js` proxy target if you change the port.

**Key operations fail with "forbidden"**
- Only admin users can create, rotate, or delete keys. Log in with an admin account.

**BitSecure shows "offline"**
- Run `python3 crypto/server.py` from the project root.
- Make sure the Python dependencies are installed: `pip3 install flask flask-cors numpy pandas scipy scikit-learn joblib tqdm --break-system-packages`
- The service must be on port 7777.

---

## 12. Security Audit (BitSecure)

The Security Audit page uses a trained Random Forest classifier to analyze the randomness quality of your key material. This is the BitSecure ML pipeline integrated directly into the KMS.

### How It Works

1. You select an active key from the dropdown.
2. The frontend fetches the key's raw base64 material from the Go backend (`/kms/keyMaterial`).
3. That material is sent to the local BitSecure Python service (`/bitsecure/analyze`).
4. The service extracts 18 statistical features from the key's bits and runs them through the trained model.
5. Results are displayed in the dashboard — no key material ever leaves your local network.

### What You See

**ML Verdict** — the Random Forest's classification: `SECURE` or `POTENTIALLY WEAK`, with a probability breakdown and confidence score.

**Bit Heatmap** — a 16×16 grid showing the first 256 bits of the key. Indigo = 1, white = 0. A truly random key should look like noise with no visible patterns.

**Byte Distribution** — a bar chart of the first 32 byte values. Secure keys have roughly uniform distribution.

**Autocorrelation Profile** — serial correlation at lags 1–5. Values near 0 mean bits are independent. High values (shown in amber) indicate structure — a hallmark of weak PRNGs like LCGs and LFSRs.

**NIST SP 800-22 Tests** — three core NIST randomness tests run locally:
- Frequency (Monobit) — proportion of 1s should be ~0.5
- Runs Test — oscillation rate between 0s and 1s
- Block Frequency — frequency within 128-bit blocks

**Feature Importance** — which of the 18 statistical features the model weighted most heavily for this prediction.

**All 18 Features** — the raw computed values for every feature used by the model.

### Model Performance

The Random Forest was trained on 320,000 bit sequences from 8 generators (5 weak: LCG, LFSR, MT19937, RC4-weak, C-rand; 3 secure: /dev/urandom, AES-CTR-DRBG, ChaCha20):

| Sequence Length | ROC-AUC | Notes |
|---|---|---|
| 256 bits | 0.958 | Higher false-positive rate at short lengths |
| 4096 bits | 0.947 | Good discrimination |
| 16384 bits | 0.975 | Best performance |

Keys generated by this system use Go's `crypto/rand` (backed by the OS CSPRNG) and will typically score as secure. A "Potentially Weak" result at 256 bits is not alarming — the model has a higher false-positive rate at short lengths. It's a signal to investigate, not a definitive failure.
