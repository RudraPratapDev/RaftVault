import { NavLink, Outlet, useNavigate } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import {
  LayoutDashboard, KeyRound, ShieldCheck, Users, ScrollText,
  LogOut, HelpCircle, Lock, ShieldAlert, Layers
} from 'lucide-react';

const NAV = [
  { to: '/', label: 'Overview', icon: LayoutDashboard, end: true },
  { to: '/keys', label: 'Key Registry', icon: KeyRound },
  { to: '/crypto', label: 'Encrypt / Decrypt', icon: ShieldCheck },
  { to: '/audit', label: 'Audit Log', icon: ScrollText },
  { to: '/internals', label: 'Crypto Internals', icon: Layers },
  { to: '/security', label: 'Security Audit', icon: ShieldAlert },
];

const ADMIN_NAV = [
  { to: '/users', label: 'User Management', icon: Users },
];

export default function Layout() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  return (
    <div className="min-h-screen bg-[#F9FAFB] flex">
      {/* Sidebar */}
      <div className="w-64 bg-white border-r border-gray-200 flex flex-col pt-8 shrink-0">
        {/* Brand */}
        <div className="px-6 flex items-center gap-3 mb-10">
          <div className="w-8 h-8 rounded bg-indigo-600 flex items-center justify-center text-white font-bold text-sm">
            <Lock size={14} />
          </div>
          <div>
            <h2 className="font-bold text-gray-900 text-sm">Glass Foundry</h2>
            <div className="text-[10px] font-bold text-gray-400 tracking-widest">KMS V2.0</div>
          </div>
        </div>

        {/* Nav */}
        <nav className="flex-1 space-y-0.5 px-3">
          {NAV.map(({ to, label, icon: Icon, end }) => (
            <NavLink
              key={to}
              to={to}
              end={end}
              className={({ isActive }) =>
                `flex items-center gap-3 px-3 py-2.5 rounded-lg cursor-pointer text-xs font-semibold tracking-wider uppercase transition-colors ${
                  isActive
                    ? 'text-indigo-600 bg-indigo-50'
                    : 'text-gray-500 hover:bg-gray-50 hover:text-gray-900'
                }`
              }
            >
              <Icon size={15} />
              {label}
            </NavLink>
          ))}

          {user?.role === 'admin' && (
            <>
              <div className="pt-4 pb-1 px-3">
                <span className="text-[9px] font-bold text-gray-400 tracking-widest uppercase">Admin</span>
              </div>
              {ADMIN_NAV.map(({ to, label, icon: Icon }) => (
                <NavLink
                  key={to}
                  to={to}
                  className={({ isActive }) =>
                    `flex items-center gap-3 px-3 py-2.5 rounded-lg cursor-pointer text-xs font-semibold tracking-wider uppercase transition-colors ${
                      isActive
                        ? 'text-indigo-600 bg-indigo-50'
                        : 'text-gray-500 hover:bg-gray-50 hover:text-gray-900'
                    }`
                  }
                >
                  <Icon size={15} />
                  {label}
                </NavLink>
              ))}
            </>
          )}
        </nav>

        {/* Footer */}
        <div className="p-6 border-t border-gray-100 space-y-3">
          <div className="flex items-center gap-2 px-3 py-2">
            <div className="w-6 h-6 rounded-full bg-slate-800 text-white flex items-center justify-center text-[10px] font-bold">
              {user?.username?.[0]?.toUpperCase()}
            </div>
            <div>
              <div className="text-xs font-semibold text-gray-800">{user?.username}</div>
              <div className="text-[10px] text-gray-400 uppercase tracking-wider">{user?.role}</div>
            </div>
          </div>
          <button className="flex items-center gap-3 text-gray-500 hover:text-gray-900 text-xs font-semibold tracking-wider uppercase px-3 py-1.5 w-full">
            <HelpCircle size={14} />
            Help
          </button>
          <button
            onClick={handleLogout}
            className="flex items-center gap-3 text-gray-500 hover:text-red-600 text-xs font-semibold tracking-wider uppercase px-3 py-1.5 w-full transition-colors"
          >
            <LogOut size={14} />
            Logout
          </button>
        </div>
      </div>

      {/* Main */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Top bar */}
        <div className="h-14 border-b border-gray-200 bg-white flex items-center justify-between px-8 shrink-0">
          <div className="flex items-center gap-2 text-[10px] font-bold uppercase tracking-widest text-gray-600">
            <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
            Vault: Operational
          </div>
          <div className="text-[10px] font-bold text-gray-400 tracking-widest uppercase">
            AES-256-GCM · HKDF-SHA256 · RSA-OAEP · HMAC Chain
          </div>
        </div>

        {/* Page content */}
        <div className="flex-1 overflow-auto">
          <Outlet />
        </div>
      </div>
    </div>
  );
}
