import { useState, useEffect, useCallback } from 'react';
import { api } from '../api';
import Pagination from '../components/Pagination';

interface Props {
  table: string;
  title: string;
  file: string;
}

function fmtDate(d: string) {
  if (!d) return '-';
  const dt = new Date(d);
  if (isNaN(dt.getTime())) return d;
  return dt.toLocaleDateString('es-CO', { year: 'numeric', month: '2-digit', day: '2-digit' })
    + ' ' + dt.toLocaleTimeString('es-CO', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

export default function DataPage({ table, title, file }: Props) {
  const [subTab, setSubTab] = useState<'data' | 'history'>('data');
  const [data, setData] = useState<any[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState('');
  const [searchInput, setSearchInput] = useState('');

  // History state
  const [histRecords, setHistRecords] = useState<any[]>([]);
  const [histTotal, setHistTotal] = useState(0);
  const [histPage, setHistPage] = useState(1);
  const [histSearch, setHistSearch] = useState('');
  const [histSearchInput, setHistSearchInput] = useState('');
  const [histStatus, setHistStatus] = useState('');
  const [histDateFrom, setHistDateFrom] = useState('');
  const [histDateTo, setHistDateTo] = useState('');

  const [detail, setDetail] = useState<any>(null);

  const fetchData = useCallback(async () => {
    const fetchers: Record<string, (p: number, s: string) => Promise<any>> = {
      clients: api.getClients,
      products: api.getProducts,
      movements: api.getMovements,
      cartera: api.getCartera,
    };
    const result = await fetchers[table](page, search);
    setData(result.data || []);
    setTotal(result.total || 0);
  }, [table, page, search]);

  const fetchHistory = useCallback(async () => {
    const result = await api.getSyncHistory(table, histPage, histSearch, histDateFrom, histDateTo, histStatus);
    setHistRecords(result.records || []);
    setHistTotal(result.total || 0);
  }, [table, histPage, histSearch, histDateFrom, histDateTo, histStatus]);

  useEffect(() => {
    if (subTab === 'data') fetchData();
    else fetchHistory();
  }, [subTab, fetchData, fetchHistory]);

  const totalPages = Math.ceil(total / 50) || 1;
  const histTotalPages = Math.ceil(histTotal / 50) || 1;

  const doSearch = () => { setSearch(searchInput); setPage(1); };
  const clearSearch = () => { setSearchInput(''); setSearch(''); setPage(1); };
  const doHistSearch = () => { setHistSearch(histSearchInput); setHistPage(1); };
  const clearHistSearch = () => { setHistSearchInput(''); setHistSearch(''); setHistStatus(''); setHistDateFrom(''); setHistDateTo(''); setHistPage(1); };

  const renderDataTable = () => {
    if (table === 'clients') {
      return (
        <table className="data-table">
          <thead><tr><th>NIT</th><th>Nombre</th><th>Tipo Doc</th><th>Empresa</th><th>Codigo</th><th>Estado</th></tr></thead>
          <tbody>{data.map((r, i) => (
            <tr key={i}>
              <td>{r.nit}</td><td>{r.nombre}</td><td>{r.tipo_doc}</td>
              <td>{r.empresa}</td><td>{r.codigo}</td>
              <td><span className={`status ${r.sync_status}`}>{r.sync_status}</span></td>
            </tr>
          ))}</tbody>
        </table>
      );
    }
    if (table === 'products') {
      return (
        <table className="data-table">
          <thead><tr><th>Codigo</th><th>Nombre</th><th>Grupo</th><th>Cuenta</th><th>Estado</th></tr></thead>
          <tbody>{data.map((r, i) => (
            <tr key={i}>
              <td>{r.code}</td><td>{r.nombre}</td><td>{r.grupo}</td>
              <td>{r.cuenta_contable}</td>
              <td><span className={`status ${r.sync_status}`}>{r.sync_status}</span></td>
            </tr>
          ))}</tbody>
        </table>
      );
    }
    if (table === 'movements') {
      return (
        <table className="data-table">
          <thead><tr><th>Tipo</th><th>Num Doc</th><th>Fecha</th><th>NIT</th><th>Descripcion</th><th>Estado</th></tr></thead>
          <tbody>{data.map((r, i) => (
            <tr key={i}>
              <td>{r.tipo_comprobante}</td><td>{r.numero_doc}</td><td>{r.fecha}</td>
              <td>{r.nit_tercero}</td><td>{r.descripcion}</td>
              <td><span className={`status ${r.sync_status}`}>{r.sync_status}</span></td>
            </tr>
          ))}</tbody>
        </table>
      );
    }
    // cartera
    return (
      <table className="data-table">
        <thead><tr><th>Tipo</th><th>NIT</th><th>Cuenta</th><th>Fecha</th><th>Descripcion</th><th>D/C</th><th>Estado</th></tr></thead>
        <tbody>{data.map((r, i) => (
          <tr key={i}>
            <td>{r.tipo_registro}</td><td>{r.nit_tercero}</td><td>{r.cuenta_contable}</td>
            <td>{r.fecha}</td><td>{r.descripcion}</td><td>{r.tipo_mov}</td>
            <td><span className={`status ${r.sync_status}`}>{r.sync_status}</span></td>
          </tr>
        ))}</tbody>
      </table>
    );
  };

  return (
    <>
      <div className="topbar"><h2>{title}</h2></div>
      <div className="content">
        <div className="subtabs">
          <div className={`subtab ${subTab === 'data' ? 'active' : ''}`} onClick={() => setSubTab('data')}>
            Datos SQLite ({file})
          </div>
          <div className={`subtab ${subTab === 'history' ? 'active' : ''}`} onClick={() => setSubTab('history')}>
            Historial de Envios
          </div>
          {subTab === 'history' && (
            <a className="btn-sm btn-export subtab-export" href={api.exportHistoryURL(table)} target="_blank" rel="noreferrer">Exportar CSV</a>
          )}
        </div>

        {subTab === 'data' ? (
          <>
            <div className="search-box">
              <input
                placeholder="Buscar..."
                value={searchInput}
                onChange={e => setSearchInput(e.target.value)}
                onKeyUp={e => e.key === 'Enter' && doSearch()}
              />
              <button onClick={doSearch}>Buscar</button>
              {search && <button className="btn-clear" onClick={clearSearch}>X</button>}
            </div>
            <p className="result-count">{total} registros{search ? ' encontrados' : ''} - Pagina {page} de {totalPages}</p>
            {data.length === 0 ? (
              <div className="empty-state"><h3>Sin datos</h3><p>No se encontraron registros</p></div>
            ) : renderDataTable()}
            <Pagination page={page} totalPages={totalPages} onPageChange={setPage} />
          </>
        ) : (
          <>
            <div className="search-box">
              <input
                placeholder="Buscar por key, error..."
                value={histSearchInput}
                onChange={e => setHistSearchInput(e.target.value)}
                onKeyUp={e => e.key === 'Enter' && doHistSearch()}
              />
              <select value={histStatus} onChange={e => { setHistStatus(e.target.value); setHistPage(1); }}>
                <option value="">Todos</option>
                <option value="sent">Enviados</option>
                <option value="error">Con Error</option>
              </select>
              <button onClick={doHistSearch}>Buscar</button>
              {(histSearch || histStatus || histDateFrom || histDateTo) && (
                <button className="btn-clear" onClick={clearHistSearch}>X</button>
              )}
            </div>
            <div className="search-box" style={{ marginTop: -4 }}>
              <label className="date-label">Desde</label>
              <input type="date" value={histDateFrom} onChange={e => { setHistDateFrom(e.target.value); setHistPage(1); }} />
              <label className="date-label">Hasta</label>
              <input type="date" value={histDateTo} onChange={e => { setHistDateTo(e.target.value); setHistPage(1); }} />
            </div>
            <p className="result-count">{histTotal} registros - Pagina {histPage} de {histTotalPages}</p>
            {histRecords.length === 0 ? (
              <div className="empty-state"><h3>Sin historial</h3><p>Aun no se han enviado registros</p></div>
            ) : (
              <table className="data-table">
                <thead><tr><th>Key</th><th>Accion</th><th>Estado</th><th>Fecha</th><th>Error</th><th>Acciones</th></tr></thead>
                <tbody>{histRecords.map((r, i) => (
                  <tr key={i}>
                    <td>{r.key}</td>
                    <td>{r.sync_action}</td>
                    <td><span className={`status ${r.sync_status}`}>{r.sync_status}</span></td>
                    <td>{fmtDate(r.updated_at)}</td>
                    <td style={{ maxWidth: 200 }}>{r.sync_error ? (
                      <span className="error-link" onClick={() => setDetail(r)}>{r.sync_error.substring(0, 50)}{r.sync_error.length > 50 ? '...' : ''}</span>
                    ) : '-'}</td>
                    <td><button className="btn-sm btn-resend" onClick={() => setDetail(r)}>Ver</button></td>
                  </tr>
                ))}</tbody>
              </table>
            )}
            <Pagination page={histPage} totalPages={histTotalPages} onPageChange={setHistPage} />
          </>
        )}

        {detail && (
          <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setDetail(null); }}>
            <div className="modal">
              <div className="modal-header">
                <h3>Detalle del Registro</h3>
                <button className="btn-clear" onClick={() => setDetail(null)}>X</button>
              </div>
              <div className="modal-body">
                <div className="detail-row"><span className="label">Tabla:</span><span>{detail.table}</span></div>
                <div className="detail-row"><span className="label">Key:</span><span>{detail.key}</span></div>
                <div className="detail-row"><span className="label">Accion:</span><span>{detail.sync_action}</span></div>
                <div className="detail-row"><span className="label">Estado:</span><span className={`status ${detail.sync_status}`}>{detail.sync_status}</span></div>
                <div className="detail-row"><span className="label">Fecha:</span><span>{fmtDate(detail.updated_at)}</span></div>
                {detail.sync_error && (
                  <div className="detail-section">
                    <span className="label">Error:</span>
                    <div className="detail-error">{detail.sync_error}</div>
                  </div>
                )}
                {detail.data && (
                  <div className="detail-section">
                    <span className="label">Data:</span>
                    <pre className="detail-data">{(() => { try { return JSON.stringify(JSON.parse(detail.data), null, 2); } catch { return detail.data; } })()}</pre>
                  </div>
                )}
              </div>
              <div className="modal-footer">
                <button className="btn-clear" onClick={() => setDetail(null)}>Cerrar</button>
              </div>
            </div>
          </div>
        )}
      </div>
    </>
  );
}
