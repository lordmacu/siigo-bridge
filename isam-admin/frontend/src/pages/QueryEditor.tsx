import { useState, useEffect } from 'react'
import * as api from '../utils/api'

interface Filter {
  field: string
  operator: string
  value: string
}

export default function QueryEditor() {
  const [tables, setTables] = useState<any[]>([])
  const [selectedTable, setSelectedTable] = useState('')
  const [filters, setFilters] = useState<Filter[]>([{ field: '', operator: '=', value: '' }])
  const [orderBy, setOrderBy] = useState('')
  const [orderDir, setOrderDir] = useState('asc')
  const [limit, setLimit] = useState(100)
  const [groupBy, setGroupBy] = useState('')
  const [distinct, setDistinct] = useState('')
  const [results, setResults] = useState<any>(null)
  const [loading, setLoading] = useState(false)
  const [explain, setExplain] = useState('')

  useEffect(() => {
    api.listTables().then(t => setTables(t || [])).catch(() => {})
  }, [])

  const currentTable = tables.find(t => t.name === selectedTable)
  const schemaFields = currentTable?.schema?.fields || []

  const addFilter = () => setFilters([...filters, { field: '', operator: '=', value: '' }])
  const removeFilter = (i: number) => setFilters(filters.filter((_, j) => j !== i))
  const updateFilter = (i: number, updates: Partial<Filter>) => {
    const f = [...filters]
    f[i] = { ...f[i], ...updates }
    setFilters(f)
  }

  const executeQuery = async () => {
    if (!selectedTable) return alert('Select a table first')
    setLoading(true)
    try {
      const validFilters = filters.filter(f => f.field && f.value)
      const query: any = {
        table: selectedTable,
        filters: validFilters,
        limit,
      }
      if (orderBy) { query.order_by = orderBy; query.order_dir = orderDir }
      if (groupBy) query.group_by = groupBy
      if (distinct) query.distinct = distinct

      const res = await api.executeQuery(query)
      setResults(res)
      setExplain(res.explain || '')
    } catch (e: any) {
      alert('Query error: ' + e.message)
    }
    setLoading(false)
  }

  return (
    <div>
      <div className="page-header">
        <h2>Query Editor</h2>
        <p>Build and execute queries against opened tables</p>
      </div>

      {/* Query Builder */}
      <div className="card">
        <h3>Query Builder</h3>

        {/* Table selection */}
        <div className="form-group" style={{ marginTop: '1rem' }}>
          <label>Table</label>
          <select value={selectedTable} onChange={e => setSelectedTable(e.target.value)}>
            <option value="">Select table...</option>
            {tables.map(t => (
              <option key={t.name} value={t.name}>
                {t.name} ({t.record_count} records){!t.schema ? ' [no schema]' : ''}
              </option>
            ))}
          </select>
        </div>

        {/* Filters */}
        {schemaFields.length > 0 && (
          <div style={{ marginTop: '1rem' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <label>WHERE</label>
              <button className="btn btn-sm" onClick={addFilter}>+ Filter</button>
            </div>
            {filters.map((f, i) => (
              <div key={i} style={{ display: 'flex', gap: '0.5rem', marginTop: '0.5rem' }}>
                <select value={f.field} onChange={e => updateFilter(i, { field: e.target.value })} style={{ width: 200 }}>
                  <option value="">Field...</option>
                  {schemaFields.map((sf: any) => <option key={sf.name} value={sf.name}>{sf.name}</option>)}
                </select>
                <select value={f.operator} onChange={e => updateFilter(i, { operator: e.target.value })} style={{ width: 140 }}>
                  <option value="=">=</option>
                  <option value="!=">!=</option>
                  <option value="contains">contains</option>
                  <option value="starts_with">starts with</option>
                  <option value="ends_with">ends with</option>
                  <option value=">">{'>'}</option>
                  <option value="<">{'<'}</option>
                  <option value=">=">{'≥'}</option>
                  <option value="<=">{'≤'}</option>
                </select>
                <input value={f.value} onChange={e => updateFilter(i, { value: e.target.value })}
                  placeholder="value" style={{ flex: 1 }}
                  onKeyDown={e => e.key === 'Enter' && executeQuery()} />
                {filters.length > 1 && (
                  <button className="btn btn-sm btn-danger" onClick={() => removeFilter(i)}>X</button>
                )}
              </div>
            ))}
          </div>
        )}

        {/* Order / Limit / Group */}
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 100px 1fr 1fr 100px', gap: '0.75rem', marginTop: '1rem' }}>
          <div className="form-group">
            <label>ORDER BY</label>
            <select value={orderBy} onChange={e => setOrderBy(e.target.value)}>
              <option value="">None</option>
              {schemaFields.map((f: any) => <option key={f.name} value={f.name}>{f.name}</option>)}
            </select>
          </div>
          <div className="form-group">
            <label>Dir</label>
            <select value={orderDir} onChange={e => setOrderDir(e.target.value)}>
              <option value="asc">ASC</option>
              <option value="desc">DESC</option>
            </select>
          </div>
          <div className="form-group">
            <label>GROUP BY</label>
            <select value={groupBy} onChange={e => { setGroupBy(e.target.value); setDistinct('') }}>
              <option value="">None</option>
              {schemaFields.map((f: any) => <option key={f.name} value={f.name}>{f.name}</option>)}
            </select>
          </div>
          <div className="form-group">
            <label>DISTINCT</label>
            <select value={distinct} onChange={e => { setDistinct(e.target.value); setGroupBy('') }}>
              <option value="">None</option>
              {schemaFields.map((f: any) => <option key={f.name} value={f.name}>{f.name}</option>)}
            </select>
          </div>
          <div className="form-group">
            <label>LIMIT</label>
            <input type="number" value={limit} onChange={e => setLimit(Number(e.target.value))} />
          </div>
        </div>

        <button className="btn btn-primary" onClick={executeQuery} disabled={loading} style={{ marginTop: '0.5rem' }}>
          {loading ? 'Running...' : 'Execute Query'}
        </button>
      </div>

      {/* Results */}
      {results && (
        <div className="card" style={{ overflowX: 'auto' }}>
          <div className="card-header">
            <h3>Results ({results.count || 0})</h3>
            {results.type && <span className="badge badge-blue">{results.type}</span>}
          </div>

          {/* Standard records */}
          {results.type === 'records' && results.records?.length > 0 && (
            <table className="data-table">
              <thead>
                <tr>
                  <th>#</th>
                  {Object.keys(results.records[0].fields || {}).map(k => <th key={k}>{k}</th>)}
                </tr>
              </thead>
              <tbody>
                {results.records.map((r: any) => (
                  <tr key={r._index}>
                    <td style={{ color: 'var(--text-muted)' }}>{r._index}</td>
                    {Object.entries(r.fields || {}).map(([k, v]: any) => (
                      <td key={k}>{v}</td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          {/* Group by results */}
          {results.type === 'group_by' && results.groups && (
            <table className="data-table">
              <thead>
                <tr><th>{results.field}</th><th>Count</th></tr>
              </thead>
              <tbody>
                {Object.entries(results.groups).sort(([,a]: any, [,b]: any) => b - a).map(([k, v]: any) => (
                  <tr key={k}><td className="col-key">{k || '(empty)'}</td><td className="col-num">{v}</td></tr>
                ))}
              </tbody>
            </table>
          )}

          {/* Distinct results */}
          {results.type === 'distinct' && results.values && (
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: '0.5rem' }}>
              {results.values.map((v: string, i: number) => (
                <span key={i} className="badge badge-blue">{v || '(empty)'}</span>
              ))}
            </div>
          )}

          {results.type === 'records' && (!results.records || results.records.length === 0) && (
            <div className="empty-state"><h3>No records match</h3></div>
          )}
        </div>
      )}

      {/* Explain */}
      {explain && (
        <div className="card">
          <h3>Query Plan</h3>
          <pre style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', whiteSpace: 'pre-wrap' }}>{explain}</pre>
        </div>
      )}
    </div>
  )
}
