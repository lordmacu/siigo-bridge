import { useState, useEffect, useRef, useCallback } from 'react';
import { api } from '../api';
import EmptyState from '../components/EmptyState';
import PageHeader from '../components/PageHeader';

const QUICK_QUERIES = [
  { label: 'Historial envios', query: 'SELECT * FROM sync_history ORDER BY id DESC' },
  { label: 'Logs recientes', query: 'SELECT * FROM logs ORDER BY id DESC' },
  { label: 'Errores', query: "SELECT * FROM sync_history WHERE status='error' ORDER BY id DESC" },
];

const HISTORY_KEY = 'explorer_history';
const MAX_HISTORY = 20;

// SQL keywords that precede table names
const TABLE_CONTEXT_RE = /\b(?:FROM|JOIN|INTO|UPDATE|TABLE)\s+$/i;
// SQL keywords that precede column names
const COLUMN_CONTEXT_RE = /\b(?:SELECT|WHERE|AND|OR|ORDER\s+BY|GROUP\s+BY|SET|ON|HAVING|BY)\s+$/i;
// Also match after comma in SELECT list or conditions
const AFTER_COMMA_RE = /,\s*$/;

interface TableInfo {
  name: string;
  count: number | null;
}

interface Suggestion {
  value: string;
  label: string;
  kind: 'table' | 'column' | 'keyword';
}

function loadHistory(): string[] {
  try {
    return JSON.parse(localStorage.getItem(HISTORY_KEY) || '[]');
  } catch { return []; }
}

function saveHistory(history: string[]) {
  localStorage.setItem(HISTORY_KEY, JSON.stringify(history.slice(0, MAX_HISTORY)));
}

function addToHistory(query: string) {
  const h = loadHistory().filter(q => q !== query);
  h.unshift(query);
  saveHistory(h);
}

// Cache for table columns
const columnCache: Record<string, { name: string; type: string }[]> = {};

async function fetchColumns(tableName: string): Promise<{ name: string; type: string }[]> {
  if (columnCache[tableName]) return columnCache[tableName];
  try {
    const r = await api.query(`PRAGMA table_info(${tableName})`, 100, 0);
    if (r.data && r.data.length > 0) {
      const cols = r.data.map((row: Record<string, unknown>) => ({
        name: String(row.name),
        type: String(row.type || 'TEXT'),
      }));
      columnCache[tableName] = cols;
      return cols;
    }
  } catch { /* ignore */ }
  return [];
}

// Extract table name from query (first FROM or JOIN)
function detectTable(sql: string): string | null {
  const m = sql.match(/\bFROM\s+(\w+)/i) || sql.match(/\bJOIN\s+(\w+)/i);
  return m ? m[1] : null;
}

const SQL_KEYWORDS = new Set([
  'SELECT', 'FROM', 'WHERE', 'AND', 'OR', 'NOT', 'IN', 'IS', 'NULL',
  'LIKE', 'BETWEEN', 'AS', 'ON', 'JOIN', 'LEFT', 'RIGHT', 'INNER',
  'OUTER', 'CROSS', 'ORDER', 'BY', 'GROUP', 'HAVING', 'LIMIT', 'OFFSET',
  'INSERT', 'INTO', 'VALUES', 'UPDATE', 'SET', 'DELETE', 'CREATE', 'TABLE',
  'DROP', 'ALTER', 'INDEX', 'DISTINCT', 'COUNT', 'SUM', 'AVG', 'MIN', 'MAX',
  'CASE', 'WHEN', 'THEN', 'ELSE', 'END', 'ASC', 'DESC', 'UNION', 'ALL',
  'EXISTS', 'COALESCE', 'CAST', 'PRAGMA',
]);

const SQL_FUNCTIONS = new Set([
  'COUNT', 'SUM', 'AVG', 'MIN', 'MAX', 'COALESCE', 'CAST',
  'LOWER', 'UPPER', 'TRIM', 'LENGTH', 'SUBSTR', 'REPLACE',
  'DATE', 'TIME', 'DATETIME', 'STRFTIME', 'TYPEOF', 'IFNULL',
  'ABS', 'ROUND', 'RANDOM', 'TOTAL', 'GROUP_CONCAT',
]);

