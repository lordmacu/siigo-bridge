import { useState, useEffect } from 'react'
import * as api from '../utils/api'

export default function SchemaList() {
  const [schemas, setSchemas] = useState<any[]>([])
  const [selected, setSelected] = useState<any>(null)
  const [loading, setLoading] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      const s = await api.listSchemas()
      setSchemas(s || [])
    } catch {}
    setLoading(false)
  }

  useEffect(() => { load() }, [])

  const viewSchema = async (name: string) => {
    try {
      const s = await api.getSchema(name)
      setSelected(s)
    } catch (e: any) {
      alert('Error: ' + e.message)
    }
  }

  const deleteSchema = async (name: string) => {
    if (!confirm(`Delete schema "${name}"?`)) return
    try {
      await api.deleteSchema(name)
      setSelected(null)
      load()
    } catch (e: any) {
      alert('Error: ' + e.message)
    }
  }

  return (
    <div>
      <div className="page-header">
        <h2>Saved Schemas</h2>
        <p>Reusable table definitions for creating new ISAM files</p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem' }}>
        {/* Schema list */}
        <div className="card">
          <h3>Schemas ({schemas.length})</h3>
          {loading && <p style={{ color: 'var(--text-muted)' }}>Loading...</p>}
          {schemas.length === 0 && !loading && (
            <div className="empty-state">
              <h3>No schemas saved yet</h3>
              <p>Use the Import Wizard or Create Table to define schemas</p>
            </div>
          )}
          {schemas.map(s => (
            <div key={s.name} className="file-entry" onClick={() => viewSchema(s.name)}
              style={{ borderLeft: selected?.name === s.name ? '3px solid var(--accent)' : undefined }}>
              <span className="icon" style={{ fontSize: '1rem' }}>{'\uD83D\uDCC4'}</span>
              <div style={{ flex: 1 }}>
                <span className="name" style={{ fontWeight: 600 }}>{s.name}</span>
                <br />
                <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                  {s.record_size}B | {s.fields?.length || 0} fields
                  {s.description && ` | ${s.description}`}
                </span>
              </div>
            </div>
          ))}
        </div>

        {/* Schema detail */}
        <div className="card">
          {selected ? (
            <>
              <div className="card-header">
                <h3>{selected.name}</h3>
                <button className="btn btn-sm btn-danger" onClick={() => deleteSchema(selected.name)}>Delete</button>
              </div>
              {selected.description && <p style={{ color: 'var(--text-secondary)', marginBottom: '1rem' }}>{selected.description}</p>}
              <div style={{ fontSize: '0.8125rem', marginBottom: '1rem' }}>
                <span className="badge badge-blue">{selected.record_size} bytes</span>{' '}
                <span className="badge badge-green">{selected.fields?.length} fields</span>{' '}
                {selected.source_file && <span className="badge badge-amber">from {selected.source_file}</span>}
              </div>

              <table className="data-table">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Offset</th>
                    <th>Length</th>
                    <th>Type</th>
                    <th>Key</th>
                  </tr>
                </thead>
                <tbody>
                  {selected.fields?.map((f: any, i: number) => (
                    <tr key={i}>
                      <td className="col-key">{f.name}</td>
                      <td className="col-num">{f.offset}</td>
                      <td className="col-num">{f.length}</td>
                      <td className="col-type">{f.type}</td>
                      <td>{f.is_key ? '\u2705' : ''}</td>
                    </tr>
                  ))}
                </tbody>
              </table>

              {/* Record layout bar */}
              <div style={{ marginTop: '1rem' }}>
                <h4 style={{ fontSize: '0.8125rem', marginBottom: '0.5rem' }}>Record Layout</h4>
                <div style={{ display: 'flex', height: 24, borderRadius: 4, overflow: 'hidden', border: '1px solid var(--border)' }}>
                  {selected.fields?.map((f: any, i: number) => {
                    const pct = (f.length / selected.record_size) * 100
                    const colors = ['#3b82f6', '#22c55e', '#f59e0b', '#a855f7', '#ef4444', '#06b6d4']
                    return (
                      <div key={i} title={`${f.name}: ${f.offset}-${f.offset + f.length}`}
                        style={{
                          width: `${pct}%`,
                          background: colors[i % colors.length],
                          display: 'flex', alignItems: 'center', justifyContent: 'center',
                          fontSize: '0.5625rem', color: 'white', fontWeight: 600,
                        }}>
                        {pct > 8 ? f.name : ''}
                      </div>
                    )
                  })}
                </div>
              </div>

              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '0.5rem' }}>
                Created: {selected.created_at} | Updated: {selected.updated_at}
              </p>
            </>
          ) : (
            <div className="empty-state">
              <h3>Select a schema</h3>
              <p>Click on a schema to view its field definitions</p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
