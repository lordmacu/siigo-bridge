import { useLocation, useNavigate } from 'react-router-dom';
import { useState, useEffect, useCallback } from 'react';
import { api, UserInfo } from '../api';

const navItems = [
  { path: '/', label: 'Dashboard', badge: '', module: 'dashboard' },
  { path: '/clients', label: 'Clientes', badge: 'Z17', module: 'clients' },
  { path: '/products', label: 'Productos', badge: 'Z04', module: 'products' },
  { path: '/movements', label: 'Movimientos', badge: 'Z49', module: 'movements' },
  { path: '/cartera', label: 'Cartera', badge: 'Z09', module: 'cartera' },
  { path: '/plan-cuentas', label: 'Plan Cuentas', badge: 'Z03', module: 'plan_cuentas' },
  { path: '/activos-fijos', label: 'Activos Fijos', badge: 'Z27', module: 'activos_fijos' },
  { path: '/saldos-terceros', label: 'Saldos x Tercero', badge: 'Z25', module: 'saldos_terceros' },
  { path: '/saldos-consolidados', label: 'Saldos Consol.', badge: 'Z28', module: 'saldos_consolidados' },
  { path: '/documentos', label: 'Documentos', badge: 'Z11', module: 'documentos' },
  { path: '/terceros-ampliados', label: 'Terceros Amp.', badge: 'Z08A', module: 'terceros_ampliados' },
  { path: '/movimientos-inventario', label: 'Mov. Inventario', badge: 'Z16', module: 'movimientos_inventario' },
  { path: '/saldos-inventario', label: 'Saldos Inv.', badge: 'Z15', module: 'saldos_inventario' },
  { path: '/activos-fijos-detalle', label: 'Activos Det.', badge: 'Z27A', module: 'activos_fijos_detalle' },
  { path: '/audit-trail-terceros', label: 'Audit Terceros', badge: 'Z11N', module: 'audit_trail_terceros' },
  { path: '/field-mappings', label: 'Mapeo Campos', badge: '', module: 'field-mappings' },
  { path: '/errors', label: 'Errores', badge: '', module: 'errors' },
  { path: '/logs', label: 'Logs', badge: '', module: 'logs' },
  { path: '/explorer', label: 'SQL Explorer', badge: '', module: 'explorer' },
  { path: '/config', label: 'Configuracion', badge: '', module: 'config' },
  { path: '/users', label: 'Usuarios', badge: '', module: 'users' },
  { path: '/audit', label: 'Auditoria', badge: '', module: 'config' },
];

export default function Sidebar({ onLogout, open, userInfo }: { onLogout?: () => void; open?: boolean; userInfo?: UserInfo | null }) {
  const location = useLocation();
  const navigate = useNavigate();
  const [syncing, setSyncing] = useState(false);
  const [paused, setPaused] = useState(false);

  const pollStatus = useCallback(async () => {
    try {
      const s = await api.getSyncStatus();
      setSyncing(s.syncing);
      setPaused(s.paused);
    } catch { /* ignore */ }
  }, []);

  useEffect(() => {
    pollStatus();
    const interval = setInterval(pollStatus, 3000);
    return () => clearInterval(interval);
  }, [pollStatus]);

  const handleSync = async () => {
    await api.syncNow();
    setSyncing(true);
  };

  const handleTogglePause = async () => {
    if (paused) {
      await api.resume();
      setPaused(false);
    } else {
      await api.pause();
      setPaused(true);
    }
  };

  return (
    <div className={`sidebar ${open ? 'open' : ''}`}>
      <div className="sidebar-header">
        <h1>Siigo Sync</h1>
        <small>Middleware Manager</small>
      </div>
      <div className="nav-items">
        {navItems.filter(item => {
          if (!userInfo || userInfo.role === 'root' || userInfo.role === 'admin') return true;
          return item.module === 'dashboard' || userInfo.permissions.includes(item.module);
        }).map(item => (
          <div
            key={item.path}
            className={`nav-item ${location.pathname === item.path ? 'active' : ''}`}
            onClick={() => navigate(item.path)}
          >
            <span>{item.label}</span>
            {item.badge && <span className="badge">{item.badge}</span>}
          </div>
        ))}
      </div>
      <div className="sidebar-footer">
        <div className={`sync-status ${syncing ? 'active' : paused ? 'paused' : 'running'}`}>
          {syncing ? 'Sincronizando...' : paused ? 'Pausado' : 'Escuchando'}
        </div>
        <button
          className={`sync-btn ${syncing ? 'syncing' : ''}`}
          onClick={handleSync}
          disabled={syncing}
        >
          {syncing ? 'Sincronizando...' : 'Sincronizar Ahora'}
        </button>
        <button
          className={`pause-btn ${paused ? 'paused' : ''}`}
          onClick={handleTogglePause}
        >
          {paused ? 'Reanudar Auto-Sync' : 'Pausar Auto-Sync'}
        </button>
        {userInfo && (
          <div className="sidebar-user">
            <span className="sidebar-username">{userInfo.username}</span>
            <span className="sidebar-role">{userInfo.role}</span>
          </div>
        )}
        {onLogout && (
          <button className="logout-btn" onClick={onLogout}>
            Cerrar Sesion
          </button>
        )}
      </div>
    </div>
  );
}
