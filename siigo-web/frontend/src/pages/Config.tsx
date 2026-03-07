import { useState, useEffect } from 'react';
import { api } from '../api';
import { showToast } from '../components/Toast';

export default function Config() {
  const [dataPath, setDataPath] = useState('');
  const [baseURL, setBaseURL] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [interval, setInterval] = useState(60);
  const [testMsg, setTestMsg] = useState('');

  useEffect(() => {
    api.getConfig().then(cfg => {
      setDataPath(cfg.siigo?.data_path || '');
      setBaseURL(cfg.finearom?.base_url || '');
      setEmail(cfg.finearom?.email || '');
      setPassword(cfg.finearom?.password || '');
      setInterval(cfg.sync?.interval_seconds || 60);
    });
  }, []);

  const handleSave = async () => {
    const r = await api.saveConfig({
      data_path: dataPath,
      base_url: baseURL,
      email,
      password,
      interval,
    });
    showToast(r.status === 'ok' ? 'success' : 'error', r.status === 'ok' ? 'Configuracion guardada' : 'Error al guardar');
  };

  const handleTest = async () => {
    setTestMsg('Probando conexion...');
    const r = await api.testConnection();
    if (r.status === 'ok') {
      setTestMsg('Conexion exitosa!');
    } else {
      setTestMsg('Error: ' + (r.message || 'Fallo la conexion'));
    }
  };

  const handleClearDB = async () => {
    if (!confirm('Seguro que quieres vaciar todas las tablas de SQLite?')) return;
    const r = await api.clearDatabase();
    showToast(r.status === 'ok' ? 'success' : 'error', r.status === 'ok' ? 'Base de datos vaciada' : 'Error');
  };

  return (
    <>
      <div className="topbar"><h2>Configuracion</h2></div>
      <div className="content">
        <div className="config-form">
          <h3 className="config-section-title">Siigo</h3>
          <div className="form-group">
            <label>Data Path (ruta de archivos ISAM)</label>
            <input value={dataPath} onChange={e => setDataPath(e.target.value)} placeholder="C:\DEMOS01\" />
          </div>

          <h3 className="config-section-title">Finearom API</h3>
          <div className="form-group">
            <label>Base URL</label>
            <input value={baseURL} onChange={e => setBaseURL(e.target.value)} placeholder="https://ordenes.finearom.co/api" />
          </div>
          <div className="form-group">
            <label>Email</label>
            <input value={email} onChange={e => setEmail(e.target.value)} placeholder="siigo-sync@finearom.com" />
          </div>
          <div className="form-group">
            <label>Password</label>
            <input type="password" value={password} onChange={e => setPassword(e.target.value)} />
          </div>

          <h3 className="config-section-title">Sincronizacion</h3>
          <div className="form-group">
            <label>Intervalo (segundos)</label>
            <input type="number" value={interval} onChange={e => setInterval(parseInt(e.target.value) || 60)} />
          </div>

          <div className="config-actions">
            <button className="btn-save" onClick={handleSave}>Guardar Configuracion</button>
            <button className="btn-test" onClick={handleTest}>Probar Conexion</button>
          </div>

          {testMsg && (
            <div className={`config-msg ${testMsg.includes('exitosa') ? 'success' : testMsg.includes('Error') ? 'error' : 'loading'}`}>
              {testMsg}
            </div>
          )}

          <h3 className="config-section-title danger">Debug</h3>
          <button className="btn-danger" onClick={handleClearDB}>Vaciar Base de Datos (SQLite)</button>
        </div>
      </div>
    </>
  );
}
