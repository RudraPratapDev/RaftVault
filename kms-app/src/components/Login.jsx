import { useState } from 'react';
import { useAuth } from '../context/AuthContext';
import { useNavigate } from 'react-router-dom';
import { Lock } from 'lucide-react';

export default function Login() {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const { login } = useAuth();
  const navigate = useNavigate();

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      await login(username, password);
      navigate('/');
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen bg-pattern flex flex-col items-center justify-center p-4 relative overflow-hidden">
      {/* Top right security status */}
      <div className="absolute top-6 right-6 bg-white/60 backdrop-blur-md px-4 py-2 rounded-full border border-white/50 text-xs font-semibold text-gray-700 tracking-wider flex items-center gap-2 shadow-sm">
        <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
        SECURITY STATUS: ACTIVE
      </div>

      <div className="w-full max-w-md">
        <div className="bg-white rounded-2xl shadow-[0_20px_60px_-15px_rgba(0,0,0,0.05)] border border-gray-100 p-10 md:p-12 relative z-10">
          <div className="mb-10">
            <h1 className="text-2xl font-bold text-gray-900 tracking-tight mb-2">The Glass Foundry</h1>
            <p className="text-[11px] font-semibold text-gray-500 tracking-widest uppercase leading-relaxed max-w-[200px]">
              Architectural Knowledge<br/>Management
            </p>
          </div>

          <form onSubmit={handleSubmit} className="space-y-8">
            {error && (
              <div className="p-3 bg-red-50 border border-red-100 text-red-600 text-sm rounded-lg">
                {error}
              </div>
            )}

            <div className="space-y-1.5">
              <label className="text-[11px] font-bold text-gray-500 tracking-widest uppercase">
                Operator ID
              </label>
              <input
                type="text"
                required
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="FOUNDRY_ADMIN_01"
                className="w-full bg-transparent border-b border-gray-200 py-3 text-gray-800 placeholder-gray-300 focus:outline-none focus:border-indigo-600 transition-colors tracking-wide"
              />
            </div>

            <div className="space-y-1.5">
              <div className="flex justify-between items-center">
                <label className="text-[11px] font-bold text-gray-500 tracking-widest uppercase">
                  Master Key
                </label>
                <button type="button" className="text-xs font-medium text-indigo-500 hover:text-indigo-700">
                  Recover
                </button>
              </div>
              <input
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••••••••"
                className="w-full bg-transparent border-b border-gray-200 py-3 text-gray-800 placeholder-gray-300 focus:outline-none focus:border-indigo-600 transition-colors tracking-widest text-lg"
              />
            </div>

            <button
              type="submit"
              disabled={loading}
              className="w-full bg-indigo-600 hover:bg-indigo-700 text-white font-medium py-4 rounded-lg transition-colors shadow-lg shadow-indigo-600/20 disabled:opacity-70 mt-4"
            >
              {loading ? 'Initializing...' : 'Initialize Session'}
            </button>
          </form>

          <div className="mt-12 pt-8 border-t border-gray-100">
            <div className="flex items-center gap-2 text-gray-400 mb-3">
              <Lock size={14} />
              <span className="text-xs font-medium">Encrypted AES-256 Protocol</span>
            </div>
            <p className="text-[11px] text-gray-400 leading-relaxed">
              Access is restricted to authorized personnel. All terminal activity is recorded under KMS v2.0 Archival Protocols.
            </p>
          </div>
        </div>

        {/* Bottom footer links */}
        <div className="mt-8 grid grid-cols-3 gap-4 px-4">
          <div className="text-[10px] font-bold text-gray-400 tracking-widest uppercase leading-loose">
            System<br/>Health
          </div>
          <div className="text-[10px] font-bold text-gray-400 tracking-widest uppercase leading-loose text-center">
            Legal<br/>Curator
          </div>
          <div className="text-[10px] font-bold text-gray-400 tracking-widest uppercase leading-loose text-right">
            Foundry<br/>Support
          </div>
        </div>
      </div>
    </div>
  );
}
