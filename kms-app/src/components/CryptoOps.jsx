import { useState, useEffect } from 'react';
import { useAuth } from '../context/AuthContext';
import { apiFetch } from '../utils/api';
import { Lock, Unlock, Copy, Check, ChevronDown } from 'lucide-react';

export default function CryptoOps() {
  const { token } = useAuth();
  const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };

  const [keys, setKeys] = useState([]);
  const [tab, setTab] = useState('encrypt');

  // Encrypt state
  const [encKeyId, setEncKeyId] = useState('');
  const [plaintext, setPlaintext] = useState('');
  const [ciphertext, setCiphertext] = useState('');
  const [encLoading, setEncLoading] = useState(false);
  const [encError, setEncError] = useState('');

  // Decrypt state
  const [decKeyId, setDecKeyId] = useState('');
  const [decCiphertext, setDecCiphertext] = useState('');
  const [decPlaintext, setDecPlaintext] = useState('');
  const [decLoading, setDecLoading] = useState(false);
  const [decError, setDecError] = useState('');

  const [copied, setCopied] = useState(false);

  useEffect(() => {
    apiFetch('/kms/listKeys', { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.ok ? r.json() : { keys: [] })
      .then(d => setKeys((d.keys || []).filter(k => k.status === 'active')));
  }, []);

  const handleEncrypt = async (e) => {
    e.preventDefault();
    setEncError('');
    setCiphertext('');
    if (!encKeyId) { setEncError('Select a key.'); return; }
    if (!plaintext.trim()) { setEncError('Enter plaintext to encrypt.'); return; }
    setEncLoading(true);
    try {
      const res = await apiFetch('/kms/encrypt', {
        method: 'POST',
        headers,
        body: JSON.stringify({ key_id: encKeyId, plaintext }),
      });
      const data = await res.json();
      if (!res.ok) { setEncError(data.error || 'Encryption failed'); return; }
      setCiphertext(data.ciphertext);
    } catch (e) { setEncError('Network error'); }
    finally { setEncLoading(false); }
  };

  const handleDecrypt = async (e) => {
    e.preventDefault();
    setDecError('');
    setDecPlaintext('');
    if (!decKeyId) { setDecError('Select a key.'); return; }
    if (!decCiphertext.trim()) { setDecError('Enter ciphertext to decrypt.'); return; }
    setDecLoading(true);
    try {
      const res = await apiFetch('/kms/decrypt', {
        method: 'POST',
        headers,
        body: JSON.stringify({ key_id: decKeyId, ciphertext: decCiphertext.trim() }),
      });
      const data = await res.json();
      if (!res.ok) { setDecError(data.error || 'Decryption failed'); return; }
      setDecPlaintext(data.plaintext);
    } catch (e) { setDecError('Network error'); }
    finally { setDecLoading(false); }
  };

  const copyToClipboard = (text) => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const KeySelect = ({ value, onChange, placeholder }) => (
    <div className="relative">
      <select
        value={value}
        onChange={e => onChange(e.target.value)}
        className="w-full appearance-none border border-gray-200 rounded-lg px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-200 focus:border-indigo-400 transition bg-white font-mono pr-8"
      >
        <option value="">{placeholder}</option>
        {keys.map(k => (
          <option key={k.key_id} value={k.key_id}>{k.key_id}</option>
        ))}
      </select>
      <ChevronDown size={14} className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 pointer-events-none" />
    </div>
  );

  return (
    <div className="p-8 max-w-3xl">
      <h1 className="text-3xl font-bold text-gray-900 tracking-tight mb-2">Encrypt / Decrypt</h1>
      <p className="text-sm text-gray-500 mb-8">
        Perform AES-256-GCM cryptographic operations. All operations are logged to the immutable audit trail.
      </p>

      {/* Tabs */}
      <div className="flex gap-1 bg-gray-100 rounded-lg p-1 mb-6 w-fit">
        <button
          onClick={() => setTab('encrypt')}
          className={`flex items-center gap-2 px-5 py-2 rounded-md text-xs font-bold tracking-wider uppercase transition-all ${
            tab === 'encrypt' ? 'bg-white text-indigo-600 shadow-sm' : 'text-gray-500 hover:text-gray-700'
          }`}
        >
          <Lock size={13} /> Encrypt
        </button>
        <button
          onClick={() => setTab('decrypt')}
          className={`flex items-center gap-2 px-5 py-2 rounded-md text-xs font-bold tracking-wider uppercase transition-all ${
            tab === 'decrypt' ? 'bg-white text-indigo-600 shadow-sm' : 'text-gray-500 hover:text-gray-700'
          }`}
        >
          <Unlock size={13} /> Decrypt
        </button>
      </div>

      {tab === 'encrypt' && (
        <form onSubmit={handleEncrypt} className="space-y-5">
          <div className="bg-white rounded-xl border border-gray-100 shadow-sm p-6 space-y-5">
            <div>
              <label className="text-[11px] font-bold text-gray-500 tracking-widest uppercase block mb-2">Select Key</label>
              <KeySelect value={encKeyId} onChange={setEncKeyId} placeholder="— Choose an active key —" />
            </div>
            <div>
              <label className="text-[11px] font-bold text-gray-500 tracking-widest uppercase block mb-2">Plaintext</label>
              <textarea
                value={plaintext}
                onChange={e => setPlaintext(e.target.value)}
                rows={5}
                placeholder="Enter the data you want to encrypt..."
                className="w-full border border-gray-200 rounded-lg px-4 py-3 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-200 focus:border-indigo-400 transition resize-none font-mono"
              />
            </div>
            {encError && <div className="p-3 bg-red-50 border border-red-100 text-red-600 text-sm rounded-lg">{encError}</div>}
            <button
              type="submit"
              disabled={encLoading}
              className="w-full bg-indigo-600 hover:bg-indigo-700 text-white font-bold text-xs tracking-widest uppercase py-3 rounded-lg transition-colors shadow-sm shadow-indigo-600/20 disabled:opacity-60 flex items-center justify-center gap-2"
            >
              <Lock size={13} />
              {encLoading ? 'Encrypting...' : 'Encrypt Data'}
            </button>
          </div>

          {ciphertext && (
            <div className="bg-white rounded-xl border border-emerald-100 shadow-sm p-6">
              <div className="flex items-center justify-between mb-3">
                <div className="text-[11px] font-bold text-emerald-600 tracking-widest uppercase">Ciphertext Output</div>
                <button
                  type="button"
                  onClick={() => copyToClipboard(ciphertext)}
                  className="flex items-center gap-1.5 text-xs font-semibold text-gray-500 hover:text-indigo-600 transition-colors"
                >
                  {copied ? <Check size={12} className="text-emerald-500" /> : <Copy size={12} />}
                  {copied ? 'Copied!' : 'Copy'}
                </button>
              </div>
              <div className="bg-gray-50 rounded-lg p-4 font-mono text-xs text-gray-700 break-all leading-relaxed border border-gray-100">
                {ciphertext}
              </div>
              <p className="text-[11px] text-gray-400 mt-2">
                This ciphertext includes the key version prefix for automatic version-aware decryption.
              </p>
            </div>
          )}
        </form>
      )}

      {tab === 'decrypt' && (
        <form onSubmit={handleDecrypt} className="space-y-5">
          <div className="bg-white rounded-xl border border-gray-100 shadow-sm p-6 space-y-5">
            <div>
              <label className="text-[11px] font-bold text-gray-500 tracking-widest uppercase block mb-2">Select Key</label>
              <KeySelect value={decKeyId} onChange={setDecKeyId} placeholder="— Choose the key used for encryption —" />
            </div>
            <div>
              <label className="text-[11px] font-bold text-gray-500 tracking-widest uppercase block mb-2">Ciphertext</label>
              <textarea
                value={decCiphertext}
                onChange={e => setDecCiphertext(e.target.value)}
                rows={5}
                placeholder="Paste the base64-encoded ciphertext here..."
                className="w-full border border-gray-200 rounded-lg px-4 py-3 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-200 focus:border-indigo-400 transition resize-none font-mono"
              />
            </div>
            {decError && <div className="p-3 bg-red-50 border border-red-100 text-red-600 text-sm rounded-lg">{decError}</div>}
            <button
              type="submit"
              disabled={decLoading}
              className="w-full bg-indigo-600 hover:bg-indigo-700 text-white font-bold text-xs tracking-widest uppercase py-3 rounded-lg transition-colors shadow-sm shadow-indigo-600/20 disabled:opacity-60 flex items-center justify-center gap-2"
            >
              <Unlock size={13} />
              {decLoading ? 'Decrypting...' : 'Decrypt Data'}
            </button>
          </div>

          {decPlaintext && (
            <div className="bg-white rounded-xl border border-emerald-100 shadow-sm p-6">
              <div className="flex items-center justify-between mb-3">
                <div className="text-[11px] font-bold text-emerald-600 tracking-widest uppercase">Decrypted Plaintext</div>
                <button
                  type="button"
                  onClick={() => copyToClipboard(decPlaintext)}
                  className="flex items-center gap-1.5 text-xs font-semibold text-gray-500 hover:text-indigo-600 transition-colors"
                >
                  {copied ? <Check size={12} className="text-emerald-500" /> : <Copy size={12} />}
                  {copied ? 'Copied!' : 'Copy'}
                </button>
              </div>
              <div className="bg-gray-50 rounded-lg p-4 font-mono text-xs text-gray-700 break-all leading-relaxed border border-gray-100 whitespace-pre-wrap">
                {decPlaintext}
              </div>
            </div>
          )}
        </form>
      )}

      {keys.length === 0 && (
        <div className="mt-4 p-4 bg-amber-50 border border-amber-100 rounded-lg text-sm text-amber-700">
          No active keys available. Create a key in the Key Registry first.
        </div>
      )}
    </div>
  );
}
