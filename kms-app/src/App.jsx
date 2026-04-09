import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { AuthProvider, useAuth } from './context/AuthContext';
import Login from './components/Login';
import Layout from './components/Layout';
import Overview from './components/Overview';
import Keys from './components/Keys';
import CryptoOps from './components/CryptoOps';
import Users from './components/Users';
import AuditLog from './components/AuditLog';
import SecurityAudit from './components/SecurityAudit';

const ProtectedRoute = ({ children }) => {
  const { token, loading } = useAuth();
  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50 text-indigo-600 text-sm font-semibold tracking-widest uppercase">
        Initializing Vault...
      </div>
    );
  }
  if (!token) return <Navigate to="/login" replace />;
  return children;
};

const AdminRoute = ({ children }) => {
  const { user } = useAuth();
  if (user?.role !== 'admin') return <Navigate to="/" replace />;
  return children;
};

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route
            path="/"
            element={
              <ProtectedRoute>
                <Layout />
              </ProtectedRoute>
            }
          >
            <Route index element={<Overview />} />
            <Route path="keys" element={<Keys />} />
            <Route path="crypto" element={<CryptoOps />} />
            <Route path="audit" element={<AuditLog />} />
            <Route path="security" element={<SecurityAudit />} />
            <Route
              path="users"
              element={
                <AdminRoute>
                  <Users />
                </AdminRoute>
              }
            />
          </Route>
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  );
}
