const BASE = '/api';

function getToken(): string | null {
  return localStorage.getItem('siigo_token');
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (token) headers['Authorization'] = `Bearer ${token}`;
  return headers;
}

async function get(path: string) {
  const res = await fetch(BASE + path, { headers: authHeaders() });
  if (res.status === 401) {
    localStorage.removeItem('siigo_token');
    window.dispatchEvent(new Event('auth-expired'));
    throw new Error('No autorizado');
  }
  return res.json();
}

async function post(path: string, body?: object) {
  const res = await fetch(BASE + path, {
    method: 'POST',
    headers: authHeaders(),
    body: body ? JSON.stringify(body) : undefined,
  });
  if (res.status === 401) {
    localStorage.removeItem('siigo_token');
    window.dispatchEvent(new Event('auth-expired'));
    throw new Error('No autorizado');
  }
  return res.json();
}

export const api = {
  login: async (username: string, password: string) => {
    const res = await fetch(BASE + '/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    });
    return res.json();
  },
  checkAuth: async (): Promise<Record<string, unknown>> => {
    try {
      const res = await fetch(BASE + '/check-auth', { headers: authHeaders() });
      if (!res.ok) return { status: 'error' };
      return res.json();
    } catch {
      return { status: 'error' };
    }
  },
  logout: () => {
    localStorage.removeItem('siigo_token');
    localStorage.removeItem('siigo_user');
  },
  isLoggedIn: () => !!getToken(),

  getStats: () => get('/stats'),
  getConfig: () => get('/config'),
  saveConfig: (data: object) => post('/config', data),
  getISAMInfo: () => get('/isam-info'),
  getExtfhStatus: () => get('/extfh-status'),
  getClients: (page: number, search: string) => get(`/clients?page=${page}&search=${encodeURIComponent(search)}`),
  getProducts: (page: number, search: string) => get(`/products?page=${page}&search=${encodeURIComponent(search)}`),
  getMovements: (page: number, search: string) => get(`/movements?page=${page}&search=${encodeURIComponent(search)}`),
  getCartera: (page: number, search: string) => get(`/cartera?page=${page}&search=${encodeURIComponent(search)}`),
  getPlanCuentas: (page: number, search: string) => get(`/plan-cuentas?page=${page}&search=${encodeURIComponent(search)}`),
  getActivosFijos: (page: number, search: string) => get(`/activos-fijos?page=${page}&search=${encodeURIComponent(search)}`),
  getSaldosTerceros: (page: number, search: string) => get(`/saldos-terceros?page=${page}&search=${encodeURIComponent(search)}`),
  getSaldosConsolidados: (page: number, search: string) => get(`/saldos-consolidados?page=${page}&search=${encodeURIComponent(search)}`),
  getDocumentos: (page: number, search: string) => get(`/documentos?page=${page}&search=${encodeURIComponent(search)}`),
  getTercerosAmpliados: (page: number, search: string) => get(`/terceros-ampliados?page=${page}&search=${encodeURIComponent(search)}`),
  getMovimientosInventario: (page: number, search: string) => get(`/movimientos-inventario?page=${page}&search=${encodeURIComponent(search)}`),
  getSaldosInventario: (page: number, search: string) => get(`/saldos-inventario?page=${page}&search=${encodeURIComponent(search)}`),
  getActivosFijosDetalle: (page: number, search: string) => get(`/activos-fijos-detalle?page=${page}&search=${encodeURIComponent(search)}`),
  getAuditTrailTerceros: (page: number, search: string) => get(`/audit-trail-terceros?page=${page}&search=${encodeURIComponent(search)}`),
  getTransaccionesDetalle: (page: number, search: string) => get(`/transacciones-detalle?page=${page}&search=${encodeURIComponent(search)}`),
  getPeriodosContables: (page: number, search: string) => get(`/periodos-contables?page=${page}&search=${encodeURIComponent(search)}`),
  getCondicionesPago: (page: number, search: string) => get(`/condiciones-pago?page=${page}&search=${encodeURIComponent(search)}`),
  getLibrosAuxiliares: (page: number, search: string) => get(`/libros-auxiliares?page=${page}&search=${encodeURIComponent(search)}`),
  getCodigosDane: (page: number, search: string) => get(`/codigos-dane?page=${page}&search=${encodeURIComponent(search)}`),
  getActividadesIca: (page: number, search: string) => get(`/actividades-ica?page=${page}&search=${encodeURIComponent(search)}`),
  getConceptosPila: (page: number, search: string) => get(`/conceptos-pila?page=${page}&search=${encodeURIComponent(search)}`),
  getClasificacionCuentas: (page: number, search: string) => get(`/clasificacion-cuentas?page=${page}&search=${encodeURIComponent(search)}`),
  getHistorial: (page: number, search: string) => get(`/historial?page=${page}&search=${encodeURIComponent(search)}`),
  getMaestros: (page: number, search: string) => get(`/maestros?page=${page}&search=${encodeURIComponent(search)}`),
  getSyncHistory: (table: string, page: number, search?: string, dateFrom?: string, dateTo?: string, status?: string) => {
    const params = new URLSearchParams({ table, page: String(page) });
    if (search) params.set('search', search);
    if (dateFrom) params.set('date_from', dateFrom);
    if (dateTo) params.set('date_to', dateTo);
    if (status) params.set('status', status);
    return get(`/sync-history?${params}`);
  },
  getLogs: (page: number, level?: string, source?: string, search?: string) => {
    const params = new URLSearchParams({ page: String(page) });
    if (level) params.set('level', level);
    if (source) params.set('source', source);
    if (search) params.set('search', search);
    return get(`/logs?${params}`);
  },
  syncNow: () => post('/sync-now'),
  pause: () => post('/pause'),
  resume: () => post('/resume'),
  getSyncStatus: () => get('/sync-status'),
  sendResume: () => post('/send-resume'),
  retryErrors: (table: string) => post('/retry-errors', { table }),
  testConnection: () => post('/test-connection'),
  clearDatabase: () => post('/clear-database'),
  clearLogs: () => post('/clear-logs'),
  refreshCache: (which: string) => post('/refresh-cache', { which }),
  getFieldMappings: () => get('/field-mappings'),
  saveFieldMappings: (mappings: Record<string, FieldMap[]>) => post('/field-mappings', mappings),
  getSendEnabled: () => get('/send-enabled'),
  saveSendEnabled: (enabled: Record<string, boolean>) => post('/send-enabled', enabled),
  getDetectEnabled: () => get('/detect-enabled'),
  saveDetectEnabled: (enabled: Record<string, boolean>) => post('/detect-enabled', enabled),
  getErrorSummary: () => get('/error-summary'),
  getPublicAPIConfig: () => get('/public-api-config'),
  savePublicAPIConfig: (data: object) => post('/public-api-config', data),
  getTelegramConfig: () => get('/telegram-config'),
  saveTelegramConfig: (data: object) => post('/telegram-config', data),
  testTelegram: () => post('/telegram-test'),
  exportHistoryURL: (table?: string) => {
    const params = new URLSearchParams();
    if (table) params.set('table', table);
    const token = getToken();
    if (token) params.set('token', token);
    return BASE + '/export-history?' + params.toString();
  },
  exportLogsURL: () => {
    const token = getToken();
    return BASE + '/export-logs' + (token ? `?token=${token}` : '');
  },
  query: (sql: string, limit: number, offset: number) => post('/query', { query: sql, limit, offset }),
  getAllowEditDelete: () => get('/allow-edit-delete'),
  saveAllowEditDelete: (enabled: boolean) => post('/allow-edit-delete', { enabled }),
  getRecord: (table: string, id: number) => get(`/record?table=${table}&id=${id}`),
  updateRecord: (table: string, id: number, fields: Record<string, unknown>) => {
    return fetch(BASE + `/record?table=${table}&id=${id}`, {
      method: 'PUT',
      headers: authHeaders(),
      body: JSON.stringify({ fields }),
    }).then(r => r.json());
  },
  deleteRecord: (table: string, id: number) => {
    return fetch(BASE + `/record?table=${table}&id=${id}`, {
      method: 'DELETE',
      headers: authHeaders(),
    }).then(r => r.json());
  },

  // User management
  getUsers: () => get('/users'),
  createUser: (data: { username: string; password: string; role: string; permissions: string[] }) =>
    post('/users', data),
  updateUser: (id: number, data: object) => {
    return fetch(BASE + `/users/${id}`, {
      method: 'PUT',
      headers: authHeaders(),
      body: JSON.stringify(data),
    }).then(r => r.json());
  },
  deleteUser: (id: number) => {
    return fetch(BASE + `/users/${id}`, {
      method: 'DELETE',
      headers: authHeaders(),
    }).then(r => r.json());
  },

  // User info helpers
  getUserInfo: (): UserInfo | null => {
    const raw = localStorage.getItem('siigo_user');
    return raw ? JSON.parse(raw) : null;
  },
  setUserInfo: (info: UserInfo) => {
    localStorage.setItem('siigo_user', JSON.stringify(info));
  },
  clearUserInfo: () => {
    localStorage.removeItem('siigo_user');
  },

  // Audit trail
  getAuditTrail: (page: number) => get(`/audit-trail?page=${page}`),

  // Change history
  getChangeHistory: (table: string, key: string) => get(`/change-history?table=${table}&key=${key}`),

  // Sync stats for charts
  getSyncStatsHistory: (hours: number) => get(`/sync-stats-history?hours=${hours}`),

  // Bulk actions
  bulkAction: (table: string, ids: number[], action: string) =>
    post('/bulk-action', { table, ids, action }),

  // Backup/Restore
  backupURL: () => {
    const token = getToken();
    return BASE + '/backup' + (token ? `?token=${token}` : '');
  },
  restore: async (file: File) => {
    const form = new FormData();
    form.append('file', file);
    const res = await fetch(BASE + '/restore', {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${getToken()}` },
      body: form,
    });
    return res.json();
  },

  // Webhooks
  getWebhookConfig: () => get('/webhook-config'),
  saveWebhookConfig: (data: object) => post('/webhook-config', data),
  testWebhook: (url: string, secret?: string) => post('/webhook-test', { url, secret }),

  // Server info (LAN IPs, URLs)
  getServerInfo: () => get('/server-info'),

  // Setup wizard
  getSetupStatus: () => get('/setup-status'),
  setupPopulate: (table: string) => post('/setup-populate', { table }),
  setupComplete: () => post('/setup-complete'),

  // User preferences (stored in SQLite)
  getUserPrefs: (key: string = 'dashboard') => get(`/user-prefs?key=${key}`),
  saveUserPrefs: (data: object, key: string = 'dashboard') => post(`/user-prefs?key=${key}`, data),
};

export interface UserInfo {
  username: string;
  role: string;
  permissions: string[];
}

export interface FieldMap {
  source: string;
  api_key: string;
  label: string;
  enabled: boolean;
}
