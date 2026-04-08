import './style.css';

// Wails bindings
import {
    GetConfig, SaveConfig, GetISAMInfo,
    GetClientes, GetProductos, GetMovimientos, RefreshCache,
    TestConnection, SyncNow, IsSyncing,
    GetSentRecords, GetLogs, GetStats, ResendRecord,
    PauseSync, ResumeSync, IsPaused, ClearDatabase, ClearLogs,
    SearchSentRecords, SearchSentRecordsWithDates,
    GetRecordDetail
} from '../wailsjs/go/main/App';

function fmtDate(d) {
    if (!d) return '-';
    const dt = new Date(d);
    if (isNaN(dt)) return d;
    return dt.toLocaleDateString('es-CO', { year:'numeric', month:'2-digit', day:'2-digit' })
        + ' ' + dt.toLocaleTimeString('es-CO', { hour:'2-digit', minute:'2-digit', second:'2-digit' });
}

let currentTab = 'dashboard';
let syncing = false;
let paused = false;

const app = document.getElementById('app');

function render() {
    app.innerHTML = `
    <div class="sidebar">
        <div class="sidebar-header">
            <h1>Siigo Sync</h1>
            <small>Middleware Manager</small>
        </div>
        <div class="nav-items">
            <div class="nav-item ${currentTab === 'dashboard' ? 'active' : ''}" onclick="switchTab('dashboard')">
                <span>Dashboard</span>
            </div>
            <div class="nav-item ${currentTab === 'clients' ? 'active' : ''}" onclick="switchTab('clients')">
                <span>Clientes</span>
                <span class="badge">Z17</span>
            </div>
            <div class="nav-item ${currentTab === 'products' ? 'active' : ''}" onclick="switchTab('products')">
                <span>Productos</span>
                <span class="badge">Z06</span>
            </div>
            <div class="nav-item ${currentTab === 'movements' ? 'active' : ''}" onclick="switchTab('movements')">
                <span>Movimientos</span>
                <span class="badge">Z49</span>
            </div>
            <div class="nav-item ${currentTab === 'logs' ? 'active' : ''}" onclick="switchTab('logs')">
                <span>Logs</span>
            </div>
            <div class="nav-item ${currentTab === 'config' ? 'active' : ''}" onclick="switchTab('config')">
                <span>Configuracion</span>
            </div>
        </div>
        <div class="sidebar-footer">
            <div class="sync-status ${syncing ? 'active' : paused ? 'paused' : 'running'}">
                ${syncing ? 'Sincronizando...' : paused ? 'Pausado' : 'Escuchando'}
            </div>
            <button class="sync-btn ${syncing ? 'syncing' : ''}" onclick="doSync()" ${syncing ? 'disabled' : ''}>
                ${syncing ? 'Sincronizando...' : 'Sincronizar Ahora'}
            </button>
            <button class="pause-btn ${paused ? 'paused' : ''}" onclick="togglePause()">
                ${paused ? 'Reanudar Auto-Sync' : 'Pausar Auto-Sync'}
            </button>
        </div>
    </div>
    <div class="main">
        <div class="topbar">
            <h2>${getTabTitle()}</h2>
            <div id="topbar-actions"></div>
        </div>
        <div class="content" id="content"></div>
    </div>`;

    loadTabContent();
}

function getTabTitle() {
    const titles = {
        dashboard: 'Dashboard',
        clients: 'Clientes (Z17 - Terceros)',
        products: 'Productos (Z06 - Inventario)',
        movements: 'Movimientos (Z49 - Transacciones)',
        logs: 'Registro de Actividad',
        config: 'Configuracion'
    };
    return titles[currentTab] || '';
}

window.switchTab = function(tab) {
    currentTab = tab;
    historyPage = 1;
    logsPage = 1;
    searchQuery = '';
    dataSearchQuery = '';
    dateFrom = '';
    dateTo = '';
    statusFilter = '';
    dataPage = 1;
    render();
};

window.doSync = async function() {
    syncing = true;
    render();
    const result = await SyncNow();
    showToast(result.startsWith('Error') ? 'error' : 'success', result);

    // Poll syncing status
    const poll = setInterval(async () => {
        const s = await IsSyncing();
        if (!s) {
            syncing = false;
            clearInterval(poll);
            render();
        }
    }, 2000);
};

window.resend = async function(id) {
    const result = await ResendRecord(id);
    showToast(result === 'ok' ? 'success' : 'error', result === 'ok' ? 'Reenviado exitosamente' : result);
    closeModal();
    loadTabContent();
};

