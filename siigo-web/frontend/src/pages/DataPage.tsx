import { useState, useEffect, useCallback } from 'react';
import { api } from '../api';
import { showToast } from '../components/Toast';
import Pagination from '../components/Pagination';
import EmptyState from '../components/EmptyState';
import PageHeader from '../components/PageHeader';
import StatusBadge from '../components/StatusBadge';
import Modal from '../components/Modal';
import TabBar from '../components/TabBar';
import SearchBox from '../components/SearchBox';
import { fmtDate } from '../utils/format';
import { TABLE_CONFIGS } from '../config/tables';

interface Props {
  table: string;
  title: string;
  file: string;
}

export default function DataPage({ table, title, file }: Props) {
  const cfg = TABLE_CONFIGS[table];
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

  // Edit/Delete state
  const [allowEditDelete, setAllowEditDelete] = useState(false);
  const [editRecord, setEditRecord] = useState<any>(null);
  const [editFields, setEditFields] = useState<Record<string, string>>({});
  const [deleteConfirm, setDeleteConfirm] = useState<any>(null);

  // Bulk selection
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [bulkLoading, setBulkLoading] = useState(false);

  // Column sorting
  const [sortCol, setSortCol] = useState('');
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc');

  // Change history modal
  const [changeHistory, setChangeHistory] = useState<any[] | null>(null);
  const [changeHistoryKey, setChangeHistoryKey] = useState('');

  useEffect(() => {
    api.getAllowEditDelete().then(r => setAllowEditDelete(r.enabled === true)).catch(() => {});
  }, []);

  const fetchData = useCallback(async () => {
    const path = cfg?.apiPath || table;
    const result = await api.getTableData(path, page, search);
    setData(result.data || []);
    setTotal(result.total || 0);
  }, [table, page, search, cfg]);

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

  const editable = cfg?.editableFields || [];

  const openEdit = (record: any) => {
    setEditRecord(record);
    const fields: Record<string, string> = {};
    for (const f of editable) {
      fields[f] = record[f] ?? '';
    }
    setEditFields(fields);
  };

  const handleSaveEdit = async () => {
    if (!editRecord) return;
    const r = await api.updateRecord(table, editRecord.id, editFields);
    if (r.status === 'ok') {
      showToast('success', 'Registro actualizado');
      setEditRecord(null);
      fetchData();
    } else {
      showToast('error', r.error || 'Error al actualizar');
    }
  };

  const handleDelete = async () => {
    if (!deleteConfirm) return;
    const r = await api.deleteRecord(table, deleteConfirm.id);
    if (r.status === 'ok') {
      showToast('success', 'Registro eliminado');
      setDeleteConfirm(null);
      fetchData();
    } else {
      showToast('error', r.error || 'Error al eliminar');
    }
  };

  const getRecordLabel = (r: any) => {
    return r.nit || r.code || r.codigo || r.codigo_cuenta || r.record_key || r.cuenta_contable || r.numero_doc || `ID ${r.id}`;
  };

  const getRecordKey = (r: any) => {
    return r.record_key || r.nit || r.code || r.codigo || r.codigo_cuenta || r.cuenta_contable || r.numero_doc || '';
  };

  const toggleSelect = (id: number) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };

  const toggleSelectAll = () => {
    if (selected.size === data.length) {
      setSelected(new Set());
    } else {
      setSelected(new Set(data.map(r => r.id)));
    }
  };

  const handleBulk = async (action: string) => {
    if (selected.size === 0) return;
    const label = action === 'delete' ? 'eliminar' : action === 'retry' ? 'reintentar' : 'resetear';
    if (!confirm(`${label} ${selected.size} registros seleccionados?`)) return;
    setBulkLoading(true);
    try {
      const r = await api.bulkAction(table, Array.from(selected), action);
      showToast(r.status === 'ok' ? 'success' : 'error', r.status === 'ok' ? `${r.affected || selected.size} registros actualizados` : (r.error || 'Error'));
      setSelected(new Set());
      fetchData();
    } catch { showToast('error', 'Error en operacion masiva'); }
    setBulkLoading(false);
  };

  const openChangeHistory = async (r: any) => {
    const key = getRecordKey(r);
    if (!key) return;
    setChangeHistoryKey(key);
    try {
      const res = await api.getChangeHistory(table, key);
      setChangeHistory(res.entries || []);
    } catch {
      setChangeHistory([]);
    }
  };

  const handleSort = (col: string) => {
    if (sortCol === col) {
      setSortDir(d => d === 'asc' ? 'desc' : 'asc');
    } else {
      setSortCol(col);
      setSortDir('asc');
    }
  };

  const sortedData = sortCol ? [...data].sort((a, b) => {
    const va = a[sortCol] ?? '';
    const vb = b[sortCol] ?? '';
    const cmp = typeof va === 'number' && typeof vb === 'number' ? va - vb : String(va).localeCompare(String(vb));
    return sortDir === 'asc' ? cmp : -cmp;
  }) : data;

  const sortTh = (label: string, col: string) => (
    <th className="sortable-th" onClick={() => handleSort(col)} style={{ cursor: 'pointer', userSelect: 'none' }}>
      {label} {sortCol === col ? (sortDir === 'asc' ? '▲' : '▼') : ''}
    </th>
  );

  // Format a cell value based on column type
  const fmtCell = (value: any, col: { type: string; format?: string; bold?: boolean }) => {
    if (value == null || value === '') return col.type === 'name' || col.type === 'desc' ? '-' : '';
    if (col.type === 'bool') return value ? 'Si' : 'No';
    if (col.format === 'money' && typeof value === 'number') {
      return value.toLocaleString('es-CO', { minimumFractionDigits: 2 });
    }
    if (col.format === 'percent' && typeof value === 'number') {
      return value.toLocaleString('es-CO', { minimumFractionDigits: 3, maximumFractionDigits: 3 }) + '%';
    }
    return String(value);
  };

  const renderDataTable = () => {
    if (!cfg) return <EmptyState title="Tabla no configurada" message={`No hay configuracion para "${table}"`} />;

    return (
      <table className="data-table">
        <thead>
          <tr>
            <th style={{ width: 32 }}>
              <input type="checkbox" checked={data.length > 0 && selected.size === data.length} onChange={toggleSelectAll} />
            </th>
            {cfg.columns.map(c => sortTh(c.label, c.field))}
            {sortTh('Estado', 'sync_status')}
            <th>Acciones</th>
          </tr>
        </thead>
        <tbody>
          {sortedData.map((r, i) => (
            <tr key={i}>
              <td style={{ width: 32 }}>
                <input type="checkbox" checked={selected.has(r.id)} onChange={() => toggleSelect(r.id)} />
              </td>
              {cfg.columns.map(c => (
                <td key={c.field} className={`col-${c.type === 'bool' ? 'type' : c.type}`} style={c.bold ? { fontWeight: 600 } : undefined}>
                  {fmtCell(r[c.field], c)}
                </td>
              ))}
              <td><StatusBadge status={r.sync_status} /></td>
              <td>
                <button className="btn-sm btn-resend" onClick={() => openChangeHistory(r)} title="Historial" style={{ marginRight: 4 }}>Hist</button>
                {allowEditDelete && <>
                  <button className="btn-sm btn-edit" onClick={() => openEdit(r)} title="Editar">Editar</button>
                  <button className="btn-sm btn-danger-sm" onClick={() => setDeleteConfirm(r)} title="Eliminar" style={{ marginLeft: 4 }}>Eliminar</button>
                </>}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    );
  };

  return (
    <>
      <PageHeader title={title} />
      <div className="content">
        <TabBar
          tabs={[{ key: 'data', label: `Datos SQLite (${file})` }, { key: 'history', label: 'Historial de Envios' }]}
          activeTab={subTab}
          onTabChange={t => setSubTab(t as 'data' | 'history')}
          variant="sub"
        >
          {subTab === 'history' && (
            <a className="btn-sm btn-export subtab-export" href={api.exportHistoryURL(table)} target="_blank" rel="noreferrer">Exportar CSV</a>
          )}
        </TabBar>

        {subTab === 'data' ? (
          <>
            <SearchBox value={searchInput} onChange={setSearchInput} onSearch={doSearch} onClear={clearSearch} showClear={!!search} />
            <p className="result-count">{total} registros{search ? ' encontrados' : ''} - Pagina {page} de {totalPages}</p>
            {data.length === 0 ? (
              <EmptyState title="Sin datos" message="No se encontraron registros" />
            ) : <div className="table-wrapper">{renderDataTable()}</div>}
            <Pagination page={page} totalPages={totalPages} onPageChange={setPage} />
          </>
        ) : (
          <>
            <SearchBox value={histSearchInput} onChange={setHistSearchInput} onSearch={doHistSearch} onClear={clearHistSearch}
              placeholder="Buscar por key, error..." showClear={!!(histSearch || histStatus || histDateFrom || histDateTo)}>
              <select value={histStatus} onChange={e => { setHistStatus(e.target.value); setHistPage(1); }}>
                <option value="">Todos</option>
                <option value="sent">Enviados</option>
                <option value="error">Con Error</option>
              </select>
            </SearchBox>
            <div className="search-box" style={{ marginTop: -4 }}>
              <label className="date-label">Desde</label>
              <input type="date" value={histDateFrom} onChange={e => { setHistDateFrom(e.target.value); setHistPage(1); }} />
              <label className="date-label">Hasta</label>
              <input type="date" value={histDateTo} onChange={e => { setHistDateTo(e.target.value); setHistPage(1); }} />
            </div>
            <p className="result-count">{histTotal} registros - Pagina {histPage} de {histTotalPages}</p>
            {histRecords.length === 0 ? (
              <EmptyState title="Sin historial" message="Aun no se han enviado registros" />
            ) : (
              <div className="table-wrapper"><table className="data-table">
                <thead><tr><th>Key</th><th>Accion</th><th>Estado</th><th>Fecha</th><th>Error</th><th>Acciones</th></tr></thead>
                <tbody>{histRecords.map((r, i) => (
                  <tr key={i}>
                    <td className="col-key">{r.key}</td>
                    <td className="col-type">{r.sync_action}</td>
                    <td><StatusBadge status={r.sync_status} /></td>
                    <td className="col-date">{fmtDate(r.updated_at)}</td>
                    <td style={{ maxWidth: 200 }}>{r.sync_error ? (
                      <span className="error-link" onClick={() => setDetail(r)}>{r.sync_error.substring(0, 50)}{r.sync_error.length > 50 ? '...' : ''}</span>
                    ) : '-'}</td>
                    <td><button className="btn-sm btn-resend" onClick={() => setDetail(r)}>Ver</button></td>
                  </tr>
                ))}</tbody>
              </table></div>
            )}
            <Pagination page={histPage} totalPages={histTotalPages} onPageChange={setHistPage} />
          </>
        )}

        {/* Bulk action bar */}
        {selected.size > 0 && (
          <div className="bulk-bar">
            <span>{selected.size} seleccionados</span>
            <div className="bulk-actions">
              <button className="btn-bulk retry" onClick={() => handleBulk('retry')} disabled={bulkLoading}>Reintentar</button>
              <button className="btn-bulk" onClick={() => handleBulk('reset')} disabled={bulkLoading}>Reset a Pending</button>
              {allowEditDelete && (
                <button className="btn-bulk delete" onClick={() => handleBulk('delete')} disabled={bulkLoading}>Eliminar</button>
              )}
              <button className="btn-bulk" onClick={() => setSelected(new Set())} disabled={bulkLoading}>Deseleccionar</button>
            </div>
          </div>
        )}

        {/* Change history modal */}
        {changeHistory !== null && (
          <Modal title={`Historial de Cambios — ${changeHistoryKey}`} onClose={() => setChangeHistory(null)} maxWidth={700}
            bodyStyle={{ maxHeight: 500, overflowY: 'auto' }}
            footer={<button className="btn-clear" onClick={() => setChangeHistory(null)}>Cerrar</button>}>
            {changeHistory.length === 0 ? (
              <p style={{ color: '#94a3b8' }}>No hay cambios registrados para este registro.</p>
            ) : changeHistory.map((ch, i) => (
              <div key={i} className="change-diff">
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
                  <span style={{ color: '#94a3b8', fontSize: 12 }}>{fmtDate(ch.changed_at)}</span>
                  <span className={`status ${ch.action}`}>{ch.action}</span>
                </div>
                {ch.changes && (() => {
                  try {
                    const changes = typeof ch.changes === 'string' ? JSON.parse(ch.changes) : ch.changes;
                    return Object.entries(changes).map(([field, vals]: [string, any]) => (
                      <div key={field} className="change-field">
                        <span className="change-label">{field}</span>
                        <span className="change-old">{vals.old ?? '-'}</span>
                        <span className="change-arrow">&rarr;</span>
                        <span className="change-new">{vals.new ?? '-'}</span>
                      </div>
                    ));
                  } catch { return <pre style={{ fontSize: 11, color: '#94a3b8' }}>{ch.changes}</pre>; }
                })()}
              </div>
            ))}
          </Modal>
        )}

        {/* Detail modal (history) */}
        {detail && (
          <Modal title="Detalle del Registro" onClose={() => setDetail(null)}
            footer={<button className="btn-clear" onClick={() => setDetail(null)}>Cerrar</button>}>
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
          </Modal>
        )}

        {/* Edit modal */}
        {editRecord && (
          <Modal title="Editar Registro" onClose={() => setEditRecord(null)}
            footer={<><button className="btn-clear" onClick={() => setEditRecord(null)}>Cancelar</button><button className="btn-save" onClick={handleSaveEdit}>Guardar Cambios</button></>}>
            <p style={{ color: '#94a3b8', fontSize: 13, marginBottom: 12 }}>ID: {editRecord.id} | {getRecordLabel(editRecord)}</p>
            {Object.keys(editFields).map(field => (
              <div className="form-group" key={field} style={{ marginBottom: 10 }}>
                <label style={{ fontSize: 12, color: '#cbd5e1', marginBottom: 2, display: 'block' }}>{field}</label>
                <input
                  value={editFields[field]}
                  onChange={e => setEditFields({ ...editFields, [field]: e.target.value })}
                  style={{ width: '100%', padding: '6px 10px', background: '#0f172a', border: '1px solid #334155', borderRadius: 4, color: '#e2e8f0', fontSize: 13 }}
                />
              </div>
            ))}
          </Modal>
        )}

        {/* Delete confirmation modal */}
        {deleteConfirm && (
          <Modal title="Confirmar Eliminacion" onClose={() => setDeleteConfirm(null)}
            footer={<><button className="btn-clear" onClick={() => setDeleteConfirm(null)}>Cancelar</button><button className="btn-danger" onClick={handleDelete}>Eliminar</button></>}>
            <p style={{ color: '#f87171', marginBottom: 8 }}>Esta accion no se puede deshacer.</p>
            <p style={{ color: '#e2e8f0' }}>Eliminar el registro <strong>{getRecordLabel(deleteConfirm)}</strong> de <strong>{table}</strong>?</p>
          </Modal>
        )}
      </div>
    </>
  );
}
