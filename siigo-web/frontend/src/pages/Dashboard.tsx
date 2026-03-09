import { useState, useEffect, useCallback } from 'react';
import { api } from '../api';
import { showToast } from '../components/Toast';
import EmptyState from '../components/EmptyState';
import Toggle from '../components/Toggle';
import PageHeader from '../components/PageHeader';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from 'recharts';

interface Stats { [key: string]: number }
interface ISAMFile { file: string; record_size: number; records: number; num_keys: number; used_extfh: boolean; mod_time: string }
interface SyncStat { table_name: string; total: number; pending: number; synced: number; errors: number; created_at: string }

interface DashPrefs {
  visibleCards: string[];
  chartTables: string[];
  chartMetric: 'pending' | 'errors' | 'total';
  showISAM: boolean;
  showChart: boolean;
}

const ALL_TABLES = [
  { key: 'clients', label: 'Clientes', color: '#4ade80', colorName: 'green' },
  { key: 'products', label: 'Productos', color: '#60a5fa', colorName: 'blue' },
  { key: 'movements', label: 'Movimientos', color: '#facc15', colorName: 'yellow' },
  { key: 'cartera', label: 'Cartera', color: '#c084fc', colorName: 'purple' },
  { key: 'plan_cuentas', label: 'Plan Cuentas', color: '#34d399', colorName: 'green' },
  { key: 'activos_fijos', label: 'Activos Fijos', color: '#38bdf8', colorName: 'blue' },
  { key: 'saldos_terceros', label: 'Saldos x Tercero', color: '#fbbf24', colorName: 'yellow' },
  { key: 'saldos_consolidados', label: 'Saldos Consol.', color: '#a78bfa', colorName: 'purple' },
  { key: 'documentos', label: 'Documentos', color: '#2dd4bf', colorName: 'green' },
  { key: 'terceros_ampliados', label: 'Terceros Amp.', color: '#818cf8', colorName: 'blue' },
  { key: 'transacciones_detalle', label: 'Trans. Detalle', color: '#fb923c', colorName: 'yellow' },
  { key: 'periodos_contables', label: 'Periodos', color: '#e879f9', colorName: 'purple' },
  { key: 'condiciones_pago', label: 'Cond. Pago', color: '#a3e635', colorName: 'green' },
  { key: 'libros_auxiliares', label: 'Libros Aux.', color: '#22d3ee', colorName: 'blue' },
  { key: 'codigos_dane', label: 'DANE', color: '#fcd34d', colorName: 'yellow' },
  { key: 'actividades_ica', label: 'ICA', color: '#d946ef', colorName: 'purple' },
  { key: 'conceptos_pila', label: 'PILA', color: '#86efac', colorName: 'green' },
  { key: 'activos_fijos_detalle', label: 'Activos Det.', color: '#93c5fd', colorName: 'blue' },
  { key: 'audit_trail_terceros', label: 'Audit Trail', color: '#fdba74', colorName: 'yellow' },
  { key: 'clasificacion_cuentas', label: 'Clasif. Cuentas', color: '#f0abfc', colorName: 'purple' },
];

const DEFAULT_CARDS = ['clients', 'products', 'movements', 'cartera'];
const DEFAULT_CHART = ['clients', 'products', 'movements', 'cartera'];