window.showDetail = async function(id) {
    const r = await GetRecordDetail(id);
    if (!r) return;

    let dataFormatted = r.data;
    try { dataFormatted = JSON.stringify(JSON.parse(r.data), null, 2); } catch(e) {}

    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.id = 'detail-modal';
    modal.onclick = function(e) { if (e.target === modal) closeModal(); };
    modal.innerHTML = `
    <div class="modal">
        <div class="modal-header">
            <h3>Detalle del Registro</h3>
            <button class="btn-clear" onclick="closeModal()">X</button>
        </div>
        <div class="modal-body">
            <div class="detail-row"><span class="label">ID:</span><span>${r.id}</span></div>
            <div class="detail-row"><span class="label">Tabla:</span><span>${r.table}</span></div>
            <div class="detail-row"><span class="label">Archivo:</span><span>${r.source_file}</span></div>
            <div class="detail-row"><span class="label">Key:</span><span>${r.key}</span></div>
            <div class="detail-row"><span class="label">Estado:</span><span class="status ${r.status}">${r.status}</span></div>
            <div class="detail-row"><span class="label">Enviado:</span><span>${fmtDate(r.sent_at)}</span></div>
            <div class="detail-row"><span class="label">Creado:</span><span>${fmtDate(r.created_at)}</span></div>
            ${r.error ? `<div class="detail-section">
                <span class="label">Error completo:</span>
                <div class="detail-error">${r.error}</div>
            </div>` : ''}
            <div class="detail-section">
                <span class="label">Data enviada:</span>
                <pre class="detail-data">${dataFormatted}</pre>
            </div>
        </div>
        <div class="modal-footer">
            ${r.status === 'error' ? `<button class="btn-resend" onclick="resend(${r.id})">Reenviar</button>` : ''}
            <button class="btn-clear" onclick="closeModal()">Cerrar</button>
        </div>
    </div>`;
    document.body.appendChild(modal);
};

window.closeModal = function() {
    const m = document.getElementById('detail-modal');
    if (m) m.remove();
};

async function loadTabContent() {
    const content = document.getElementById('content');
    if (!content) return;

    switch (currentTab) {
        case 'dashboard': await renderDashboard(content); break;
        case 'clients': await renderDataTab(content, 'clients', 'Z17'); break;
        case 'products': await renderDataTab(content, 'products', 'Z06'); break;
        case 'movements': await renderDataTab(content, 'movements', 'Z49'); break;
        case 'logs': await renderLogs(content); break;
        case 'config': await renderConfig(content); break;
    }
}

async function renderDashboard(el) {
    const [stats, isamInfo] = await Promise.all([GetStats(), GetISAMInfo()]);

    el.innerHTML = `
    <div class="stats-row">
        <div class="stat-card">
            <div class="label">Clientes Enviados</div>
            <div class="value green">${stats.clients_sent || 0}</div>
        </div>
        <div class="stat-card">
            <div class="label">Productos Enviados</div>
            <div class="value blue">${stats.products_sent || 0}</div>
        </div>
        <div class="stat-card">
            <div class="label">Movimientos Enviados</div>
            <div class="value yellow">${stats.movements_sent || 0}</div>
        </div>
        <div class="stat-card">
            <div class="label">Errores</div>
            <div class="value red">${stats.errors || 0}</div>
        </div>
    </div>

    <h3 style="margin-bottom: 12px; font-size: 14px; color: #94a3b8;">Archivos ISAM (Siigo)</h3>
    <div class="file-info">
        ${(isamInfo || []).map(f => `
        <div class="file-card">
            <h3>${f.file}</h3>
            <div class="info-row"><span class="label">Registros:</span><span>${f.records >= 0 ? f.records : 'Error'}</span></div>
            <div class="info-row"><span class="label">Tam. registro:</span><span>${f.record_size || '-'} bytes</span></div>
            <div class="info-row"><span class="label">Modificado:</span><span>${f.mod_time || '-'}</span></div>
        </div>`).join('')}
    </div>`;
}

let dataSubTab = 'history'; // 'data' or 'history'
let historyPage = 1;
let logsPage = 1;
let searchQuery = '';
let dataSearchQuery = '';
let dateFrom = '';
let dateTo = '';
let statusFilter = '';
let dataPage = 1;

