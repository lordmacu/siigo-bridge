import { useState, useEffect, useCallback } from 'react';
import { api } from '../api';
import Pagination from '../components/Pagination';
import { showToast } from '../components/Toast';

interface LogEntry {
  id: number;
  level: string;
  source: string;
  message: string;
  created_at: string;
}

function fmtDate(d: string) {
  if (!d) return '-';
  const dt = new Date(d);
  if (isNaN(dt.getTime())) return d;
  return dt.toLocaleDateString('es-CO', { year: 'numeric', month: '2-digit', day: '2-digit' })
    + ' ' + dt.toLocaleTimeString('es-CO', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

export default function Logs() {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);

  const fetchLogs = useCallback(async () => {
    const result = await api.getLogs(page);
    setLogs(result.logs || []);
    setTotal(result.total || 0);
  }, [page]);

  useEffect(() => { fetchLogs(); }, [fetchLogs]);

  const totalPages = Math.ceil(total / 100) || 1;

  const handleClear = async () => {
    if (!confirm('Seguro que quieres limpiar todos los logs?')) return;
    const r = await api.clearLogs();
    showToast(r.status === 'ok' ? 'success' : 'error', r.status === 'ok' ? 'Logs limpiados' : 'Error');
    setPage(1);
    fetchLogs();
  };

  return (
    <>
      <div className="topbar">
        <h2>Registro de Actividad</h2>
        <button className="btn-sm btn-resend" onClick={handleClear}>Limpiar Logs</button>
      </div>
      <div className="content">
        <p className="result-count">{total} entradas - Pagina {page} de {totalPages}</p>
        {logs.length === 0 ? (
          <div className="empty-state"><h3>Sin logs</h3></div>
        ) : (
          <div>
            {logs.map(l => (
              <div key={l.id} className={`log-entry ${l.level}`}>
                <span className="time">{fmtDate(l.created_at)}</span>
                <span className="source">[{l.source}]</span>
                <span className="msg">{l.message}</span>
              </div>
            ))}
          </div>
        )}
        <Pagination page={page} totalPages={totalPages} onPageChange={setPage} />
      </div>
    </>
  );
}
