import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import * as api from '../utils/api'

type WizardStep = 'select' | 'analyze' | 'fields' | 'save'

interface DetectedField {
  offset: number
  length: number
  type: string
  sample: string
  confidence: number
  name: string
  isKey: boolean
}

export default function ImportWizard() {
  const [searchParams] = useSearchParams()
  const [step, setStep] = useState<WizardStep>('select')
  const [filePath, setFilePath] = useState(searchParams.get('path') || '')
  const [fileInfo, setFileInfo] = useState<any>(null)
  const [hexData, setHexData] = useState<any>(null)
  const [fields, setFields] = useState<DetectedField[]>([])
  const [schemaName, setSchemaName] = useState('')
  const [description, setDescription] = useState('')
  const [selectedRecord, setSelectedRecord] = useState(0)
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  // Auto-analyze if path came from URL
  useEffect(() => {
    if (filePath && step === 'select') {
      analyzeFile()
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Step 1: Select file
  const analyzeFile = async () => {
    if (!filePath.trim()) return
    setLoading(true)
    try {
      const info = await api.getFileInfo(filePath)
      setFileInfo(info)

      const hex = await api.getFileHex(filePath)
      setHexData(hex)

      const detected = await api.detectFields(filePath)

      // Map detected fields with default names
      const mappedFields: DetectedField[] = (detected.detected_fields || []).map(
        (f: any, i: number) => ({
          ...f,
          name: `field_${i + 1}`,
          isKey: i === 0,
        })
      )
      setFields(mappedFields)
      setSchemaName(info.name.toLowerCase())
      setStep('analyze')
    } catch (e: any) {
      alert('Error: ' + e.message)
    }
    setLoading(false)
  }

  // Step 2: Review hex + detected fields
  const goToFields = () => setStep('fields')

  // Step 3: Name fields
  const updateField = (i: number, updates: Partial<DetectedField>) => {
    const newFields = [...fields]
    newFields[i] = { ...newFields[i], ...updates }
    // Ensure only one key
    if (updates.isKey) {
      newFields.forEach((f, j) => { if (j !== i) f.isKey = false })
    }
    setFields(newFields)
  }

  const removeField = (i: number) => {
    setFields(fields.filter((_, j) => j !== i))
  }

  const addField = () => {
    const lastEnd = fields.length > 0 ? fields[fields.length - 1].offset + fields[fields.length - 1].length : 0
    setFields([...fields, {
      offset: lastEnd,
      length: 10,
      type: 'text',
      sample: '',
      confidence: 1,
      name: `field_${fields.length + 1}`,
      isKey: false,
    }])
  }

  // Step 4: Save schema
  const saveSchema = async () => {
    if (!schemaName.trim()) return alert('Schema name is required')
    if (!fields.some(f => f.isKey)) return alert('One field must be marked as key')

    const keyField = fields.find(f => f.isKey)!
    const schema = {
      name: schemaName,
      description: description,
      record_size: fileInfo.record_size,
      key_offset: keyField.offset,
      key_length: keyField.length,
      source_file: filePath,
      fields: fields.map(f => ({
        name: f.name,
        offset: f.offset,
        length: f.length,
        type: f.type === 'date' ? 'date' : f.type === 'numeric' ? 'int' : f.type === 'binary' ? 'bcd' : 'string',
        is_key: f.isKey,
      })),
    }

    try {
      await api.saveSchema(schema)

      // Also open the table with the schema
      await api.openTable(filePath, schemaName, schemaName)

      alert('Schema saved! Table opened.')
      navigate(`/table/${schemaName}`)
    } catch (e: any) {
      alert('Error saving: ' + e.message)
    }
  }

  return (
    <div>
      <div className="page-header">
        <h2>Import Wizard</h2>
        <p>Load an ISAM file, detect fields, name columns, and save the schema</p>
      </div>

      {/* Steps indicator */}
      <div className="tabs" style={{ marginBottom: '1.5rem' }}>
        <div className={`tab ${step === 'select' ? 'active' : ''}`}>1. Select File</div>
        <div className={`tab ${step === 'analyze' ? 'active' : ''}`}>2. Analyze</div>
        <div className={`tab ${step === 'fields' ? 'active' : ''}`}>3. Define Fields</div>
        <div className={`tab ${step === 'save' ? 'active' : ''}`}>4. Save Schema</div>
      </div>

      {/* Step 1: Select file */}
      {step === 'select' && (
        <div className="card">
          <h3>Select ISAM File</h3>
          <div className="form-group" style={{ marginTop: '1rem' }}>
            <label>Full path to ISAM file</label>
            <div style={{ display: 'flex', gap: '0.5rem' }}>
              <input
                value={filePath}
                onChange={e => setFilePath(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && analyzeFile()}
                placeholder="C:\SIIWI02\Z17"
              />
              <button className="btn btn-primary" onClick={analyzeFile} disabled={loading}>
                {loading ? 'Analyzing...' : 'Analyze'}
              </button>
            </div>
          </div>
          <p style={{ fontSize: '0.8125rem', color: 'var(--text-muted)', marginTop: '0.5rem' }}>
            Enter the full path to any Micro Focus ISAM file. The wizard will auto-detect
            field boundaries and let you name them.
          </p>
        </div>
      )}

      {/* Step 2: Analyze - show hex + detected fields */}
      {step === 'analyze' && fileInfo && hexData && (
        <div>
          <div className="card">
            <div className="card-header">
              <h3>File Analysis: {fileInfo.name}</h3>
              <div className="btn-group">
                <span className="badge badge-blue">{fileInfo.record_size} B/record</span>
                <span className="badge badge-green">{fileInfo.record_count} records</span>
              </div>
            </div>

            {/* Field map visualization */}
            <h4 style={{ fontSize: '0.875rem', marginBottom: '0.5rem' }}>Detected Field Map</h4>
            <div className="field-map">
              {fields.map((f, i) =>
                Array.from({ length: f.length }, (_, byteIdx) => (
                  <div
                    key={`${f.offset + byteIdx}`}
                    className={`byte-cell ${f.type}`}
                    title={`${f.name} [${f.offset}+${f.length}] = ${f.type}`}
                  >
                    {byteIdx === 0 ? i + 1 : ''}
                  </div>
                ))
              )}
            </div>

            <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', margin: '0.5rem 0' }}>
              {fields.length} fields detected. Colors:
              <span style={{ color: 'var(--accent-light)' }}> text</span>,
              <span style={{ color: 'var(--success)' }}> numeric</span>,
              <span style={{ color: '#c4b5fd' }}> date</span>,
              <span style={{ color: 'var(--danger)' }}> binary/BCD</span>
            </div>
          </div>

          {/* Hex viewer */}
          <div className="card">
            <div className="card-header">
              <h3>Hex View</h3>
              <select value={selectedRecord} onChange={e => setSelectedRecord(Number(e.target.value))} style={{ width: 150 }}>
                {hexData.samples?.map((_: any, i: number) => (
                  <option key={i} value={i}>Record {i}</option>
                ))}
              </select>
            </div>
            {hexData.samples?.[selectedRecord] && (
              <div className="hex-viewer">
                {hexData.samples[selectedRecord].hex.map((line: string, i: number) => (
                  <div key={i}>
                    <span className="offset">{(i * 16).toString(16).padStart(4, '0')}</span>{'  '}
                    <span>{line.split(' ').map((b: string, j: number) => (
                      <span key={j} className={`hex-byte ${b === '00' ? 'null' : ''}`}>{b} </span>
                    ))}</span>
                    {'  '}
                    <span className="ascii">
                      {hexData.samples[selectedRecord].ascii.substring(i * 16, (i + 1) * 16)}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div style={{ marginTop: '1rem' }}>
            <button className="btn" onClick={() => setStep('select')}>Back</button>
            {' '}
            <button className="btn btn-primary" onClick={goToFields}>Next: Define Fields</button>
          </div>
        </div>
      )}

      {/* Step 3: Define/name fields */}
      {step === 'fields' && (
        <div className="card">
          <div className="card-header">
            <h3>Define Fields ({fileInfo?.record_size} bytes total)</h3>
            <button className="btn btn-sm" onClick={addField}>+ Add Field</button>
          </div>

          {/* Header */}
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
              <input value={f.name} onChange={e => updateField(i, { name: e.target.value })} placeholder="field name" />
              <input type="number" value={f.offset} onChange={e => updateField(i, { offset: Number(e.target.value) })} />
              <input type="number" value={f.length} onChange={e => updateField(i, { length: Number(e.target.value) })} />
              <select value={f.type} onChange={e => updateField(i, { type: e.target.value })}>
                <option value="text">Text</option>
                <option value="numeric">Numeric</option>
                <option value="date">Date (YYYYMMDD)</option>
                <option value="binary">BCD / Binary</option>
              </select>
              <input type="checkbox" checked={f.isKey} onChange={e => updateField(i, { isKey: e.target.checked })}
                style={{ width: 'auto' }} />
              <button className="btn btn-sm btn-danger" onClick={() => removeField(i)}>X</button>
            </div>
          ))}

          {/* Sample preview */}
          {hexData?.samples?.[0] && fields.length > 0 && (
            <div style={{ marginTop: '1rem' }}>
              <h4 style={{ fontSize: '0.875rem', marginBottom: '0.5rem' }}>Preview (Record 0)</h4>
              <table className="data-table">
                <thead>
                  <tr>
                    {fields.map(f => <th key={f.name}>{f.name}</th>)}
                  </tr>
                </thead>
                <tbody>
                  {hexData.samples.slice(0, 5).map((sample: any, si: number) => (
                    <tr key={si}>
                      {fields.map(f => (
                        <td key={f.name}>{sample.ascii.substring(f.offset, f.offset + f.length).trim()}</td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <div style={{ marginTop: '1rem' }}>
            <button className="btn" onClick={() => setStep('analyze')}>Back</button>
            {' '}
            <button className="btn btn-primary" onClick={() => setStep('save')}>Next: Save Schema</button>
          </div>
        </div>
      )}

      {/* Step 4: Save schema */}
      {step === 'save' && (() => {
        const keyField = fields.find(f => f.isKey)
        const schemaJson = {
          name: schemaName || 'unnamed',
          description,
          record_size: fileInfo?.record_size,
          key_offset: keyField?.offset ?? 0,
          key_length: keyField?.length ?? 0,
          source_file: filePath,
          fields: fields.map(f => ({
            name: f.name,
            offset: f.offset,
            length: f.length,
            type: f.type === 'date' ? 'date' : f.type === 'numeric' ? 'int' : f.type === 'binary' ? 'bcd' : 'string',
            is_key: f.isKey,
          })),
        }
        const jsonStr = JSON.stringify(schemaJson, null, 2)

        const downloadJson = () => {
          const blob = new Blob([jsonStr], { type: 'application/json' })
          const url = URL.createObjectURL(blob)
          const a = document.createElement('a')
          a.href = url
          a.download = `${schemaName || 'schema'}.json`
          a.click()
          URL.revokeObjectURL(url)
        }

        return (
          <div>
            <div className="card">
              <h3>Save Schema</h3>
              <div className="form-group" style={{ marginTop: '1rem' }}>
                <label>Schema Name</label>
                <input value={schemaName} onChange={e => setSchemaName(e.target.value)} placeholder="my_table" />
              </div>
              <div className="form-group">
                <label>Description (optional)</label>
                <input value={description} onChange={e => setDescription(e.target.value)} placeholder="Describe this table..." />
              </div>

              <div style={{ background: 'var(--bg-primary)', border: '1px solid var(--border)', borderRadius: 'var(--radius)', padding: '1rem', margin: '1rem 0' }}>
                <h4 style={{ fontSize: '0.875rem', marginBottom: '0.5rem' }}>Summary</h4>
                <p style={{ fontSize: '0.8125rem' }}>
                  Record size: <strong>{fileInfo?.record_size}</strong> bytes<br />
                  Fields: <strong>{fields.length}</strong><br />
                  Key: <strong>{keyField?.name || 'none'}</strong>
                  {keyField && <> (offset {keyField.offset}, length {keyField.length})</>}<br />
                  Source: <code>{filePath}</code>
                </p>
              </div>

              <div className="btn-group">
                <button className="btn" onClick={() => setStep('fields')}>Back</button>
                <button className="btn btn-success" onClick={saveSchema}>Save Schema & Open Table</button>
              </div>
            </div>

            {/* JSON Preview */}
            <div className="card" style={{ marginTop: '1rem' }}>
              <div className="card-header">
                <h3>Schema JSON</h3>
                <button className="btn btn-sm" onClick={downloadJson}>Download .json</button>
              </div>
              <pre style={{
                background: 'var(--bg-primary)', border: '1px solid var(--border)',
                borderRadius: 'var(--radius)', padding: '1rem', fontSize: '0.75rem',
                maxHeight: 400, overflowY: 'auto', color: 'var(--accent-light)',
              }}>
                {jsonStr}
              </pre>
              <p style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '0.5rem' }}>
                This JSON is saved to <code>data/schemas/{schemaName || 'name'}.json</code> on the server
                and can be reused to create new ISAM files with the same structure.
              </p>
            </div>
          </div>
        )
      })()}
    </div>
  )
}
