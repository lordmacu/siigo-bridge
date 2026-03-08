import { useState, useEffect } from 'react';
import { api, FieldMap } from '../api';
import { showToast } from '../components/Toast';

const MODULE_LABELS: Record<string, string> = {
  clients: 'Clientes (Z17)',
  products: 'Productos (Z06CP)',
  movements: 'Movimientos (Z49)',
  cartera: 'Cartera (Z09)',
};

export default function FieldMappings() {
  const [mappings, setMappings] = useState<Record<string, FieldMap[]>>({});
  const [sendEnabled, setSendEnabled] = useState<Record<string, boolean>>({});
  const [activeModule, setActiveModule] = useState('clients');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api.getFieldMappings().then(setMappings).catch(() => {});
    api.getSendEnabled().then(setSendEnabled).catch(() => {});
  }, []);

  const toggleField = (module: string, index: number) => {
    setMappings(prev => {
      const updated = { ...prev };
      updated[module] = [...updated[module]];
      updated[module][index] = { ...updated[module][index], enabled: !updated[module][index].enabled };
      return updated;
    });
  };

  const updateApiKey = (module: string, index: number, value: string) => {
    setMappings(prev => {
      const updated = { ...prev };
      updated[module] = [...updated[module]];
      updated[module][index] = { ...updated[module][index], api_key: value };
      return updated;
    });
  };

  const toggleSend = async (module: string) => {
    const updated = { ...sendEnabled, [module]: !sendEnabled[module] };
    setSendEnabled(updated);
    try {
      await api.saveSendEnabled(updated);
      showToast('success', `${MODULE_LABELS[module] || module}: envio ${updated[module] ? 'activado' : 'desactivado'}`);
    } catch {
      showToast('error', 'Error al guardar');
    }
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const r = await api.saveFieldMappings(mappings);
      showToast(r.status === 'ok' ? 'success' : 'error', r.status === 'ok' ? 'Mapeo guardado' : 'Error al guardar');
    } catch {
      showToast('error', 'Error de conexion');
    }
    setSaving(false);
  };

  const modules = Object.keys(mappings);
  const fields = mappings[activeModule] || [];
  const enabledCount = fields.filter(f => f.enabled).length;

  return (
    <>
      <div className="topbar"><h2>Mapeo de Campos</h2></div>
      <div className="content">
        <p className="mapping-desc">
          Configura que campos se envian al endpoint de Finearom por cada modulo.
          Solo los campos habilitados seran incluidos en el JSON enviado al API.
        </p>

        <div className="module-tabs">
          {modules.map(mod => (
            <div
              key={mod}
              className={`module-tab ${activeModule === mod ? 'active' : ''}`}
              onClick={() => setActiveModule(mod)}
            >
              {MODULE_LABELS[mod] || mod}
            </div>
          ))}
        </div>

        <div className="mapping-panel">
          <div className="mapping-header">
            <h3>{MODULE_LABELS[activeModule] || activeModule}</h3>
            <span className="field-count">{enabledCount} de {fields.length} campos habilitados</span>
          </div>

          <div className="send-toggle-row">
            <label className="toggle-switch">
              <input
                type="checkbox"
                checked={sendEnabled[activeModule] !== false}
                onChange={() => toggleSend(activeModule)}
              />
              <span className="toggle-slider"></span>
            </label>
            <span className={`send-toggle-label ${sendEnabled[activeModule] !== false ? 'active' : 'inactive'}`}>
              {sendEnabled[activeModule] !== false ? 'Envio al servidor ACTIVO' : 'Envio al servidor DESACTIVADO'}
            </span>
          </div>

          <div className="mapping-table-wrapper">
            <table className="data-table mapping-table">
              <thead>
                <tr>
                  <th style={{ width: 60 }}>Enviar</th>
                  <th>Campo Origen</th>
                  <th>Clave API (JSON key)</th>
                  <th>Descripcion</th>
                </tr>
              </thead>
              <tbody>
                {fields.map((field, i) => (
                  <tr key={field.source} className={field.enabled ? '' : 'disabled-row'}>
                    <td>
                      <label className="toggle-switch">
                        <input
                          type="checkbox"
                          checked={field.enabled}
                          onChange={() => toggleField(activeModule, i)}
                        />
                        <span className="toggle-slider"></span>
                      </label>
                    </td>
                    <td><code>{field.source}</code></td>
                    <td>
                      <input
                        className="api-key-input"
                        value={field.api_key}
                        onChange={e => updateApiKey(activeModule, i, e.target.value)}
                        disabled={!field.enabled}
                      />
                    </td>
                    <td>{field.label}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="mapping-preview">
            <h4>Vista previa del JSON que se enviara:</h4>
            <pre className="json-preview">
{JSON.stringify(
  fields
    .filter(f => f.enabled)
    .reduce((acc, f) => { acc[f.api_key] = `<${f.source}>`; return acc; }, {} as Record<string, string>),
  null, 2
)}
            </pre>
          </div>

          <div className="config-actions">
            <button className="btn-save" onClick={handleSave} disabled={saving}>
              {saving ? 'Guardando...' : 'Guardar Mapeo'}
            </button>
          </div>
        </div>
      </div>
    </>
  );
}
