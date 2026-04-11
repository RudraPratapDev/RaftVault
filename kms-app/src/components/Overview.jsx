import { useState, useEffect } from 'react';
import { useAuth } from '../context/AuthContext';
import { apiFetch } from '../utils/api';
import { useNavigate } from 'react-router-dom';
import { KeyRound, ShieldCheck, ScrollText, Users, ArrowRight, RefreshCw } from 'lucide-react';

export default function Overview() {
  const { token } = useAuth();
  const navigate = useNavigate();
  const [keys, setKeys] = useState([]);
  const [auditLog, setAuditLog] = useState([]);
  const [users, setUsers] = useState([]);
  const [loading, setLoading] = useState(true);

  const fetchData = async () => {
    setLoading(true);
    try {
      const headers = { Authorization: `Bearer ${token}` };
      const [rKeys, rAudit, rUsers] = await Promise.all([
        apiFetch('/kms/listKeys', { headers }),
        apiFetch('/kms/auditLog', { headers }),
        apiFetch('/kms/listUsers', { headers }),
      ]);
      if (rKeys.ok) setKeys((await rKeys.json()).keys || []);
      if (rAudit.ok) setAuditLog((await rAudit.json()).audit_trail || []);
      if (rUsers.ok) setUsers((await rUsers.json()).users || []);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchData(); }, []);

  const activeKeys = keys.filter(k => k.status === 'active').length;
  const deletedKeys = keys.filter(k => k.status === 'deleted').length;
  const recentAudit = [...auditLog].reverse().slice(0, 5);

  const statCards = [
    { label: 'Active Keys', value: activeKeys, icon: KeyRound, color: 'indigo', action: () => navigate('/keys') },
    { label: 'Crypto Ops', value: auditLog.length, icon: ShieldCheck, color: 'emerald', action: () => navigate('/crypto') },
    { label: 'Audit Entries', value: auditLog.length, icon: ScrollText, color: 'amber', action: () => navigate('/audit') },
    { label: 'Users', value: users.length, icon: Users, color: 'violet', action: () => navigate('/users') },
  ];

  const colorMap = {
    indigo: 'bg-indigo-50 text-indigo-600',
    emerald: 'bg-emerald-50 text-emerald-600',
    amber: 'bg-amber-50 text-amber-600',
    violet: 'bg-violet-50 text-violet-600',
  };

  return (
    <div className="p-8 max-w-5xl">
      <div className="flex items-center justify-between mb-2">
        <h1 className="text-3xl font-bold text-gray-900 tracking-tight">Systems Overview</h1>
        <button
          onClick={fetchData}
          className="flex items-center gap-2 text-xs font-semibold text-gray-500 hover:text-indigo-600 transition-colors"
        >
          <RefreshCw size={13} className={loading ? 'animate-spin' : ''} />
          Refresh
        </button>
      </div>
      <p className="text-sm text-gray-500 mb-8 leading-relaxed">
        Vault integrity is optimal. All cryptographic pathways are active and synchronized.
      </p>

      {/* Stat Cards */}
      <div className="grid grid-cols-4 gap-4 mb-8">
        {statCards.map(({ label, value, icon: Icon, color, action }) => (
          <button
            key={label}
            onClick={action}
            className="bg-white rounded-xl border border-gray-100 p-5 shadow-sm text-left hover:shadow-md hover:border-indigo-100 transition-all group"
          >
            <div className={`w-9 h-9 rounded-lg flex items-center justify-center mb-3 ${colorMap[color]}`}>
              <Icon size={16} />
            </div>
            <div className="text-2xl font-bold text-gray-900 mb-1">
              {loading ? '—' : value}
            </div>
            <div className="text-[10px] font-bold text-gray-400 tracking-widest uppercase flex items-center gap-1">
              {label}
              <ArrowRight size={10} className="opacity-0 group-hover:opacity-100 transition-opacity" />
            </div>
          </button>
        ))}
      </div>

      <div className="grid grid-cols-3 gap-6">
        {/* Keys Summary */}
        <div className="col-span-2 bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
          <div className="flex items-center justify-between px-6 py-4 border-b border-gray-100">
            <h3 className="text-sm font-bold text-gray-900">Key Registry</h3>
            <button onClick={() => navigate('/keys')} className="text-xs font-semibold text-indigo-600 hover:text-indigo-800 flex items-center gap-1">
              Manage <ArrowRight size={12} />
            </button>
          </div>
          {loading ? (
            <div className="p-8 text-center text-sm text-gray-400">Loading...</div>
          ) : keys.length === 0 ? (
            <div className="p-8 text-center">
              <KeyRound size={24} className="mx-auto text-gray-300 mb-2" />
              <p className="text-sm text-gray-400">No keys created yet.</p>
              <button onClick={() => navigate('/keys')} className="mt-3 text-xs font-semibold text-indigo-600 hover:underline">
                Create your first key →
              </button>
            </div>
          ) : (
            <table className="w-full text-left text-sm">
              <thead className="bg-gray-50 text-[10px] font-bold text-gray-500 uppercase tracking-widest">
                <tr>
                  <th className="px-6 py-3">Key ID</th>
                  <th className="px-6 py-3">Versions</th>
                  <th className="px-6 py-3">Status</th>
                  <th className="px-6 py-3">Created</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {keys.slice(0, 5).map(key => (
                  <tr key={key.key_id} className="hover:bg-gray-50 transition-colors">
                    <td className="px-6 py-3 font-mono text-xs text-gray-900">{key.key_id}</td>
                    <td className="px-6 py-3 text-gray-600">{key.versions?.length ?? 1}</td>
                    <td className="px-6 py-3">
                      <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-wider ${
                        key.status === 'active' ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-600'
                      }`}>
                        {key.status}
                      </span>
                    </td>
                    <td className="px-6 py-3 text-xs text-gray-400">
                      {new Date(key.created_at).toLocaleDateString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        {/* Recent Audit */}
        <div className="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
          <div className="flex items-center justify-between px-5 py-4 border-b border-gray-100">
            <h3 className="text-sm font-bold text-gray-900">Recent Activity</h3>
            <button onClick={() => navigate('/audit')} className="text-xs font-semibold text-indigo-600 hover:text-indigo-800 flex items-center gap-1">
              All <ArrowRight size={12} />
            </button>
          </div>
          {loading ? (
            <div className="p-6 text-center text-sm text-gray-400">Loading...</div>
          ) : recentAudit.length === 0 ? (
            <div className="p-6 text-center">
              <ScrollText size={20} className="mx-auto text-gray-300 mb-2" />
              <p className="text-xs text-gray-400">No activity yet.</p>
            </div>
          ) : (
            <div className="divide-y divide-gray-100">
              {recentAudit.map((entry, i) => (
                <div key={i} className="px-5 py-3">
                  <div className="flex items-center gap-2 mb-0.5">
                    <span className={`text-[9px] font-bold px-1.5 py-0.5 rounded uppercase tracking-wider ${
                      entry.action === 'ENCRYPT' ? 'bg-indigo-50 text-indigo-600' : 'bg-amber-50 text-amber-600'
                    }`}>
                      {entry.action}
                    </span>
                    <span className="text-[10px] text-gray-400 font-mono">{entry.key_id}</span>
                  </div>
                  <div className="text-[10px] text-gray-500">{entry.username} · {new Date(entry.timestamp).toLocaleTimeString()}</div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Crypto Health */}
      <div className="mt-6 bg-indigo-600 rounded-xl p-6 text-white relative overflow-hidden shadow-lg shadow-indigo-600/20">
        <div className="absolute -right-6 -top-6 w-40 h-40 bg-indigo-500/40 rounded-full blur-2xl pointer-events-none"></div>
        <div className="relative z-10">
          <div className="flex items-center justify-between mb-4">
            <div>
              <div className="text-[10px] font-bold tracking-widest uppercase text-indigo-200 mb-1">Security Layer</div>
              <h3 className="text-lg font-bold mb-1">Cryptographic Stack</h3>
            </div>
            <div className="text-right">
              <div className="text-4xl font-bold tracking-tighter">
                {keys.length === 0 ? '—' : `${Math.round((activeKeys / keys.length) * 100)}%`}
              </div>
              <div className="text-[10px] font-bold text-indigo-200 uppercase tracking-widest">Key Health</div>
            </div>
          </div>
          <div className="grid grid-cols-4 gap-3">
            {[
              { label: 'Envelope Enc.', sub: 'KEK/DEK separation' },
              { label: 'HKDF-SHA256', sub: 'Key derivation' },
              { label: 'HMAC Chain', sub: 'Tamper-evident audit' },
              { label: 'RSA-OAEP', sub: 'Key export / KEM' },
            ].map(({ label, sub }) => (
              <div key={label} className="bg-indigo-500/40 rounded-lg px-3 py-2.5 text-center">
                <div className="text-xs font-bold text-white">{label}</div>
                <div className="text-[10px] text-indigo-200 mt-0.5">{sub}</div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
