import { useLocation, useNavigate } from 'react-router-dom';
import { useState, useEffect, useCallback } from 'react';
import { api } from '../api';

const navItems = [
  { path: '/', label: 'Dashboard', badge: '' },
  { path: '/clients', label: 'Clientes', badge: 'Z17' },
  { path: '/products', label: 'Productos', badge: 'Z06CP' },
  { path: '/movements', label: 'Movimientos', badge: 'Z49' },
  { path: '/cartera', label: 'Cartera', badge: 'Z09' },
  { path: '/logs', label: 'Logs', badge: '' },
  { path: '/config', label: 'Configuracion', badge: '' },
];

export default function Sidebar() {
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
    <div className="sidebar">
      <div className="sidebar-header">
        <h1>Siigo Sync</h1>
        <small>Middleware Manager</small>
      </div>
      <div className="nav-items">
        {navItems.map(item => (
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
      </div>
    </div>
  );
}
