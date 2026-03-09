import { useState, useEffect } from 'react';
import { api } from '../api';
import { showToast } from '../components/Toast';
import EmptyState from '../components/EmptyState';
import PageHeader from '../components/PageHeader';
import { fmtDateShort as fmtDate } from '../utils/format';

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
  plan_cuentas: 'Plan Cuentas',
  activos_fijos: 'Activos Fijos',
  saldos_terceros: 'Saldos Terceros',
  saldos_consolidados: 'Saldos Consolidados',
  documentos: 'Documentos',
  terceros_ampliados: 'Terceros Ampliados',
  transacciones_detalle: 'Trans. Detalle',
  periodos_contables: 'Periodos',
  condiciones_pago: 'Cond. Pago',
  libros_auxiliares: 'Libros Auxiliares',
  codigos_dane: 'DANE',
  actividades_ica: 'ICA',
  conceptos_pila: 'PILA',
  activos_fijos_detalle: 'Activos Detalle',
  audit_trail_terceros: 'Audit Trail',
  clasificacion_cuentas: 'Clasif. Cuentas',
};

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
      <PageHeader title="Resumen de Errores" />
      <div className="content">
        {errors.length === 0 ? (
          <EmptyState title="Sin errores" message="No hay registros con errores de sincronizacion" />
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
