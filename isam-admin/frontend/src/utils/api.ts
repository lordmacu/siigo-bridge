// API client for ISAM Admin backend

const BASE = '/api';

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(BASE + url, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || res.statusText);
  }
  return res.json();
}

// File browser
export const browseFiles = (path: string) =>
  request<any>(`/files/browse?path=${encodeURIComponent(path)}`);

export const getFileInfo = (path: string) =>
  request<any>(`/files/info?path=${encodeURIComponent(path)}`);

export const getFileHex = (path: string) =>
  request<any>(`/files/hex?path=${encodeURIComponent(path)}`);

export const detectFields = (path: string) =>
  request<any>(`/files/detect?path=${encodeURIComponent(path)}`);

// Tables
export const listTables = () => request<any[]>('/tables');

export const openTable = (path: string, name: string, schemaName?: string) =>
  request<any>('/tables', {
    method: 'POST',
    body: JSON.stringify({ path, name, schema_name: schemaName }),
  });

export const closeTable = (name: string) =>
  request<any>(`/tables?name=${encodeURIComponent(name)}`, { method: 'DELETE' });

// Records
export const getRecords = (name: string, page = 1, pageSize = 50, search = '') =>
  request<any>(
    `/records?name=${encodeURIComponent(name)}&page=${page}&page_size=${pageSize}&search=${encodeURIComponent(search)}`
  );

export const getRecord = (name: string, index: number) =>
  request<any>(`/record?name=${encodeURIComponent(name)}&index=${index}`);

export const insertRecord = (name: string, fields: Record<string, string>) =>
  request<any>(`/record?name=${encodeURIComponent(name)}`, {
    method: 'POST',
    body: JSON.stringify(fields),
  });

export const updateRecord = (name: string, index: number, fields: Record<string, string>) =>
  request<any>(`/record?name=${encodeURIComponent(name)}&index=${index}`, {
    method: 'PUT',
    body: JSON.stringify(fields),
  });

export const deleteRecord = (name: string, index: number) =>
  request<any>(`/record?name=${encodeURIComponent(name)}&index=${index}`, {
    method: 'DELETE',
  });

// Schemas
export const listSchemas = () => request<any[]>('/schemas');

export const getSchema = (name: string) =>
  request<any>(`/schemas?name=${encodeURIComponent(name)}`);

export const saveSchema = (schema: any) =>
  request<any>('/schemas', { method: 'POST', body: JSON.stringify(schema) });

export const deleteSchema = (name: string) =>
  request<any>(`/schemas?name=${encodeURIComponent(name)}`, { method: 'DELETE' });

export const createFileFromSchema = (schemaName: string, filePath: string, force = false) =>
  request<any>('/schemas/create-file', {
    method: 'POST',
    body: JSON.stringify({ schema_name: schemaName, file_path: filePath, force }),
  });

// Query
export const executeQuery = (query: any) =>
  request<any>('/query', { method: 'POST', body: JSON.stringify(query) });
