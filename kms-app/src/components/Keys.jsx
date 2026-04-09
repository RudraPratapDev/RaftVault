import { useState, useEffect } from 'react';
import { useAuth } from '../context/AuthContext';
import { apiFetch } from '../utils/api';
import { Plus, Trash2, RefreshCw, ChevronDown, ChevronUp, KeyRound, AlertTriangle } from 'lucide-react';

export default function Keys() {
  const { token, user } = useAuth();
  const isAdmin = user?.role === 'admin';
  const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };

  const [keys, setKeys] = useState([]);
  const [loading, setLoading] = useState(true);
  const [expanded, setExpanded] = useState({});
  const [newKeyId, setNewKeyId] = useState('');
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [actionLoading, setActionLoading] = useState({});
  const [confirmDelete, setConfirmDelete] = useState(null);

  const fetchKeys = async () => {
    setLoading(true);
    try {
      const res = await apiFetch('/kms/listKeys', { headers: { Authorization: `Bearer ${token}` } });
      if (res.ok) setKeys((await res.json()).keys || []);
    } catch (e) { console.error(e); }
    finally { setLoading(false); }
  };

  useEffect(() => { fetchKeys(); }, []);

  const notify = (msg, isError = false) => {
    if (isError) { setError(msg); setSuccess(''); }
    else { setSuccess(msg); setError(''); }
    setTimeout(() => { setError(''); setSuccess(''); }, 4000);
  };

  const handleCreate = async (e) => {
    e.preventDefault();
    if (!newKeyId.trim()) return;
    setCreating(true);
    try {
      const res = await apiFetch('/kms/createKey', {
        method: 'POST',
        headers,
        body: JSON.stringify({ key_id: newKeyId.trim() }),
      });
      const data = await res.json();
      if (!res.ok) { notify(data.error || 'Failed to create key', true); return; }
      notify(`Key "${newKeyId.trim()}" created successfully.`);
      setNewKeyId('');
      fetchKeys();
    } catch (e) { notify('Network error', true); }
    finally { setCreating(false); }
  };

  const handleRotate = async (keyId) => {
    setActionLoading(p => ({ ...p, [keyId + '_rotate']: true }));
    try {
      const res = await apiFetch('/kms/rotateKey', {
        method: 'POST',
        headers,
        body: JSON.stringify({ key_id: keyId }),
      });
      const data = await res.json();
      if (!res.ok) { notify(data.error || 'Failed to rotate key', true); return; }
      notify(`Key "${keyId}" rotated to version ${data.key?.versions?.length ?? '?'}.`);
      fetchKeys();
    } catch (e) { notify('Network error', true); }
    finally { setActionLoading(p => ({ ...p, [keyId + '_rotate']: false })); }
  };

  const handleDelete = async (keyId) => {
    setActionLoading(p => ({ ...p, [keyId + '_delete']: true }));
    try {
      const res = await apiFetch('/kms/deleteKey', {
        method: 'POST',
        headers,
        body: JSON.stringify({ key_id: keyId }),
      });
      const data = await res.json();
      if (!res.ok) { notify(data.error || 'Failed to delete key', true); return; }
      notify(`Key "${keyId}" archived.`);
      setConfirmDelete(null);
      fetchKeys();
    } catch (e) { notify('Network error', true); }
    finally { setActionLoading(p => ({ ...p, [keyId + '_delete']: false })); }
  };

  const toggleExpand = (keyId) => setExpanded(p => ({ ...p, [keyId]: !p[keyId] }));

  return (
    <div className="p-8 max-w-4xl">
      <div className="flex items-center justify-between mb-2">
        <h1 className="text-3xl font-bold text-gray-900 tracking-tight">Key Registry</h1>
        <button onClick={fetchKeys} className="flex items-center gap-2 text-xs font-semibold text-gray-500 hover:text-indigo-600 transition-colors">
          <RefreshCw size={13} className={loading ? 'animate-spin' : ''} />
          Refresh
        </button>
      </div>
      <p className="text-sm text-gray-500 mb-8">Manage cryptographic keys. All operations are committed through consensus and logged immutably.</p>

      {error && <div className="mb-4 p-3 bg-red-50 border border-red-100 text-red-600 text-sm rounded-lg">{error}</div>}
      {success && <div className="mb-4 p-3 bg-emerald-50 border border-emerald-100 text-emerald-700 text-sm rounded-lg">{success}</div>}

      {/* Create Key */}
      {isAdmin && (
        <form onSubmit={handleCreate} className="bg-white rounded-xl border border-gray-100 shadow-sm p-6 mb-6">
          <h3 className="text-sm font-bold text-gray-900 mb-4">Create New Key</h3>
          <div className="flex gap-3">
            <input
              type="text"
              value={newKeyId}
              onChange={e => setNewKeyId(e.target.value)}
              placeholder="e.g. prod-db-key-001"
              className="flex-1 border border-gray-200 rounded-lg px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-200 focus:border-indigo-400 transition font-mono"
            />
            <button
              type="submit"
              disabled={creating || !newKeyId.trim()}
              className="bg-indigo-600 hover:bg-indigo-700 text-white px-5 py-2.5 rounded-lg text-xs font-bold tracking-wider uppercase flex items-center gap-2 disabled:opacity-60 transition-colors shadow-sm shadow-indigo-600/20"
            >
              <Plus size={14} />
              {creating ? 'Creating...' : 'Create Key'}
            </button>
          </div>
          <p className="text-[11px] text-gray-400 mt-2">A 256-bit AES key will be generated and stored securely.</p>
        </form>
      )}

      {/* Keys Table */}
      <div className="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
        <div className="px-6 py-4 border-b border-gray-100 flex items-center justify-between">
          <h3 className="text-sm font-bold text-gray-900">All Keys</h3>
          <span className="text-[10px] font-bold text-gray-400 tracking-widest uppercase">{keys.length} total</span>
        </div>

        {loading ? (
          <div className="p-10 text-center text-sm text-gray-400">Loading keys...</div>
        ) : keys.length === 0 ? (
          <div className="p-10 text-center">
            <KeyRound size={28} className="mx-auto text-gray-300 mb-3" />
            <p className="text-sm text-gray-400">No keys found. Create your first key above.</p>
          </div>
        ) : (
          <div className="divide-y divide-gray-100">
            {keys.map(key => (
              <div key={key.key_id}>
                <div className="flex items-center justify-between px-6 py-4 hover:bg-gray-50 transition-colors">
                  <div className="flex items-center gap-4 min-w-0">
                    <button onClick={() => toggleExpand(key.key_id)} className="text-gray-400 hover:text-gray-700 shrink-0">
                      {expanded[key.key_id] ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
                    </button>
                    <div className="min-w-0">
                      <div className="font-mono text-sm text-gray-900 font-semibold truncate">{key.key_id}</div>
                      <div className="text-[11px] text-gray-400 mt-0.5">
                        Created {new Date(key.created_at).toLocaleString()} · {key.versions?.length ?? 1} version{(key.versions?.length ?? 1) !== 1 ? 's' : ''}
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-3 shrink-0 ml-4">
                    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-wider ${
                      key.status === 'active' ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-500'
                    }`}>
                      {key.status}
                    </span>
                    {isAdmin && key.status === 'active' && (
                      <>
                        <button
                          onClick={() => handleRotate(key.key_id)}
                          disabled={actionLoading[key.key_id + '_rotate']}
                          className="flex items-center gap-1.5 text-xs font-semibold text-indigo-600 hover:text-indigo-800 disabled:opacity-50 transition-colors"
                        >
                          <RefreshCw size={12} className={actionLoading[key.key_id + '_rotate'] ? 'animate-spin' : ''} />
                          Rotate
                        </button>
                        <button
                          onClick={() => setConfirmDelete(key.key_id)}
                          className="flex items-center gap-1.5 text-xs font-semibold text-red-500 hover:text-red-700 transition-colors"
                        >
                          <Trash2 size={12} />
                          Delete
                        </button>
                      </>
                    )}
                  </div>
                </div>

                {/* Expanded versions */}
                {expanded[key.key_id] && key.versions?.length > 0 && (
                  <div className="bg-gray-50 border-t border-gray-100 px-6 py-4">
                    <div className="text-[10px] font-bold text-gray-400 tracking-widest uppercase mb-3">Key Versions</div>
                    <div className="space-y-2">
                      {key.versions.map(v => (
                        <div key={v.version} className="flex items-center gap-4 text-xs">
                          <span className="w-16 font-bold text-gray-600">v{v.version}</span>
                          <span className="font-mono text-gray-400 truncate flex-1">{v.key_material?.slice(0, 32)}…</span>
                          <span className="text-gray-400 shrink-0">{new Date(v.created_at).toLocaleString()}</span>
                          {v.version === key.versions.length && (
                            <span className="text-[9px] font-bold bg-indigo-50 text-indigo-600 px-1.5 py-0.5 rounded uppercase tracking-wider">Current</span>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Delete Confirm Modal */}
      {confirmDelete && (
        <div className="fixed inset-0 bg-black/30 backdrop-blur-sm flex items-center justify-center z-50">
          <div className="bg-white rounded-2xl shadow-xl p-8 max-w-sm w-full mx-4">
            <div className="flex items-center gap-3 mb-4">
              <div className="w-10 h-10 rounded-full bg-red-50 flex items-center justify-center">
                <AlertTriangle size={18} className="text-red-500" />
              </div>
              <h3 className="text-base font-bold text-gray-900">Archive Key?</h3>
            </div>
            <p className="text-sm text-gray-500 mb-6 leading-relaxed">
              Key <span className="font-mono font-semibold text-gray-800">{confirmDelete}</span> will be marked as deleted. Existing ciphertext encrypted with this key cannot be decrypted afterwards.
            </p>
            <div className="flex gap-3">
              <button
                onClick={() => setConfirmDelete(null)}
                className="flex-1 border border-gray-200 text-gray-700 font-semibold text-sm py-2.5 rounded-lg hover:bg-gray-50 transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={() => handleDelete(confirmDelete)}
                disabled={actionLoading[confirmDelete + '_delete']}
                className="flex-1 bg-red-500 hover:bg-red-600 text-white font-semibold text-sm py-2.5 rounded-lg transition-colors disabled:opacity-60"
              >
                {actionLoading[confirmDelete + '_delete'] ? 'Archiving...' : 'Archive Key'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