async function renderDataTab(el, tableName, sourceFile) {
    el.innerHTML = `
    <div class="subtabs">
        <div class="subtab ${dataSubTab === 'data' ? 'active' : ''}" onclick="setSubTab('data')">Datos ISAM (${sourceFile})</div>
        <div class="subtab ${dataSubTab === 'history' ? 'active' : ''}" onclick="setSubTab('history')">Historial de Envios</div>
    </div>
    <div id="subtab-content">Cargando...</div>`;

    window.setSubTab = function(st) {
        dataSubTab = st;
        renderDataTab(el, tableName, sourceFile);
    };

    const subtabContent = document.getElementById('subtab-content');

    if (dataSubTab === 'data') {
        let result;
        if (tableName === 'clients') result = await GetClientes(dataPage, dataSearchQuery);
        else if (tableName === 'products') result = await GetProductos(dataPage, dataSearchQuery);
        else result = await GetMovimientos(dataPage, dataSearchQuery);

        const data = result.data || [];
        const totalPages = Math.ceil(result.total / 50) || 1;

        if (data.length === 0 && dataPage === 1 && !dataSearchQuery) {
            subtabContent.innerHTML = '<div class="empty-state"><h3>Sin datos</h3><p>No se encontraron registros en el archivo ISAM</p></div>';
            return;
        }

        window.doDataSearch = function() {
            dataSearchQuery = document.getElementById('data-search').value;
            dataPage = 1;
            renderDataTab(el, tableName, sourceFile);
        };
        window.clearDataSearch = function() {
            dataSearchQuery = '';
            dataPage = 1;
            renderDataTab(el, tableName, sourceFile);
        };
        window.goDataPage = function(p) { dataPage = p; renderDataTab(el, tableName, sourceFile); };
        window.doRefreshCache = async function() {
            subtabContent.innerHTML = '<p style="color:#facc15">Recargando datos del archivo ISAM...</p>';
            await RefreshCache(tableName);
            dataPage = 1;
            renderDataTab(el, tableName, sourceFile);
        };

        const searchBox = `<div class="search-box">
            <input type="text" id="data-search" placeholder="Buscar..." value="${dataSearchQuery}"
                onkeyup="if(event.key==='Enter') doDataSearch();">
            <button onclick="doDataSearch()">Buscar</button>
            ${dataSearchQuery ? `<button class="btn-clear" onclick="clearDataSearch()">X</button>` : ''}
            <button class="btn-clear" onclick="doRefreshCache()" title="Recargar datos del archivo">&#8635;</button>
        </div>`;

        const paginationHtml = totalPages > 1 ? `<div class="pagination">
            <button ${dataPage <= 1 ? 'disabled' : ''} onclick="goDataPage(${dataPage - 1})">Anterior</button>
            <span>Pagina ${dataPage} de ${totalPages}</span>
            <button ${dataPage >= totalPages ? 'disabled' : ''} onclick="goDataPage(${dataPage + 1})">Siguiente</button>
        </div>` : '';

        let tableHtml = '';
        if (tableName === 'clients') {
            tableHtml = `<table class="data-table"><thead><tr>
                <th>NIT</th><th>Nombre</th><th>Empresa</th><th>Codigo</th><th>Hash</th>
            </tr></thead><tbody>${data.map(r => `<tr>
                <td>${r.numero_doc || ''}</td><td>${r.nombre || ''}</td>
                <td>${r.empresa || ''}</td><td>${r.codigo || ''}</td>
                <td style="font-family:monospace;font-size:10px">${r.hash || ''}</td>
            </tr>`).join('')}</tbody></table>`;
        } else if (tableName === 'products') {
            tableHtml = `<table class="data-table"><thead><tr>
                <th>Codigo</th><th>Nombre</th><th>Hash</th>
            </tr></thead><tbody>${data.map(r => `<tr>
                <td>${r.codigo || ''}</td><td>${r.nombre || ''}</td>
                <td style="font-family:monospace;font-size:10px">${r.hash || ''}</td>
            </tr>`).join('')}</tbody></table>`;
        } else {
            tableHtml = `<table class="data-table"><thead><tr>
                <th>Tipo</th><th>Num Doc</th><th>Fecha</th><th>NIT</th><th>Descripcion</th><th>Valor</th>
            </tr></thead><tbody>${data.map(r => `<tr>
                <td>${r.tipo_comprobante || ''}</td><td>${r.numero_doc || ''}</td>
                <td>${r.fecha || ''}</td><td>${r.nit_tercero || ''}</td>
                <td>${r.descripcion || ''}</td><td>${r.valor || ''}</td>
            </tr>`).join('')}</tbody></table>`;
        }

        subtabContent.innerHTML = searchBox
            + `<p style="margin-bottom:8px;color:#64748b;font-size:12px">${result.total} registros${dataSearchQuery ? ' encontrados' : ''} - Pagina ${dataPage} de ${totalPages}</p>`
            + tableHtml + paginationHtml;
    } else {
        const hasFilters = searchQuery || dateFrom || dateTo || statusFilter;
        const result = hasFilters
            ? await SearchSentRecordsWithDates(tableName, searchQuery, dateFrom, dateTo, statusFilter, historyPage)
            : await GetSentRecords(tableName, historyPage);
        const records = result.records || [];
        const totalPages = Math.ceil(result.total / 50) || 1;

        if (records.length === 0 && historyPage === 1 && !hasFilters) {
            subtabContent.innerHTML = '<div class="empty-state"><h3>Sin historial</h3><p>Aun no se han enviado registros. Usa el boton "Sincronizar Ahora"</p></div>';
            return;
        }

        window.goHistoryPage = function(p) { historyPage = p; renderDataTab(el, tableName, sourceFile); };
        window.doHistorySearch = function() {
            searchQuery = document.getElementById('history-search').value;
            dateFrom = document.getElementById('date-from').value;
            dateTo = document.getElementById('date-to').value;
            statusFilter = document.getElementById('status-filter').value;
            historyPage = 1;
            renderDataTab(el, tableName, sourceFile);
        };
        window.clearHistorySearch = function() {
            searchQuery = '';
            dateFrom = '';
            dateTo = '';
            statusFilter = '';
            historyPage = 1;
            renderDataTab(el, tableName, sourceFile);
        };

        subtabContent.innerHTML = `
        <div class="search-box">
            <input type="text" id="history-search" placeholder="Buscar por key, error..." value="${searchQuery}"
                onkeyup="if(event.key==='Enter') doHistorySearch();">
            <select id="status-filter" onchange="doHistorySearch()">
                <option value="">Todos</option>
                <option value="sent" ${statusFilter === 'sent' ? 'selected' : ''}>Enviados</option>
                <option value="error" ${statusFilter === 'error' ? 'selected' : ''}>Con Error</option>
            </select>
            <button onclick="doHistorySearch()">Buscar</button>
            ${hasFilters ? `<button class="btn-clear" onclick="clearHistorySearch()">X</button>` : ''}
        </div>
        <div class="search-box" style="margin-top:-4px">
            <label style="color:#94a3b8;font-size:12px;min-width:40px">Desde</label>
            <input type="date" id="date-from" value="${dateFrom}" onchange="doHistorySearch()">
            <label style="color:#94a3b8;font-size:12px;min-width:40px">Hasta</label>
            <input type="date" id="date-to" value="${dateTo}" onchange="doHistorySearch()">
        </div>
        <p style="margin-bottom:12px;color:#64748b;font-size:12px">${result.total} registros${hasFilters ? ' encontrados' : ' en total'} - Pagina ${historyPage} de ${totalPages}</p>
        <table class="data-table"><thead><tr>
            <th>Key</th><th>Estado</th><th>Enviado</th><th>Error</th><th>Acciones</th>
        </tr></thead><tbody>${records.map(r => `<tr>
            <td>${r.key}</td>
            <td><span class="status ${r.status}">${r.status}</span></td>
            <td>${fmtDate(r.sent_at)}</td>
            <td style="max-width:200px">${r.error ? `<span class="error-link" onclick="showDetail(${r.id})">${r.error.substring(0, 50)}${r.error.length > 50 ? '...' : ''}</span>` : '-'}</td>
            <td><button class="btn-sm btn-resend" onclick="showDetail(${r.id})">Ver</button>${r.status === 'error' ? ` <button class="btn-sm btn-resend" onclick="resend(${r.id})">Reenviar</button>` : ''}</td>
        </tr>`).join('')}</tbody></table>
        ${totalPages > 1 ? `<div class="pagination">
            <button ${historyPage <= 1 ? 'disabled' : ''} onclick="goHistoryPage(${historyPage - 1})">Anterior</button>
            <span>Pagina ${historyPage} de ${totalPages}</span>
            <button ${historyPage >= totalPages ? 'disabled' : ''} onclick="goHistoryPage(${historyPage + 1})">Siguiente</button>
        </div>` : ''}`;
    }
}

