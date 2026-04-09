import { useLocation, useNavigate } from 'react-router-dom';
import { useState, useEffect, useCallback } from 'react';
import { api, UserInfo } from '../api';

const navItems = [
  { path: '/', label: 'Dashboard', badge: '', module: 'dashboard' },
  { path: '/clients', label: 'Clientes', badge: 'Z17', module: 'clients' },
  { path: '/products', label: 'Productos', badge: 'Z04', module: 'products' },
  { path: '/cartera', label: 'Cartera', badge: 'Z09', module: 'cartera' },
  { path: '/documentos', label: 'Documentos', badge: 'Z11', module: 'documentos' },
  { path: '/condiciones-pago', label: 'Condiciones Pago', badge: 'Z05', module: 'condiciones_pago' },
  { path: '/codigos-dane', label: 'Codigos DANE', badge: 'ZDANE', module: 'codigos_dane' },
  { path: '/formulas', label: 'Formulas', badge: 'Z06R', module: 'formulas' },
  { path: '/vendedores-areas', label: 'Vendedores/Areas', badge: 'Z06A', module: 'vendedores_areas' },
  { path: '/notas-documentos', label: 'Notas Documentos', badge: 'Z49', module: 'notas_documentos' },
  { path: '/facturas-electronicas', label: 'Fact. Electronicas', badge: 'Z09ELE', module: 'facturas_electronicas' },
  { path: '/detalle-movimientos', label: 'Detalle Movimientos', badge: 'Z17', module: 'detalle_movimientos' },
  { path: '/field-mappings', label: 'Mapeo Campos', badge: '', module: 'field-mappings' },
  { path: '/errors', label: 'Errores', badge: '', module: 'errors' },
  { path: '/logs', label: 'Logs', badge: '', module: 'logs' },
  { path: '/explorer', label: 'SQL Explorer', badge: '', module: 'explorer' },
  { path: '/config', label: 'Configuracion', badge: '', module: 'config' },
  { path: '/users', label: 'Usuarios', badge: '', module: 'users' },
  { path: '/audit', label: 'Auditoria', badge: '', module: 'config' },
];

// Modules that are not data tables — always shown regardless of record count
const alwaysShow = new Set(['dashboard', 'field-mappings', 'errors', 'logs', 'explorer', 'config', 'users']);

export default function Sidebar({ onLogout, open, userInfo }: { onLogout?: () => void; open?: boolean; userInfo?: UserInfo | null }) {
  const location = useLocation();
  const navigate = useNavigate();
  const [syncing, setSyncing] = useState(false);
  const [paused, setPaused] = useState(false);
  const [watcherActive, setWatcherActive] = useState(false);
  const [tableCounts, setTableCounts] = useState<Record<string, number>>({});
  const [version, setVersion] = useState<string>('');
  const [updateAvailable, setUpdateAvailable] = useState(false);
  const [latestVersion, setLatestVersion] = useState<string>('');

  const pollStatus = useCallback(async () => {
    try {
      const s = await api.getSyncStatus();
      setSyncing(s.syncing);
      setPaused(s.paused);
      setWatcherActive(s.watcher_active === true);
    } catch { /* ignore */ }
  }, []);

  // Load stats to know which tables have data
  useEffect(() => {
    const loadCounts = async () => {
      try {
        const stats = await api.getStats();
        const counts: Record<string, number> = {};
        for (const [key, val] of Object.entries(stats)) {
          if (key.endsWith('_total')) {
            const table = key.replace('_total', '');
            counts[table] = val as number;
          }
        }
        setTableCounts(counts);
      } catch { /* ignore */ }
    };
    loadCounts();
    const interval = setInterval(loadCounts, 60000); // refresh every 60s
    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    pollStatus();
    const interval = setInterval(pollStatus, 3000);
    return () => clearInterval(interval);
  }, [pollStatus]);

  // Load version and check for updates
  useEffect(() => {
    const loadVersion = async () => {
      try {
        const health = await fetch('/health').then(r => r.json());
        setVersion(health.version || '');
      } catch { /* ignore */ }

      try {
        const upd = await api.checkUpdate();
        if (upd.update_available) {
          setUpdateAvailable(true);
          setLatestVersion(upd.latest_version || '');
        }
      } catch { /* ignore */ }
    };
    loadVersion();
  }, []);

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
          // Permission check
          if (userInfo && userInfo.role !== 'root' && userInfo.role !== 'admin') {
            if (item.module !== 'dashboard' && !userInfo.permissions.includes(item.module)) return false;
          }
          // Hide data tables with 0 records (system pages always visible)
          if (!alwaysShow.has(item.module) && item.badge && Object.keys(tableCounts).length > 0) {
            const count = tableCounts[item.module] ?? 0;
            if (count === 0) return false;
          }
          return true;
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
          {syncing ? 'Sincronizando...' : paused ? 'Pausado' : watcherActive ? 'Vigilando cambios' : 'Escuchando'}
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
        {version && (
          <div className="sidebar-version" style={{
            marginTop: '10px',
            padding: '6px 8px',
            fontSize: '11px',
            color: updateAvailable ? '#fbbf24' : '#94a3b8',
            textAlign: 'center',
            borderTop: '1px solid #334155',
            cursor: updateAvailable ? 'pointer' : 'default'
          }}
          onClick={() => {
            if (updateAvailable && confirm(`Actualizar a v${latestVersion}?\n\nEl programa se reiniciara automaticamente.`)) {
              api.applyUpdate();
            }
          }}
          title={updateAvailable ? `Click para actualizar a v${latestVersion}` : ''}
          >
            {updateAvailable ? `v${version} → v${latestVersion} (actualizar)` : `v${version}`}
          </div>
        )}
      </div>
    </div>
  );
}
