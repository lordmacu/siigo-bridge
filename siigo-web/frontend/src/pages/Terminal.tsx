import { useEffect, useRef, useState } from 'react';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import '@xterm/xterm/css/xterm.css';
import PageHeader from '../components/PageHeader';
import { api } from '../api';
import { showToast } from '../components/Toast';

export default function Terminal() {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string>('');
  const [pinSet, setPinSet] = useState<boolean | null>(null);
  const [pinInput, setPinInput] = useState('');
  const [showSetPinModal, setShowSetPinModal] = useState(false);
  const [newPin, setNewPin] = useState('');
  const [newPin2, setNewPin2] = useState('');

  useEffect(() => {
    api.terminalPinStatus().then(r => setPinSet(!!r.set)).catch(() => setPinSet(false));
  }, []);

  const connect = (pin?: string) => {
    const token = localStorage.getItem('siigo_token');
    if (!token) {
      setError('No hay sesion activa');
      return;
    }
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    let url = `${proto}//${window.location.host}/api/terminal/ws?token=${encodeURIComponent(token)}`;
    if (pin) url += `&pin=${encodeURIComponent(pin)}`;
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      setConnected(true);
      setError('');
      sessionStorage.setItem('terminal_pin', pin || '');
      const term = termRef.current!;
      const fit = fitRef.current!;
      setTimeout(() => {
        fit.fit();
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
      }, 100);
    };

    ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(ev.data);
        if (msg.type === 'output') {
          termRef.current?.write(msg.data);
        } else if (msg.type === 'exit') {
          const detail = msg.data ? ` - ${msg.data}` : '';
          termRef.current?.write(`\r\n\x1b[33m[Sesion terminada, codigo=${msg.code}${detail}]\x1b[0m\r\n`);
          setConnected(false);
        }
      } catch { /* ignore */ }
    };

    ws.onerror = () => setError('Error de conexion');
    ws.onclose = (ev) => {
      setConnected(false);
      if (ev.code === 1006 && !connected) {
        setError('PIN incorrecto o acceso denegado');
        sessionStorage.removeItem('terminal_pin');
      }
      termRef.current?.write('\r\n\x1b[31m[Desconectado]\x1b[0m\r\n');
    };
  };

  const attemptConnect = () => {
    if (pinSet) {
      const cached = sessionStorage.getItem('terminal_pin');
      if (cached) { connect(cached); return; }
      if (!pinInput) { setError('Ingresa el PIN'); return; }
      connect(pinInput);
    } else {
      connect();
    }
  };

  const disconnect = () => {
    wsRef.current?.close();
    wsRef.current = null;
  };

  const savePin = async () => {
    if (newPin !== newPin2) { showToast('error', 'Los PINs no coinciden'); return; }
    if (newPin.length < 8) { showToast('error', 'Minimo 8 caracteres'); return; }
    try {
      const r = await api.terminalSetPin(newPin);
      if (r.status === 'ok') {
        showToast('success', 'PIN guardado');
        setPinSet(r.set);
        setShowSetPinModal(false);
        setNewPin(''); setNewPin2('');
        sessionStorage.removeItem('terminal_pin');
      } else {
        showToast('error', r.error || 'Error');
      }
    } catch {
      showToast('error', 'Error de red');
    }
  };

  const clearPin = async () => {
    if (!confirm('Desactivar el PIN? Cualquier admin podra abrir la terminal.')) return;
    try {
      const r = await api.terminalSetPin('');
      if (r.status === 'ok') {
        showToast('success', 'PIN desactivado');
        setPinSet(false);
        sessionStorage.removeItem('terminal_pin');
      }
    } catch { showToast('error', 'Error'); }
  };

  useEffect(() => {
    if (!containerRef.current) return;

    const term = new XTerm({
      fontFamily: 'Consolas, "Courier New", monospace',
      fontSize: 13,
      theme: {
        background: '#0f172a',
        foreground: '#e2e8f0',
        cursor: '#10b981',
        selectionBackground: '#334155',
        black: '#1e293b',
        red: '#ef4444',
        green: '#10b981',
        yellow: '#f59e0b',
        blue: '#3b82f6',
        magenta: '#a855f7',
        cyan: '#06b6d4',
        white: '#e2e8f0',
        brightBlack: '#475569',
        brightRed: '#f87171',
        brightGreen: '#34d399',
        brightYellow: '#fbbf24',
        brightBlue: '#60a5fa',
        brightMagenta: '#c084fc',
        brightCyan: '#22d3ee',
        brightWhite: '#f8fafc',
      },
      cursorBlink: true,
      scrollback: 5000,
    });

    const fit = new FitAddon();
    term.loadAddon(fit);
    term.loadAddon(new WebLinksAddon());

    term.open(containerRef.current);
    fit.fit();

    term.onData((data) => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.send(JSON.stringify({ type: 'input', data }));
      }
    });

    termRef.current = term;
    fitRef.current = fit;

    const onResize = () => {
      if (!fitRef.current || !termRef.current) return;
      fitRef.current.fit();
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.send(JSON.stringify({
          type: 'resize',
          cols: termRef.current.cols,
          rows: termRef.current.rows,
        }));
      }
    };
    window.addEventListener('resize', onResize);

    term.writeln('\x1b[36mTerminal web - Siigo Bridge\x1b[0m');
    term.writeln('Ingresa el PIN y click "Conectar" para iniciar sesion shell.');
    term.writeln('');

    return () => {
      window.removeEventListener('resize', onResize);
      wsRef.current?.close();
      term.dispose();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <>
      <PageHeader title="Terminal" />
      <div className="config-container" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
          <div style={{
            width: 10, height: 10, borderRadius: '50%',
            background: connected ? '#10b981' : '#64748b'
          }} />
          <span style={{ fontSize: 13, color: connected ? '#10b981' : '#94a3b8' }}>
            {connected ? 'Conectado' : 'Desconectado'}
          </span>
          <span style={{ fontSize: 12, color: '#64748b' }}>
            {pinSet === null ? '' : pinSet ? '[PIN activo]' : '[sin PIN]'}
          </span>
          {error && <span style={{ color: '#ef4444', fontSize: 13 }}>{error}</span>}
          <div style={{ flex: 1 }} />
          {!connected && pinSet && (
            <input
              type="password"
              placeholder="PIN"
              value={pinInput}
              onChange={e => setPinInput(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && attemptConnect()}
              style={{
                padding: '6px 10px', background: '#1e293b', border: '1px solid #334155',
                borderRadius: 4, color: '#e2e8f0', width: 180, fontFamily: 'monospace'
              }}
            />
          )}
          {connected ? (
            <button className="btn-danger" onClick={disconnect}>Desconectar</button>
          ) : (
            <button className="btn-save" onClick={attemptConnect}>Conectar</button>
          )}
          <button className="btn-test" onClick={() => setShowSetPinModal(true)}>
            {pinSet ? 'Cambiar PIN' : 'Establecer PIN'}
          </button>
          {pinSet && <button className="btn-test" onClick={clearPin}>Quitar PIN</button>}
        </div>
        <div
          ref={containerRef}
          style={{
            height: 'calc(100vh - 220px)',
            background: '#0f172a',
            border: '1px solid #334155',
            borderRadius: 6,
            padding: 8,
          }}
        />
        <p className="form-hint" style={{ fontSize: 12 }}>
          Shell persistente ({navigator.userAgent.includes('Windows') ? 'PowerShell' : 'bash'}) con PTY real.
          Acceso: rol admin/root + PIN opcional. El PIN se guarda como bcrypt y requiere minimo 8 caracteres.
        </p>
      </div>

      {showSetPinModal && (
        <div style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)',
          display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000
        }}>
          <div style={{
            background: '#1e293b', padding: 24, borderRadius: 8, width: 400,
            border: '1px solid #334155'
          }}>
            <h3 style={{ margin: '0 0 16px 0', color: '#e2e8f0' }}>
              {pinSet ? 'Cambiar PIN de terminal' : 'Establecer PIN de terminal'}
            </h3>
            <p className="form-hint" style={{ marginBottom: 16 }}>
              Minimo 8 caracteres. Se guarda como bcrypt en config.json. Deja vacio + guardar para desactivar.
            </p>
            <input
              type="password"
              placeholder="Nuevo PIN"
              value={newPin}
              onChange={e => setNewPin(e.target.value)}
              autoFocus
              style={{
                width: '100%', padding: 10, marginBottom: 10,
                background: '#0f172a', border: '1px solid #334155', color: '#e2e8f0',
                borderRadius: 4, fontFamily: 'monospace'
              }}
            />
            <input
              type="password"
              placeholder="Repetir PIN"
              value={newPin2}
              onChange={e => setNewPin2(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && savePin()}
              style={{
                width: '100%', padding: 10, marginBottom: 16,
                background: '#0f172a', border: '1px solid #334155', color: '#e2e8f0',
                borderRadius: 4, fontFamily: 'monospace'
              }}
            />
            <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
              <button className="btn-test" onClick={() => { setShowSetPinModal(false); setNewPin(''); setNewPin2(''); }}>
                Cancelar
              </button>
              <button className="btn-save" onClick={savePin}>Guardar</button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
