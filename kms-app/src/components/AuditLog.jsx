import { useState, useEffect } from 'react';
import { useAuth } from '../context/AuthContext';
import { apiFetch } from '../utils/api';
import { RefreshCw, ScrollText, Lock, Unlock, Search } from 'lucide-react';

export default function AuditLog() {
  const { token } = useAuth();
  const [log, setLog] = useState([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState('');
  const [actionFilter, setActionFilter] = useState('ALL');

  const fetchLog = async () => {
    setLoading(true);
    try {
      const res = await apiFetch('/kms/auditLog', {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setLog([...(data.audit_trail || [])].reverse());
      }
    } catch (e) { console.error(e); }
    finally { setLoading(false); }
  };

  useEffect(() => { fetchLog(); }, []);

  const filtered = log.filter(entry => {
    const matchAction = actionFilter === 'ALL' || entry.action === actionFilter;
    const matchSearch = !filter ||
      entry.key_id?.toLowerCase().includes(filter.toLowerCase()) ||
      entry.username?.toLowerCase().includes(filter.toLowerCase());
    return matchAction && matchSearch;
  });

  const encryptCount = log.filter(e => e.action === 'ENCRYPT').length;
  const decryptCount = log.filter(e => e.action === 'DECRYPT').length;

  return (
    <div className="p-8 max-w-4xl">
      <div className="flex items-center justify-between mb-2">
        <h1 className="text-3xl font-bold text-gray-900 tracking-tight">Audit Log</h1>
        <button onClick={fetchLog} className="flex items-center gap-2 text-xs font-semibold text-gray-500 hover:text-indigo-600 transition-colors">
          <RefreshCw size={13} className={loading ? 'animate-spin' : ''} />
          Refresh
        </button>
      </div>
      <p className="text-sm text-gray-500 mb-8">
        Immutable cryptographic audit trail. Every encrypt and decrypt operation is recorded here via consensus.
      </p>

      {/* Stats */}
      <div className="grid grid-cols-3 gap-4 mb-6">
        {[
          { label: 'Total Operations', value: log.length, color: 'text-gray-900' },
          { label: 'Encryptions', value: encryptCount, color: 'text-indigo-600' },
          { label: 'Decryptions', value: decryptCount, color: 'text-amber-600' },
        ].map(({ label, value, color }) => (
          <div key={label} className="bg-white rounded-xl border border-gray-100 shadow-sm p-5">
            <div className={`text-2xl font-bold mb-1 ${color}`}>{loading ? '—' : value}</div>
            <div className="text-[10px] font-bold text-gray-400 tracking-widest uppercase">{label}</div>
          </div>
        ))}
      </div>

      {/* Filters */}
      <div className="flex gap-3 mb-4">
        <div className="relative flex-1">
          <Search size={13} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
          <input
            type="text"
            value={filter}
            onChange={e => setFilter(e.target.value)}
            placeholder="Filter by key ID or username..."
            className="w-full border border-gray-200 rounded-lg pl-9 pr-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-200 focus:border-indigo-400 transition"
          />
        </div>
        <div className="flex gap-1 bg-gray-100 rounded-lg p-1">
          {['ALL', 'ENCRYPT', 'DECRYPT'].map(a => (
            <button
              key={a}
              onClick={() => setActionFilter(a)}
              className={`px-4 py-1.5 rounded-md text-[10px] font-bold tracking-wider uppercase transition-all ${
                actionFilter === a ? 'bg-white text-indigo-600 shadow-sm' : 'text-gray-500 hover:text-gray-700'
              }`}
            >
              {a}
            </button>
          ))}
        </div>
      </div>

      {/* Log Table */}
      <div className="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
        <div className="px-6 py-4 border-b border-gray-100 flex items-center justify-between">
          <h3 className="text-sm font-bold text-gray-900">Operations</h3>
          <span className="text-[10px] font-bold text-gray-400 tracking-widest uppercase">{filtered.length} entries</span>
        </div>

        {loading ? (
          <div className="p-10 text-center text-sm text-gray-400">Loading audit log...</div>
        ) : filtered.length === 0 ? (
          <div className="p-10 text-center">
            <ScrollText size={28} className="mx-auto text-gray-300 mb-3" />
            <p className="text-sm text-gray-400">
              {log.length === 0 ? 'No operations recorded yet. Encrypt or decrypt data to see entries here.' : 'No entries match your filter.'}
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="bg-gray-50 text-[10px] font-bold text-gray-500 uppercase tracking-widest">
                <tr>
                  <th className="px-6 py-3">Timestamp</th>
                  <th className="px-6 py-3">Action</th>
                  <th className="px-6 py-3">Key ID</th>
                  <th className="px-6 py-3">User</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {filtered.map((entry, i) => (
                  <tr key={i} className="hover:bg-gray-50 transition-colors">
                    <td className="px-6 py-3 text-xs text-gray-500 font-mono whitespace-nowrap">
                      {new Date(entry.timestamp).toLocaleString()}
                    </td>
                    <td className="px-6 py-3">
                      <span className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-wider ${
                        entry.action === 'ENCRYPT'
                          ? 'bg-indigo-50 text-indigo-600'
                          : 'bg-amber-50 text-amber-600'
                      }`}>
                        {entry.action === 'ENCRYPT' ? <Lock size={9} /> : <Unlock size={9} />}
                        {entry.action}
                      </span>
                    </td>
                    <td className="px-6 py-3 font-mono text-xs text-gray-800">{entry.key_id}</td>
                    <td className="px-6 py-3 text-xs text-gray-600">{entry.username}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <p className="text-[11px] text-gray-400 mt-4 leading-relaxed">
        This log is immutable and committed through distributed consensus. Entries cannot be modified or deleted.
      </p>
    </div>
  );
}
