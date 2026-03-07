const BASE = '/api';

async function get(path: string) {
  const res = await fetch(BASE + path);
  return res.json();
}

async function post(path: string, body?: object) {
  const res = await fetch(BASE + path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  });
  return res.json();
}

export const api = {
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
};
