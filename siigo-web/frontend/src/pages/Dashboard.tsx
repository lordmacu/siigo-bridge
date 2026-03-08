import { useState, useEffect, useCallback } from 'react';
import { api } from '../api';
import { showToast } from '../components/Toast';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from 'recharts';

interface Stats { [key: string]: number }
interface ISAMFile { file: string; record_size: number; records: number; num_keys: number; used_extfh: boolean; mod_time: string }
interface SyncStat { table_name: string; total: number; pending: number; synced: number; errors: number; created_at: string }

export default function Dashboard() {
  const [stats, setStats] = useState<Stats>({});
  const [isamInfo, setIsamInfo] = useState<ISAMFile[]>([]);
  const [syncing, setSyncing] = useState(false);
  const [sendPaused, setSendPaused] = useState(false);
  const [sendFailCount, setSendFailCount] = useState(0);
  const [sendEnabled, setSendEnabled] = useState<Record<string, boolean>>({});
  const [lastRefresh, setLastRefresh] = useState('');
  const [chartData, setChartData] = useState<Record<string, unknown>[]>([]);
  const [chartHours, setChartHours] = useState(24);

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
      setSendEnabled(status.send_enabled || {});
      setLastRefresh(new Date().toLocaleTimeString('es-CO'));
    } catch { /* ignore */ }
  }, []);

  const loadChart = useCallback(async () => {
    try {
      const res = await api.getSyncStatsHistory(chartHours);
      const entries: SyncStat[] = res.entries || [];

      // Group by timestamp, merge tables
      const byTime: Record<string, Record<string, unknown>> = {};
      entries.forEach((e: SyncStat) => {
        const t = e.created_at.slice(11, 16); // HH:MM
        if (!byTime[t]) byTime[t] = { time: t };
        byTime[t][e.table_name + '_total'] = e.total;
        byTime[t][e.table_name + '_pending'] = e.pending;
        byTime[t][e.table_name + '_errors'] = e.errors;
      });
      setChartData(Object.values(byTime));
    } catch { /* ignore */ }
  }, [chartHours]);

  useEffect(() => {
    refresh();
    loadChart();
    const interval = setInterval(refresh, 10000);
    const chartInterval = setInterval(loadChart, 60000);
    return () => { clearInterval(interval); clearInterval(chartInterval); };
  }, [refresh, loadChart]);

  // SSE for real-time updates
  useEffect(() => {
    let es: EventSource | null = null;
    try {
      const token = localStorage.getItem('siigo_token');
      es = new EventSource(`/api/events${token ? `?token=${token}` : ''}`);
      es.addEventListener('sync_complete', () => {
        refresh();
        loadChart();
      });
      es.addEventListener('send_complete', () => {
        refresh();
      });
      es.onerror = () => { es?.close(); };
    } catch { /* ignore */ }
    return () => es?.close();
  }, [refresh, loadChart]);

  const cards = [
    { label: 'Clientes', total: stats.clients_total || 0, synced: stats.clients_synced || 0, pending: stats.clients_pending || 0, errors: stats.clients_errors || 0, color: 'green' },
    { label: 'Productos', total: stats.products_total || 0, synced: stats.products_synced || 0, pending: stats.products_pending || 0, errors: stats.products_errors || 0, color: 'blue' },
    { label: 'Movimientos', total: stats.movements_total || 0, synced: stats.movements_synced || 0, pending: stats.movements_pending || 0, errors: stats.movements_errors || 0, color: 'yellow' },
    { label: 'Cartera', total: stats.cartera_total || 0, synced: stats.cartera_synced || 0, pending: stats.cartera_pending || 0, errors: stats.cartera_errors || 0, color: 'purple' },
    { label: 'Plan Cuentas', total: stats.plan_cuentas_total || 0, synced: stats.plan_cuentas_synced || 0, pending: stats.plan_cuentas_pending || 0, errors: stats.plan_cuentas_errors || 0, color: 'green' },
    { label: 'Activos Fijos', total: stats.activos_fijos_total || 0, synced: stats.activos_fijos_synced || 0, pending: stats.activos_fijos_pending || 0, errors: stats.activos_fijos_errors || 0, color: 'blue' },
    { label: 'Saldos x Tercero', total: stats.saldos_terceros_total || 0, synced: stats.saldos_terceros_synced || 0, pending: stats.saldos_terceros_pending || 0, errors: stats.saldos_terceros_errors || 0, color: 'yellow' },
    { label: 'Saldos Consol.', total: stats.saldos_consolidados_total || 0, synced: stats.saldos_consolidados_synced || 0, pending: stats.saldos_consolidados_pending || 0, errors: stats.saldos_consolidados_errors || 0, color: 'purple' },
    { label: 'Documentos', total: stats.documentos_total || 0, synced: stats.documentos_synced || 0, pending: stats.documentos_pending || 0, errors: stats.documentos_errors || 0, color: 'green' },
    { label: 'Terceros Amp.', total: stats.terceros_ampliados_total || 0, synced: stats.terceros_ampliados_synced || 0, pending: stats.terceros_ampliados_pending || 0, errors: stats.terceros_ampliados_errors || 0, color: 'blue' },
  ];

  const totalPending = cards.reduce((s, c) => s + c.pending, 0);
  const totalErrors = cards.reduce((s, c) => s + c.errors, 0);
  const anySendEnabled = Object.values(sendEnabled).some(v => v === true);

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
        {!anySendEnabled && !sendPaused && (
          <div className="alert alert-warning">
            Envio al servidor deshabilitado. Activa el envio por tabla en <strong>Mapeo de Campos</strong>.
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

        {/* Sync History Chart */}
        {chartData.length > 0 && (
          <>
            <div className="chart-header">
              <h3 className="section-title">Historial de Sincronizacion</h3>
              <div className="chart-period">
                {[6, 12, 24, 48, 72].map(h => (
                  <button
                    key={h}
                    className={`chart-period-btn ${chartHours === h ? 'active' : ''}`}
                    onClick={() => setChartHours(h)}
                  >{h}h</button>
                ))}
              </div>
            </div>
            <div className="chart-container">
              <ResponsiveContainer width="100%" height={250}>
                <LineChart data={chartData}>
                  <CartesianGrid strokeDasharray="3 3" stroke="#334155" />
                  <XAxis dataKey="time" stroke="#94a3b8" fontSize={11} />
                  <YAxis stroke="#94a3b8" fontSize={11} />
                  <Tooltip
                    contentStyle={{ background: '#1e293b', border: '1px solid #334155', borderRadius: 8, fontSize: 12 }}
                    labelStyle={{ color: '#e2e8f0' }}
                  />
                  <Legend />
                  <Line type="monotone" dataKey="clients_pending" name="Clientes pend." stroke="#10b981" strokeWidth={2} dot={false} />
                  <Line type="monotone" dataKey="products_pending" name="Productos pend." stroke="#3b82f6" strokeWidth={2} dot={false} />
                  <Line type="monotone" dataKey="movements_pending" name="Movimientos pend." stroke="#eab308" strokeWidth={2} dot={false} />
                  <Line type="monotone" dataKey="cartera_pending" name="Cartera pend." stroke="#a855f7" strokeWidth={2} dot={false} />
                  <Line type="monotone" dataKey="clients_errors" name="Clientes err." stroke="#ef4444" strokeWidth={1} dot={false} strokeDasharray="5 5" />
                </LineChart>
              </ResponsiveContainer>
            </div>
          </>
        )}

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
