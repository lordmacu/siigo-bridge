import { useState, useEffect } from 'react';
import { api } from '../api';
import { showToast } from '../components/Toast';
import SectionTitle from '../components/SectionTitle';
import ToggleRow from '../components/ToggleRow';
import Toggle from '../components/Toggle';
import PageHeader from '../components/PageHeader';
import FormGroup from '../components/FormGroup';
import Alert from '../components/Alert';
import TabBar from '../components/TabBar';

type Tab = 'general' | 'sync' | 'api' | 'telegram' | 'integrations' | 'advanced';

const TAB_LABELS: Record<Tab, string> = {
  general: 'General',
  sync: 'Sincronizacion',
  api: 'Servidor & API',
  telegram: 'Telegram',
  integrations: 'Integraciones',
  advanced: 'Avanzado',
};

const TABLE_LABELS: Record<string, string> = {
  clients: 'Clientes (Z17)',
  products: 'Productos (Z04)',
  cartera: 'Cartera (Z09)',
  documentos: 'Documentos (Z11)',
  condiciones_pago: 'Condiciones Pago (Z05)',
  codigos_dane: 'Codigos DANE',
  formulas: 'Formulas/Recetas (Z06)',
  vendedores_areas: 'Vendedores/Areas (Z06A)',
  notas_documentos: 'Notas Documentos (Z49)',
  facturas_electronicas: 'Fact. Electronicas (Z09ELE)',
  detalle_movimientos: 'Detalle Movimientos (Z17)',
};

interface WebhookDef {
  url: string;
  secret: string;
  events: string[];
  active: boolean;
}

