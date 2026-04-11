import { useState } from 'react';
import { Layers, Key, Link, Shield, ArrowDown, ArrowRight, ChevronDown, ChevronUp, Lock, Unlock } from 'lucide-react';

// ── Section wrapper ───────────────────────────────────────────────────────────
function Section({ title, icon: Icon, color, children, defaultOpen = true }) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <div className="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
      <button
        onClick={() => setOpen(v => !v)}
        className="w-full flex items-center justify-between px-6 py-4 hover:bg-gray-50 transition-colors"
      >
        <div className="flex items-center gap-3">
          <div className={`w-8 h-8 rounded-lg flex items-center justify-center ${color}`}>
            <Icon size={15} />
          </div>
          <span className="text-sm font-bold text-gray-900">{title}</span>
        </div>
        {open ? <ChevronUp size={14} className="text-gray-400" /> : <ChevronDown size={14} className="text-gray-400" />}
      </button>
      {open && <div className="px-6 pb-6 pt-2">{children}</div>}
    </div>
  );
}

// ── Code block ────────────────────────────────────────────────────────────────
function Code({ children }) {
  return (
    <pre className="bg-gray-900 text-emerald-400 rounded-lg p-4 text-[11px] font-mono leading-relaxed overflow-x-auto mt-3">
      {children}
    </pre>
  );
}

// ── Diagram box ───────────────────────────────────────────────────────────────
function Box({ label, sub, color = 'bg-indigo-50 border-indigo-200 text-indigo-800' }) {
  return (
    <div className={`rounded-lg border px-4 py-3 text-center ${color}`}>
      <div className="text-xs font-bold">{label}</div>
      {sub && <div className="text-[10px] opacity-70 mt-0.5">{sub}</div>}
    </div>
  );
}

function Arrow({ label }) {
  return (
    <div className="flex flex-col items-center gap-0.5 my-1">
      <ArrowDown size={16} className="text-gray-400" />
      {label && <span className="text-[10px] text-gray-400 font-mono">{label}</span>}
    </div>
  );
}

