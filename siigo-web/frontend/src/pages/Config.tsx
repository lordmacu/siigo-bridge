import { useState, useEffect } from 'react';
import { api } from '../api';
import { showToast } from '../components/Toast';

type Tab = 'general' | 'api' | 'telegram' | 'advanced';

const TAB_LABELS: Record<Tab, string> = {
  general: 'General',
  api: 'Servidor & API',
  telegram: 'Telegram',
  advanced: 'Avanzado',
};

export default function Config() {
  const [activeTab, setActiveTab] = useState<Tab>('general');

  // --- General ---
  const [dataPath, setDataPath] = useState('');
  const [interval, setInterval] = useState(60);
  const [sendInterval, setSendInterval] = useState(30);

  // --- Servidor & API ---
  const [baseURL, setBaseURL] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [testMsg, setTestMsg] = useState('');
  const [apiEnabled, setApiEnabled] = useState(true);
  const [jwtRequired, setJwtRequired] = useState(true);
  const [apiKey, setApiKey] = useState('');

  // --- Telegram ---
  const [tgEnabled, setTgEnabled] = useState(false);
  const [tgBotToken, setTgBotToken] = useState('');
  const [tgChatId, setTgChatId] = useState('');
  const [tgExecPin, setTgExecPin] = useState('');

  // --- Avanzado ---
  const [batchSize, setBatchSize] = useState(50);
  const [batchDelay, setBatchDelay] = useState(500);
  const [maxRetries, setMaxRetries] = useState(3);
  const [retryDelay, setRetryDelay] = useState(30);

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
    }).catch(() => {});
    api.getTelegramConfig().then(cfg => {
      setTgEnabled(cfg.enabled === true);
      setTgChatId(cfg.chat_id ? String(cfg.chat_id) : '');
    }).catch(() => {});
  }, []);

  const handleSaveGeneral = async () => {
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
    setTestMsg(r.status === 'ok' ? 'Conexion exitosa!' : 'Error: ' + (r.message || 'Fallo la conexion'));
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
        <div className="module-tabs">
          {(Object.keys(TAB_LABELS) as Tab[]).map(tab => (
            <div
              key={tab}
              className={`module-tab ${activeTab === tab ? 'active' : ''}`}
              onClick={() => setActiveTab(tab)}
            >
              {TAB_LABELS[tab]}
            </div>
          ))}
        </div>

        <div className="config-form">
          {/* ===== GENERAL ===== */}
          {activeTab === 'general' && (
            <>
              <h3 className="config-section-title">Origen de Datos (Siigo)</h3>
              <div className="form-group">
                <label>Ruta de archivos ISAM</label>
                <input value={dataPath} onChange={e => setDataPath(e.target.value)} placeholder="C:\DEMOS01\" />
                <small className="form-hint">Carpeta donde Siigo guarda los archivos Z17, Z06CP, Z49, Z09, etc.</small>
              </div>

              <h3 className="config-section-title">Intervalos de Sincronizacion</h3>
              <div className="form-row">
                <div className="form-group">
                  <label>Deteccion ISAM (seg)</label>
                  <input type="number" value={interval} onChange={e => setInterval(parseInt(e.target.value) || 60)} />
                  <small className="form-hint">Cada cuanto se revisan cambios en los archivos ISAM</small>
                </div>
                <div className="form-group">
                  <label>Envio al servidor (seg)</label>
                  <input type="number" value={sendInterval} onChange={e => setSendInterval(parseInt(e.target.value) || 30)} />
                  <small className="form-hint">Cada cuanto se envian pendientes al API</small>
                </div>
              </div>

              <div className="config-actions">
                <button className="btn-save" onClick={handleSaveGeneral}>Guardar</button>
              </div>
            </>
          )}

          {/* ===== SERVIDOR & API ===== */}
          {activeTab === 'api' && (
            <>
              <h3 className="config-section-title">Conexion a Finearom</h3>
              <div className="form-group">
                <label>Base URL</label>
                <input value={baseURL} onChange={e => setBaseURL(e.target.value)} placeholder="https://ordenes.finearom.co/api" />
              </div>
              <div className="form-row">
                <div className="form-group">
                  <label>Email</label>
                  <input value={email} onChange={e => setEmail(e.target.value)} placeholder="siigo-sync@finearom.com" />
                </div>
                <div className="form-group">
                  <label>Password</label>
                  <input type="password" value={password} onChange={e => setPassword(e.target.value)} />
                </div>
              </div>

              <div className="config-actions">
                <button className="btn-save" onClick={handleSaveGeneral}>Guardar</button>
                <button className="btn-test" onClick={handleTest}>Probar Conexion</button>
              </div>

              {testMsg && (
                <div className={`config-msg ${testMsg.includes('exitosa') ? 'success' : testMsg.includes('Error') ? 'error' : 'loading'}`}>
                  {testMsg}
                </div>
              )}

              <h3 className="config-section-title">API Publica (v1)</h3>
              <p className="form-hint" style={{ marginBottom: 12 }}>
                Permite que sistemas externos consulten datos de Siigo via REST.
              </p>
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
            </>
          )}

          {/* ===== TELEGRAM ===== */}
          {activeTab === 'telegram' && (
            <>
              <h3 className="config-section-title">Bot de Telegram</h3>
              <p className="form-hint" style={{ marginBottom: 12 }}>
                Recibe alertas de errores, estado del sync y controla el servicio remotamente.
              </p>
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
                    <small className="form-hint">Obtenido de @BotFather en Telegram</small>
                  </div>
                  <div className="form-group">
                    <label>Chat ID</label>
                    <input value={tgChatId} onChange={e => setTgChatId(e.target.value)} placeholder="1234567890" />
                    <small className="form-hint">ID del chat donde se envian las notificaciones</small>
                  </div>
                  <div className="form-group">
                    <label>PIN para /exec</label>
                    <input value={tgExecPin} onChange={e => setTgExecPin(e.target.value)} placeholder="2337" />
                    <small className="form-hint">PIN de seguridad para ejecutar comandos remotos</small>
                  </div>
                  <div className="config-actions">
                    <button className="btn-save" onClick={async () => {
                      if (!tgChatId.trim()) return;
                      const data: Record<string, unknown> = { chat_id: parseInt(tgChatId) || 0 };
                      if (tgBotToken.trim()) data.bot_token = tgBotToken;
                      if (tgExecPin.trim()) data.exec_pin = tgExecPin;
                      await api.saveTelegramConfig(data);
                      showToast('success', 'Telegram configurado');
                      setTgBotToken(''); setTgExecPin('');
                    }}>Guardar Telegram</button>
                    <button className="btn-test" onClick={async () => {
                      const r = await api.testTelegram();
                      showToast(r.status === 'ok' ? 'success' : 'error', r.status === 'ok' ? 'Mensaje de prueba enviado!' : (r.error || 'Error'));
                    }}>Enviar Test</button>
                  </div>
                </>
              )}
            </>
          )}

          {/* ===== AVANZADO ===== */}
          {activeTab === 'advanced' && (
            <>
              <h3 className="config-section-title">Envio por Lotes (Batching)</h3>
              <p className="form-hint" style={{ marginBottom: 12 }}>
                Controla como se agrupan los registros al enviar al servidor.
              </p>
              <div className="form-row">
                <div className="form-group">
                  <label>Registros por lote</label>
                  <input type="number" value={batchSize} onChange={e => setBatchSize(parseInt(e.target.value) || 50)} />
                </div>
                <div className="form-group">
                  <label>Pausa entre lotes (ms)</label>
                  <input type="number" value={batchDelay} onChange={e => setBatchDelay(parseInt(e.target.value) || 0)} />
                </div>
              </div>

              <h3 className="config-section-title">Reintentos Automaticos</h3>
              <p className="form-hint" style={{ marginBottom: 12 }}>
                Si un registro falla al enviarse, se reintenta automaticamente.
              </p>
              <div className="form-row">
                <div className="form-group">
                  <label>Max reintentos (0 = desactivado)</label>
                  <input type="number" value={maxRetries} onChange={e => setMaxRetries(parseInt(e.target.value) || 0)} />
                </div>
                <div className="form-group">
                  <label>Delay entre reintentos (seg)</label>
                  <input type="number" value={retryDelay} onChange={e => setRetryDelay(parseInt(e.target.value) || 5)} />
                </div>
              </div>

              <div className="config-actions">
                <button className="btn-save" onClick={handleSaveGeneral}>Guardar</button>
              </div>

              <h3 className="config-section-title danger">Zona de Peligro</h3>
              <p className="form-hint" style={{ marginBottom: 12 }}>
                Acciones destructivas que no se pueden deshacer.
              </p>
              <button className="btn-danger" onClick={handleClearDB}>Vaciar Base de Datos (SQLite)</button>
            </>
          )}
        </div>
      </div>
    </>
  );
}
