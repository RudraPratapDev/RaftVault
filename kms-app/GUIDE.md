# Glass Foundry KMS — User Guide

A complete guide to using the Glass Foundry Key Management System application.

---

## 1. Prerequisites & Setup

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

The sidebar has five sections:

| Section | Description |
|---|---|
| Overview | Dashboard with stats and recent activity |
| Key Registry | Create, rotate, and delete cryptographic keys |
| Encrypt / Decrypt | Perform AES-256-GCM crypto operations |
| Audit Log | Immutable record of all crypto operations |
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
