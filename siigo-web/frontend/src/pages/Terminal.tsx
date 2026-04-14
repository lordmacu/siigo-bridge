import { useEffect, useRef, useState } from 'react';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import '@xterm/xterm/css/xterm.css';
import PageHeader from '../components/PageHeader';

export default function Terminal() {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string>('');

  const connect = () => {
    const token = localStorage.getItem('siigo_token');
    if (!token) {
      setError('No hay sesion activa');
      return;
    }
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${proto}//${window.location.host}/api/terminal/ws?token=${encodeURIComponent(token)}`;
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      setConnected(true);
      setError('');
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
    ws.onclose = () => {
      setConnected(false);
      termRef.current?.write('\r\n\x1b[31m[Desconectado]\x1b[0m\r\n');
    };
  };

  const disconnect = () => {
    wsRef.current?.close();
    wsRef.current = null;
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
      allowTransparency: false,
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
    term.writeln('Click "Conectar" para iniciar sesion shell en el servidor.');
    term.writeln('');

    connect();

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
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <div style={{
            width: 10, height: 10, borderRadius: '50%',
            background: connected ? '#10b981' : '#64748b'
          }} />
          <span style={{ fontSize: 13, color: connected ? '#10b981' : '#94a3b8' }}>
            {connected ? 'Conectado' : 'Desconectado'}
          </span>
          {error && <span style={{ color: '#ef4444', fontSize: 13, marginLeft: 8 }}>{error}</span>}
          <div style={{ flex: 1 }} />
          {connected ? (
            <button className="btn-danger" onClick={disconnect}>Desconectar</button>
          ) : (
            <button className="btn-save" onClick={connect}>Conectar</button>
          )}
        </div>
        <div
          ref={containerRef}
          style={{
            height: 'calc(100vh - 200px)',
            background: '#0f172a',
            border: '1px solid #334155',
            borderRadius: 6,
            padding: 8,
          }}
        />
        <p className="form-hint" style={{ fontSize: 12 }}>
          Shell persistente ({navigator.userAgent.includes('Windows') ? 'PowerShell' : 'bash'}) con PTY real, colores ANSI, Ctrl+C, historial.
          La sesion corre como el mismo usuario que ejecuta Siigo Bridge.
        </p>
      </div>
    </>
  );
}
