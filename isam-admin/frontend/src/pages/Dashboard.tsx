import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import * as api from '../utils/api'

export default function Dashboard() {
  const [path, setPath] = useState('C:\\')
  const [entries, setEntries] = useState<any[]>([])
  const [parent, setParent] = useState('')
  const [tables, setTables] = useState<any[]>([])
  const [loading, setLoading] = useState(false)
  const [schemas, setSchemas] = useState<any[]>([])
  const navigate = useNavigate()

  const browse = async (dir: string) => {
    setLoading(true)
    try {
      const data = await api.browseFiles(dir)
      setPath(data.path)
      setParent(data.parent)
      setEntries(data.entries || [])
    } catch (e: any) {
      console.error(e)
    }
    setLoading(false)
  }

  const refreshTables = async () => {
    try {
      const t = await api.listTables()
      setTables(t || [])
    } catch {}
  }

  const loadSchemas = async () => {
    try {
      const s = await api.listSchemas()
      setSchemas(s || [])
    } catch {}
  }

  useEffect(() => { browse(path); refreshTables(); loadSchemas() }, [])

  const openISAM = async (entry: any) => {
    // Check if we have a matching schema
    const name = entry.name.replace(/\.[^/.]+$/, '')
    try {
      await api.openTable(entry.path, name)
      await refreshTables()
      navigate(`/table/${name}`)
    } catch (e: any) {
      alert('Error opening: ' + e.message)
    }
  }

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return bytes + ' B'
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
  }

  return (
    <div>
      <div className="page-header">
        <h2>Dashboard</h2>
        <p>Browse ISAM files and manage opened tables</p>
      </div>

      {/* Opened Tables */}
      {tables.length > 0 && (
        <div className="card" style={{ marginBottom: '1.5rem' }}>
          <div className="card-header">
            <h3>Opened Tables ({tables.length})</h3>
          </div>
          <table className="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Path</th>
                <th>Record Size</th>
                <th>Records</th>
                <th>Schema</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {tables.map((t: any) => (
                <tr key={t.name}>
                  <td className="col-key">{t.name}</td>
                  <td style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>{t.path}</td>
                  <td className="col-num">{t.record_size} B</td>
                  <td className="col-num">{t.record_count}</td>
                  <td>{t.schema ? <span className="badge badge-green">Yes</span> : <span className="badge badge-amber">No</span>}</td>
                  <td>
                    <div className="btn-group">
                      <button className="btn btn-sm btn-primary" onClick={() => navigate(`/table/${t.name}`)}>View</button>
                      <button className="btn btn-sm" onClick={async () => { await api.closeTable(t.name); refreshTables() }}>Close</button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* File Browser */}
      <div className="card">
        <div className="card-header">
          <h3>File Browser</h3>
          <div className="btn-group">
            <input
              value={path}
              onChange={e => setPath(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && browse(path)}
              style={{ width: 400 }}
              placeholder="Enter path..."
            />
            <button className="btn btn-primary" onClick={() => browse(path)}>Go</button>
          </div>
        </div>

        {loading && <p style={{ color: 'var(--text-muted)' }}>Loading...</p>}

        <div>
          {parent && parent !== path && (
            <div className="file-entry dir" onClick={() => browse(parent)}>
              <span className="icon">..</span>
              <span className="name">(parent directory)</span>
            </div>
          )}
          {entries.map((e: any, i: number) => (
            <div
              key={i}
              className={`file-entry ${e.is_dir ? 'dir' : ''} ${e.is_isam ? 'isam' : ''}`}
              onClick={() => e.is_dir ? browse(e.path) : e.is_isam ? openISAM(e) : null}
            >
              <span className="icon">{e.is_dir ? '\uD83D\uDCC1' : e.is_isam ? '\uD83D\uDDC3' : '\uD83D\uDCC4'}</span>
              <span className="name">
                {e.name}
                {e.is_isam && (
                  <span style={{ marginLeft: 8 }}>
                    <span className="badge badge-blue">{e.rec_size}B</span>
                    {' '}
                    <span className="badge badge-green">{e.records} records</span>
                  </span>
                )}
              </span>
              <span className="meta">{!e.is_dir && formatSize(e.size)}</span>
            </div>
          ))}
          {entries.length === 0 && !loading && (
            <div className="empty-state">
              <h3>Empty directory</h3>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
