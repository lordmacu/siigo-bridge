import { useState, useEffect } from 'react';
import { api, FieldMap } from '../api';
import { showToast } from '../components/Toast';
import ToggleRow from '../components/ToggleRow';
import Toggle from '../components/Toggle';
import PageHeader from '../components/PageHeader';
import TabBar from '../components/TabBar';

const MODULE_LABELS: Record<string, string> = {
  clients: 'Clientes (Z17)',
  products: 'Productos (Z04)',
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
      <PageHeader title="Mapeo de Campos" />
      <div className="content">
        <p className="mapping-desc">
          Configura que campos se envian al endpoint de Finearom por cada modulo.
          Solo los campos habilitados seran incluidos en el JSON enviado al API.
        </p>

        <TabBar
          tabs={modules.map(mod => ({ key: mod, label: MODULE_LABELS[mod] || mod }))}
          activeTab={activeModule}
          onTabChange={setActiveModule}
        />

        <div className="mapping-panel">
          <div className="mapping-header">
            <h3>{MODULE_LABELS[activeModule] || activeModule}</h3>
            <span className="field-count">{enabledCount} de {fields.length} campos habilitados</span>
          </div>

          <ToggleRow checked={sendEnabled[activeModule] === true}
            onChange={() => toggleSend(activeModule)}
            activeLabel="Envio al servidor ACTIVO" inactiveLabel="Envio al servidor DESACTIVADO" />

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
                      <Toggle checked={field.enabled} onChange={() => toggleField(activeModule, i)} />
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
