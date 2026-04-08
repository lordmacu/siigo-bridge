import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import * as api from '../utils/api'

const STORAGE_KEY = 'isam-admin-last-path'
const HISTORY_KEY = 'isam-admin-path-history'
const MAX_HISTORY = 10

function getLastPath(): string {
  return localStorage.getItem(STORAGE_KEY) || 'C:\\'
}

function savePath(p: string) {
  localStorage.setItem(STORAGE_KEY, p)
  // Update history
  const history: string[] = JSON.parse(localStorage.getItem(HISTORY_KEY) || '[]')
  const filtered = history.filter(h => h !== p)
  filtered.unshift(p)
  localStorage.setItem(HISTORY_KEY, JSON.stringify(filtered.slice(0, MAX_HISTORY)))
}

function getHistory(): string[] {
  return JSON.parse(localStorage.getItem(HISTORY_KEY) || '[]')
}

export default function Dashboard() {
  const [path, setPath] = useState(getLastPath)
  const [entries, setEntries] = useState<any[]>([])
  const [parent, setParent] = useState('')
  const [tables, setTables] = useState<any[]>([])
  const [loading, setLoading] = useState(false)
  const [schemas, setSchemas] = useState<any[]>([])
  const [showHistory, setShowHistory] = useState(false)
  const [showAll, setShowAll] = useState(false)
  const navigate = useNavigate()

  const browse = async (dir: string) => {
    setLoading(true)
    setShowHistory(false)
    try {
      const data = await api.browseFiles(dir)
      setPath(data.path)
      setParent(data.parent)
      setEntries(data.entries || [])
      savePath(data.path)
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

  const hasSchemaFor = (entry: any): any => {
    // Check if any saved schema matches this file (by source_file or by name)
    const entryName = entry.name.replace(/\.[^/.]+$/, '').toLowerCase()
    return schemas.find((s: any) =>
      s.source_file === entry.path ||
      s.name.toLowerCase() === entryName
    )
  }

  const openISAM = async (entry: any) => {
    const schema = hasSchemaFor(entry)
    const name = entry.name.replace(/\.[^/.]+$/, '')
    try {
      await api.openTable(entry.path, name, schema?.name)
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
          <h3>File Browser
            {entries.length > 0 && (
              <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)', fontWeight: 400, marginLeft: 8 }}>
                {entries.filter((e: any) => e.is_isam && e.records > 0).length} ISAM / {entries.filter((e: any) => e.is_dir && !e.is_empty && e.has_isam).length} dirs
                {!showAll && entries.filter((e: any) => !e.is_dir && !e.is_isam).length > 0 && (
                  <> &middot; <a href="#" onClick={e => { e.preventDefault(); setShowAll(!showAll) }} style={{ fontSize: '0.75rem' }}>
                    {showAll ? 'ISAM only' : `show all (${entries.length})`}
                  </a></>
                )}
                {showAll && (
                  <> &middot; <a href="#" onClick={e => { e.preventDefault(); setShowAll(false) }} style={{ fontSize: '0.75rem' }}>
                    ISAM only
                  </a></>
                )}
              </span>
            )}
          </h3>
          <div style={{ position: 'relative' }}>
            <div className="btn-group">
              <input
                value={path}
                onChange={e => setPath(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && browse(path)}
                onFocus={() => getHistory().length > 1 && setShowHistory(true)}
                style={{ width: 400 }}
                placeholder="Enter path..."
              />
              <button className="btn btn-primary" onClick={() => browse(path)}>Go</button>
              {getHistory().length > 1 && (
                <button className="btn btn-sm" onClick={() => setShowHistory(!showHistory)}
                  title="Recent paths">
                  {'\u25BC'}
                </button>
              )}
            </div>
            {showHistory && (
              <div style={{
                position: 'absolute', top: '100%', left: 0, right: 0, marginTop: 4,
                background: 'var(--bg-secondary)', border: '1px solid var(--border)',
                borderRadius: 'var(--radius)', zIndex: 50, maxHeight: 200, overflowY: 'auto',
                boxShadow: 'var(--shadow)',
              }}>
                {getHistory().map((h, i) => (
                  <div key={i} onClick={() => browse(h)}
                    style={{
                      padding: '0.4rem 0.75rem', cursor: 'pointer', fontSize: '0.8125rem',
                      color: h === path ? 'var(--accent-light)' : 'var(--text-secondary)',
                      borderBottom: '1px solid var(--border)',
                    }}
                    onMouseEnter={e => (e.currentTarget.style.background = 'var(--bg-tertiary)')}
                    onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
                  >
                    {h}
                  </div>
                ))}
              </div>
            )}
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
          {entries
            .filter((e: any) => {
              if (e.is_isam && e.records > 0) return true
              if (e.is_dir && !e.is_empty && e.has_isam) return true
              return showAll
            })
            .sort((a: any, b: any) => {
              // Directories first, then ISAM files sorted by records (most first), then others by size
              if (a.is_dir && !b.is_dir) return -1
              if (!a.is_dir && b.is_dir) return 1
              if (a.is_dir && b.is_dir) return a.name.localeCompare(b.name)
              return (b.records || b.size || 0) - (a.records || a.size || 0)
            })
            .map((e: any, i: number) => {
            const schema = e.is_isam ? hasSchemaFor(e) : null
            return (
              <div
                key={i}
                className={`file-entry ${e.is_dir ? 'dir' : ''} ${e.is_isam ? 'isam' : ''}`}
                onClick={() => e.is_dir ? browse(e.path) : null}
              >
                <span className="icon">{e.is_dir ? '\uD83D\uDCC1' : e.is_isam ? '\uD83D\uDDC3' : '\uD83D\uDCC4'}</span>
                <span className="name" style={{ flex: 1 }}>
                  {e.name}
                  {e.is_isam && (
                    <span style={{ marginLeft: 8 }}>
                      <span className="badge badge-blue">{e.rec_size}B</span>
                      {' '}
                      <span className="badge badge-green">{e.records} records</span>
                      {schema && <span className="badge badge-green" style={{ marginLeft: 4 }}>schema: {schema.name}</span>}
                    </span>
                  )}
                </span>
                {e.is_isam && (
                  <span className="btn-group" onClick={ev => ev.stopPropagation()}>
                    {schema ? (
                      <button className="btn btn-sm btn-primary" onClick={() => openISAM(e)}>Open</button>
                    ) : (
                      <>
                        <button className="btn btn-sm btn-primary" onClick={() => navigate(`/import?path=${encodeURIComponent(e.path)}`)}>Import</button>
                        <button className="btn btn-sm" onClick={() => openISAM(e)}>Open raw</button>
                      </>
                    )}
                  </span>
                )}
                {!e.is_isam && !e.is_dir && <span className="meta">{formatSize(e.size)}</span>}
              </div>
            )
          })}
          {entries.filter((e: any) => showAll || e.is_isam || (e.is_dir && !e.is_empty && e.has_isam)).length === 0 && !loading && (
            <div className="empty-state">
              <h3>{entries.length > 0 ? 'No ISAM files here' : 'Empty directory'}</h3>
              {entries.length > 0 && !showAll && (
                <p style={{ marginTop: '0.5rem' }}>
                  {entries.length} file(s) hidden.{' '}
                  <button className="btn btn-sm" onClick={() => setShowAll(true)}>Show all files</button>
                </p>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