export default function Config() {
  const [activeTab, setActiveTab] = useState<Tab>('general');

  // --- General ---
  const [dataPath, setDataPath] = useState('');
  const [pathValidation, setPathValidation] = useState<{ valid?: boolean; message?: string; error?: string; files?: string[]; count?: number } | null>(null);
  const [validatingPath, setValidatingPath] = useState(false);
  const [interval, setInterval] = useState(60);
  const [sendInterval, setSendInterval] = useState(30);

  // --- Servidor & API ---
  const [serverPort, setServerPort] = useState('3210');
  const [baseURL, setBaseURL] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [testMsg, setTestMsg] = useState('');
  const [apiEnabled, setApiEnabled] = useState(true);
  const [jwtRequired, setJwtRequired] = useState(true);
  const [apiKey, setApiKey] = useState('');
  const [globalSend, setGlobalSend] = useState(false);

  // --- Telegram ---
  const [tgEnabled, setTgEnabled] = useState(false);
  const [tgBotToken, setTgBotToken] = useState('');
  const [tgChatId, setTgChatId] = useState('');
  const [tgExecPin, setTgExecPin] = useState('');
  const [tgNotify, setTgNotify] = useState({
    server_start: true,
    sync_complete: false,
    sync_errors: false,
    login_failed: false,
    changes: false,
    db_cleared: false,
    max_retries: false,
  });

  // --- Integraciones (Webhooks + URLs) ---
  const [whEnabled, setWhEnabled] = useState(false);
  const [whHooks, setWhHooks] = useState<WebhookDef[]>([]);
  const [serverInfo, setServerInfo] = useState<{ lan_urls: string[]; tunnel_url: string } | null>(null);

  // --- Sincronizacion ---
  const [detectEnabled, setDetectEnabled] = useState<Record<string, boolean>>({});
  const [sendEnabled, setSendEnabled] = useState<Record<string, boolean>>({});

  // --- Avanzado ---
  const [batchSize, setBatchSize] = useState(50);
  const [batchDelay, setBatchDelay] = useState(500);
  const [maxRetries, setMaxRetries] = useState(3);
  const [retryDelay, setRetryDelay] = useState(30);
  const [allowEditDelete, setAllowEditDelete] = useState(false);

  useEffect(() => {
    api.getConfig().then(cfg => {
      setDataPath(cfg.siigo?.data_path || '');
      setServerPort(cfg.server?.port || '3210');
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
      setTgNotify({
        server_start: cfg.notify_server_start !== false,
        sync_complete: cfg.notify_sync_complete === true,
        sync_errors: cfg.notify_sync_errors === true,
        login_failed: cfg.notify_login_failed === true,
        changes: cfg.notify_changes === true,
        db_cleared: cfg.notify_db_cleared === true,
        max_retries: cfg.notify_max_retries === true,
      });
    }).catch(() => {});
    api.getAllowEditDelete().then(r => setAllowEditDelete(r.enabled === true)).catch(() => {});
    fetch('/api/global-send', { headers: { 'Authorization': `Bearer ${localStorage.getItem('siigo_token')}` } }).then(r => r.json()).then(r => setGlobalSend(r.enabled === true)).catch(() => {});
    api.getDetectEnabled().then(setDetectEnabled).catch(() => {});
    api.getSendEnabled().then(setSendEnabled).catch(() => {});
    api.getWebhookConfig().then(cfg => {
      setWhEnabled(cfg.enabled === true);
      setWhHooks(cfg.hooks || []);
    }).catch(() => {});
    api.getServerInfo().then(setServerInfo).catch(() => {});
  }, []);

  const handleSaveGeneral = async () => {
    // Validate ISAM path before saving
    if (dataPath.trim()) {
      try {
        const v = await api.validatePath(dataPath);
        setPathValidation(v);
        if (!v.valid) {
          showToast('error', v.error || 'Ruta de datos invalida');
          return;
        }
      } catch {
        showToast('error', 'No se pudo validar la ruta');
        return;
      }
    }

    const r = await api.saveConfig({
      data_path: dataPath,
      port: serverPort,
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
    if (!confirm('Seguro que quieres vaciar todas las tablas de SQLite? Se mostrara el wizard de setup nuevamente.')) return;
    const r = await api.clearDatabase();
    if (r.status === 'ok') {
      // Reload to trigger wizard (setup_complete is now false)
      window.location.reload();
    } else {
      showToast('error', 'Error al vaciar base de datos');
    }
  };

  return (
    <>
      <PageHeader title="Configuracion" />
      <div className="content">
        <TabBar
          tabs={(Object.keys(TAB_LABELS) as Tab[]).map(k => ({ key: k, label: TAB_LABELS[k] }))}
          activeTab={activeTab}
          onTabChange={t => setActiveTab(t as Tab)}
        />

        <div className="config-form">
          {/* ===== GENERAL ===== */}
          {activeTab === 'general' && (
            <>
              <SectionTitle>Origen de Datos (Siigo)</SectionTitle>
              <FormGroup label="Ruta de archivos ISAM" hint="Carpeta donde Siigo guarda los archivos Z17, Z04, Z49, Z09, etc.">
                <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                  <input style={{ flex: 1 }} value={dataPath} onChange={e => { setDataPath(e.target.value); setPathValidation(null); }} placeholder="C:\SIIWI02" />
                  <button className="btn-small" style={{ padding: '8px 14px', whiteSpace: 'nowrap' }} disabled={validatingPath || !dataPath.trim()} onClick={async () => {
                    setValidatingPath(true);
                    try {
                      const r = await api.validatePath(dataPath);
                      setPathValidation(r);
                    } catch { setPathValidation({ valid: false, error: 'Error de conexion' }); }
                    setValidatingPath(false);
                  }}>
                    {validatingPath ? 'Validando...' : 'Validar'}
                  </button>
                </div>
                {pathValidation && (
                  <div className={`path-validation ${pathValidation.valid ? 'valid' : 'invalid'}`}>
                    <span className="path-validation-icon">{pathValidation.valid ? '\u2713' : '\u2717'}</span>
                    <span>{pathValidation.valid ? pathValidation.message : pathValidation.error}</span>
                    {pathValidation.valid && pathValidation.count && (
                      <span className="path-file-count">{pathValidation.count} archivos detectados</span>
                    )}
                  </div>
                )}
              </FormGroup>

              <SectionTitle>Sincronizacion</SectionTitle>
              <Alert variant="info" style={{ marginBottom: 12 }}>
                <strong>Deteccion:</strong> Los cambios en archivos ISAM se detectan automaticamente en tiempo real (file watcher).<br />
                <strong>Envio:</strong> Los registros pendientes se envian al API cada {sendInterval}s.
              </Alert>
              <div className="form-row">
                <FormGroup label="Envio al servidor (seg)" hint="Cada cuanto se envian pendientes al API">
                  <input type="number" value={sendInterval} onChange={e => setSendInterval(parseInt(e.target.value) || 30)} />
                </FormGroup>
              </div>

              <div className="config-actions">
                <button className="btn-save" onClick={handleSaveGeneral}>Guardar</button>
              </div>
            </>
          )}

          {/* ===== SINCRONIZACION ===== */}
          {activeTab === 'sync' && (
            <>
              <div className="sync-legend">
                <div className="sync-legend-item">
                  <span className="sync-legend-dot detect"></span>
                  <div>
                    <strong>Deteccion (ISAM → SQLite)</strong>
                    <p>Vigila los archivos ISAM en tiempo real. Cuando Siigo escribe un cambio, se detecta y se guarda en la base de datos local.</p>
                  </div>
                </div>
                <div className="sync-legend-item">
                  <span className="sync-legend-dot send"></span>
                  <div>
                    <strong>Envio (SQLite → Laravel)</strong>
                    <p>Envia los registros detectados como nuevos o modificados al servidor de Finearom. Solo funciona si la deteccion esta activa.</p>
                  </div>
                </div>
              </div>

              <div className="sync-toggles-grid">
                <div className="sync-toggles-header">
                  <span className="sync-col-table">Tabla</span>
                  <span className="sync-col-toggle sync-col-detect">Deteccion</span>
                  <span className="sync-col-toggle sync-col-send">Envio</span>
                </div>
                {Object.keys(TABLE_LABELS).map(table => (
                  <div key={table} className="sync-toggles-row">
                    <span className="sync-col-table">{TABLE_LABELS[table]}</span>
                    <span className="sync-col-toggle sync-col-detect">
                      <Toggle checked={detectEnabled[table] !== false} onChange={async () => {
                        const v = detectEnabled[table] === false;
                        const updated = { ...detectEnabled, [table]: v };
                        setDetectEnabled(updated);
                        try {
                          await api.saveDetectEnabled(updated);
                          showToast('success', `${TABLE_LABELS[table]}: deteccion ${v ? 'activada' : 'desactivada'}`);
                        } catch { showToast('error', 'Error al guardar'); }
                      }} />
                    </span>
                    <span className="sync-col-toggle sync-col-send">
                      <Toggle checked={sendEnabled[table] === true} onChange={async () => {
                        const v = !sendEnabled[table];
                        const updated = { ...sendEnabled, [table]: v };
                        setSendEnabled(updated);
                        try {
                          await api.saveSendEnabled(updated);
                          showToast('success', `${TABLE_LABELS[table]}: envio ${v ? 'activado' : 'desactivado'}`);
                        } catch { showToast('error', 'Error al guardar'); }
                      }} />
                    </span>
                  </div>
                ))}
              </div>

              <div className="config-actions" style={{ marginTop: 16 }}>
                <button className="btn-test" onClick={async () => {
                  const allDetect: Record<string, boolean> = {};
                  Object.keys(TABLE_LABELS).forEach(t => allDetect[t] = true);
                  setDetectEnabled(allDetect);
                  await api.saveDetectEnabled(allDetect);
                  showToast('success', 'Todas las detecciones activadas');
                }}>Activar toda deteccion</button>
                <button className="btn-test" onClick={async () => {
                  const allDetect: Record<string, boolean> = {};
                  Object.keys(TABLE_LABELS).forEach(t => allDetect[t] = false);
                  setDetectEnabled(allDetect);
                  await api.saveDetectEnabled(allDetect);
                  showToast('success', 'Todas las detecciones desactivadas');
                }}>Desactivar toda deteccion</button>
              </div>
            </>
          )}

          {/* ===== SERVIDOR & API ===== */}
          {activeTab === 'api' && (
            <>
              <SectionTitle>Servidor</SectionTitle>
              <FormGroup label="Puerto" hint="Puerto para localhost, LAN y tunel Cloudflare. Requiere reiniciar el servidor para aplicar cambios.">
                <input value={serverPort} onChange={e => setServerPort(e.target.value)} placeholder="3210" style={{ maxWidth: 120 }} />
              </FormGroup>
              <div className="config-actions" style={{ marginBottom: 24 }}>
                <button className="btn-save" onClick={handleSaveGeneral}>Guardar</button>
              </div>

              <SectionTitle>Conexion a Finearom</SectionTitle>
              <ToggleRow checked={globalSend} onChange={async () => {
                    const val = !globalSend;
                    await fetch('/api/global-send', { method: 'POST', headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${localStorage.getItem('siigo_token')}` }, body: JSON.stringify({ enabled: val }) });
                    setGlobalSend(val);
                  }} activeLabel="Envio a Finearom ACTIVO" inactiveLabel="Envio a Finearom DESACTIVADO" />
              <FormGroup label="Base URL">
                <input value={baseURL} onChange={e => setBaseURL(e.target.value)} placeholder="https://ordenes.finearom.co/api" />
              </FormGroup>
              <div className="form-row">
                <FormGroup label="Email">
                  <input value={email} onChange={e => setEmail(e.target.value)} placeholder="siigo-sync@finearom.com" />
                </FormGroup>
                <FormGroup label="Password">
                  <input type="password" value={password} onChange={e => setPassword(e.target.value)} />
                </FormGroup>
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

              <SectionTitle>API Publica (v1)</SectionTitle>
              <p className="form-hint" style={{ marginBottom: 12 }}>
                Permite que sistemas externos consulten datos de Siigo via REST.
              </p>
              <ToggleRow checked={apiEnabled} onChange={async () => {
                    const v = !apiEnabled;
                    setApiEnabled(v);
                    await api.savePublicAPIConfig({ enabled: v });
                    showToast('success', `API publica ${v ? 'activada' : 'desactivada'}`);
                  }} activeLabel="API publica ACTIVA" inactiveLabel="API publica DESACTIVADA" />
              {apiEnabled && (
                <>
                  <ToggleRow checked={jwtRequired} onChange={async () => {
                        const v = !jwtRequired;
                        setJwtRequired(v);
                        await api.savePublicAPIConfig({ jwt_required: v });
                        showToast('success', v ? 'JWT activado - se requiere token' : 'JWT desactivado - modo pruebas (sin auth)');
                      }} activeLabel="JWT requerido" inactiveLabel="Sin autenticacion (modo pruebas)" style={{ marginTop: 8 }} />
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
                    <Alert variant="warning" style={{ marginTop: 8 }}>
                      Modo pruebas activo: los endpoints /api/v1/* no requieren JWT. Cualquiera con la URL puede consultar los datos.
                    </Alert>
                  )}
                </>
              )}
            </>
          )}

          {/* ===== TELEGRAM ===== */}
          {activeTab === 'telegram' && (
            <>
              <SectionTitle>Bot de Telegram</SectionTitle>
              <p className="form-hint" style={{ marginBottom: 12 }}>
                Recibe alertas de errores, estado del sync y controla el servicio remotamente.
              </p>
              <ToggleRow checked={tgEnabled} onChange={async () => {
                    const v = !tgEnabled;
                    setTgEnabled(v);
                    await api.saveTelegramConfig({ enabled: v });
                    showToast('success', `Telegram ${v ? 'activado' : 'desactivado'}`);
                  }} activeLabel="Notificaciones ACTIVAS" inactiveLabel="Notificaciones DESACTIVADAS" />
              {tgEnabled && (
                <>
                  <FormGroup label="Bot Token" hint="Obtenido de @BotFather en Telegram" style={{ marginTop: 12 }}>
                    <input value={tgBotToken} onChange={e => setTgBotToken(e.target.value)} placeholder="123456789:ABCdef..." />
                  </FormGroup>
                  <FormGroup label="Chat ID" hint="ID del chat donde se envian las notificaciones">
                    <input value={tgChatId} onChange={e => setTgChatId(e.target.value)} placeholder="1234567890" />
                  </FormGroup>
                  <FormGroup label="PIN para /exec" hint="PIN de seguridad para ejecutar comandos remotos">
                    <input value={tgExecPin} onChange={e => setTgExecPin(e.target.value)} placeholder="2337" />
                  </FormGroup>

                  <SectionTitle style={{ marginTop: 20 }}>Tipos de Notificacion</SectionTitle>
                  <p className="form-hint" style={{ marginBottom: 12 }}>
                    Selecciona que notificaciones quieres recibir por Telegram.
                  </p>
                  <div className="notify-toggles">
                    {([
                      ['server_start', 'Inicio del servidor'],
                      ['sync_complete', 'Sync completado (adds/edits)'],
                      ['sync_errors', 'Errores de sync'],
                      ['login_failed', 'Login fallido al API'],
                      ['changes', 'Cambios detectados en ISAM'],
                      ['db_cleared', 'Base de datos vaciada'],
                      ['max_retries', 'Reintentos agotados'],
                    ] as [string, string][]).map(([key, label]) => (
                      <ToggleRow key={key} checked={(tgNotify as Record<string, boolean>)[key]}
                        onChange={async () => {
                          const v = !(tgNotify as Record<string, boolean>)[key];
                          setTgNotify(prev => ({ ...prev, [key]: v }));
                          await api.saveTelegramConfig({ [`notify_${key}`]: v });
                          showToast('success', `${label}: ${v ? 'activado' : 'desactivado'}`);
                        }} label={label} style={{ marginBottom: 4 }} />
                    ))}
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

          {/* ===== INTEGRACIONES ===== */}
          {activeTab === 'integrations' && (
            <>
              <SectionTitle>URLs de Conexion</SectionTitle>
              <p className="form-hint" style={{ marginBottom: 12 }}>
                Usa estas URLs para conectar herramientas externas (Power BI, Postman, etc.) desde la red local.
              </p>
              {serverInfo && (() => {
                const primary = serverInfo.lan_urls[0] || '';
                const base = primary || serverInfo.tunnel_url;
                const services = [
                  { label: 'OData (Power BI)', path: '/odata' },
                  { label: 'API v1', path: '/api/v1' },
                  { label: 'Swagger Docs', path: '/api/v1/docs' },
                ];
                return (
                  <div className="connection-urls">
                    {services.map(svc => (
                      <div key={svc.path} className="url-row">
                        <div className="url-group">
                          <label>{svc.label}</label>
                          <div className="url-copy">
                            <code>{base}{svc.path}</code>
                            <button className="btn-sm btn-copy" onClick={() => { navigator.clipboard.writeText(base + svc.path); showToast('success', 'URL copiada'); }}>Copiar</button>
                          </div>
                        </div>
                      </div>
                    ))}
                    {serverInfo.lan_urls.length > 1 && (
                      <details style={{ marginTop: 8 }}>
                        <summary style={{ color: '#94a3b8', fontSize: 12, cursor: 'pointer' }}>
                          Otras IPs de red ({serverInfo.lan_urls.length - 1})
                        </summary>
                        <div style={{ marginTop: 8 }}>
                          {serverInfo.lan_urls.slice(1).map((url: string) => (
                            <div key={url} className="url-row" style={{ padding: '4px 0' }}>
                              <div className="url-copy">
                                <code style={{ fontSize: 12 }}>{url}</code>
                                <button className="btn-sm btn-copy" onClick={() => { navigator.clipboard.writeText(url); showToast('success', 'URL copiada'); }}>Copiar</button>
                              </div>
                            </div>
                          ))}
                        </div>
                      </details>
                    )}
                    {serverInfo.tunnel_url && (
                      <div className="url-row" style={{ borderTop: '1px solid #334155', paddingTop: 12, marginTop: 8 }}>
                        <div className="url-group">
                          <label>Tunnel (acceso publico)</label>
                          <div className="url-copy">
                            <code>{serverInfo.tunnel_url}</code>
                            <button className="btn-sm btn-copy" onClick={() => { navigator.clipboard.writeText(serverInfo.tunnel_url); showToast('success', 'URL copiada'); }}>Copiar</button>
                          </div>
                        </div>
                      </div>
                    )}
                  </div>
                );
              })()}

              <Alert variant="info" style={{ marginTop: 12 }}>
                <strong>Power BI:</strong> Get Data &rarr; OData Feed &rarr; pega la URL de OData &rarr; en Headers agrega <code>Authorization: Bearer {'<token>'}</code>
              </Alert>

              <SectionTitle style={{ marginTop: 24 }}>Webhooks</SectionTitle>
              <p className="form-hint" style={{ marginBottom: 12 }}>
                Notifica a sistemas externos (Laravel, Zapier, n8n) cuando ocurren eventos en el middleware.
              </p>
              <ToggleRow checked={whEnabled} onChange={async () => {
                    const v = !whEnabled;
                    setWhEnabled(v);
                    await api.saveWebhookConfig({ enabled: v });
                    showToast('success', `Webhooks ${v ? 'activados' : 'desactivados'}`);
                  }} activeLabel="Webhooks ACTIVOS" inactiveLabel="Webhooks DESACTIVADOS" />

              {whEnabled && (
                <>
                  {whHooks.map((hook, idx) => (
                    <div key={idx} className="webhook-card">
                      <div className="form-group">
                        <label>URL</label>
                        <input value={hook.url} onChange={e => {
                          const h = [...whHooks]; h[idx] = { ...h[idx], url: e.target.value }; setWhHooks(h);
                        }} placeholder="https://tu-app.com/webhook" />
                      </div>
                      <div className="form-group">
                        <label>Secret (HMAC-SHA256, opcional)</label>
                        <input value={hook.secret} onChange={e => {
                          const h = [...whHooks]; h[idx] = { ...h[idx], secret: e.target.value }; setWhHooks(h);
                        }} placeholder="clave-secreta" />
                      </div>
                      <div className="form-group">
                        <label>Eventos</label>
                        <div className="webhook-events">
                          {['sync_complete', 'send_complete', 'send_paused', 'record_change'].map(ev => (
                            <label key={ev} className="webhook-event-check">
                              <input type="checkbox" checked={hook.events.includes(ev)} onChange={() => {
                                const h = [...whHooks];
                                const events = h[idx].events.includes(ev) ? h[idx].events.filter(e => e !== ev) : [...h[idx].events, ev];
                                h[idx] = { ...h[idx], events };
                                setWhHooks(h);
                              }} />
                              <span>{ev}</span>
                            </label>
                          ))}
                        </div>
                      </div>
                      <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                        <span style={{ marginRight: 8 }}><Toggle checked={hook.active} onChange={() => {
                            const h = [...whHooks]; h[idx] = { ...h[idx], active: !h[idx].active }; setWhHooks(h);
                          }} /></span>
                        <span style={{ color: hook.active ? '#6ee7b7' : '#94a3b8', fontSize: 12 }}>{hook.active ? 'Activo' : 'Inactivo'}</span>
                        <button className="btn-sm btn-danger-sm" style={{ marginLeft: 'auto' }} onClick={() => {
                          setWhHooks(whHooks.filter((_, i) => i !== idx));
                        }}>Eliminar</button>
                        <button className="btn-sm btn-resend" onClick={async () => {
                          await api.testWebhook(hook.url, hook.secret);
                          showToast('success', 'Test enviado a ' + hook.url);
                        }}>Test</button>
                      </div>
                    </div>
                  ))}

                  <div className="config-actions">
                    <button className="btn-test" onClick={() => {
                      setWhHooks([...whHooks, { url: '', secret: '', events: ['sync_complete', 'send_complete'], active: true }]);
                    }}>+ Agregar Webhook</button>
                    <button className="btn-save" onClick={async () => {
                      const valid = whHooks.filter(h => h.url.trim());
                      await api.saveWebhookConfig({ enabled: whEnabled, hooks: valid });
                      setWhHooks(valid);
                      showToast('success', 'Webhooks guardados');
                    }}>Guardar Webhooks</button>
                  </div>
                </>
              )}
            </>
          )}

          {/* ===== AVANZADO ===== */}
          {activeTab === 'advanced' && (
            <>
              <SectionTitle>Envio por Lotes (Batching)</SectionTitle>
              <p className="form-hint" style={{ marginBottom: 12 }}>
                Controla como se agrupan los registros al enviar al servidor.
              </p>
              <div className="form-row">
                <FormGroup label="Registros por lote">
                  <input type="number" value={batchSize} onChange={e => setBatchSize(parseInt(e.target.value) || 50)} />
                </FormGroup>
                <FormGroup label="Pausa entre lotes (ms)">
                  <input type="number" value={batchDelay} onChange={e => setBatchDelay(parseInt(e.target.value) || 0)} />
                </FormGroup>
              </div>

              <SectionTitle>Reintentos Automaticos</SectionTitle>
              <p className="form-hint" style={{ marginBottom: 12 }}>
                Si un registro falla al enviarse, se reintenta automaticamente.
              </p>
              <div className="form-row">
                <FormGroup label="Max reintentos (0 = desactivado)">
                  <input type="number" value={maxRetries} onChange={e => setMaxRetries(parseInt(e.target.value) || 0)} />
                </FormGroup>
                <FormGroup label="Delay entre reintentos (seg)">
                  <input type="number" value={retryDelay} onChange={e => setRetryDelay(parseInt(e.target.value) || 5)} />
                </FormGroup>
              </div>

              <div className="config-actions">
                <button className="btn-save" onClick={handleSaveGeneral}>Guardar</button>
              </div>

              <SectionTitle>Edicion de Registros</SectionTitle>
              <p className="form-hint" style={{ marginBottom: 12 }}>
                Permite editar y eliminar registros individuales desde las paginas de datos.
              </p>
              <ToggleRow checked={allowEditDelete} onChange={async () => {
                    const v = !allowEditDelete;
                    setAllowEditDelete(v);
                    await api.saveAllowEditDelete(v);
                    showToast('success', v ? 'Edicion/eliminacion habilitada' : 'Edicion/eliminacion deshabilitada');
                  }} activeLabel="Editar/Eliminar HABILITADO" inactiveLabel="Editar/Eliminar DESHABILITADO" />
              {allowEditDelete && (
                <Alert variant="warning" style={{ marginTop: 8 }}>
                  Los registros editados se marcaran como "pending" y se re-enviaran al servidor. Los registros eliminados se pierden permanentemente.
                </Alert>
              )}

              <SectionTitle>Backup & Restauracion</SectionTitle>
              <p className="form-hint" style={{ marginBottom: 12 }}>
                Descarga una copia de la base de datos SQLite o restaura desde un archivo .db anterior.
              </p>
              <div className="config-actions">
                <a className="btn-save" href={api.backupURL()} download style={{ textDecoration: 'none', textAlign: 'center' }}>
                  Descargar Backup (.db)
                </a>
                <label className="btn-test" style={{ cursor: 'pointer', textAlign: 'center' }}>
                  Restaurar desde archivo
                  <input type="file" accept=".db,.sqlite,.sqlite3" style={{ display: 'none' }} onChange={async (e) => {
                    const file = e.target.files?.[0];
                    if (!file) return;
                    if (!confirm(`Restaurar la base de datos desde "${file.name}"? Esto reemplazara TODOS los datos actuales.`)) {
                      e.target.value = '';
                      return;
                    }
                    try {
                      const r = await api.restore(file);
                      showToast(r.status === 'ok' ? 'success' : 'error', r.status === 'ok' ? 'Base de datos restaurada exitosamente' : (r.error || 'Error al restaurar'));
                    } catch {
                      showToast('error', 'Error al subir el archivo');
                    }
                    e.target.value = '';
                  }} />
                </label>
              </div>

              <SectionTitle danger>Zona de Peligro</SectionTitle>
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