async function renderLogs(el) {
    const result = await GetLogs(logsPage);
    const logs = result.logs || [];
    const totalPages = Math.ceil(result.total / 100) || 1;

    if (logs.length === 0 && logsPage === 1) {
        el.innerHTML = '<div class="empty-state"><h3>Sin logs</h3></div>';
        return;
    }

    window.goLogsPage = function(p) { logsPage = p; renderLogs(el); };

    window.doClearLogs = async function() {
        if (!confirm('¿Seguro que quieres limpiar todos los logs?')) return;
        const r = await ClearLogs();
        showToast(r === 'ok' ? 'success' : 'error', r === 'ok' ? 'Logs limpiados' : r);
        logsPage = 1;
        renderLogs(el);
    };

    el.innerHTML = `
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px">
        <p style="color:#64748b;font-size:12px">${result.total} entradas - Pagina ${logsPage} de ${totalPages}</p>
        <button class="btn-sm btn-resend" onclick="doClearLogs()">Limpiar Logs</button>
    </div>
    <div>${logs.map(l => `
        <div class="log-entry ${l.level}">
            <span class="time">${fmtDate(l.created_at)}</span>
            <span class="source">[${l.source}]</span>
            <span class="msg">${l.message}</span>
        </div>`).join('')}
    </div>
    ${totalPages > 1 ? `<div class="pagination">
        <button ${logsPage <= 1 ? 'disabled' : ''} onclick="goLogsPage(${logsPage - 1})">Anterior</button>
        <span>Pagina ${logsPage} de ${totalPages}</span>
        <button ${logsPage >= totalPages ? 'disabled' : ''} onclick="goLogsPage(${logsPage + 1})">Siguiente</button>
    </div>` : ''}`;
}

