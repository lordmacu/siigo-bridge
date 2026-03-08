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
  checkAuth: async (): Promise<boolean> => {
    try {
      const res = await fetch(BASE + '/check-auth', { headers: authHeaders() });
      return res.ok;
    } catch {
      return false;
    }
  },
  logout: () => {
    localStorage.removeItem('siigo_token');
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
  getSyncHistory: (table: string, page: number, search?: string, dateFrom?: string, dateTo?: string, status?: string) => {
    const params = new URLSearchParams({ table, page: String(page) });
    if (search) params.set('search', search);
    if (dateFrom) params.set('date_from', dateFrom);
    if (dateTo) params.set('date_to', dateTo);
    if (status) params.set('status', status);
    return get(`/sync-history?${params}`);
  },
  getLogs: (page: number) => get(`/logs?page=${page}`),
  syncNow: () => post('/sync-now'),
  pause: () => post('/pause'),
  resume: () => post('/resume'),
  getSyncStatus: () => get('/sync-status'),
  retryErrors: (table: string) => post('/retry-errors', { table }),
  testConnection: () => post('/test-connection'),
  clearDatabase: () => post('/clear-database'),
  clearLogs: () => post('/clear-logs'),
  refreshCache: (which: string) => post('/refresh-cache', { which }),
  getFieldMappings: () => get('/field-mappings'),
  saveFieldMappings: (mappings: Record<string, FieldMap[]>) => post('/field-mappings', mappings),
  getSendEnabled: () => get('/send-enabled'),
  saveSendEnabled: (enabled: Record<string, boolean>) => post('/send-enabled', enabled),
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
};

export interface FieldMap {
  source: string;
  api_key: string;
  label: string;
  enabled: boolean;
}
