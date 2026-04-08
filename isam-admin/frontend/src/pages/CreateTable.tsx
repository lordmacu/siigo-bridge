import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import * as api from '../utils/api'

interface FieldDef {
  name: string
  offset: number
  length: number
  type: string
  isKey: boolean
  decimals: number
}

export default function CreateTable() {
  const [name, setName] = useState('')
  const [filePath, setFilePath] = useState('')
  const [recordSize, setRecordSize] = useState(256)
  const [fields, setFields] = useState<FieldDef[]>([
    { name: 'id', offset: 0, length: 10, type: 'string', isKey: true, decimals: 0 },
  ])
  const [schemas, setSchemas] = useState<any[]>([])
  const [fromSchema, setFromSchema] = useState('')
  const navigate = useNavigate()

  useEffect(() => {
    api.listSchemas().then(s => setSchemas(s || [])).catch(() => {})
  }, [])

  const addField = () => {
    const lastEnd = fields.length > 0 ? fields[fields.length - 1].offset + fields[fields.length - 1].length : 0
    setFields([...fields, {
      name: `field_${fields.length + 1}`,
      offset: lastEnd,
      length: 20,
      type: 'string',
      isKey: false,
      decimals: 0,
    }])
  }

  const updateField = (i: number, updates: Partial<FieldDef>) => {
    const newFields = [...fields]
    newFields[i] = { ...newFields[i], ...updates }
    if (updates.isKey) {
      newFields.forEach((f, j) => { if (j !== i) f.isKey = false })
    }
    setFields(newFields)
  }

  const removeField = (i: number) => setFields(fields.filter((_, j) => j !== i))

  const autoOffset = () => {
    let offset = 0
    const newFields = fields.map(f => {
      const updated = { ...f, offset }
      offset += f.length
      return updated
    })
    setFields(newFields)
    setRecordSize(offset)
  }

  const loadFromSchema = async () => {
    if (!fromSchema) return
    try {
      const schema = await api.getSchema(fromSchema)
      setName(schema.name)
      setRecordSize(schema.record_size)
      setFields(schema.fields.map((f: any) => ({
        name: f.name,
        offset: f.offset,
        length: f.length,
        type: f.type || 'string',
        isKey: f.is_key || false,
        decimals: f.decimals || 0,
      })))
    } catch (e: any) {
      alert('Error loading schema: ' + e.message)
    }
  }

  const createFile = async () => {
    if (!name.trim()) return alert('Name is required')
    if (!filePath.trim()) return alert('File path is required')
    if (!fields.some(f => f.isKey)) return alert('One field must be the key')

    // First save as schema
    const keyField = fields.find(f => f.isKey)!
    const schema = {
      name,
      description: `Created from ISAM Admin`,
      record_size: recordSize,
      key_offset: keyField.offset,
      key_length: keyField.length,
      fields: fields.map(f => ({
        name: f.name,
        offset: f.offset,
        length: f.length,
        type: f.type,
        is_key: f.isKey,
        decimals: f.decimals,
      })),
    }

    try {
      await api.saveSchema(schema)
      await api.createFileFromSchema(name, filePath)
      await api.openTable(filePath, name, name)
      alert('File created successfully!')
      navigate(`/table/${name}`)
    } catch (e: any) {
      alert('Error: ' + e.message)
    }
  }

  // Calculate used bytes
  const usedBytes = fields.reduce((max, f) => Math.max(max, f.offset + f.length), 0)

  return (
    <div>
      <div className="page-header">
        <h2>Create New ISAM File</h2>
        <p>Define the schema and create a new ISAM file from scratch</p>
      </div>

      {/* Load from existing schema */}
      {schemas.length > 0 && (
        <div className="card">
          <div className="card-header">
            <h3>Load from Existing Schema</h3>
            <div style={{ display: 'flex', gap: '0.5rem' }}>
              <select value={fromSchema} onChange={e => setFromSchema(e.target.value)} style={{ width: 200 }}>
                <option value="">Select schema...</option>
                {schemas.map(s => <option key={s.name} value={s.name}>{s.name}</option>)}
              </select>
              <button className="btn" onClick={loadFromSchema}>Load</button>
            </div>
          </div>
        </div>
      )}

      {/* Basic info */}
      <div className="card">
        <h3>File Settings</h3>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 120px', gap: '1rem', marginTop: '1rem' }}>
          <div className="form-group">
            <label>Table Name</label>
            <input value={name} onChange={e => setName(e.target.value)} placeholder="my_table" />
          </div>
          <div className="form-group">
            <label>File Path</label>
            <input value={filePath} onChange={e => setFilePath(e.target.value)} placeholder="C:\DATA\MYFILE" />
          </div>
          <div className="form-group">
            <label>Record Size</label>
            <input type="number" value={recordSize} onChange={e => setRecordSize(Number(e.target.value))} />
          </div>
        </div>
        <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
          Used: {usedBytes}/{recordSize} bytes ({recordSize - usedBytes} free)
        </p>
      </div>

      {/* Field definitions */}
      <div className="card">
        <div className="card-header">
          <h3>Fields ({fields.length})</h3>
          <div className="btn-group">
            <button className="btn btn-sm" onClick={autoOffset}>Auto-offset</button>
            <button className="btn btn-sm btn-primary" onClick={addField}>+ Add Field</button>
          </div>
        </div>

        <div className="field-row" style={{ fontWeight: 600, color: 'var(--text-secondary)', fontSize: '0.75rem' }}>
          <span>Name</span>
          <span>Offset</span>
          <span>Length</span>
          <span>Type</span>
          <span>Key</span>
          <span></span>
        </div>

        {fields.map((f, i) => (
          <div className="field-row" key={i}>
            <input value={f.name} onChange={e => updateField(i, { name: e.target.value })} />
            <input type="number" value={f.offset} onChange={e => updateField(i, { offset: Number(e.target.value) })} />
            <input type="number" value={f.length} onChange={e => updateField(i, { length: Number(e.target.value) })} />
            <select value={f.type} onChange={e => updateField(i, { type: e.target.value })}>
              <option value="string">String</option>
              <option value="int">Integer</option>
              <option value="float">Float</option>
              <option value="date">Date</option>
              <option value="bcd">BCD (Packed)</option>
            </select>
            <input type="checkbox" checked={f.isKey} onChange={e => updateField(i, { isKey: e.target.checked })}
              style={{ width: 'auto' }} />
            <button className="btn btn-sm btn-danger" onClick={() => removeField(i)}>X</button>
          </div>
        ))}
      </div>

      {/* Record layout visualization */}
      <div className="card">
        <h3 style={{ marginBottom: '0.5rem' }}>Record Layout</h3>
        <div style={{ display: 'flex', height: 30, borderRadius: 4, overflow: 'hidden', border: '1px solid var(--border)' }}>
          {fields.map((f, i) => {
            const pct = (f.length / recordSize) * 100
            const colors = ['#3b82f6', '#22c55e', '#f59e0b', '#a855f7', '#ef4444', '#06b6d4', '#f97316']
            return (
              <div key={i} title={`${f.name}: offset ${f.offset}, ${f.length} bytes`}
                style={{
                  width: `${pct}%`,
                  background: colors[i % colors.length],
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                  fontSize: '0.625rem', color: 'white', fontWeight: 600,
                  minWidth: pct > 3 ? undefined : 2,
                }}>
                {pct > 5 ? f.name : ''}
              </div>
            )
          })}
          {usedBytes < recordSize && (
            <div style={{
              width: `${((recordSize - usedBytes) / recordSize) * 100}%`,
              background: 'var(--bg-tertiary)',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
              fontSize: '0.625rem', color: 'var(--text-muted)',
            }}>
              free
            </div>
          )}
        </div>
      </div>

      <div style={{ marginTop: '1rem' }}>
        <button className="btn btn-success" onClick={createFile} style={{ padding: '0.75rem 2rem', fontSize: '1rem' }}>
          Create ISAM File
        </button>
      </div>
    </div>
  )
}