async function renderConfig(el) {
    const cfg = await GetConfig();
    if (!cfg) return;

    el.innerHTML = `
    <div class="config-form">
        <h3 style="margin-bottom:16px;color:#94a3b8;font-size:14px;">Siigo</h3>
        <div class="form-group">
            <label>Data Path (ruta de archivos ISAM)</label>
            <input id="cfg-datapath" value="${cfg.siigo?.data_path || ''}" placeholder="C:\\SIIWI02">
        </div>

        <h3 style="margin:24px 0 16px;color:#94a3b8;font-size:14px;">Finearom API</h3>
        <div class="form-group">
            <label>Base URL</label>
            <input id="cfg-baseurl" value="${cfg.finearom?.base_url || ''}" placeholder="https://ordenes.finearom.co/api">
        </div>
        <div class="form-group">
            <label>Email</label>
            <input id="cfg-email" value="${cfg.finearom?.email || ''}" placeholder="siigo-sync@finearom.com">
        </div>
        <div class="form-group">
            <label>Password</label>
            <input id="cfg-password" type="password" value="${cfg.finearom?.password || ''}">
        </div>

        <h3 style="margin:24px 0 16px;color:#94a3b8;font-size:14px;">Sincronizacion</h3>
        <div class="form-group">
            <label>Intervalo (segundos)</label>
            <input id="cfg-interval" type="number" value="${cfg.sync?.interval_seconds || 60}">
        </div>

        <div style="display:flex;gap:12px;margin-top:24px">
            <button class="btn-save" onclick="saveConfig()">Guardar Configuracion</button>
            <button class="btn-test" onclick="testConn()">Probar Conexion</button>
        </div>

        <h3 style="margin:32px 0 16px;color:#f87171;font-size:14px;">Debug</h3>
        <button class="btn-danger" onclick="clearDB()">Vaciar Base de Datos (SQLite)</button>
        <div id="config-msg" style="margin-top:12px"></div>
    </div>`;
}

window.saveConfig = async function() {
    const result = await SaveConfig(
        document.getElementById('cfg-datapath').value,
        document.getElementById('cfg-baseurl').value,
        document.getElementById('cfg-email').value,
        document.getElementById('cfg-password').value,
        parseInt(document.getElementById('cfg-interval').value) || 60
    );
    showToast(result === 'ok' ? 'success' : 'error', result === 'ok' ? 'Configuracion guardada' : result);
};

window.testConn = async function() {
    document.getElementById('config-msg').innerHTML = '<span style="color:#facc15">Probando conexion...</span>';
    const result = await TestConnection();
    const ok = result === 'ok';
    document.getElementById('config-msg').innerHTML = `<span style="color:${ok ? '#4ade80' : '#f87171'}">${ok ? 'Conexion exitosa!' : result}</span>`;
};

function showToast(type, msg) {
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = msg;
    document.body.appendChild(toast);
    setTimeout(() => toast.remove(), 3000);
}

window.togglePause = async function() {
    if (paused) {
        await ResumeSync();
        paused = false;
    } else {
        await PauseSync();
        paused = true;
    }
    render();
};

window.clearDB = async function() {
    if (!confirm('¿Seguro que quieres vaciar todas las tablas de SQLite? Esto borrará historial y logs.')) return;
    const result = await ClearDatabase();
    showToast(result === 'ok' ? 'success' : 'error', result === 'ok' ? 'Base de datos vaciada' : result);
    render();
};

// Init
(async () => {
    paused = await IsPaused();
    render();
})();