// ── Main ──────────────────────────────────────────────────────────────────────
export default function CryptoInternals() {
  return (
    <div className="p-8 max-w-4xl space-y-6">
      <div className="mb-2">
        <h1 className="text-3xl font-bold text-gray-900 tracking-tight">Crypto Internals</h1>
        <p className="text-sm text-gray-500 mt-1">
          A deep-dive into every cryptographic primitive used in this KMS — how they work, why they're used, and what they protect against.
        </p>
      </div>

      {/* 1. Envelope Encryption */}
      <Section title="1. Envelope Encryption (KEK / DEK)" icon={Layers} color="bg-violet-50 text-violet-600">
        <p className="text-sm text-gray-600 leading-relaxed mb-4">
          This is the most important architectural pattern in any real KMS (AWS KMS, Google Cloud KMS, HashiCorp Vault all use it).
          The master key stored in Raft <span className="font-semibold text-gray-800">never directly encrypts your data</span>.
          Instead, a fresh random Data Encryption Key (DEK) is generated per operation, used to encrypt the data,
          and then the DEK itself is encrypted by the Key Encryption Key (KEK) and stored alongside the ciphertext.
        </p>

        {/* Diagram */}
        <div className="flex flex-col items-center gap-0 mb-6">
          <Box label="Master Secret (256-bit)" sub="Stored in Raft cluster" color="bg-gray-100 border-gray-300 text-gray-700" />
          <Arrow label="HKDF-SHA256(master, context)" />
          <Box label="KEK — Key Encryption Key" sub="Derived at runtime, never stored" color="bg-violet-50 border-violet-200 text-violet-800" />
          <Arrow label="AES-256-GCM(KEK, DEK)" />
          <div className="flex items-center gap-6">
            <Box label="Wrapped DEK" sub="Travels with ciphertext" color="bg-amber-50 border-amber-200 text-amber-800" />
            <div className="flex flex-col items-center">
              <Box label="Fresh DEK (random)" sub="Generated per operation" color="bg-amber-50 border-amber-200 text-amber-800" />
              <Arrow label="AES-256-GCM(DEK, plaintext)" />
              <Box label="Ciphertext" sub="Your encrypted data" color="bg-indigo-50 border-indigo-200 text-indigo-800" />
            </div>
          </div>
        </div>

        <div className="bg-gray-50 rounded-lg p-4 text-[11px] text-gray-600 leading-relaxed border border-gray-100 mb-3">
          <span className="font-bold text-gray-800">Final output format:</span>{' '}
          <code className="font-mono bg-white px-1.5 py-0.5 rounded border border-gray-200">
            version(4B) | wrappedDEKLen(2B) | wrappedDEK(AES-GCM) | ciphertext(AES-GCM)
          </code>
          <br /><br />
          The version prefix allows automatic key version detection on decrypt — you can rotate keys and still decrypt old ciphertext.
        </div>

        <div className="grid grid-cols-2 gap-3 text-[11px]">
          <div className="bg-emerald-50 border border-emerald-100 rounded-lg p-3">
            <div className="font-bold text-emerald-700 mb-1">Why this matters</div>
            <ul className="text-gray-600 space-y-1 list-disc list-inside">
              <li>Rotating the master key only re-wraps the DEK — no re-encryption of data</li>
              <li>Compromise of one DEK only exposes one message</li>
              <li>KEK never leaves memory — derived fresh each time</li>
            </ul>
          </div>
          <div className="bg-blue-50 border border-blue-100 rounded-lg p-3">
            <div className="font-bold text-blue-700 mb-1">Real-world analogy</div>
            <p className="text-gray-600">
              Like a safety deposit box: the bank (KEK) holds a key that unlocks a box containing your key (DEK),
              which opens your actual vault (data). The bank never sees your data.
            </p>
          </div>
        </div>

        <Code>{`// Go implementation (internal/kms/kms.go)

// 1. Derive KEK from master secret using HKDF
info := []byte(fmt.Sprintf("%s:%d:%s", keyID, version, createdAt))
kek  := HKDF(masterSecret, salt=nil, info, length=32)

// 2. Generate a fresh random DEK per operation
dek := make([]byte, 32)
io.ReadFull(rand.Reader, dek)

// 3. Wrap DEK with KEK (AES-256-GCM)
wrappedDEK := AES_GCM_Encrypt(kek, dek)

// 4. Encrypt plaintext with DEK (AES-256-GCM)
ciphertext := AES_GCM_Encrypt(dek, plaintext)

// 5. Output: version || wrappedDEKLen || wrappedDEK || ciphertext`}</Code>
      </Section>

      {/* 2. HKDF */}
      <Section title="2. HKDF-SHA256 Key Derivation" icon={Key} color="bg-amber-50 text-amber-600">
        <p className="text-sm text-gray-600 leading-relaxed mb-4">
          HKDF (HMAC-based Key Derivation Function, RFC 5869) is used to derive the KEK from the master secret.
          Raw random bytes are not ideal as key material — HKDF extracts entropy and expands it into a
          cryptographically strong key bound to a specific context.
        </p>

        <div className="flex flex-col items-center gap-0 mb-5">
          <div className="flex items-center gap-4">
            <Box label="Master Secret (IKM)" sub="Input Key Material" color="bg-gray-100 border-gray-300 text-gray-700" />
            <ArrowRight size={16} className="text-gray-400" />
            <Box label="HKDF-Extract" sub="HMAC-SHA256(salt, IKM)" color="bg-amber-50 border-amber-200 text-amber-800" />
            <ArrowRight size={16} className="text-gray-400" />
            <Box label="PRK" sub="Pseudo-Random Key" color="bg-amber-100 border-amber-300 text-amber-900" />
          </div>
          <Arrow />
          <div className="flex items-center gap-4">
            <Box label="HKDF-Expand" sub="HMAC-SHA256(PRK, info || counter)" color="bg-amber-50 border-amber-200 text-amber-800" />
            <ArrowRight size={16} className="text-gray-400" />
            <Box label="KEK (32 bytes)" sub="Context-bound output" color="bg-violet-50 border-violet-200 text-violet-800" />
          </div>
        </div>

        <div className="bg-gray-50 rounded-lg p-4 text-[11px] text-gray-600 border border-gray-100 mb-3">
          <span className="font-bold text-gray-800">Context binding (info parameter):</span>{' '}
          <code className="font-mono bg-white px-1.5 py-0.5 rounded border border-gray-200">
            "{'{'}keyID{'}'}:{'{'}version{'}'}:{'{'}createdAt{'}'}"
          </code>
          <br /><br />
          This means the same master secret produces a <span className="font-semibold">different KEK for every key version</span>.
          Rotating a key changes the context, which changes the KEK — old ciphertext still decrypts because the version is embedded in the output.
        </div>

        <Code>{`// HKDF-Extract: HMAC-SHA256(salt, IKM) → PRK
func hkdfExtract(salt, ikm []byte) []byte {
    mac := hmac.New(sha256.New, salt)
    mac.Write(ikm)
    return mac.Sum(nil)
}

// HKDF-Expand: iterative HMAC to produce output key material
func hkdfExpand(prk, info []byte, length int) []byte {
    var okm, t []byte
    for i := byte(1); len(okm) < length; i++ {
        mac := hmac.New(sha256.New, prk)
        mac.Write(t); mac.Write(info); mac.Write([]byte{i})
        t = mac.Sum(nil)
        okm = append(okm, t...)
    }
    return okm[:length]
}`}</Code>
      </Section>

      {/* 3. HMAC-SHA256 Audit Chain */}
      <Section title="3. HMAC-SHA256 Chained Audit Trail" icon={Link} color="bg-indigo-50 text-indigo-600">
        <p className="text-sm text-gray-600 leading-relaxed mb-4">
          Every audit entry is linked to the previous one via HMAC-SHA256. This creates a tamper-evident chain —
          modifying any past entry invalidates all subsequent hashes, making tampering immediately detectable.
          This is the same principle used in blockchain and certificate transparency logs.
        </p>

        <div className="flex items-center gap-2 overflow-x-auto pb-2 mb-5">
          {[
            { label: 'Genesis', sub: '0000…0000', color: 'bg-gray-100 border-gray-300 text-gray-700' },
            { label: 'Entry 1', sub: 'HMAC(prev|data)', color: 'bg-indigo-50 border-indigo-200 text-indigo-800' },
            { label: 'Entry 2', sub: 'HMAC(prev|data)', color: 'bg-indigo-50 border-indigo-200 text-indigo-800' },
            { label: 'Entry N', sub: 'HMAC(prev|data)', color: 'bg-indigo-50 border-indigo-200 text-indigo-800' },
          ].map((b, i) => (
            <div key={i} className="flex items-center gap-2 shrink-0">
              <Box {...b} />
              {i < 3 && <ArrowRight size={16} className="text-gray-300" />}
            </div>
          ))}
        </div>

        <div className="bg-gray-50 rounded-lg p-4 text-[11px] text-gray-600 border border-gray-100 mb-3">
          <span className="font-bold text-gray-800">Hash formula:</span>
          <br />
          <code className="font-mono bg-white px-1.5 py-0.5 rounded border border-gray-200 mt-1 block">
            current_hash = HMAC-SHA256(auditKey, prev_hash | timestamp | username | action | key_id)
          </code>
          <br />
          The chain is verified by re-computing every HMAC from the genesis hash and checking each link.
          Go to the Audit Log page and click "Verify Chain" to see this live.
        </div>

        <div className="grid grid-cols-2 gap-3 text-[11px] mb-3">
          <div className="bg-red-50 border border-red-100 rounded-lg p-3">
            <div className="font-bold text-red-700 mb-1">What happens if tampered?</div>
            <p className="text-gray-600">
              Changing entry 3's action from DECRYPT to ENCRYPT changes its hash.
              Entry 4's previous_hash no longer matches → chain breaks at entry 4.
              The verifier reports exactly which entry was tampered.
            </p>
          </div>
          <div className="bg-emerald-50 border border-emerald-100 rounded-lg p-3">
            <div className="font-bold text-emerald-700 mb-1">Distributed guarantee</div>
            <p className="text-gray-600">
              Audit entries are committed through Raft consensus — all 3 nodes hold the same chain.
              An attacker would need to tamper with a majority of nodes simultaneously.
            </p>
          </div>
        </div>

        <Code>{`// internal/kms/kms.go — applyAuditLog
func (s *KMSStore) applyAuditLog(p AuditLogPayload) (interface{}, error) {
    p.Entry.PreviousHash = s.lastHash  // link to previous entry

    mac := hmac.New(sha256.New, s.auditHMACKey)
    data := fmt.Sprintf("%s|%s|%s|%s|%s",
        p.Entry.PreviousHash, p.Entry.Timestamp,
        p.Entry.Username, p.Entry.Action, p.Entry.KeyID)
    mac.Write([]byte(data))

    p.Entry.CurrentHash = fmt.Sprintf("%x", mac.Sum(nil))
    s.lastHash = p.Entry.CurrentHash  // advance chain head
    s.auditTrail = append(s.auditTrail, p.Entry)
}`}</Code>
      </Section>

      {/* 4. RSA-OAEP */}
      <Section title="4. RSA-OAEP Key Export (KEM Pattern)" icon={Shield} color="bg-emerald-50 text-emerald-600">
        <p className="text-sm text-gray-600 leading-relaxed mb-4">
          RSA-OAEP (Optimal Asymmetric Encryption Padding) is used to export key material to a specific recipient.
          The user provides their RSA-2048 public key; the server wraps the key material with it.
          Only the holder of the corresponding private key can unwrap it.
          This is the Key Encapsulation Mechanism (KEM) pattern — the key never travels in plaintext.
        </p>

        <div className="flex flex-col items-center gap-0 mb-5">
          <div className="flex items-center gap-4">
            <Box label="Key Material (256-bit)" sub="From Raft store" color="bg-gray-100 border-gray-300 text-gray-700" />
            <ArrowRight size={16} className="text-gray-400" />
            <Box label="RSA-OAEP-SHA256" sub="Encrypt with recipient's pubkey" color="bg-emerald-50 border-emerald-200 text-emerald-800" />
            <ArrowRight size={16} className="text-gray-400" />
            <Box label="Wrapped Key (base64)" sub="Safe to transmit" color="bg-emerald-100 border-emerald-300 text-emerald-900" />
          </div>
          <Arrow label="Only recipient's private key can unwrap" />
          <Box label="Plaintext Key Material" sub="Recovered by recipient" color="bg-gray-100 border-gray-300 text-gray-700" />
        </div>

        <div className="grid grid-cols-2 gap-3 text-[11px] mb-3">
          <div className="bg-blue-50 border border-blue-100 rounded-lg p-3">
            <div className="font-bold text-blue-700 mb-1">Why OAEP over PKCS#1 v1.5?</div>
            <p className="text-gray-600">
              PKCS#1 v1.5 is vulnerable to Bleichenbacher's padding oracle attack.
              OAEP uses a hash function (SHA-256) and random padding, making it semantically secure (IND-CCA2).
            </p>
          </div>
          <div className="bg-violet-50 border border-violet-100 rounded-lg p-3">
            <div className="font-bold text-violet-700 mb-1">Demo it yourself</div>
            <p className="text-gray-600 font-mono text-[10px] leading-relaxed">
              openssl genrsa -out priv.pem 2048<br />
              openssl rsa -in priv.pem -pubout -out pub.pem<br />
              # Paste pub.pem into Key Registry → Export
            </p>
          </div>
        </div>

        <Code>{`// internal/api/server.go — handleExportKey
wrappedBytes, err := rsa.EncryptOAEP(
    sha256.New(),   // hash function for OAEP padding
    rand.Reader,    // randomness source
    rsaPub,         // recipient's RSA-2048 public key
    keyBytes,       // 256-bit key material to wrap
    nil,            // optional label (unused)
)
// Returns base64(wrappedBytes) — safe to transmit over any channel`}</Code>
      </Section>

      {/* 5. AES-256-GCM */}
      <Section title="5. AES-256-GCM — Authenticated Encryption" icon={Lock} color="bg-rose-50 text-rose-600" defaultOpen={false}>
        <p className="text-sm text-gray-600 leading-relaxed mb-4">
          AES-256-GCM (Galois/Counter Mode) provides both confidentiality and integrity in a single pass.
          Unlike AES-CBC, GCM produces an authentication tag that detects any tampering with the ciphertext.
          This is used for both the DEK wrapping and the data encryption layers.
        </p>

        <div className="grid grid-cols-3 gap-3 text-[11px] mb-4">
          {[
            { label: 'Key size', value: '256 bits', note: 'Maximum AES key size' },
            { label: 'Nonce size', value: '96 bits (12B)', note: 'Random per operation' },
            { label: 'Auth tag', value: '128 bits (16B)', note: 'Detects tampering' },
          ].map(({ label, value, note }) => (
            <div key={label} className="bg-gray-50 border border-gray-100 rounded-lg p-3 text-center">
              <div className="text-[10px] text-gray-400 uppercase tracking-widest mb-1">{label}</div>
              <div className="font-bold text-gray-800 text-sm">{value}</div>
              <div className="text-[10px] text-gray-400 mt-0.5">{note}</div>
            </div>
          ))}
        </div>

        <div className="bg-gray-50 rounded-lg p-4 text-[11px] text-gray-600 border border-gray-100 mb-3">
          <span className="font-bold text-gray-800">Output format:</span>{' '}
          <code className="font-mono bg-white px-1.5 py-0.5 rounded border border-gray-200">
            nonce(12B) || ciphertext || auth_tag(16B)
          </code>
          <br /><br />
          The nonce is prepended to the ciphertext. On decryption, GCM verifies the auth tag before returning plaintext —
          if the ciphertext was modified even by 1 bit, decryption fails with an error.
        </div>

        <Code>{`// Go's cipher.NewGCM automatically handles nonce and auth tag
block, _ := aes.NewCipher(key)   // AES-256 block cipher
gcm, _   := cipher.NewGCM(block) // GCM mode wrapper

// Encrypt: nonce prepended, auth tag appended automatically
nonce := make([]byte, gcm.NonceSize()) // 12 bytes
io.ReadFull(rand.Reader, nonce)
ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
//                      ↑ dst  ↑ nonce  ↑ data  ↑ additional data

// Decrypt: verifies auth tag, returns error if tampered
plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)`}</Code>
      </Section>

      {/* Summary table */}
      <div className="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
        <div className="px-6 py-4 border-b border-gray-100">
          <span className="text-sm font-bold text-gray-900">Crypto Stack Summary</span>
        </div>
        <table className="w-full text-sm">
          <thead className="bg-gray-50 text-[10px] font-bold text-gray-500 uppercase tracking-widest">
            <tr>
              <th className="px-6 py-3 text-left">Primitive</th>
              <th className="px-6 py-3 text-left">Used For</th>
              <th className="px-6 py-3 text-left">Security Property</th>
              <th className="px-6 py-3 text-left">Standard</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100 text-xs">
            {[
              ['AES-256-GCM', 'Data encryption (DEK layer)', 'IND-CCA2, authenticated', 'NIST FIPS 197 + SP 800-38D'],
              ['AES-256-GCM', 'DEK wrapping (KEK layer)', 'IND-CCA2, authenticated', 'NIST FIPS 197 + SP 800-38D'],
              ['HKDF-SHA256', 'KEK derivation from master secret', 'Pseudorandom, context-bound', 'RFC 5869'],
              ['HMAC-SHA256', 'Audit chain integrity', 'Tamper-evident, unforgeable', 'RFC 2104 + FIPS 198'],
              ['RSA-OAEP-SHA256', 'Key export / wrapping', 'IND-CCA2, semantic security', 'PKCS#1 v2.2 / RFC 8017'],
              ['crypto/rand', 'Nonce + DEK generation', 'Cryptographically secure RNG', 'OS entropy (urandom)'],
            ].map(([prim, use, prop, std]) => (
              <tr key={prim + use} className="hover:bg-gray-50">
                <td className="px-6 py-3 font-mono font-semibold text-indigo-700">{prim}</td>
                <td className="px-6 py-3 text-gray-700">{use}</td>
                <td className="px-6 py-3 text-gray-500">{prop}</td>
                <td className="px-6 py-3 text-gray-400">{std}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