function highlightSQL(sql: string): React.ReactNode[] {
  // Tokenize SQL into parts with types
  const tokens: { text: string; type: 'keyword' | 'function' | 'string' | 'number' | 'operator' | 'table' | 'column' | 'paren' | 'plain' }[] = [];
  let i = 0;

  // Track context for coloring tables vs columns
  const tableContextWords = new Set(['FROM', 'JOIN', 'INTO', 'UPDATE', 'TABLE']);
  let lastKeyword = '';

  while (i < sql.length) {
    // Strings (single-quoted)
    if (sql[i] === "'") {
      let j = i + 1;
      while (j < sql.length && (sql[j] !== "'" || sql[j + 1] === "'")) {
        if (sql[j] === "'" && sql[j + 1] === "'") j += 2;
        else j++;
      }
      if (j < sql.length) j++;
      tokens.push({ text: sql.substring(i, j), type: 'string' });
      i = j;
      continue;
    }

    // Numbers
    if (/\d/.test(sql[i]) && (i === 0 || /[\s,=(><+\-*/]/.test(sql[i - 1]))) {
      let j = i;
      while (j < sql.length && /[\d.]/.test(sql[j])) j++;
      tokens.push({ text: sql.substring(i, j), type: 'number' });
      i = j;
      continue;
    }

    // Parentheses
    if (sql[i] === '(' || sql[i] === ')') {
      tokens.push({ text: sql[i], type: 'paren' });
      i++;
      continue;
    }

    // Operators and special chars
    if (/[=<>!+\-*/%,;]/.test(sql[i])) {
      let j = i;
      while (j < sql.length && /[=<>!]/.test(sql[j])) j++;
      if (j === i) j++;
      tokens.push({ text: sql.substring(i, j), type: 'operator' });
      i = j;
      continue;
    }

    // Whitespace
    if (/\s/.test(sql[i])) {
      let j = i;
      while (j < sql.length && /\s/.test(sql[j])) j++;
      tokens.push({ text: sql.substring(i, j), type: 'plain' });
      i = j;
      continue;
    }

    // Words (identifiers, keywords)
    if (/[a-zA-Z_]/.test(sql[i])) {
      let j = i;
      while (j < sql.length && /[a-zA-Z0-9_]/.test(sql[j])) j++;
      const word = sql.substring(i, j);
      const upper = word.toUpperCase();

      if (SQL_KEYWORDS.has(upper)) {
        tokens.push({ text: word, type: 'keyword' });
        lastKeyword = upper;
      } else if (SQL_FUNCTIONS.has(upper)) {
        tokens.push({ text: word, type: 'function' });
      } else if (tableContextWords.has(lastKeyword)) {
        tokens.push({ text: word, type: 'table' });
        lastKeyword = '';
      } else {
        tokens.push({ text: word, type: 'column' });
      }
      i = j;
      continue;
    }

    // Wildcard *
    if (sql[i] === '*') {
      tokens.push({ text: '*', type: 'operator' });
      i++;
      continue;
    }

    // Fallback
    tokens.push({ text: sql[i], type: 'plain' });
    i++;
  }

  return tokens.map((t, idx) => {
    const cls = `sql-${t.type}`;
    return <span key={idx} className={cls}>{t.text}</span>;
  });
}

// Query Builder types
interface QBFilter {
  id: number;
  column: string;
  operator: string;
  value: string;
}

const QB_OPERATORS = [
  { value: '=', label: '= (igual)' },
  { value: '!=', label: '!= (diferente)' },
  { value: 'LIKE', label: 'Contiene' },
  { value: 'NOT LIKE', label: 'No contiene' },
  { value: '>', label: '> (mayor)' },
  { value: '<', label: '< (menor)' },
  { value: '>=', label: '>= (mayor o igual)' },
  { value: '<=', label: '<= (menor o igual)' },
  { value: 'IS NULL', label: 'Es vacio' },
  { value: 'IS NOT NULL', label: 'No es vacio' },
];

let qbFilterId = 0;

export default function Explorer() {
  const [activeTab, setActiveTab] = useState<'sql' | 'builder' | 'files'>('sql');
  const [isamFileName, setIsamFileName] = useState('');
  const [isamDownloading, setIsamDownloading] = useState(false);
  const [isamError, setIsamError] = useState('');
  const [fileDownloadKey, setFileDownloadKey] = useState('');
  const [isamCopied, setIsamCopied] = useState(false);
  const [newKeyInput, setNewKeyInput] = useState('');
  const [keySaving, setKeySaving] = useState(false);
  const [tables, setTables] = useState<TableInfo[]>([]);
  const [query, setQuery] = useState('SELECT * FROM clients');
  const [limit, setLimit] = useState(20);
  const [columns, setColumns] = useState<string[]>([]);
  const [data, setData] = useState<Record<string, unknown>[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [lastQuery, setLastQuery] = useState('');
  const [selectedTable, setSelectedTable] = useState('');
  const [schema, setSchema] = useState<{ name: string; type: string }[]>([]);
  const [showSchema, setShowSchema] = useState(false);
  const [history, setHistory] = useState<string[]>(loadHistory);
  const [showHistory, setShowHistory] = useState(false);
  const [copied, setCopied] = useState('');
  const [execTime, setExecTime] = useState(0);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Query Builder state
  const [qbTable, setQbTable] = useState('');
  const [qbColumns, setQbColumns] = useState<string[]>([]);
  const [qbAvailCols, setQbAvailCols] = useState<{ name: string; type: string }[]>([]);
  const [qbFilters, setQbFilters] = useState<QBFilter[]>([]);
  const [qbSortCol, setQbSortCol] = useState('');
  const [qbSortDir, setQbSortDir] = useState<'ASC' | 'DESC'>('ASC');
  const [qbLimit, setQbLimit] = useState(50);
  const [qbPreview, setQbPreview] = useState('');

  // Autocomplete state
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [acIndex, setAcIndex] = useState(0);
  const [acVisible, setAcVisible] = useState(false);
  const [acWordStart, setAcWordStart] = useState(0);
  const acRef = useRef<HTMLDivElement>(null);
  const tablesRef = useRef<TableInfo[]>([]);

  // Keep tablesRef in sync
  useEffect(() => { tablesRef.current = tables; }, [tables]);

  // Fetch file download key when files tab is opened
  useEffect(() => {
    if (activeTab !== 'files') return;
    api.getConfig().then((cfg: Record<string, unknown>) => {
      setFileDownloadKey((cfg.file_download_key as string) || '');
    }).catch(() => {});
  }, [activeTab]);

  // Query Builder: load columns when table changes
  useEffect(() => {
    if (!qbTable) { setQbAvailCols([]); setQbColumns([]); return; }
    fetchColumns(qbTable).then(cols => {
      setQbAvailCols(cols);
      setQbColumns([]); // all = select *
      setQbFilters([]);
      setQbSortCol('');
    });
  }, [qbTable]);

  // Query Builder: generate SQL preview
  useEffect(() => {
    if (!qbTable) { setQbPreview(''); return; }
    const colsPart = qbColumns.length > 0 ? qbColumns.join(', ') : '*';
    let sql = `SELECT ${colsPart} FROM ${qbTable}`;

    const validFilters = qbFilters.filter(f => f.column && f.operator);
    if (validFilters.length > 0) {
      const clauses = validFilters.map(f => {
        if (f.operator === 'IS NULL' || f.operator === 'IS NOT NULL') {
          return `${f.column} ${f.operator}`;
        }
        if (f.operator === 'LIKE' || f.operator === 'NOT LIKE') {
          return `${f.column} ${f.operator} '%${f.value.replace(/'/g, "''")   }%'`;
        }
        return `${f.column} ${f.operator} '${f.value.replace(/'/g, "''")}'`;
      });
      sql += ` WHERE ${clauses.join(' AND ')}`;
    }

    if (qbSortCol) {
      sql += ` ORDER BY ${qbSortCol} ${qbSortDir}`;
    }

    sql += ` LIMIT ${qbLimit}`;
    setQbPreview(sql);
  }, [qbTable, qbColumns, qbFilters, qbSortCol, qbSortDir, qbLimit]);

  const qbAddFilter = () => {
    setQbFilters(prev => [...prev, { id: ++qbFilterId, column: qbAvailCols[0]?.name || '', operator: '=', value: '' }]);
  };

  const qbRemoveFilter = (id: number) => {
    setQbFilters(prev => prev.filter(f => f.id !== id));
  };

  const qbUpdateFilter = (id: number, field: keyof QBFilter, value: string) => {
    setQbFilters(prev => prev.map(f => f.id === id ? { ...f, [field]: value } : f));
  };

  const qbToggleColumn = (col: string) => {
    setQbColumns(prev => prev.includes(col) ? prev.filter(c => c !== col) : [...prev, col]);
  };

  const qbSelectAllColumns = () => setQbColumns([]);

  const qbExecute = () => {
    if (!qbPreview) return;
    setQuery(qbPreview);
    setOffset(0);
    execute(qbPreview, 0);
  };

  const qbCopyToSQL = () => {
    if (!qbPreview) return;
    setQuery(qbPreview);
    setActiveTab('sql');
  };

  // Tables removed from active sync (kept in SQLite for backward compat, hidden from UI)
  const HIDDEN_TABLES = new Set([
    'movements', 'saldos_consolidados', 'transacciones_detalle', 'periodos_contables',
    'clasificacion_cuentas', 'activos_fijos', 'activos_fijos_detalle', 'actividades_ica',
    'conceptos_pila', 'movimientos_inventario', 'saldos_inventario', 'docs_inventario',
    'terceros_ampliados', 'audit_trail_terceros',
  ]);

  // Load table list with row counts on mount
  useEffect(() => {
    api.query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name", 100, 0)
      .then(async (r) => {
        if (!r.data) return;
        const names = r.data.map((row: Record<string, unknown>) => String(row.name))
          .filter((name: string) => !HIDDEN_TABLES.has(name));
        const infos: TableInfo[] = await Promise.all(
          names.map(async (name: string) => {
            try {
              const cr = await api.query(`SELECT COUNT(*) as cnt FROM ${name}`, 1, 0);
              return { name, count: cr.data?.[0]?.cnt as number ?? null };
            } catch { return { name, count: null }; }
          })
        );
        setTables(infos);
        // Pre-cache columns for all tables
        for (const info of infos) {
          fetchColumns(info.name);
        }
      })
      .catch(() => {});
  }, []);

  // Compute autocomplete suggestions
  const computeSuggestions = useCallback(async (sql: string, cursorPos: number): Promise<Suggestion[]> => {
    const before = sql.substring(0, cursorPos);
    const tList = tablesRef.current;

    // Get current partial word being typed
    const wordMatch = before.match(/(\w*)$/);
    const partial = wordMatch ? wordMatch[1] : '';
    const wordStart = cursorPos - partial.length;
    const textBefore = before.substring(0, wordStart);

    setAcWordStart(wordStart);

    const partialLower = partial.toLowerCase();
    const results: Suggestion[] = [];

    // Determine context
    const isTableCtx = TABLE_CONTEXT_RE.test(textBefore);
    const isColumnCtx = COLUMN_CONTEXT_RE.test(textBefore) || AFTER_COMMA_RE.test(textBefore);

    if (isTableCtx) {
      // Suggest table names
      for (const t of tList) {
        if (!partial || t.name.toLowerCase().startsWith(partialLower)) {
          results.push({
            value: t.name,
            label: `${t.name}${t.count !== null ? ` (${t.count.toLocaleString()})` : ''}`,
            kind: 'table',
          });
        }
      }
    } else if (isColumnCtx) {
      // Suggest columns from detected table
      const tableName = detectTable(sql);
      if (tableName) {
        const cols = await fetchColumns(tableName);
        for (const col of cols) {
          if (!partial || col.name.toLowerCase().startsWith(partialLower)) {
            results.push({
              value: col.name,
              label: `${col.name} (${col.type})`,
              kind: 'column',
            });
          }
        }
      }
      // Also suggest * after SELECT
      if (/\bSELECT\s+$/i.test(textBefore) && (!partial || '*'.startsWith(partialLower))) {
        results.unshift({ value: '*', label: '* (all columns)', kind: 'keyword' });
      }
    } else if (partial.length >= 2) {
      // Suggest tables if partial matches
      for (const t of tList) {
        if (t.name.toLowerCase().startsWith(partialLower)) {
          results.push({
            value: t.name,
            label: `${t.name}${t.count !== null ? ` (${t.count.toLocaleString()})` : ''}`,
            kind: 'table',
          });
        }
      }
      // Suggest SQL keywords
      const keywords = ['SELECT', 'FROM', 'WHERE', 'AND', 'OR', 'ORDER BY', 'GROUP BY',
        'HAVING', 'LIMIT', 'JOIN', 'LEFT JOIN', 'INNER JOIN', 'ON', 'AS',
        'COUNT(*)', 'DISTINCT', 'LIKE', 'IN', 'NOT', 'NULL', 'IS', 'BETWEEN'];
      for (const kw of keywords) {
        if (kw.toLowerCase().startsWith(partialLower) && kw.toLowerCase() !== partialLower) {
          results.push({ value: kw, label: kw, kind: 'keyword' });
        }
      }
    }

    return results.slice(0, 12);
  }, []);

  // Apply a suggestion
  const applySuggestion = useCallback((suggestion: Suggestion) => {
    const ta = textareaRef.current;
    if (!ta) return;

    const curPos = ta.selectionStart;
    const before = query.substring(0, acWordStart);
    const after = query.substring(curPos);
    const newQuery = before + suggestion.value + ' ' + after;
    setQuery(newQuery);
    setAcVisible(false);
    setSuggestions([]);

    setTimeout(() => {
      ta.focus();
      const pos = acWordStart + suggestion.value.length + 1;
      ta.setSelectionRange(pos, pos);
    }, 0);
  }, [query, acWordStart]);

  // Handle textarea input for autocomplete
  const handleInput = useCallback(async (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const newQuery = e.target.value;
    const cursorPos = e.target.selectionStart;
    setQuery(newQuery);

    const results = await computeSuggestions(newQuery, cursorPos);
    if (results.length > 0) {
      setSuggestions(results);
      setAcIndex(0);
      setAcVisible(true);
    } else {
      setAcVisible(false);
      setSuggestions([]);
    }
  }, [computeSuggestions]);

  // Handle keyboard in textarea for autocomplete navigation
  const handleKeyDown = useCallback((e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
      e.preventDefault();
      setAcVisible(false);
      execute();
      return;
    }

    if (e.key === 'Escape') {
      if (acVisible) {
        e.preventDefault();
        setAcVisible(false);
      }
      return;
    }

    if (!acVisible || suggestions.length === 0) return;

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setAcIndex(prev => (prev + 1) % suggestions.length);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setAcIndex(prev => (prev - 1 + suggestions.length) % suggestions.length);
    } else if (e.key === 'Tab' || e.key === 'Enter') {
      e.preventDefault();
      applySuggestion(suggestions[acIndex]);
    }
  }, [acVisible, suggestions, acIndex, applySuggestion]);

  // Close autocomplete when clicking outside
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (acRef.current && !acRef.current.contains(e.target as Node) &&
          textareaRef.current && !textareaRef.current.contains(e.target as Node)) {
        setAcVisible(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  const execute = async (sql?: string, newOffset?: number) => {
    const q = sql ?? query;
    const o = newOffset ?? 0;
    setLoading(true);
    setError('');
    setAcVisible(false);
    const t0 = performance.now();
    try {
      const r = await api.query(q.trim(), limit, o);
      setExecTime(Math.round(performance.now() - t0));
      if (r.error) {
        setError(r.error);
        setData([]);
        setColumns([]);
        setTotal(0);
      } else {
        setColumns(r.columns || []);
        setData(r.data || []);
        setTotal(r.total ?? 0);
        setOffset(o);
        setLastQuery(q);
        addToHistory(q);
        setHistory(loadHistory());
      }
    } catch (e: unknown) {
      setExecTime(Math.round(performance.now() - t0));
      setError(e instanceof Error ? e.message : 'Error de conexion');
    }
    setLoading(false);
  };

  const page = Math.floor(offset / limit) + 1;
  const totalPages = Math.max(1, Math.ceil(total / limit));

  const goPage = (p: number) => {
    execute(lastQuery, (p - 1) * limit);
  };

  const selectTable = async (table: string) => {
    if (!table) return;
    setSelectedTable(table);
    const q = `SELECT * FROM ${table}`;
    setQuery(q);
    setOffset(0);
    execute(q, 0);
    const cols = await fetchColumns(table);
    if (cols.length > 0) {
      setSchema(cols);
      setShowSchema(true);
    }
  };

  const setQuickQuery = (q: string) => {
    setQuery(q);
    setOffset(0);
    execute(q, 0);
  };

  const clickFilter = (col: string, val: unknown) => {
    if (val === null || val === undefined) return;
    const strVal = String(val);
    const table = extractTable(lastQuery);
    let q: string;
    if (table) {
      q = `SELECT * FROM ${table} WHERE ${col} = '${strVal.replace(/'/g, "''")}'`;
    } else {
      q = `${lastQuery.replace(/\s+ORDER\s+BY\s+.*/i, '')} AND ${col} = '${strVal.replace(/'/g, "''")}'`;
    }
    setQuery(q);
    setOffset(0);
    execute(q, 0);
  };

  const copyCell = (val: unknown) => {
    const text = val === null || val === undefined ? '' : String(val);
    navigator.clipboard.writeText(text).then(() => {
      setCopied(text.substring(0, 30));
      setTimeout(() => setCopied(''), 1500);
    }).catch(() => {});
  };

  const exportCSV = () => {
    if (columns.length === 0 || data.length === 0) return;
    const escape = (v: unknown) => {
      const s = v === null || v === undefined ? '' : String(v);
      return s.includes(',') || s.includes('"') || s.includes('\n') ? `"${s.replace(/"/g, '""')}"` : s;
    };
    const header = columns.map(escape).join(',');
    const rows = data.map(row => columns.map(c => escape(row[c])).join(','));
    const csv = [header, ...rows].join('\n');
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `explorer_${Date.now()}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const useHistoryQuery = (q: string) => {
    setQuery(q);
    setShowHistory(false);
    setOffset(0);
    execute(q, 0);
  };

  const clearHistory = () => {
    saveHistory([]);
    setHistory([]);
  };

  // Compute dropdown position relative to textarea
  const getAcStyle = (): React.CSSProperties => {
    const ta = textareaRef.current;
    if (!ta) return { display: 'none' };
    const rect = ta.getBoundingClientRect();
    const style = getComputedStyle(ta);
    const lineHeight = parseInt(style.lineHeight) || 20;
    const text = ta.value.substring(0, ta.selectionStart);
    const lines = text.split('\n');
    const currentLine = lines.length - 1;
    const charWidth = 7.8;
    const charInLine = lines[lines.length - 1].length;
    const padLeft = parseInt(style.paddingLeft) || 14;

    return {
      position: 'absolute' as const,
      top: (currentLine + 1) * lineHeight + 8,
      left: Math.min(charInLine * charWidth + padLeft, rect.width - 260),
      zIndex: 4000,
    };
  };

  // Shared results section (used by both tabs)
  const resultsSection = (
    <>
      {error && <div className="explorer-error">{error}</div>}
      {copied && <div className="explorer-copied">Copiado: {copied}</div>}
      {columns.length > 0 && (
        <>
          <div className="explorer-meta">
            {total >= 0 ? total.toLocaleString() : '?'} resultados — Pagina {page} de {totalPages}
            {execTime > 0 && <span className="explorer-time"> — {execTime}ms</span>}
            <button className="btn-sm btn-export" style={{ marginLeft: 12 }} onClick={exportCSV}>Exportar CSV</button>
          </div>
          <div className="table-wrapper">
            <table className="data-table explorer-table">
              <thead>
                <tr>
                  <th className="row-num">#</th>
                  {columns.map(c => <th key={c}>{c}</th>)}
                </tr>
              </thead>
              <tbody>
                {data.map((row, i) => (
                  <tr key={i}>
                    <td className="row-num">{offset + i + 1}</td>
                    {columns.map(c => (
                      <td
                        key={c}
                        title={`Click: copiar | Doble-click: filtrar por ${c}="${String(row[c] ?? '')}"`}
                        onClick={() => copyCell(row[c])}
                        onDoubleClick={() => clickFilter(c, row[c])}
                        className="explorer-cell"
                      >
                        {formatCell(row[c])}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {totalPages > 1 && (
            <div className="pagination">
              <button onClick={() => goPage(1)} disabled={page <= 1}>Primera</button>
              <button onClick={() => goPage(page - 1)} disabled={page <= 1}>Anterior</button>
              <span>Pagina {page} de {totalPages}</span>
              <button onClick={() => goPage(page + 1)} disabled={page >= totalPages}>Siguiente</button>
              <button onClick={() => goPage(totalPages)} disabled={page >= totalPages}>Ultima</button>
            </div>
          )}
        </>
      )}
    </>
  );

  return (
    <>
      <PageHeader title="SQL Explorer" />
      <div className="content">
        {/* Tab bar */}
        <div className="explorer-tabs">
          <button
            className={`explorer-tab ${activeTab === 'builder' ? 'active' : ''}`}
            onClick={() => setActiveTab('builder')}
          >
            Consulta Asistida
          </button>
          <button
            className={`explorer-tab ${activeTab === 'sql' ? 'active' : ''}`}
            onClick={() => setActiveTab('sql')}
          >
            SQL Avanzado
          </button>
          <button
            className={`explorer-tab ${activeTab === 'files' ? 'active' : ''}`}
            onClick={() => setActiveTab('files')}
          >
            Archivos ISAM
          </button>
        </div>

        {/* Tab: Query Builder */}
        {activeTab === 'builder' && (
          <div className="qb-container">
            {/* Step 1: Table */}
            <div className="qb-section">
              <div className="qb-section-header">
                <span className="qb-step">1</span>
                <span>Selecciona una tabla</span>
              </div>
              <select
                className="qb-select"
                value={qbTable}
                onChange={e => setQbTable(e.target.value)}
              >
                <option value="">-- Seleccionar tabla --</option>
                {tables.map(t => (
                  <option key={t.name} value={t.name}>
                    {t.name} {t.count !== null ? `(${t.count.toLocaleString()} registros)` : ''}
                  </option>
                ))}
              </select>
            </div>

            {qbTable && qbAvailCols.length > 0 && (
              <>
                {/* Step 2: Columns */}
                <div className="qb-section">
                  <div className="qb-section-header">
                    <span className="qb-step">2</span>
                    <span>Columnas a mostrar</span>
                    <button className="qb-link" onClick={qbSelectAllColumns}>
                      {qbColumns.length === 0 ? 'Todas seleccionadas' : 'Seleccionar todas'}
                    </button>
                  </div>
                  <div className="qb-columns-grid">
                    {qbAvailCols.map(col => (
                      <label key={col.name} className={`qb-col-chip ${qbColumns.length === 0 || qbColumns.includes(col.name) ? 'selected' : ''}`}>
                        <input
                          type="checkbox"
                          checked={qbColumns.length === 0 || qbColumns.includes(col.name)}
                          onChange={() => qbToggleColumn(col.name)}
                        />
                        <span className="qb-col-name">{col.name}</span>
                        <span className="qb-col-type">{col.type}</span>
                      </label>
                    ))}
                  </div>
                </div>

                {/* Step 3: Filters */}
                <div className="qb-section">
                  <div className="qb-section-header">
                    <span className="qb-step">3</span>
                    <span>Filtros</span>
                    <button className="qb-btn-add" onClick={qbAddFilter}>+ Agregar filtro</button>
                  </div>
                  {qbFilters.length === 0 && (
                    <div className="qb-hint">Sin filtros — se mostraran todos los registros</div>
                  )}
                  {qbFilters.map(f => (
                    <div key={f.id} className="qb-filter-row">
                      <select
                        value={f.column}
                        onChange={e => qbUpdateFilter(f.id, 'column', e.target.value)}
                        className="qb-filter-col"
                      >
                        {qbAvailCols.map(c => (
                          <option key={c.name} value={c.name}>{c.name}</option>
                        ))}
                      </select>
                      <select
                        value={f.operator}
                        onChange={e => qbUpdateFilter(f.id, 'operator', e.target.value)}
                        className="qb-filter-op"
                      >
                        {QB_OPERATORS.map(op => (
                          <option key={op.value} value={op.value}>{op.label}</option>
                        ))}
                      </select>
                      {f.operator !== 'IS NULL' && f.operator !== 'IS NOT NULL' && (
                        <input
                          type="text"
                          value={f.value}
                          onChange={e => qbUpdateFilter(f.id, 'value', e.target.value)}
                          placeholder="Valor..."
                          className="qb-filter-val"
                        />
                      )}
                      <button className="qb-filter-remove" onClick={() => qbRemoveFilter(f.id)} title="Eliminar filtro">
                        x
                      </button>
                    </div>
                  ))}
                </div>

                {/* Step 4: Sort */}
                <div className="qb-section">
                  <div className="qb-section-header">
                    <span className="qb-step">4</span>
                    <span>Ordenar por</span>
                  </div>
                  <div className="qb-sort-row">
                    <select
                      value={qbSortCol}
                      onChange={e => setQbSortCol(e.target.value)}
                      className="qb-select"
                    >
                      <option value="">Sin orden especifico</option>
                      {qbAvailCols.map(c => (
                        <option key={c.name} value={c.name}>{c.name}</option>
                      ))}
                    </select>
                    {qbSortCol && (
                      <div className="qb-sort-dir">
                        <button
                          className={`qb-dir-btn ${qbSortDir === 'ASC' ? 'active' : ''}`}
                          onClick={() => setQbSortDir('ASC')}
                        >
                          Ascendente
                        </button>
                        <button
                          className={`qb-dir-btn ${qbSortDir === 'DESC' ? 'active' : ''}`}
                          onClick={() => setQbSortDir('DESC')}
                        >
                          Descendente
                        </button>
                      </div>
                    )}
                  </div>
                </div>

                {/* Step 5: Limit */}
                <div className="qb-section">
                  <div className="qb-section-header">
                    <span className="qb-step">5</span>
                    <span>Cantidad de resultados</span>
                  </div>
                  <select
                    value={qbLimit}
                    onChange={e => setQbLimit(Number(e.target.value))}
                    className="qb-select qb-select-sm"
                  >
                    <option value={10}>10</option>
                    <option value={20}>20</option>
                    <option value={50}>50</option>
                    <option value={100}>100</option>
                    <option value={200}>200</option>
                    <option value={500}>500</option>
                  </select>
                </div>

                {/* Preview & Execute */}
                {qbPreview && (
                  <div className="qb-preview">
                    <div className="qb-preview-header">
                      <span>Consulta generada:</span>
                      <button className="qb-link" onClick={qbCopyToSQL}>Editar en SQL</button>
                    </div>
                    <pre className="qb-preview-sql">{highlightSQL(qbPreview)}</pre>
                    <div className="qb-actions">
                      <button className="btn-save" onClick={qbExecute} disabled={loading}>
                        {loading ? 'Ejecutando...' : 'Ejecutar consulta'}
                      </button>
                    </div>
                  </div>
                )}
              </>
            )}

            {!qbTable && (
              <div className="qb-empty">
                Selecciona una tabla para comenzar a construir tu consulta
              </div>
            )}

            {resultsSection}
          </div>
        )}

        {/* Tab: SQL Explorer (original) */}
        {activeTab === 'sql' && (
          <>
            <div className="explorer-toolbar">
              <div className="explorer-table-select">
                <label>Tabla:</label>
                <select onChange={e => selectTable(e.target.value)} value={selectedTable}>
                  <option value="" disabled>Seleccionar tabla...</option>
                  {tables.map(t => (
                    <option key={t.name} value={t.name}>
                      {t.name} {t.count !== null ? `(${t.count.toLocaleString()})` : ''}
                    </option>
                  ))}
                </select>
              </div>
              <div className="explorer-quick">
                {QUICK_QUERIES.map(q => (
                  <button
                    key={q.label}
                    className={`explorer-chip ${lastQuery === q.query ? 'active' : ''}`}
                    onClick={() => setQuickQuery(q.query)}
                  >
                    {q.label}
                  </button>
                ))}
                <button
                  className={`explorer-chip ${showHistory ? 'active' : ''}`}
                  onClick={() => setShowHistory(!showHistory)}
                >
                  Historial ({history.length})
                </button>
              </div>
            </div>

            {/* Query History Panel */}
            {showHistory && (
              <div className="explorer-history">
                <div className="explorer-history-header">
                  <span>Consultas recientes</span>
                  {history.length > 0 && (
                    <button className="btn-clear" onClick={clearHistory}>Limpiar</button>
                  )}
                </div>
                {history.length === 0 ? (
                  <div className="explorer-history-empty">Sin historial</div>
                ) : (
                  <div className="explorer-history-list">
                    {history.map((q, i) => (
                      <div key={i} className="explorer-history-item" onClick={() => useHistoryQuery(q)}>
                        <code>{q.length > 100 ? q.substring(0, 100) + '...' : q}</code>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Schema Panel */}
            {showSchema && schema.length > 0 && (
              <div className="explorer-schema">
                <div className="explorer-schema-header">
                  <span>Estructura: <strong>{selectedTable}</strong> ({schema.length} columnas)</span>
                  <button className="btn-clear" onClick={() => setShowSchema(false)}>Cerrar</button>
                </div>
                <div className="explorer-schema-cols">
                  {schema.map(col => (
                    <span key={col.name} className="schema-col" onClick={() => {
                      const q = `SELECT ${col.name}, COUNT(*) as total FROM ${selectedTable} GROUP BY ${col.name} ORDER BY total DESC`;
                      setQuery(q);
                      execute(q, 0);
                    }}>
                      <strong>{col.name}</strong>
                      <small>{col.type}</small>
                    </span>
                  ))}
                </div>
              </div>
            )}

            <div className="explorer-input" style={{ position: 'relative' }}>
              <div className="sql-editor-wrapper">
                <pre className="sql-highlight" aria-hidden="true">
                  {highlightSQL(query)}{'\n'}
                </pre>
                <textarea
                  ref={textareaRef}
                  className="explorer-textarea"
                  value={query}
                  onChange={handleInput}
                  onKeyDown={handleKeyDown}
                  placeholder="SELECT * FROM clients WHERE nit LIKE '%900%'"
                  rows={3}
                  spellCheck={false}
                />
              </div>

              {/* Autocomplete dropdown */}
              {acVisible && suggestions.length > 0 && (
                <div ref={acRef} className="ac-dropdown" style={getAcStyle()}>
                  {suggestions.map((s, i) => (
                    <div
                      key={s.value + s.kind}
                      className={`ac-item ${i === acIndex ? 'active' : ''} ac-${s.kind}`}
                      onMouseDown={(e) => { e.preventDefault(); applySuggestion(s); }}
                      onMouseEnter={() => setAcIndex(i)}
                    >
                      <span className="ac-icon">
                        {s.kind === 'table' ? 'T' : s.kind === 'column' ? 'C' : 'K'}
                      </span>
                      <span className="ac-label">{s.label}</span>
                    </div>
                  ))}
                </div>
              )}

              <div className="explorer-controls">
                <div className="explorer-limit">
                  <label>Limite:</label>
                  <select value={limit} onChange={e => setLimit(Number(e.target.value))}>
                    <option value={20}>20</option>
                    <option value={50}>50</option>
                    <option value={100}>100</option>
                    <option value={200}>200</option>
                    <option value={500}>500</option>
                  </select>
                </div>
                <div className="explorer-actions">
                  {columns.length > 0 && (
                    <button className="btn-export" onClick={exportCSV}>
                      Exportar CSV
                    </button>
                  )}
                  <button className="btn-save" onClick={() => execute()} disabled={loading || !query.trim()}>
                    {loading ? 'Ejecutando...' : 'Ejecutar (Ctrl+Enter)'}
                  </button>
                </div>
              </div>
            </div>

            {resultsSection}

            {!loading && columns.length === 0 && !error && (
              <EmptyState title="SQL Explorer" message="Selecciona una tabla o escribe una consulta SELECT. Solo lectura.">
                <div className="explorer-tips">
                  <p><strong>Tips:</strong></p>
                  <p>Autocomplete: escribe y aparecen sugerencias (Tab/Enter para aceptar)</p>
                  <p>Click en una celda = copiar valor</p>
                  <p>Doble-click en una celda = filtrar por ese valor</p>
                  <p>Click en columna del schema = ver valores unicos</p>
                  <p>Ctrl+Enter = ejecutar consulta</p>
                </div>
              </EmptyState>
            )}
          </>
        )}

        {/* Tab: Archivos ISAM */}
        {activeTab === 'files' && (
          <div style={{ maxWidth: 600, margin: '2rem auto', display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>

            {/* Download card */}
            <div className="card" style={{ padding: '1.5rem' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.6rem', marginBottom: '0.4rem' }}>
                <span style={{ fontSize: '1.2rem' }}>📁</span>
                <h3 style={{ margin: 0, fontSize: '1rem', fontWeight: 600 }}>Descargar archivo ISAM</h3>
              </div>
              <p style={{ color: 'var(--text-secondary)', fontSize: '0.83rem', margin: '0 0 1rem' }}>
                Nombre del archivo en la carpeta de datos Siigo — solo el nombre, sin ruta ni extensión.
              </p>
              <div style={{ display: 'flex', gap: '0.5rem' }}>
                <input
                  type="text"
                  placeholder="Ej: Z49, Z072026, Z092026"
                  value={isamFileName}
                  onChange={e => { setIsamFileName(e.target.value.toUpperCase()); setIsamError(''); }}
                  onKeyDown={e => { if (e.key === 'Enter') document.getElementById('btn-isam-download')?.click(); }}
                  style={{ flex: 1, padding: '0.5rem 0.75rem', borderRadius: 6, border: '1px solid var(--border)', background: 'var(--bg-secondary)', color: 'var(--text)', fontSize: '0.9rem', fontFamily: 'monospace' }}
                />
                <button
                  id="btn-isam-download"
                  className="btn btn-primary"
                  disabled={!isamFileName.trim() || isamDownloading || !fileDownloadKey}
                  onClick={async () => {
                    const name = isamFileName.trim();
                    if (!name || !fileDownloadKey) return;
                    setIsamDownloading(true);
                    setIsamError('');
                    try {
                      const res = await fetch(`/api/siigo-file?name=${encodeURIComponent(name)}&key=${encodeURIComponent(fileDownloadKey)}`);
                      if (!res.ok) {
                        const err = await res.json().catch(() => ({ error: `HTTP ${res.status}` }));
                        setIsamError(err.error || `Error ${res.status}`);
                        return;
                      }
                      const blob = await res.blob();
                      const url = URL.createObjectURL(blob);
                      const a = document.createElement('a');
                      a.href = url;
                      a.download = name;
                      document.body.appendChild(a);
                      a.click();
                      document.body.removeChild(a);
                      URL.revokeObjectURL(url);
                    } catch {
                      setIsamError('Error de conexión');
                    } finally {
                      setIsamDownloading(false);
                    }
                  }}
                >
                  {isamDownloading ? '⏳ Descargando…' : '⬇ Descargar'}
                </button>
              </div>
              {isamError && (
                <p style={{ color: 'var(--error)', marginTop: '0.6rem', fontSize: '0.83rem', margin: '0.6rem 0 0' }}>{isamError}</p>
              )}
              {!fileDownloadKey && (
                <p style={{ color: 'var(--warning, #f59e0b)', marginTop: '0.6rem', fontSize: '0.83rem' }}>
                  ⚠ Configura <code>file_download_key</code> en Ajustes → Avanzado para habilitar las descargas.
                </p>
              )}
            </div>

            {/* Key setup card */}
            <div className="card" style={{ padding: '1.5rem' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.6rem', marginBottom: '0.4rem' }}>
                <span style={{ fontSize: '1.1rem' }}>🔑</span>
                <h3 style={{ margin: 0, fontSize: '1rem', fontWeight: 600 }}>Clave de descarga</h3>
                {fileDownloadKey && <span style={{ fontSize: '0.75rem', background: 'var(--success-bg, #d1fae5)', color: 'var(--success, #059669)', padding: '0.15rem 0.5rem', borderRadius: 99, fontWeight: 600 }}>CONFIGURADA</span>}
              </div>
              <p style={{ color: 'var(--text-secondary)', fontSize: '0.83rem', margin: '0 0 0.75rem' }}>
                Clave estática que va en la URL para descargar archivos sin login. Solo con HTTPS o red local de confianza.
              </p>
              <div style={{ display: 'flex', gap: '0.5rem' }}>
                <input
                  type="text"
                  placeholder={fileDownloadKey ? '(cambia la clave actual)' : 'Nueva clave secreta…'}
                  value={newKeyInput}
                  onChange={e => setNewKeyInput(e.target.value)}
                  style={{ flex: 1, padding: '0.5rem 0.75rem', borderRadius: 6, border: '1px solid var(--border)', background: 'var(--bg-secondary)', color: 'var(--text)', fontSize: '0.9rem', fontFamily: 'monospace' }}
                />
                <button
                  className="btn btn-primary"
                  disabled={!newKeyInput.trim() || keySaving}
                  onClick={async () => {
                    setKeySaving(true);
                    try {
                      const token = localStorage.getItem('siigo_token') || '';
                      const res = await fetch('/api/file-download-key', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
                        body: JSON.stringify({ key: newKeyInput.trim() }),
                      });
                      if (res.ok) {
                        setFileDownloadKey(newKeyInput.trim());
                        setNewKeyInput('');
                      }
                    } finally {
                      setKeySaving(false);
                    }
                  }}
                >
                  {keySaving ? 'Guardando…' : 'Guardar'}
                </button>
              </div>
            </div>

            {/* cURL card */}
            {fileDownloadKey && (
              <div className="card" style={{ padding: '1.5rem' }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '0.75rem' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '0.6rem' }}>
                    <span style={{ fontSize: '1.1rem' }}>🔗</span>
                    <h3 style={{ margin: 0, fontSize: '1rem', fontWeight: 600 }}>Descarga por URL / curl</h3>
                  </div>
                  <button
                    className="btn btn-secondary"
                    style={{ fontSize: '0.78rem', padding: '0.25rem 0.65rem' }}
                    onClick={() => {
                      const cmd = `curl -O "http://TU-SERVIDOR:3210/api/siigo-file?name=${isamFileName || 'Z49'}&key=${fileDownloadKey}"`;
                      navigator.clipboard.writeText(cmd).then(() => { setIsamCopied(true); setTimeout(() => setIsamCopied(false), 2000); });
                    }}
                  >
                    {isamCopied ? '✓ Copiado' : 'Copiar'}
                  </button>
                </div>
                <pre style={{
                  background: 'var(--bg-secondary)',
                  border: '1px solid var(--border)',
                  borderRadius: 6,
                  padding: '0.75rem 1rem',
                  fontSize: '0.78rem',
                  fontFamily: 'monospace',
                  overflowX: 'auto',
                  margin: 0,
                  color: 'var(--text)',
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-all',
                }}>
{`curl -O "http://TU-SERVIDOR:3210/api/siigo-file?name=${isamFileName || 'Z49'}&key=${fileDownloadKey}"`}
                </pre>
                <p style={{ color: 'var(--text-secondary)', fontSize: '0.78rem', marginTop: '0.6rem', margin: '0.5rem 0 0' }}>
                  Reemplaza <code>TU-SERVIDOR:3210</code> con la IP o dominio del servidor. La clave viaja en la URL — úsalo solo en redes de confianza o con HTTPS.
                </p>
              </div>
            )}

            {/* Common files reference */}
            <div className="card" style={{ padding: '1.25rem 1.5rem' }}>
              <h3 style={{ margin: '0 0 0.75rem', fontSize: '0.9rem', fontWeight: 600, color: 'var(--text-secondary)' }}>ARCHIVOS FRECUENTES</h3>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '0.35rem 1.5rem', fontSize: '0.82rem' }}>
                {[
                  ['Z49', 'Notas / OC / encabezados doc.'],
                  ['Z072026', 'Cartera CxC año actual'],
                  ['Z092026', 'Movimientos contables año actual'],
                  ['Z17', 'Tipos de documento'],
                  ['Z08A', 'Terceros / clientes'],
                  ['Z04', 'Productos'],
                  ['Z03', 'Plan de cuentas'],
                  ['Z25', 'Saldos terceros'],
                ].map(([name, desc]) => (
                  <div
                    key={name}
                    style={{ display: 'flex', gap: '0.5rem', alignItems: 'baseline', cursor: 'pointer' }}
                    onClick={() => setIsamFileName(name)}
                  >
                    <code style={{ color: 'var(--accent, #6366f1)', minWidth: 80 }}>{name}</code>
                    <span style={{ color: 'var(--text-secondary)' }}>{desc}</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}
      </div>
    </>
  );
}

function formatCell(val: unknown): string {
  if (val === null || val === undefined) return '-';
  const s = String(val);
  return s.length > 120 ? s.substring(0, 120) + '...' : s;
}

function extractTable(query: string): string | null {
  const m = query.match(/FROM\s+(\w+)/i);
  return m ? m[1] : null;
}
