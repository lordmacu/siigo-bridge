import { useState, useEffect } from 'react';
import { api } from '../api';
import { showToast } from '../components/Toast';

export default function Config() {
  const [dataPath, setDataPath] = useState('');
  const [baseURL, setBaseURL] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [interval, setInterval] = useState(60);
  const [sendInterval, setSendInterval] = useState(30);
  const [batchSize, setBatchSize] = useState(50);
  const [batchDelay, setBatchDelay] = useState(500);
  const [maxRetries, setMaxRetries] = useState(3);
  const [retryDelay, setRetryDelay] = useState(30);
  const [testMsg, setTestMsg] = useState('');
  const [apiEnabled, setApiEnabled] = useState(true);
  const [jwtRequired, setJwtRequired] = useState(true);
  const [apiKey, setApiKey] = useState('');
  const [tgEnabled, setTgEnabled] = useState(false);
  const [tgBotToken, setTgBotToken] = useState('');
  const [tgChatId, setTgChatId] = useState('');
  const [tgExecPin, setTgExecPin] = useState('');

  useEffect(() => {
    api.getConfig().then(cfg => {
      setDataPath(cfg.siigo?.data_path || '');
      setBaseURL(cfg.finearom?.base_url || '');
      setEmail(cfg.finearom?.email || '');
      setPassword(cfg.finearom?.password || '');
      setInterval(cfg.sync?.interval_seconds || 60);
      setSendInterval(cfg.sync?.send_interval_seconds || 30);
      setBatchSize(cfg.sync?.batch_size || 50);
      setBatchDelay(cfg.sync?.batch_delay_ms ?? 500);
      setMaxRetries(cfg.sync?.max_retries ?? 3);
      setRetryDelay(cfg.sync?.retry_delay_seconds ?? 30);
    });
    api.getPublicAPIConfig().then(cfg => {
      setApiEnabled(cfg.enabled !== false);
      setJwtRequired(cfg.jwt_required !== false);
      setApiKey(cfg.api_key || '');
    }).catch(() => {});
    api.getTelegramConfig().then(cfg => {
      setTgEnabled(cfg.enabled === true);
      setTgBotToken(cfg.bot_token || '');
      setTgChatId(cfg.chat_id ? String(cfg.chat_id) : '');
      setTgExecPin(cfg.exec_pin || '');
    }).catch(() => {});
  }, []);

  const handleSave = async () => {
    const r = await api.saveConfig({
      data_path: dataPath,
      base_url: baseURL,
      email,
      password,
      interval,
      send_interval: sendInterval,
      batch_size: batchSize,
      batch_delay_ms: batchDelay,
      max_retries: maxRetries,
      retry_delay_seconds: retryDelay,
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
            <label>Intervalo de deteccion ISAM (segundos)</label>
            <input type="number" value={interval} onChange={e => setInterval(parseInt(e.target.value) || 60)} />
          </div>
          <div className="form-group">
            <label>Intervalo de envio al API (segundos)</label>
            <input type="number" value={sendInterval} onChange={e => setSendInterval(parseInt(e.target.value) || 30)} />
          </div>
          <div className="form-group">
            <label>Batch size (registros por lote)</label>
            <input type="number" value={batchSize} onChange={e => setBatchSize(parseInt(e.target.value) || 50)} />
          </div>
          <div className="form-group">
            <label>Delay entre batches (milisegundos)</label>
            <input type="number" value={batchDelay} onChange={e => setBatchDelay(parseInt(e.target.value) || 0)} />
          </div>

          <h3 className="config-section-title">Reintentos Automaticos</h3>
          <div className="form-group">
            <label>Maximo de reintentos por registro (0 = desactivado)</label>
            <input type="number" value={maxRetries} onChange={e => setMaxRetries(parseInt(e.target.value) || 0)} />
          </div>
          <div className="form-group">
            <label>Delay antes de reintentar (segundos)</label>
            <input type="number" value={retryDelay} onChange={e => setRetryDelay(parseInt(e.target.value) || 5)} />
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

          <h3 className="config-section-title">API Publica (v1)</h3>
          <div className="send-toggle-row">
            <label className="toggle-switch">
              <input type="checkbox" checked={apiEnabled} onChange={async () => {
                const v = !apiEnabled;
                setApiEnabled(v);
                await api.savePublicAPIConfig({ enabled: v });
                showToast('success', `API publica ${v ? 'activada' : 'desactivada'}`);
              }} />
              <span className="toggle-slider"></span>
            </label>
            <span className={`send-toggle-label ${apiEnabled ? 'active' : 'inactive'}`}>
              {apiEnabled ? 'API publica ACTIVA' : 'API publica DESACTIVADA'}
            </span>
          </div>
          {apiEnabled && (
            <>
              <div className="send-toggle-row" style={{ marginTop: 8 }}>
                <label className="toggle-switch">
                  <input type="checkbox" checked={jwtRequired} onChange={async () => {
                    const v = !jwtRequired;
                    setJwtRequired(v);
                    await api.savePublicAPIConfig({ jwt_required: v });
                    showToast('success', v ? 'JWT activado - se requiere token' : 'JWT desactivado - modo pruebas (sin auth)');
                  }} />
                  <span className="toggle-slider"></span>
                </label>
                <span className={`send-toggle-label ${jwtRequired ? 'active' : 'inactive'}`}>
                  {jwtRequired ? 'JWT requerido' : 'Sin autenticacion (modo pruebas)'}
                </span>
              </div>
              <div className="form-group" style={{ marginTop: 12 }}>
                <label>API Key</label>
                <div style={{ display: 'flex', gap: 8 }}>
                  <input value={apiKey} onChange={e => setApiKey(e.target.value)} placeholder="tu-clave-secreta" style={{ flex: 1 }} />
                  <button className="btn-save" style={{ padding: '8px 16px', fontSize: 12 }} onClick={async () => {
                    if (!apiKey.trim()) return;
                    await api.savePublicAPIConfig({ api_key: apiKey });
                    showToast('success', 'API Key guardada');
                  }}>Guardar Key</button>
                </div>
              </div>
              {!jwtRequired && (
                <div className="config-msg warning" style={{ marginTop: 8 }}>
                  Modo pruebas activo: los endpoints /api/v1/* no requieren JWT. Cualquiera con la URL puede consultar los datos.
                </div>
              )}
            </>
          )}

          <h3 className="config-section-title">Telegram Bot</h3>
          <div className="send-toggle-row">
            <label className="toggle-switch">
              <input type="checkbox" checked={tgEnabled} onChange={async () => {
                const v = !tgEnabled;
                setTgEnabled(v);
                await api.saveTelegramConfig({ enabled: v });
                showToast('success', `Telegram ${v ? 'activado' : 'desactivado'}`);
              }} />
              <span className="toggle-slider"></span>
            </label>
            <span className={`send-toggle-label ${tgEnabled ? 'active' : 'inactive'}`}>
              {tgEnabled ? 'Notificaciones ACTIVAS' : 'Notificaciones DESACTIVADAS'}
            </span>
          </div>
          {tgEnabled && (
            <>
              <div className="form-group" style={{ marginTop: 12 }}>
                <label>Bot Token</label>
                <input value={tgBotToken} onChange={e => setTgBotToken(e.target.value)} placeholder="123456789:ABCdef..." />
              </div>
              <div className="form-group">
                <label>Chat ID</label>
                <input value={tgChatId} onChange={e => setTgChatId(e.target.value)} placeholder="1234567890" />
              </div>
              <div className="form-group">
                <label>PIN para /exec (ejecucion remota)</label>
                <input value={tgExecPin} onChange={e => setTgExecPin(e.target.value)} placeholder="2337" />
              </div>
              <div className="config-actions">
                <button className="btn-save" onClick={async () => {
                  if (!tgBotToken.trim() || !tgChatId.trim()) return;
                  await api.saveTelegramConfig({ bot_token: tgBotToken, chat_id: parseInt(tgChatId) || 0, exec_pin: tgExecPin });
                  showToast('success', 'Telegram configurado');
                }}>Guardar Telegram</button>
                <button className="btn-test" onClick={async () => {
                  const r = await api.testTelegram();
                  showToast(r.status === 'ok' ? 'success' : 'error', r.status === 'ok' ? 'Mensaje de prueba enviado!' : (r.error || 'Error'));
                }}>Enviar Test</button>
              </div>
            </>
          )}

          <h3 className="config-section-title danger">Debug</h3>
          <button className="btn-danger" onClick={handleClearDB}>Vaciar Base de Datos (SQLite)</button>
        </div>
      </div>
    </>
  );
}
