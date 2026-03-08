import { useState, useEffect } from 'react';
import { Routes, Route, useLocation } from 'react-router-dom';
import Sidebar from './components/Sidebar';
import Dashboard from './pages/Dashboard';
import DataPage from './pages/DataPage';
import Logs from './pages/Logs';
import Config from './pages/Config';
import FieldMappings from './pages/FieldMappings';
import ErrorSummary from './pages/ErrorSummary';
import Login from './pages/Login';
import ToastContainer from './components/Toast';
import { api } from './api';

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const location = useLocation();

  useEffect(() => {
    if (api.isLoggedIn()) {
      api.checkAuth().then(ok => setAuthed(ok));
    } else {
      setAuthed(false);
    }

    const onExpired = () => setAuthed(false);
    window.addEventListener('auth-expired', onExpired);
    return () => window.removeEventListener('auth-expired', onExpired);
  }, []);

  // Close menu on route change
  useEffect(() => { setMenuOpen(false); }, [location.pathname]);

  if (authed === null) return null;

  if (!authed) {
    return <Login onLogin={() => setAuthed(true)} />;
  }

  return (
    <div className="app-layout">
      <button className="menu-toggle" onClick={() => setMenuOpen(!menuOpen)}>
        {menuOpen ? '\u2715' : '\u2630'}
      </button>
      {menuOpen && <div className="sidebar-overlay visible" onClick={() => setMenuOpen(false)} />}
      <Sidebar
        onLogout={() => { api.logout(); setAuthed(false); }}
        open={menuOpen}
      />
      <div className="main">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/clients" element={<DataPage table="clients" title="Clientes (Z17 - Terceros)" file="Z17" />} />
          <Route path="/products" element={<DataPage table="products" title="Productos (Z06CP - Inventario)" file="Z06CP" />} />
          <Route path="/movements" element={<DataPage table="movements" title="Movimientos (Z49 - Transacciones)" file="Z49" />} />
          <Route path="/cartera" element={<DataPage table="cartera" title="Cartera (Z09 - Cuentas por Cobrar)" file="Z09" />} />
          <Route path="/logs" element={<Logs />} />
          <Route path="/errors" element={<ErrorSummary />} />
          <Route path="/field-mappings" element={<FieldMappings />} />
          <Route path="/config" element={<Config />} />
        </Routes>
      </div>
      <ToastContainer />
    </div>
  );
}
