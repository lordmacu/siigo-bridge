import { useState, useEffect, useCallback } from 'react';
import { api } from '../api';
import { showToast } from '../components/Toast';
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

const EDITABLE_FIELDS: Record<string, string[]> = {
  clients: ['nit', 'nombre', 'tipo_doc', 'tipo_clave', 'empresa', 'codigo', 'fecha_creacion', 'tipo_cta_pref'],
  products: ['code', 'nombre', 'grupo', 'cuenta_contable', 'fecha', 'tipo_mov'],
  movements: ['tipo_comprobante', 'numero_doc', 'fecha', 'nit_tercero', 'cuenta_contable', 'descripcion', 'valor', 'tipo_mov'],
  cartera: ['tipo_registro', 'nit_tercero', 'cuenta_contable', 'fecha', 'descripcion', 'tipo_mov'],
  plan_cuentas: ['codigo_cuenta', 'nombre', 'empresa', 'naturaleza'],
  activos_fijos: ['codigo', 'nombre', 'empresa', 'nit_responsable', 'fecha_adquisicion'],
  saldos_terceros: ['cuenta_contable', 'nit_tercero'],
  saldos_consolidados: ['cuenta_contable'],
  documentos: ['tipo_comprobante', 'nit_tercero', 'cuenta_contable', 'producto_ref', 'fecha', 'descripcion', 'tipo_mov'],
  terceros_ampliados: ['nit', 'nombre', 'tipo_persona', 'representante_legal', 'direccion', 'email'],
};

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
    const fetchers: Record<string, (p: number, s: string) => Promise<any>> = {
      clients: api.getClients,
      products: api.getProducts,
      movements: api.getMovements,
      cartera: api.getCartera,
      plan_cuentas: api.getPlanCuentas,
      activos_fijos: api.getActivosFijos,
      saldos_terceros: api.getSaldosTerceros,
      saldos_consolidados: api.getSaldosConsolidados,
      documentos: api.getDocumentos,
      terceros_ampliados: api.getTercerosAmpliados,
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

  const openEdit = (record: any) => {
    setEditRecord(record);
    const fields: Record<string, string> = {};
    const editable = EDITABLE_FIELDS[table] || [];
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

  const renderCheckbox = (r: any) => (
    <td style={{ width: 32 }}>
      <input type="checkbox" checked={selected.has(r.id)} onChange={() => toggleSelect(r.id)} />
    </td>
  );

  const renderCheckboxHeader = () => (
    <th style={{ width: 32 }}>
      <input type="checkbox" checked={data.length > 0 && selected.size === data.length} onChange={toggleSelectAll} />
    </th>
  );

  const renderActionButtons = (r: any) => (
    <td>
      <button className="btn-sm btn-resend" onClick={() => openChangeHistory(r)} title="Historial" style={{ marginRight: 4 }}>Hist</button>
      {allowEditDelete && <>
        <button className="btn-sm btn-edit" onClick={() => openEdit(r)} title="Editar">Editar</button>
        <button className="btn-sm btn-danger-sm" onClick={() => setDeleteConfirm(r)} title="Eliminar" style={{ marginLeft: 4 }}>Eliminar</button>
      </>}
    </td>
  );

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

  const renderDataTable = () => {
    if (table === 'clients') {
      return (
        <table className="data-table">
          <thead><tr>{renderCheckboxHeader()}{sortTh('NIT','nit')}{sortTh('Nombre','nombre')}{sortTh('Tipo Doc','tipo_doc')}{sortTh('Empresa','empresa')}{sortTh('Codigo','codigo')}{sortTh('Estado','sync_status')}<th>Acciones</th></tr></thead>
          <tbody>{sortedData.map((r, i) => (
            <tr key={i}>
              {renderCheckbox(r)}
              <td className="col-key">{r.nit}</td>
              <td className="col-name">{r.nombre}</td>
              <td className="col-type">{r.tipo_doc}</td>
              <td className="col-code">{r.empresa}</td>
              <td className="col-code">{r.codigo}</td>
              <td><span className={`status ${r.sync_status}`}>{r.sync_status}</span></td>
              {renderActionButtons(r)}
            </tr>
          ))}</tbody>
        </table>
      );
    }
    if (table === 'products') {
      return (
        <table className="data-table">
          <thead><tr>{renderCheckboxHeader()}{sortTh('Codigo','code')}{sortTh('Nombre','nombre')}{sortTh('Grupo','grupo')}{sortTh('Cuenta','cuenta_contable')}{sortTh('Estado','sync_status')}<th>Acciones</th></tr></thead>
          <tbody>{sortedData.map((r, i) => (
            <tr key={i}>
              {renderCheckbox(r)}
              <td className="col-key">{r.code}</td>
              <td className="col-name">{r.nombre}</td>
              <td className="col-type">{r.grupo}</td>
              <td className="col-code">{r.cuenta_contable}</td>
              <td><span className={`status ${r.sync_status}`}>{r.sync_status}</span></td>
              {renderActionButtons(r)}
            </tr>
          ))}</tbody>
        </table>
      );
    }
    if (table === 'movements') {
      return (
        <table className="data-table">
          <thead><tr>{renderCheckboxHeader()}{sortTh('Tipo','tipo_comprobante')}{sortTh('Num Doc','numero_doc')}{sortTh('Fecha','fecha')}{sortTh('NIT','nit_tercero')}{sortTh('Descripcion','descripcion')}{sortTh('Estado','sync_status')}<th>Acciones</th></tr></thead>
          <tbody>{sortedData.map((r, i) => (
            <tr key={i}>
              {renderCheckbox(r)}
              <td className="col-type">{r.tipo_comprobante}</td>
              <td className="col-key">{r.numero_doc}</td>
              <td className="col-date">{r.fecha}</td>
              <td className="col-code">{r.nit_tercero}</td>
              <td className="col-desc">{r.descripcion}</td>
              <td><span className={`status ${r.sync_status}`}>{r.sync_status}</span></td>
              {renderActionButtons(r)}
            </tr>
          ))}</tbody>
        </table>
      );
    }
    if (table === 'cartera') {
      return (
        <table className="data-table">
          <thead><tr>{renderCheckboxHeader()}{sortTh('Tipo','tipo_registro')}{sortTh('NIT','nit_tercero')}{sortTh('Cuenta','cuenta_contable')}{sortTh('Fecha','fecha')}{sortTh('Descripcion','descripcion')}{sortTh('D/C','tipo_mov')}{sortTh('Estado','sync_status')}<th>Acciones</th></tr></thead>
          <tbody>{sortedData.map((r, i) => (
            <tr key={i}>
              {renderCheckbox(r)}
              <td className="col-type">{r.tipo_registro}</td>
              <td className="col-key">{r.nit_tercero}</td>
              <td className="col-code">{r.cuenta_contable}</td>
              <td className="col-date">{r.fecha}</td>
              <td className="col-desc">{r.descripcion}</td>
              <td className="col-value">{r.tipo_mov}</td>
              <td><span className={`status ${r.sync_status}`}>{r.sync_status}</span></td>
              {renderActionButtons(r)}
            </tr>
          ))}</tbody>
        </table>
      );
    }
    if (table === 'plan_cuentas') {
      return (
        <table className="data-table">
          <thead><tr>{renderCheckboxHeader()}{sortTh('Codigo','codigo_cuenta')}{sortTh('Nombre','nombre')}{sortTh('Empresa','empresa')}{sortTh('Activa','activa')}{sortTh('Auxiliar','auxiliar')}{sortTh('Naturaleza','naturaleza')}{sortTh('Estado','sync_status')}<th>Acciones</th></tr></thead>
          <tbody>{sortedData.map((r, i) => (
            <tr key={i}>
              {renderCheckbox(r)}
              <td className="col-key">{r.codigo_cuenta}</td>
              <td className="col-name">{r.nombre}</td>
              <td className="col-code">{r.empresa}</td>
              <td className="col-type">{r.activa ? 'Si' : 'No'}</td>
              <td className="col-type">{r.auxiliar ? 'Si' : 'No'}</td>
              <td className="col-desc">{r.naturaleza}</td>
              <td><span className={`status ${r.sync_status}`}>{r.sync_status}</span></td>
              {renderActionButtons(r)}
            </tr>
          ))}</tbody>
        </table>
      );
    }
    if (table === 'activos_fijos') {
      return (
        <table className="data-table">
          <thead><tr>{renderCheckboxHeader()}{sortTh('Codigo','codigo')}{sortTh('Nombre','nombre')}{sortTh('Empresa','empresa')}{sortTh('NIT Responsable','nit_responsable')}{sortTh('Fecha Adquisicion','fecha_adquisicion')}{sortTh('Estado','sync_status')}<th>Acciones</th></tr></thead>
          <tbody>{sortedData.map((r, i) => (
            <tr key={i}>
              {renderCheckbox(r)}
              <td className="col-key">{r.codigo}</td>
              <td className="col-name">{r.nombre}</td>
              <td className="col-code">{r.empresa}</td>
              <td className="col-code">{r.nit_responsable}</td>
              <td className="col-date">{r.fecha_adquisicion}</td>
              <td><span className={`status ${r.sync_status}`}>{r.sync_status}</span></td>
              {renderActionButtons(r)}
            </tr>
          ))}</tbody>
        </table>
      );
    }
    if (table === 'saldos_terceros') {
      return (
        <table className="data-table">
          <thead><tr>{renderCheckboxHeader()}{sortTh('Cuenta','cuenta_contable')}{sortTh('NIT','nit_tercero')}{sortTh('Empresa','empresa')}{sortTh('Saldo Ant.','saldo_anterior')}{sortTh('Debito','debito')}{sortTh('Credito','credito')}{sortTh('Saldo Final','saldo_final')}{sortTh('Estado','sync_status')}<th>Acciones</th></tr></thead>
          <tbody>{sortedData.map((r, i) => (
            <tr key={i}>
              {renderCheckbox(r)}
              <td className="col-code">{r.cuenta_contable}</td>
              <td className="col-key">{r.nit_tercero}</td>
              <td className="col-code">{r.empresa}</td>
              <td className="col-value">{r.saldo_anterior?.toLocaleString('es-CO', { minimumFractionDigits: 2 })}</td>
              <td className="col-value">{r.debito?.toLocaleString('es-CO', { minimumFractionDigits: 2 })}</td>
              <td className="col-value">{r.credito?.toLocaleString('es-CO', { minimumFractionDigits: 2 })}</td>
              <td className="col-value" style={{ fontWeight: 600 }}>{r.saldo_final?.toLocaleString('es-CO', { minimumFractionDigits: 2 })}</td>
              <td><span className={`status ${r.sync_status}`}>{r.sync_status}</span></td>
              {renderActionButtons(r)}
            </tr>
          ))}</tbody>
        </table>
      );
    }
    if (table === 'saldos_consolidados') {
      return (
        <table className="data-table">
          <thead><tr>{renderCheckboxHeader()}{sortTh('Cuenta','cuenta_contable')}{sortTh('Empresa','empresa')}{sortTh('Saldo Ant.','saldo_anterior')}{sortTh('Debito','debito')}{sortTh('Credito','credito')}{sortTh('Saldo Final','saldo_final')}{sortTh('Estado','sync_status')}<th>Acciones</th></tr></thead>
          <tbody>{sortedData.map((r, i) => (
            <tr key={i}>
              {renderCheckbox(r)}
              <td className="col-key">{r.cuenta_contable}</td>
              <td className="col-code">{r.empresa}</td>
              <td className="col-value">{r.saldo_anterior?.toLocaleString('es-CO', { minimumFractionDigits: 2 })}</td>
              <td className="col-value">{r.debito?.toLocaleString('es-CO', { minimumFractionDigits: 2 })}</td>
              <td className="col-value">{r.credito?.toLocaleString('es-CO', { minimumFractionDigits: 2 })}</td>
              <td className="col-value" style={{ fontWeight: 600 }}>{r.saldo_final?.toLocaleString('es-CO', { minimumFractionDigits: 2 })}</td>
              <td><span className={`status ${r.sync_status}`}>{r.sync_status}</span></td>
              {renderActionButtons(r)}
            </tr>
          ))}</tbody>
        </table>
      );
    }
    if (table === 'documentos') {
      return (
        <table className="data-table">
          <thead><tr>{renderCheckboxHeader()}{sortTh('Tipo','tipo_comprobante')}{sortTh('Cod','codigo_comp')}{sortTh('Seq','secuencia')}{sortTh('NIT','nit_tercero')}{sortTh('Cuenta','cuenta_contable')}{sortTh('Producto','producto_ref')}{sortTh('Fecha','fecha')}{sortTh('Descripcion','descripcion')}{sortTh('D/C','tipo_mov')}{sortTh('Estado','sync_status')}<th>Acciones</th></tr></thead>
          <tbody>{sortedData.map((r, i) => (
            <tr key={i}>
              {renderCheckbox(r)}
              <td className="col-type">{r.tipo_comprobante}</td>
              <td className="col-code">{r.codigo_comp}</td>
              <td className="col-code">{r.secuencia}</td>
              <td className="col-key">{r.nit_tercero}</td>
              <td className="col-code">{r.cuenta_contable}</td>
              <td className="col-code">{r.producto_ref}</td>
              <td className="col-date">{r.fecha}</td>
              <td className="col-desc">{r.descripcion}</td>
              <td className="col-value">{r.tipo_mov}</td>
              <td><span className={`status ${r.sync_status}`}>{r.sync_status}</span></td>
              {renderActionButtons(r)}
            </tr>
          ))}</tbody>
        </table>
      );
    }
    // terceros_ampliados (default)
    return (
      <table className="data-table">
        <thead><tr>{renderCheckboxHeader()}{sortTh('NIT','nit')}{sortTh('Nombre','nombre')}{sortTh('Tipo','tipo_persona')}{sortTh('Empresa','empresa')}{sortTh('Rep. Legal','representante_legal')}{sortTh('Direccion','direccion')}{sortTh('Email','email')}{sortTh('Estado','sync_status')}<th>Acciones</th></tr></thead>
        <tbody>{sortedData.map((r, i) => (
          <tr key={i}>
            {renderCheckbox(r)}
            <td className="col-key">{r.nit}</td>
            <td className="col-name">{r.nombre}</td>
            <td className="col-type">{r.tipo_persona}</td>
            <td className="col-code">{r.empresa}</td>
            <td className="col-name">{r.representante_legal || '-'}</td>
            <td className="col-desc">{r.direccion || '-'}</td>
            <td className="col-desc">{r.email || '-'}</td>
            <td><span className={`status ${r.sync_status}`}>{r.sync_status}</span></td>
            {renderActionButtons(r)}
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
            ) : <div className="table-wrapper">{renderDataTable()}</div>}
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
              <div className="table-wrapper"><table className="data-table">
                <thead><tr><th>Key</th><th>Accion</th><th>Estado</th><th>Fecha</th><th>Error</th><th>Acciones</th></tr></thead>
                <tbody>{histRecords.map((r, i) => (
                  <tr key={i}>
                    <td className="col-key">{r.key}</td>
                    <td className="col-type">{r.sync_action}</td>
                    <td><span className={`status ${r.sync_status}`}>{r.sync_status}</span></td>
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
          <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setChangeHistory(null); }}>
            <div className="modal" style={{ maxWidth: 700 }}>
              <div className="modal-header">
                <h3>Historial de Cambios — {changeHistoryKey}</h3>
                <button className="btn-clear" onClick={() => setChangeHistory(null)}>X</button>
              </div>
              <div className="modal-body" style={{ maxHeight: 500, overflowY: 'auto' }}>
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
              </div>
              <div className="modal-footer">
                <button className="btn-clear" onClick={() => setChangeHistory(null)}>Cerrar</button>
              </div>
            </div>
          </div>
        )}

        {/* Detail modal (history) */}
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

        {/* Edit modal */}
        {editRecord && (
          <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setEditRecord(null); }}>
            <div className="modal">
              <div className="modal-header">
                <h3>Editar Registro</h3>
                <button className="btn-clear" onClick={() => setEditRecord(null)}>X</button>
              </div>
              <div className="modal-body">
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
              </div>
              <div className="modal-footer">
                <button className="btn-clear" onClick={() => setEditRecord(null)}>Cancelar</button>
                <button className="btn-save" onClick={handleSaveEdit}>Guardar Cambios</button>
              </div>
            </div>
          </div>
        )}

        {/* Delete confirmation modal */}
        {deleteConfirm && (
          <div className="modal-overlay" onClick={e => { if (e.target === e.currentTarget) setDeleteConfirm(null); }}>
            <div className="modal">
              <div className="modal-header">
                <h3>Confirmar Eliminacion</h3>
                <button className="btn-clear" onClick={() => setDeleteConfirm(null)}>X</button>
              </div>
              <div className="modal-body">
                <p style={{ color: '#f87171', marginBottom: 8 }}>Esta accion no se puede deshacer.</p>
                <p style={{ color: '#e2e8f0' }}>Eliminar el registro <strong>{getRecordLabel(deleteConfirm)}</strong> de <strong>{table}</strong>?</p>
              </div>
              <div className="modal-footer">
                <button className="btn-clear" onClick={() => setDeleteConfirm(null)}>Cancelar</button>
                <button className="btn-danger" onClick={handleDelete}>Eliminar</button>
              </div>
            </div>
          </div>
        )}
      </div>
    </>
  );
}
