import { useState, useEffect } from 'react'
import { useParams } from 'react-router-dom'
import * as api from '../utils/api'

export default function TableView() {
  const { name } = useParams<{ name: string }>()
  const [data, setData] = useState<any>(null)
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const [editModal, setEditModal] = useState<any>(null)
  const [insertModal, setInsertModal] = useState(false)
  const [editFields, setEditFields] = useState<Record<string, string>>({})
  const [tables, setTables] = useState<any[]>([])

  const load = async () => {
    if (!name) return
    setLoading(true)
    try {
      const d = await api.getRecords(name, page, 50, search)
      setData(d)
      const t = await api.listTables()
      setTables(t || [])
    } catch (e: any) {
      console.error(e)
    }
    setLoading(false)
  }

  useEffect(() => { load() }, [name, page])

  const currentTable = tables.find((t: any) => t.name === name)
  const hasSchema = currentTable?.schema

  const doSearch = () => { setPage(1); load() }

  const openEdit = (rec: any) => {
    if (rec.fields) {
      setEditFields({ ...rec.fields })
    }
    setEditModal(rec)
  }

  const saveEdit = async () => {
    if (!name || !editModal) return
    try {
      await api.updateRecord(name, editModal._index, editFields)
      setEditModal(null)
      load()
    } catch (e: any) {
      alert('Error: ' + e.message)
    }
  }

  const doDelete = async (index: number) => {
    if (!name || !confirm('Delete this record?')) return
    try {
      await api.deleteRecord(name, index)
      load()
    } catch (e: any) {
      alert('Error: ' + e.message)
    }
  }

  const openInsert = () => {
    if (!hasSchema) return alert('Need a schema to insert records')
    const fields: Record<string, string> = {}
    currentTable.schema.fields.forEach((f: any) => { fields[f.name] = '' })
    setEditFields(fields)
    setInsertModal(true)
  }

  const doInsert = async () => {
    if (!name) return
    try {
      await api.insertRecord(name, editFields)
      setInsertModal(false)
      load()
    } catch (e: any) {
      alert('Error: ' + e.message)
    }
  }

  const columns = data?.records?.[0]?.fields ? Object.keys(data.records[0].fields) : []

  return (
    <div>
      <div className="page-header">
        <h2>Table: {name}</h2>
        <p>
          {data && `${data.total} records | Page ${data.page} of ${data.pages}`}
          {currentTable && ` | Record size: ${currentTable.record_size}B`}
        </p>
      </div>

      {/* Toolbar */}
      <div className="card" style={{ display: 'flex', gap: '0.75rem', alignItems: 'center', padding: '0.75rem 1rem' }}>
        <input
          value={search}
          onChange={e => setSearch(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && doSearch()}
          placeholder="Search records..."
          style={{ width: 300 }}
        />
        <button className="btn" onClick={doSearch}>Search</button>
        <div style={{ flex: 1 }} />
        {hasSchema && <button className="btn btn-success" onClick={openInsert}>+ Insert Record</button>}
        <button className="btn" onClick={load}>Refresh</button>
      </div>

      {/* Data Table */}
      <div className="card" style={{ overflowX: 'auto' }}>
        {loading ? (
          <p style={{ color: 'var(--text-muted)' }}>Loading...</p>
        ) : data?.records?.length ? (
          <table className="data-table">
            <thead>
              <tr>
                <th>#</th>
                {columns.length > 0
                  ? columns.map(c => <th key={c}>{c}</th>)
                  : <th>Raw Data</th>
                }
                {hasSchema && <th>Actions</th>}
              </tr>
            </thead>
            <tbody>
              {data.records.map((rec: any) => (
                <tr key={rec._index}>
                  <td style={{ color: 'var(--text-muted)' }}>{rec._index}</td>
                  {rec.fields
                    ? columns.map(c => (
                        <td key={c} className={getColClass(c)}>{rec.fields[c]}</td>
                      ))
                    : <td><code style={{ fontSize: '0.75rem' }}>{rec.raw}</code></td>
                  }
                  {hasSchema && (
                    <td>
                      <div className="btn-group">
                        <button className="btn btn-sm" onClick={() => openEdit(rec)}>Edit</button>
                        <button className="btn btn-sm btn-danger" onClick={() => doDelete(rec._index)}>Delete</button>
                      </div>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div className="empty-state">
            <h3>No records found</h3>
          </div>
        )}
      </div>

      {/* Pagination */}
      {data && data.pages > 1 && (
        <div className="pagination">
          <button className="btn btn-sm" disabled={page <= 1} onClick={() => setPage(page - 1)}>Prev</button>
          <span className="page-info">Page {page} of {data.pages}</span>
          <button className="btn btn-sm" disabled={page >= data.pages} onClick={() => setPage(page + 1)}>Next</button>
        </div>
      )}

      {/* Edit Modal */}
      {editModal && (
        <div className="modal-overlay" onClick={() => setEditModal(null)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <h3>Edit Record #{editModal._index}</h3>
            {Object.entries(editFields).map(([key, val]) => (
              <div className="form-group" key={key}>
                <label>{key}</label>
                <input value={val} onChange={e => setEditFields({ ...editFields, [key]: e.target.value })} />
              </div>
            ))}
            <div className="btn-group" style={{ marginTop: '1rem' }}>
              <button className="btn btn-primary" onClick={saveEdit}>Save</button>
              <button className="btn" onClick={() => setEditModal(null)}>Cancel</button>
            </div>
          </div>
        </div>
      )}

      {/* Insert Modal */}
      {insertModal && (
        <div className="modal-overlay" onClick={() => setInsertModal(false)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <h3>Insert New Record</h3>
            {Object.entries(editFields).map(([key, val]) => (
              <div className="form-group" key={key}>
                <label>{key}</label>
                <input value={val} onChange={e => setEditFields({ ...editFields, [key]: e.target.value })} />
              </div>
            ))}
            <div className="btn-group" style={{ marginTop: '1rem' }}>
              <button className="btn btn-success" onClick={doInsert}>Insert</button>
              <button className="btn" onClick={() => setInsertModal(false)}>Cancel</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function getColClass(col: string): string {
  const c = col.toLowerCase()
  if (c.includes('codigo') || c.includes('id') || c.includes('key') || c.includes('nit')) return 'col-key'
  if (c.includes('nombre') || c.includes('name') || c.includes('desc')) return 'col-name'
  if (c.includes('fecha') || c.includes('date')) return 'col-date'
  if (c.includes('saldo') || c.includes('valor') || c.includes('cantidad') || c.includes('amount')) return 'col-num'
  if (c.includes('tipo') || c.includes('type')) return 'col-type'
  return ''
}
