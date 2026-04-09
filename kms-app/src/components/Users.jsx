import { useState, useEffect } from 'react';
import { useAuth } from '../context/AuthContext';
import { apiFetch } from '../utils/api';
import { Plus, Trash2, RefreshCw, Users as UsersIcon, AlertTriangle, Eye, EyeOff, ChevronDown } from 'lucide-react';

export default function Users() {
  const { token, user: currentUser } = useAuth();
  const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };

  const [users, setUsers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [confirmDelete, setConfirmDelete] = useState(null);
  const [actionLoading, setActionLoading] = useState({});
  const [showApiKey, setShowApiKey] = useState({});
  const [newUser, setNewUser] = useState({ username: '', role: 'service' });
  const [creating, setCreating] = useState(false);
  const [createdApiKey, setCreatedApiKey] = useState(null);

  const fetchUsers = async () => {
    setLoading(true);
    try {
      const res = await apiFetch('/kms/listUsers', { headers: { Authorization: `Bearer ${token}` } });
      if (res.ok) setUsers((await res.json()).users || []);
    } catch (e) { console.error(e); }
    finally { setLoading(false); }
  };

  useEffect(() => { fetchUsers(); }, []);

  const notify = (msg, isError = false) => {
    if (isError) { setError(msg); setSuccess(''); }
    else { setSuccess(msg); setError(''); }
    setTimeout(() => { setError(''); setSuccess(''); }, 5000);
  };

  const handleCreate = async (e) => {
    e.preventDefault();
    if (!newUser.username.trim()) return;
    setCreating(true);
    setCreatedApiKey(null);
    try {
      const res = await apiFetch('/kms/createUser', {
        method: 'POST',
        headers,
        body: JSON.stringify({ username: newUser.username.trim(), role: newUser.role }),
      });
      const data = await res.json();
      if (!res.ok) { notify(data.error || 'Failed to create user', true); return; }
      setCreatedApiKey(data.api_key);
      setNewUser({ username: '', role: 'service' });
      fetchUsers();
    } catch (e) { notify('Network error', true); }
    finally { setCreating(false); }
  };

  const handleDelete = async (username) => {
    setActionLoading(p => ({ ...p, [username]: true }));
    try {
      const res = await apiFetch('/kms/deleteUser', {
        method: 'POST',
        headers,
        body: JSON.stringify({ username }),
      });
      const data = await res.json();
      if (!res.ok) { notify(data.error || 'Failed to delete user', true); return; }
      notify(`User "${username}" removed.`);
      setConfirmDelete(null);
      fetchUsers();
    } catch (e) { notify('Network error', true); }
    finally { setActionLoading(p => ({ ...p, [username]: false })); }
  };

  const toggleApiKey = (username) => setShowApiKey(p => ({ ...p, [username]: !p[username] }));

  const copyKey = (key) => {
    navigator.clipboard.writeText(key);
    notify('API key copied to clipboard.');
  };

  return (
    <div className="p-8 max-w-4xl">
      <div className="flex items-center justify-between mb-2">
        <h1 className="text-3xl font-bold text-gray-900 tracking-tight">User Management</h1>
        <button onClick={fetchUsers} className="flex items-center gap-2 text-xs font-semibold text-gray-500 hover:text-indigo-600 transition-colors">
          <RefreshCw size={13} className={loading ? 'animate-spin' : ''} />
          Refresh
        </button>
      </div>
      <p className="text-sm text-gray-500 mb-8">
        Manage operator identities and access credentials. Each user receives a unique API key for authentication.
      </p>

      {error && <div className="mb-4 p-3 bg-red-50 border border-red-100 text-red-600 text-sm rounded-lg">{error}</div>}
      {success && <div className="mb-4 p-3 bg-emerald-50 border border-emerald-100 text-emerald-700 text-sm rounded-lg">{success}</div>}

      {/* New API Key Banner */}
      {createdApiKey && (
        <div className="mb-6 p-5 bg-indigo-50 border border-indigo-200 rounded-xl">
          <div className="text-[11px] font-bold text-indigo-600 tracking-widest uppercase mb-2">User Created — Save This API Key</div>
          <p className="text-xs text-indigo-700 mb-3 leading-relaxed">
            This is the only time the API key will be shown. Copy it now and share it securely with the user.
          </p>
          <div className="flex items-center gap-3">
            <code className="flex-1 bg-white border border-indigo-200 rounded-lg px-4 py-2.5 text-sm font-mono text-indigo-800 break-all">
              {createdApiKey}
            </code>
            <button
              onClick={() => copyKey(createdApiKey)}
              className="shrink-0 bg-indigo-600 text-white text-xs font-bold px-4 py-2.5 rounded-lg hover:bg-indigo-700 transition-colors"
            >
              Copy
            </button>
          </div>
          <button onClick={() => setCreatedApiKey(null)} className="mt-3 text-xs text-indigo-500 hover:text-indigo-700 font-semibold">
            Dismiss
          </button>
        </div>
      )}

      {/* Create User */}
      <form onSubmit={handleCreate} className="bg-white rounded-xl border border-gray-100 shadow-sm p-6 mb-6">
        <h3 className="text-sm font-bold text-gray-900 mb-4">Create New User</h3>
        <div className="flex gap-3">
          <input
            type="text"
            value={newUser.username}
            onChange={e => setNewUser(p => ({ ...p, username: e.target.value }))}
            placeholder="e.g. service-account-01"
            className="flex-1 border border-gray-200 rounded-lg px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-200 focus:border-indigo-400 transition"
          />
          <div className="relative">
            <select
              value={newUser.role}
              onChange={e => setNewUser(p => ({ ...p, role: e.target.value }))}
              className="appearance-none border border-gray-200 rounded-lg px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-200 focus:border-indigo-400 transition bg-white pr-8"
            >
              <option value="service">Service</option>
              <option value="admin">Admin</option>
            </select>
            <ChevronDown size={13} className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 pointer-events-none" />
          </div>
          <button
            type="submit"
            disabled={creating || !newUser.username.trim()}
            className="bg-indigo-600 hover:bg-indigo-700 text-white px-5 py-2.5 rounded-lg text-xs font-bold tracking-wider uppercase flex items-center gap-2 disabled:opacity-60 transition-colors shadow-sm shadow-indigo-600/20"
          >
            <Plus size={14} />
            {creating ? 'Creating...' : 'Create User'}
          </button>
        </div>
        <p className="text-[11px] text-gray-400 mt-2">
          A unique API key will be generated. Service users can encrypt/decrypt. Admin users have full access.
        </p>
      </form>

      {/* Users Table */}
      <div className="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
        <div className="px-6 py-4 border-b border-gray-100 flex items-center justify-between">
          <h3 className="text-sm font-bold text-gray-900">All Users</h3>
          <span className="text-[10px] font-bold text-gray-400 tracking-widest uppercase">{users.length} total</span>
        </div>

        {loading ? (
          <div className="p-10 text-center text-sm text-gray-400">Loading users...</div>
        ) : users.length === 0 ? (
          <div className="p-10 text-center">
            <UsersIcon size={28} className="mx-auto text-gray-300 mb-3" />
            <p className="text-sm text-gray-400">No users found.</p>
          </div>
        ) : (
          <table className="w-full text-left text-sm">
            <thead className="bg-gray-50 text-[10px] font-bold text-gray-500 uppercase tracking-widest">
              <tr>
                <th className="px-6 py-3">Username</th>
                <th className="px-6 py-3">Role</th>
                <th className="px-6 py-3">API Key</th>
                <th className="px-6 py-3">Created</th>
                <th className="px-6 py-3"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {users.map(u => (
                <tr key={u.username} className="hover:bg-gray-50 transition-colors">
                  <td className="px-6 py-4">
                    <div className="flex items-center gap-2">
                      <div className="w-7 h-7 rounded-full bg-slate-100 text-slate-600 flex items-center justify-center text-xs font-bold">
                        {u.username[0]?.toUpperCase()}
                      </div>
                      <span className="font-semibold text-gray-900 text-sm">{u.username}</span>
                      {u.username === currentUser?.username && (
                        <span className="text-[9px] font-bold bg-indigo-50 text-indigo-600 px-1.5 py-0.5 rounded uppercase tracking-wider">You</span>
                      )}
                    </div>
                  </td>
                  <td className="px-6 py-4">
                    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-wider ${
                      u.role === 'admin' ? 'bg-violet-50 text-violet-700' : 'bg-gray-100 text-gray-600'
                    }`}>
                      {u.role}
                    </span>
                  </td>
                  <td className="px-6 py-4">
                    <div className="flex items-center gap-2">
                      <code className="font-mono text-xs text-gray-600 bg-gray-50 px-2 py-1 rounded border border-gray-100">
                        {showApiKey[u.username] ? u.api_key : '••••••••••••••••'}
                      </code>
                      <button onClick={() => toggleApiKey(u.username)} className="text-gray-400 hover:text-gray-700 transition-colors">
                        {showApiKey[u.username] ? <EyeOff size={13} /> : <Eye size={13} />}
                      </button>
                      {showApiKey[u.username] && (
                        <button onClick={() => copyKey(u.api_key)} className="text-xs text-indigo-500 hover:text-indigo-700 font-semibold">
                          Copy
                        </button>
                      )}
                    </div>
                  </td>
                  <td className="px-6 py-4 text-xs text-gray-400">
                    {new Date(u.created_at).toLocaleDateString()}
                  </td>
                  <td className="px-6 py-4">
                    {u.username !== 'admin' && (
                      <button
                        onClick={() => setConfirmDelete(u.username)}
                        className="flex items-center gap-1.5 text-xs font-semibold text-red-400 hover:text-red-600 transition-colors"
                      >
                        <Trash2 size={12} />
                        Remove
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
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
              <h3 className="text-base font-bold text-gray-900">Remove User?</h3>
            </div>
            <p className="text-sm text-gray-500 mb-6 leading-relaxed">
              User <span className="font-semibold text-gray-800">{confirmDelete}</span> will be permanently removed. Their API key will be invalidated immediately.
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
                disabled={actionLoading[confirmDelete]}
                className="flex-1 bg-red-500 hover:bg-red-600 text-white font-semibold text-sm py-2.5 rounded-lg transition-colors disabled:opacity-60"
              >
                {actionLoading[confirmDelete] ? 'Removing...' : 'Remove User'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
