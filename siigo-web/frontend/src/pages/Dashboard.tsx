import { useState, useEffect, useCallback } from 'react';
import { api } from '../api';
import { showToast } from '../components/Toast';

interface Stats { [key: string]: number }
interface ISAMFile { file: string; record_size: number; records: number; num_keys: number; used_extfh: boolean; mod_time: string }

export default function Dashboard() {
  const [stats, setStats] = useState<Stats>({});
  const [isamInfo, setIsamInfo] = useState<ISAMFile[]>([]);
  const [syncing, setSyncing] = useState(false);
  const [sendPaused, setSendPaused] = useState(false);
  const [sendFailCount, setSendFailCount] = useState(0);
  const [lastRefresh, setLastRefresh] = useState('');

  const refresh = useCallback(async () => {
    try {
      const [s, info, status] = await Promise.all([
        api.getStats(),
        api.getISAMInfo(),
        api.getSyncStatus(),
      ]);
      setStats(s);
      setIsamInfo(info);
      setSyncing(status.syncing);
      setSendPaused(status.send_paused === true);
      setSendFailCount(status.send_fail_count || 0);
      setLastRefresh(new Date().toLocaleTimeString('es-CO'));
    } catch { /* ignore */ }
  }, []);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 10000);
    return () => clearInterval(interval);
  }, [refresh]);

  const cards = [
    { label: 'Clientes', total: stats.clients_total || 0, synced: stats.clients_synced || 0, pending: stats.clients_pending || 0, errors: stats.clients_errors || 0, color: 'green' },
    { label: 'Productos', total: stats.products_total || 0, synced: stats.products_synced || 0, pending: stats.products_pending || 0, errors: stats.products_errors || 0, color: 'blue' },
    { label: 'Movimientos', total: stats.movements_total || 0, synced: stats.movements_synced || 0, pending: stats.movements_pending || 0, errors: stats.movements_errors || 0, color: 'yellow' },
    { label: 'Cartera', total: stats.cartera_total || 0, synced: stats.cartera_synced || 0, pending: stats.cartera_pending || 0, errors: stats.cartera_errors || 0, color: 'purple' },
  ];

  const totalPending = cards.reduce((s, c) => s + c.pending, 0);
  const totalErrors = cards.reduce((s, c) => s + c.errors, 0);

  return (
    <>
      <div className="topbar">
        <h2>Dashboard</h2>
        <div className="topbar-actions">
          <span className={`dashboard-status ${syncing ? 'syncing' : 'idle'}`}>
            {syncing ? 'Sincronizando...' : 'En espera'}
          </span>
          <span className="last-refresh">Actualizado: {lastRefresh}</span>
        </div>
      </div>
      <div className="content">
        {sendPaused && (
          <div className="alert alert-danger" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span>Envio auto-pausado tras {sendFailCount} fallos consecutivos. Revisa el servidor de destino.</span>
            <button className="btn-save" style={{ padding: '6px 16px', fontSize: 13 }} onClick={async () => {
              await api.sendResume();
              setSendPaused(false);
              setSendFailCount(0);
              showToast('success', 'Envio reactivado');
            }}>Reactivar Envio</button>
          </div>
        )}
        {(totalPending > 0 || totalErrors > 0) && (
          <div className="dashboard-alerts">
            {totalPending > 0 && (
              <div className="alert alert-warning">{totalPending} registros pendientes de envio</div>
            )}
            {totalErrors > 0 && (
              <div className="alert alert-error">{totalErrors} registros con error</div>
            )}
          </div>
        )}

        <div className="stats-row">
          {cards.map(c => {
            const pct = c.total > 0 ? Math.round((c.synced / c.total) * 100) : 0;
            return (
              <div key={c.label} className="stat-card">
                <div className="label">{c.label}</div>
                <div className={`value ${c.color}`}>{c.total}</div>
                <div className="stat-bar">
                  <div className="stat-bar-fill" style={{ width: `${pct}%` }} />
                </div>
                <div className="stat-details">
                  <span className="stat-synced">{c.synced} sincronizados ({pct}%)</span>
                  {c.pending > 0 && <span className="stat-pending">{c.pending} pendientes</span>}
                  {c.errors > 0 && <span className="stat-errors">{c.errors} errores</span>}
                </div>
              </div>
            );
          })}
        </div>

        <h3 className="section-title">Archivos ISAM (Siigo)</h3>
        <div className="file-info">
          {isamInfo.map(f => (
            <div key={f.file} className="file-card">
              <h3>{f.file}</h3>
              <div className="info-row"><span className="label">Registros:</span><span>{f.records >= 0 ? f.records : 'Error'}</span></div>
              <div className="info-row"><span className="label">Tam. registro:</span><span>{f.record_size || '-'} bytes</span></div>
              <div className="info-row"><span className="label">EXTFH:</span><span>{f.used_extfh ? 'Si' : 'No'}</span></div>
              <div className="info-row"><span className="label">Modificado:</span><span>{f.mod_time || '-'}</span></div>
            </div>
          ))}
        </div>
      </div>
    </>
  );
}
