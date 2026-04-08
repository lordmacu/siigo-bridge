import { useState, useEffect } from 'react';
import { api, FieldMap } from '../api';
import { showToast } from '../components/Toast';
import Toggle from '../components/Toggle';
import PageHeader from '../components/PageHeader';

const MODULE_LABELS: Record<string, string> = {
  clients: 'Clientes',
  products: 'Productos',
  cartera: 'Cartera',
  documentos: 'Documentos',
  condiciones_pago: 'Condiciones Pago',
  codigos_dane: 'Codigos DANE',
  formulas: 'Formulas/Recetas',
  vendedores_areas: 'Vendedores/Areas',
};

export default function FieldMappings() {
  const [mappings, setMappings] = useState<Record<string, FieldMap[]>>({});
  const [activeModule, setActiveModule] = useState('clients');
  const [saving, setSaving] = useState(false);
  const [search, setSearch] = useState('');

  useEffect(() => {
    api.getFieldMappings().then(setMappings).catch(() => {});
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

  const toggleAllFields = (module: string, enabled: boolean) => {
    setMappings(prev => {
      const updated = { ...prev };
      updated[module] = updated[module].map(f => ({ ...f, enabled }));
      return updated;
    });
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
  const filteredModules = search
    ? modules.filter(m => (MODULE_LABELS[m] || m).toLowerCase().includes(search.toLowerCase()))
    : modules;
  const fields = mappings[activeModule] || [];
  const enabledCount = fields.filter(f => f.enabled).length;

  return (
    <>
      <PageHeader title="Mapeo de Campos" />
      <div className="content">
        <p className="mapping-desc">
          Configura que campos se incluyen cuando se envia informacion al webhook o al API de Finearom.
          Solo los campos habilitados se envian. Para activar/desactivar el envio de una tabla, ve a Configuracion → Sincronizacion.
        </p>

        <div className="mapping-layout">
          <div className="mapping-sidebar">
            <input
              className="mapping-search"
              placeholder="Buscar tabla..."
              value={search}
              onChange={e => setSearch(e.target.value)}
            />
            <div className="mapping-module-list">
              {filteredModules.map(mod => {
                const modFields = mappings[mod] || [];
                const modEnabled = modFields.filter(f => f.enabled).length;
                return (
                  <button
                    key={mod}
                    className={`mapping-module-btn ${mod === activeModule ? 'active' : ''}`}
                    onClick={() => setActiveModule(mod)}
                  >
                    <span className="mapping-module-name">{MODULE_LABELS[mod] || mod}</span>
                    <span className="mapping-module-count">{modEnabled}/{modFields.length}</span>
                  </button>
                );
              })}
            </div>
          </div>

          <div className="mapping-panel">
            <div className="mapping-header">
              <h3>{MODULE_LABELS[activeModule] || activeModule}</h3>
              <span className="field-count">{enabledCount} de {fields.length} campos habilitados</span>
            </div>

            <div className="mapping-bulk-actions">
              <button className="btn-small" onClick={() => toggleAllFields(activeModule, true)}>Activar todos</button>
              <button className="btn-small" onClick={() => toggleAllFields(activeModule, false)}>Desactivar todos</button>
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
      </div>
    </>
  );
}
