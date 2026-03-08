import { useState, useEffect } from 'react';
import { api } from '../api';
import { showToast } from '../components/Toast';

interface ErrorGroup {
  table: string;
  error: string;
  count: number;
  max_retries: number;
  last_seen: string;
}

const TABLE_LABELS: Record<string, string> = {
  clients: 'Clientes',
  products: 'Productos',
  movements: 'Movimientos',
  cartera: 'Cartera',
};

function fmtDate(d: string) {
  if (!d) return '-';
  const dt = new Date(d);
  if (isNaN(dt.getTime())) return d;
  return dt.toLocaleDateString('es-CO', { year: 'numeric', month: '2-digit', day: '2-digit' })
    + ' ' + dt.toLocaleTimeString('es-CO', { hour: '2-digit', minute: '2-digit' });
}

export default function ErrorSummary() {
  const [errors, setErrors] = useState<ErrorGroup[]>([]);
  const [filter, setFilter] = useState('');

  const fetchErrors = () => {
    api.getErrorSummary().then((data: ErrorGroup[] | null) => setErrors(data || [])).catch(() => {});
  };

  useEffect(() => { fetchErrors(); }, []);

  const filtered = filter ? errors.filter(e => e.table === filter) : errors;
  const totalErrors = filtered.reduce((sum, e) => sum + e.count, 0);

  const handleRetry = async (table: string) => {
    const r = await api.retryErrors(table);
    showToast(r.status === 'ok' ? 'success' : 'error',
      r.status === 'ok' ? `${r.count} registros de ${TABLE_LABELS[table]} reencolados` : 'Error');
    fetchErrors();
  };

  const tablesWithErrors = [...new Set(errors.map(e => e.table))];

  return (
    <>
      <div className="topbar"><h2>Resumen de Errores</h2></div>
      <div className="content">
        {errors.length === 0 ? (
          <div className="empty-state">
            <h3>Sin errores</h3>
            <p>No hay registros con errores de sincronizacion</p>
          </div>
        ) : (
          <>
            <div className="error-summary-header">
              <span className="error-total">{totalErrors} errores en {filtered.length} grupos</span>
              <div className="error-summary-actions">
                <select value={filter} onChange={e => setFilter(e.target.value)}>
                  <option value="">Todos los modulos</option>
                  {tablesWithErrors.map(t => (
                    <option key={t} value={t}>{TABLE_LABELS[t] || t}</option>
                  ))}
                </select>
                {tablesWithErrors.map(t => (
                  <button key={t} className="btn-sm btn-resend" onClick={() => handleRetry(t)}>
                    Reintentar {TABLE_LABELS[t]}
                  </button>
                ))}
              </div>
            </div>

            <div className="table-wrapper">
            <table className="data-table">
              <thead>
                <tr>
                  <th>Modulo</th>
                  <th>Error</th>
                  <th style={{ width: 80 }}>Registros</th>
                  <th style={{ width: 80 }}>Max Retry</th>
                  <th style={{ width: 140 }}>Ultimo</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((e, i) => (
                  <tr key={i}>
                    <td><span className="badge">{TABLE_LABELS[e.table] || e.table}</span></td>
                    <td className="error-msg-cell">{e.error}</td>
                    <td className="center">{e.count}</td>
                    <td className="center">{e.max_retries}</td>
                    <td>{fmtDate(e.last_seen)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            </div>
          </>
        )}
      </div>
    </>
  );
}
