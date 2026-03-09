import { useState, useEffect, useCallback } from 'react';
import { api } from '../api';
import Pagination from '../components/Pagination';
import { showToast } from '../components/Toast';
import EmptyState from '../components/EmptyState';
import PageHeader from '../components/PageHeader';
import { fmtDate } from '../utils/format';

interface LogEntry {
  id: number;
  level: string;
  source: string;
  message: string;
  created_at: string;
}

export default function Logs() {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [level, setLevel] = useState('');
  const [source, setSource] = useState('');
  const [search, setSearch] = useState('');
  const [searchInput, setSearchInput] = useState('');

  const fetchLogs = useCallback(async () => {
    const result = await api.getLogs(page, level, source, search);
    setLogs(result.logs || []);
    setTotal(result.total || 0);
  }, [page, level, source, search]);

  useEffect(() => { fetchLogs(); }, [fetchLogs]);

  const totalPages = Math.ceil(total / 100) || 1;

  const handleClear = async () => {
    if (!confirm('Seguro que quieres limpiar todos los logs?')) return;
    const r = await api.clearLogs();
    showToast(r.status === 'ok' ? 'success' : 'error', r.status === 'ok' ? 'Logs limpiados' : 'Error');
    setPage(1);
    fetchLogs();
  };

  const handleSearch = () => {
    setSearch(searchInput);
    setPage(1);
  };

  return (
    <>
      <PageHeader title="Registro de Actividad">
          <a className="btn-sm btn-export" href={api.exportLogsURL()} target="_blank" rel="noreferrer">Exportar CSV</a>
          <button className="btn-sm btn-resend" onClick={handleClear}>Limpiar Logs</button>
      </PageHeader>
      <div className="content">
        <div className="logs-filters">
          <select value={level} onChange={e => { setLevel(e.target.value); setPage(1); }}>
            <option value="">Todos los niveles</option>
            <option value="info">Info</option>
            <option value="warn">Warning</option>
            <option value="error">Error</option>
          </select>
          <select value={source} onChange={e => { setSource(e.target.value); setPage(1); }}>
            <option value="">Todas las fuentes</option>
            <option value="APP">APP</option>
            <option value="DETECT">DETECT</option>
            <option value="SEND">SEND</option>
            <option value="CONFIG">CONFIG</option>
            <option value="API">API</option>
            <option value="SYNC">SYNC</option>
          </select>
          <div className="logs-search-row">
            <input
              placeholder="Buscar en mensajes..."
              value={searchInput}
              onChange={e => setSearchInput(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && handleSearch()}
            />
            <button className="btn-sm btn-primary" onClick={handleSearch}>Buscar</button>
            {(search || level || source) && (
              <button className="btn-sm btn-outline" onClick={() => { setLevel(''); setSource(''); setSearch(''); setSearchInput(''); setPage(1); }}>Limpiar filtros</button>
            )}
          </div>
        </div>
        <p className="result-count">{total} entradas - Pagina {page} de {totalPages}</p>
        {logs.length === 0 ? (
          <EmptyState title="Sin logs" />
        ) : (
          <div>
            {logs.map(l => (
              <div key={l.id} className={`log-entry ${l.level}`}>
                <span className="time">{fmtDate(l.created_at)}</span>
                <span className={`log-level-badge ${l.level}`}>{l.level.toUpperCase()}</span>
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
