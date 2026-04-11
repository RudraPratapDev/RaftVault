import { useState, useEffect } from 'react';
import { useAuth } from '../context/AuthContext';
import { apiFetch } from '../utils/api';
import { RefreshCw, ScrollText, Lock, Unlock, Search, ShieldCheck, ShieldAlert, Link, ChevronDown, ChevronUp, Key, Upload } from 'lucide-react';

function HashChip({ hash, short = true }) {
  if (!hash) return <span className="text-gray-300 font-mono text-[10px]">—</span>;
  return (
    <span className="font-mono text-[10px] text-gray-500 bg-gray-100 px-1.5 py-0.5 rounded" title={hash}>
      {short ? hash.slice(0, 12) + '…' : hash}
    </span>
  );
}

export default function AuditLog() {
  const { token } = useAuth();
  const [log, setLog] = useState([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState('');
  const [actionFilter, setActionFilter] = useState('ALL');
  const [expandedHashes, setExpandedHashes] = useState({});
  const [chainStatus, setChainStatus] = useState(null);
  const [verifying, setVerifying] = useState(false);

  const fetchLog = async () => {
    setLoading(true);
    try {
      const res = await apiFetch('/kms/auditLog', {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setLog(data.audit_trail || []);
      }
    } catch (e) { console.error(e); }
    finally { setLoading(false); }
  };

  const verifyChain = async () => {
    setVerifying(true);
    try {
      const res = await apiFetch('/kms/verifyChain', {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setChainStatus(data);
      }
    } catch (e) { console.error(e); }
    finally { setVerifying(false); }
  };

  useEffect(() => { fetchLog(); }, []);

  const filtered = [...log].reverse().filter(entry => {
    const matchAction = actionFilter === 'ALL' || entry.action === actionFilter;
    const matchSearch = !filter ||
      entry.key_id?.toLowerCase().includes(filter.toLowerCase()) ||
      entry.username?.toLowerCase().includes(filter.toLowerCase());
    return matchAction && matchSearch;
  });

  const encryptCount = log.filter(e => e.action === 'ENCRYPT').length;
  const decryptCount = log.filter(e => e.action === 'DECRYPT').length;
  const exportCount = log.filter(e => e.action === 'EXPORT').length;

  const actionMeta = {
    ENCRYPT: { color: 'bg-indigo-50 text-indigo-600', icon: <Lock size={9} /> },
    DECRYPT: { color: 'bg-amber-50 text-amber-600', icon: <Unlock size={9} /> },
    EXPORT:  { color: 'bg-violet-50 text-violet-600', icon: <Upload size={9} /> },
  };

  return (
    <div className="p-8 max-w-5xl">
      <div className="flex items-center justify-between mb-2">
        <h1 className="text-3xl font-bold text-gray-900 tracking-tight">Audit Log</h1>
        <div className="flex items-center gap-3">
          <button
            onClick={verifyChain}
            disabled={verifying || log.length === 0}
            className="flex items-center gap-2 text-xs font-semibold bg-indigo-600 text-white px-4 py-2 rounded-lg hover:bg-indigo-700 disabled:opacity-50 transition-colors shadow-sm shadow-indigo-600/20"
          >
            <ShieldCheck size={13} className={verifying ? 'animate-spin' : ''} />
            {verifying ? 'Verifying…' : 'Verify Chain'}
          </button>
          <button onClick={fetchLog} className="flex items-center gap-2 text-xs font-semibold text-gray-500 hover:text-indigo-600 transition-colors">
            <RefreshCw size={13} className={loading ? 'animate-spin' : ''} />
            Refresh
          </button>
        </div>
      </div>
      <p className="text-sm text-gray-500 mb-6">
        HMAC-SHA256 chained audit trail. Each entry's hash covers the previous entry — tampering breaks the chain.
      </p>

      {/* Chain Verification Result */}
      {chainStatus && (
        <div className={`mb-6 p-4 rounded-xl border flex items-start gap-3 ${
          chainStatus.valid
            ? 'bg-emerald-50 border-emerald-200'
            : 'bg-red-50 border-red-200'
        }`}>
          {chainStatus.valid
            ? <ShieldCheck size={18} className="text-emerald-600 mt-0.5 shrink-0" />
            : <ShieldAlert size={18} className="text-red-500 mt-0.5 shrink-0" />}
          <div>
            <div className={`text-sm font-bold mb-0.5 ${chainStatus.valid ? 'text-emerald-800' : 'text-red-700'}`}>
              {chainStatus.valid ? 'Chain Integrity Verified' : 'Chain Integrity Compromised'}
            </div>
            <div className="text-xs text-gray-600">{chainStatus.message}</div>
            {!chainStatus.valid && chainStatus.broken_at >= 0 && (
              <div className="text-xs text-red-600 mt-1 font-mono">
                Tampered entry index: {chainStatus.broken_at}
              </div>
            )}
          </div>
          <button onClick={() => setChainStatus(null)} className="ml-auto text-gray-400 hover:text-gray-600 text-xs">✕</button>
        </div>
      )}

      {/* Stats */}
      <div className="grid grid-cols-4 gap-4 mb-6">
        {[
          { label: 'Total Operations', value: log.length, color: 'text-gray-900' },
          { label: 'Encryptions', value: encryptCount, color: 'text-indigo-600' },
          { label: 'Decryptions', value: decryptCount, color: 'text-amber-600' },
          { label: 'Key Exports', value: exportCount, color: 'text-violet-600' },
        ].map(({ label, value, color }) => (
          <div key={label} className="bg-white rounded-xl border border-gray-100 shadow-sm p-5">
            <div className={`text-2xl font-bold mb-1 ${color}`}>{loading ? '—' : value}</div>
            <div className="text-[10px] font-bold text-gray-400 tracking-widest uppercase">{label}</div>
          </div>
        ))}
      </div>

      {/* HMAC Chain Explainer */}
      <div className="mb-6 bg-white rounded-xl border border-gray-100 shadow-sm p-5">
        <div className="flex items-center gap-2 mb-3">
          <Link size={14} className="text-indigo-500" />
          <span className="text-xs font-bold text-gray-700 uppercase tracking-widest">How the Hash Chain Works</span>
        </div>
        <div className="flex items-center gap-2 overflow-x-auto pb-1">
          {['Genesis\n0000…0000', 'Entry 1\nHMAC(prev|data)', 'Entry 2\nHMAC(prev|data)', 'Entry N\nHMAC(prev|data)'].map((label, i) => (
            <div key={i} className="flex items-center gap-2 shrink-0">
              <div className={`rounded-lg px-3 py-2 text-center text-[10px] font-mono leading-tight ${i === 0 ? 'bg-gray-100 text-gray-500' : 'bg-indigo-50 text-indigo-700 border border-indigo-100'}`}>
                {label.split('\n').map((l, j) => <div key={j}>{l}</div>)}
              </div>
              {i < 3 && <div className="text-gray-300 text-lg">→</div>}
            </div>
          ))}
        </div>
        <p className="text-[11px] text-gray-400 mt-3 leading-relaxed">
          Each entry's <span className="font-mono bg-gray-100 px-1 rounded">current_hash = HMAC-SHA256(key, prev_hash | timestamp | user | action | key_id)</span>.
          Modifying any past entry invalidates all subsequent hashes. Click "Verify Chain" to re-compute and validate every link.
        </p>
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
          {['ALL', 'ENCRYPT', 'DECRYPT', 'EXPORT'].map(a => (
            <button
              key={a}
              onClick={() => setActionFilter(a)}
              className={`px-3 py-1.5 rounded-md text-[10px] font-bold tracking-wider uppercase transition-all ${
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
              {log.length === 0 ? 'No operations recorded yet.' : 'No entries match your filter.'}
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="bg-gray-50 text-[10px] font-bold text-gray-500 uppercase tracking-widest">
                <tr>
                  <th className="px-4 py-3 w-6"></th>
                  <th className="px-4 py-3">Timestamp</th>
                  <th className="px-4 py-3">Action</th>
                  <th className="px-4 py-3">Key ID</th>
                  <th className="px-4 py-3">User</th>
                  <th className="px-4 py-3">Hash (current)</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {filtered.map((entry, i) => {
                  const meta = actionMeta[entry.action] || { color: 'bg-gray-100 text-gray-600', icon: <Key size={9} /> };
                  const isExpanded = expandedHashes[i];
                  return (
                    <>
                      <tr key={i} className="hover:bg-gray-50 transition-colors">
                        <td className="px-4 py-3">
                          <button onClick={() => setExpandedHashes(p => ({ ...p, [i]: !p[i] }))} className="text-gray-300 hover:text-indigo-500 transition-colors">
                            {isExpanded ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
                          </button>
                        </td>
                        <td className="px-4 py-3 text-xs text-gray-500 font-mono whitespace-nowrap">
                          {new Date(entry.timestamp).toLocaleString()}
                        </td>
                        <td className="px-4 py-3">
                          <span className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-wider ${meta.color}`}>
                            {meta.icon}{entry.action}
                          </span>
                        </td>
                        <td className="px-4 py-3 font-mono text-xs text-gray-800">{entry.key_id}</td>
                        <td className="px-4 py-3 text-xs text-gray-600">{entry.username}</td>
                        <td className="px-4 py-3"><HashChip hash={entry.current_hash} /></td>
                      </tr>
                      {isExpanded && (
                        <tr key={`${i}-exp`} className="bg-indigo-50/40">
                          <td colSpan={6} className="px-6 py-4">
                            <div className="space-y-2 text-[11px]">
                              <div className="flex items-center gap-3">
                                <span className="text-gray-400 w-28 shrink-0">Previous Hash</span>
                                <HashChip hash={entry.previous_hash} short={false} />
                              </div>
                              <div className="flex items-center gap-3">
                                <span className="text-gray-400 w-28 shrink-0">Current Hash</span>
                                <HashChip hash={entry.current_hash} short={false} />
                              </div>
                              <div className="flex items-center gap-2 mt-2 p-2 bg-white rounded-lg border border-indigo-100 font-mono text-[10px] text-gray-500 break-all">
                                HMAC-SHA256( key, "{entry.previous_hash?.slice(0,8)}…|{entry.timestamp}|{entry.username}|{entry.action}|{entry.key_id}" )
                              </div>
                            </div>
                          </td>
                        </tr>
                      )}
                    </>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <p className="text-[11px] text-gray-400 mt-4 leading-relaxed">
        This log is committed through distributed Raft consensus. Entries cannot be modified or deleted without breaking the cryptographic chain.
      </p>
    </div>
  );
}