const DEFAULT_PREFS: DashPrefs = {
  visibleCards: DEFAULT_CARDS,
  chartTables: DEFAULT_CHART,
  chartMetric: 'pending',
  showISAM: true,
  showChart: true,
};

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
  const [prefs, setPrefs] = useState<DashPrefs>(DEFAULT_PREFS);
  const [prefsLoaded, setPrefsLoaded] = useState(false);
  const [showConfig, setShowConfig] = useState(false);

  // Load prefs from SQLite on mount
  useEffect(() => {
    api.getUserPrefs('dashboard').then((data: DashPrefs | Record<string, never>) => {
      if (data && data.visibleCards) {
        setPrefs(data as DashPrefs);
      }
      setPrefsLoaded(true);
    }).catch(() => setPrefsLoaded(true));
  }, []);

  const updatePrefs = (update: Partial<DashPrefs>) => {
    setPrefs(prev => {
      const next = { ...prev, ...update };
      api.saveUserPrefs(next, 'dashboard').catch(() => { /* ignore */ });
      return next;
    });
  };

  const toggleCard = (key: string) => {
    const list = prefs.visibleCards.includes(key)
      ? prefs.visibleCards.filter(k => k !== key)
      : [...prefs.visibleCards, key];
    updatePrefs({ visibleCards: list });
  };

  const toggleChartTable = (key: string) => {
    const list = prefs.chartTables.includes(key)
      ? prefs.chartTables.filter(k => k !== key)
      : [...prefs.chartTables, key];
    updatePrefs({ chartTables: list });
  };

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
      const byTime: Record<string, Record<string, unknown>> = {};
      entries.forEach((e: SyncStat) => {
        const t = e.created_at.slice(11, 16);
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

  useEffect(() => {
    let es: EventSource | null = null;
    try {
      const token = localStorage.getItem('siigo_token');
      es = new EventSource(`/api/events${token ? `?token=${token}` : ''}`);
      es.addEventListener('sync_complete', () => { refresh(); loadChart(); });
      es.addEventListener('send_complete', () => { refresh(); });
      es.onerror = () => { es?.close(); };
    } catch { /* ignore */ }
    return () => es?.close();
  }, [refresh, loadChart]);

  const cards = ALL_TABLES
    .filter(t => prefs.visibleCards.includes(t.key))
    .map(t => ({
      ...t,
      total: stats[t.key + '_total'] || 0,
      synced: stats[t.key + '_synced'] || 0,
      pending: stats[t.key + '_pending'] || 0,
      errors: stats[t.key + '_errors'] || 0,
    }));

  const allCards = ALL_TABLES.map(t => ({
    ...t,
    total: stats[t.key + '_total'] || 0,
    pending: stats[t.key + '_pending'] || 0,
    errors: stats[t.key + '_errors'] || 0,
  }));

  const totalPending = allCards.reduce((s, c) => s + c.pending, 0);
  const totalErrors = allCards.reduce((s, c) => s + c.errors, 0);
  const anySendEnabled = Object.values(sendEnabled).some(v => v === true);

  const chartLines = ALL_TABLES.filter(t => prefs.chartTables.includes(t.key));
  const metricSuffix = '_' + prefs.chartMetric;
  const metricLabel = prefs.chartMetric === 'pending' ? 'pend.' : prefs.chartMetric === 'errors' ? 'err.' : 'total';

  return (
    <>
      <PageHeader title="Dashboard">
          <button className="btn-sm btn-outline" onClick={() => setShowConfig(!showConfig)}>
            {showConfig ? 'Cerrar' : 'Personalizar'}
          </button>
          <span className={`dashboard-status ${syncing ? 'syncing' : 'idle'}`}>
            {syncing ? 'Sincronizando...' : 'En espera'}
          </span>
          <span className="last-refresh">Actualizado: {lastRefresh}</span>
      </PageHeader>
      <div className="content">
        {/* Customization Panel */}
        {showConfig && (
          <div className="dash-config-panel">
            <div className="dash-config-section">
              <h4>Cards visibles</h4>
              <p className="dash-config-hint">Selecciona las tablas que quieres ver como tarjetas</p>
              <div className="dash-config-grid">
                {ALL_TABLES.map(t => (
                  <label key={t.key} className="dash-config-toggle">
                    <Toggle checked={prefs.visibleCards.includes(t.key)} onChange={() => toggleCard(t.key)} />
                    <span style={{ color: t.color }}>{t.label}</span>
                  </label>
                ))}
              </div>
              <div className="dash-config-quick">
                <button className="btn-sm btn-outline" onClick={() => updatePrefs({ visibleCards: ALL_TABLES.map(t => t.key) })}>Todas</button>
                <button className="btn-sm btn-outline" onClick={() => updatePrefs({ visibleCards: DEFAULT_CARDS })}>Solo principales</button>
                <button className="btn-sm btn-outline" onClick={() => updatePrefs({ visibleCards: [] })}>Ninguna</button>
              </div>
            </div>

            <div className="dash-config-section">
              <h4>Grafica de tendencia</h4>
              <div className="dash-config-row">
                <label className="dash-config-toggle">
                  <Toggle checked={prefs.showChart} onChange={() => updatePrefs({ showChart: !prefs.showChart })} />
                  <span>Mostrar grafica</span>
                </label>
                <select value={prefs.chartMetric} onChange={e => updatePrefs({ chartMetric: e.target.value as DashPrefs['chartMetric'] })}>
                  <option value="pending">Pendientes</option>
                  <option value="errors">Errores</option>
                  <option value="total">Total</option>
                </select>
              </div>
              {prefs.showChart && (
                <div className="dash-config-grid">
                  {ALL_TABLES.map(t => (
                    <label key={t.key} className="dash-config-toggle">
                      <Toggle checked={prefs.chartTables.includes(t.key)} onChange={() => toggleChartTable(t.key)} />
                      <span style={{ color: t.color }}>{t.label}</span>
                    </label>
                  ))}
                </div>
              )}
            </div>

            <div className="dash-config-section">
              <h4>Secciones</h4>
              <label className="dash-config-toggle">
                <Toggle checked={prefs.showISAM} onChange={() => updatePrefs({ showISAM: !prefs.showISAM })} />
                <span>Archivos ISAM</span>
              </label>
            </div>
          </div>
        )}

        {/* Alerts */}
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

        {/* Cards */}
        {cards.length > 0 && (
          <div className="stats-row">
            {cards.map(c => {
              const pct = c.total > 0 ? Math.round((c.synced / c.total) * 100) : 0;
              return (
                <div key={c.key} className="stat-card">
                  <div className="label">{c.label}</div>
                  <div className={`value ${c.colorName}`}>{c.total}</div>
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
        )}

        {cards.length === 0 && !showConfig && (
          <EmptyState title="Dashboard vacio" message="Presiona &quot;Personalizar&quot; para elegir que tablas mostrar" style={{ margin: '40px 0' }} />
        )}

        {/* Chart */}
        {prefs.showChart && chartData.length > 0 && chartLines.length > 0 && (
          <>
            <div className="chart-header">
              <h3 className="section-title">Tendencia: {prefs.chartMetric === 'pending' ? 'Pendientes' : prefs.chartMetric === 'errors' ? 'Errores' : 'Total'}</h3>
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
                  {chartLines.map(t => (
                    <Line
                      key={t.key}
                      type="monotone"
                      dataKey={t.key + metricSuffix}
                      name={t.label + ' ' + metricLabel}
                      stroke={t.color}
                      strokeWidth={2}
                      dot={false}
                    />
                  ))}
                </LineChart>
              </ResponsiveContainer>
            </div>
          </>
        )}

        {/* ISAM Files */}
        {prefs.showISAM && (
          <>
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
          </>
        )}
      </div>
    </>
  );
}
