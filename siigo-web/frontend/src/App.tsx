import { useState, useEffect } from 'react';
import { Routes, Route, useLocation, Navigate } from 'react-router-dom';
import Sidebar from './components/Sidebar';
import Dashboard from './pages/Dashboard';
import DataPage from './pages/DataPage';
import Logs from './pages/Logs';
import Config from './pages/Config';
import FieldMappings from './pages/FieldMappings';
import ErrorSummary from './pages/ErrorSummary';
import Explorer from './pages/Explorer';
import Users from './pages/Users';
import AuditTrail from './pages/AuditTrail';
import Login from './pages/Login';
import SetupWizard from './pages/SetupWizard';
import ToastContainer from './components/Toast';
import { api, UserInfo } from './api';

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  const [setupComplete, setSetupComplete] = useState<boolean | null>(null);
  const [userInfo, setUserInfo] = useState<UserInfo | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const location = useLocation();

  useEffect(() => {
    if (api.isLoggedIn()) {
      api.checkAuth().then(res => {
        if (res.status === 'ok') {
          setAuthed(true);
          setSetupComplete(res.setup_complete as boolean);
          const info: UserInfo = {
            username: (res.username as string) || '',
            role: (res.role as string) || 'root',
            permissions: (res.permissions as string[]) || [],
          };
          setUserInfo(info);
          api.setUserInfo(info);
        } else {
          setAuthed(false);
        }
      }).catch(() => setAuthed(false));
    } else {
      setAuthed(false);
    }

    const onExpired = () => { setAuthed(false); setUserInfo(null); };
    window.addEventListener('auth-expired', onExpired);
    return () => window.removeEventListener('auth-expired', onExpired);
  }, []);

  // Close menu on route change
  useEffect(() => { setMenuOpen(false); }, [location.pathname]);

  const handleLogin = (info: UserInfo, sc?: boolean) => {
    setAuthed(true);
    setSetupComplete(sc ?? false);
    setUserInfo(info);
    api.setUserInfo(info);
  };

  if (authed === null) return null;

  if (!authed) {
    return <Login onLogin={handleLogin} />;
  }

  if (setupComplete === false) {
    return (
      <>
        <SetupWizard onComplete={() => setSetupComplete(true)} />
        <ToastContainer />
      </>
    );
  }

  const can = (mod: string) => {
    if (!userInfo) return true;
    if (userInfo.role === 'root' || userInfo.role === 'admin') return true;
    return userInfo.permissions.includes(mod);
  };

  const guard = (mod: string, el: React.ReactElement) => can(mod) ? el : <Navigate to="/" replace />;

  return (
    <div className="app-layout">
      <button className="menu-toggle" onClick={() => setMenuOpen(!menuOpen)}>
        {menuOpen ? '\u2715' : '\u2630'}
      </button>
      {menuOpen && <div className="sidebar-overlay visible" onClick={() => setMenuOpen(false)} />}
      <Sidebar
        onLogout={() => { api.logout(); api.clearUserInfo(); setAuthed(false); setUserInfo(null); }}
        open={menuOpen}
        userInfo={userInfo}
      />
      <div className="main">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/clients" element={guard('clients', <DataPage table="clients" title="Clientes (Z17 - Terceros)" file="Z17" />)} />
          <Route path="/products" element={guard('products', <DataPage table="products" title="Productos (Z04 - Inventario)" file="Z04" />)} />
          <Route path="/movements" element={guard('movements', <DataPage table="movements" title="Movimientos (Z49 - Transacciones)" file="Z49" />)} />
          <Route path="/cartera" element={guard('cartera', <DataPage table="cartera" title="Cartera (Z09 - Cuentas por Cobrar)" file="Z09" />)} />
          <Route path="/plan-cuentas" element={guard('plan_cuentas', <DataPage table="plan_cuentas" title="Plan de Cuentas (Z03 - PUC)" file="Z03" />)} />
          <Route path="/activos-fijos" element={guard('activos_fijos', <DataPage table="activos_fijos" title="Activos Fijos (Z27 - Equipos)" file="Z27" />)} />
          <Route path="/saldos-terceros" element={guard('saldos_terceros', <DataPage table="saldos_terceros" title="Saldos por Tercero (Z25)" file="Z25" />)} />
          <Route path="/saldos-consolidados" element={guard('saldos_consolidados', <DataPage table="saldos_consolidados" title="Saldos Consolidados (Z28)" file="Z28" />)} />
          <Route path="/documentos" element={guard('documentos', <DataPage table="documentos" title="Documentos (Z11 - Facturas)" file="Z11" />)} />
          <Route path="/terceros-ampliados" element={guard('terceros_ampliados', <DataPage table="terceros_ampliados" title="Terceros Ampliados (Z08A)" file="Z08A" />)} />
          <Route path="/logs" element={guard('logs', <Logs />)} />
          <Route path="/errors" element={guard('errors', <ErrorSummary />)} />
          <Route path="/field-mappings" element={guard('field-mappings', <FieldMappings />)} />
          <Route path="/explorer" element={guard('explorer', <Explorer />)} />
          <Route path="/config" element={guard('config', <Config />)} />
          <Route path="/users" element={guard('users', <Users />)} />
          <Route path="/audit" element={guard('config', <AuditTrail />)} />
        </Routes>
      </div>
      <ToastContainer />
    </div>
  );
}
