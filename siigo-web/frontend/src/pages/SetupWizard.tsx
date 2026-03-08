import { useState, useEffect, useRef, useCallback } from 'react';
import { api } from '../api';

interface TableEntry {
  name: string;
  label: string;
  total: number;
  status: 'pending' | 'populating' | 'done' | 'error';
}

interface Props {
  onComplete: () => void;
}

export default function SetupWizard({ onComplete }: Props) {
  const [tables, setTables] = useState<TableEntry[]>([]);
  const [running, setRunning] = useState(false);
  const [hasRun, setHasRun] = useState(false);
  const [finishing, setFinishing] = useState(false);
  const tablesRef = useRef<TableEntry[]>([]);
  const pollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Keep ref in sync
  useEffect(() => {
    tablesRef.current = tables;
  }, [tables]);

  // Load initial status
  useEffect(() => {
    api.getSetupStatus().then((res: { tables: TableEntry[] }) => {
      setTables(res.tables || []);
    });
  }, []);

  const populateNextRef = useRef<() => void>(() => {});

  // Clear any pending poll timer
  const clearPollTimer = () => {
    if (pollTimerRef.current) {
      clearTimeout(pollTimerRef.current);
      pollTimerRef.current = null;
    }
  };

  // Poll setup-status as fallback when SSE doesn't arrive
  const pollStatus = useCallback(async () => {
    try {
      const res = await api.getSetupStatus();
      const serverTables: TableEntry[] = res.tables || [];
      // Find if the table we were populating is now done on the server
      const populatingTable = tablesRef.current.find(t => t.status === 'populating');
      if (populatingTable) {
        const serverEntry = serverTables.find(t => t.name === populatingTable.name);
        if (serverEntry && serverEntry.status === 'done') {
          // Server says it's done, update local state
          setTables(prev => {
            const updated = prev.map(t =>
              t.name === populatingTable.name ? { ...t, status: 'done' as const, total: serverEntry.total } : t
            );
            tablesRef.current = updated;
            return updated;
          });
          // Chain to next
          setTimeout(() => populateNextRef.current(), 300);
          return;
        }
      }
      // Still populating, poll again in 3s
      pollTimerRef.current = setTimeout(pollStatus, 3000);
    } catch {
      // Retry poll
      pollTimerRef.current = setTimeout(pollStatus, 3000);
    }
  }, []);

  // SSE listener for setup_table_done
  useEffect(() => {
    const token = localStorage.getItem('siigo_token');
    if (!token) return;

    const es = new EventSource(`/api/events?token=${token}`);

    es.addEventListener('setup_table_done', (e: MessageEvent) => {
      clearPollTimer(); // SSE arrived, no need for polling fallback
      const data = JSON.parse(e.data);
      setTables(prev => {
        const updated = prev.map(t =>
          t.name === data.table ? { ...t, status: 'done' as const, total: data.count } : t
        );
        tablesRef.current = updated;
        return updated;
      });

      // Chain to next table after short delay
      setTimeout(() => populateNextRef.current(), 300);
    });

    return () => es.close();
  }, []);

  const populateNext = useCallback(async () => {
    clearPollTimer();
    const next = tablesRef.current.find(t => t.status === 'pending');
    if (!next) {
      setRunning(false);
      setHasRun(true);
      return;
    }

    // Mark as populating locally
    setTables(prev => {
      const updated = prev.map(t =>
        t.name === next.name ? { ...t, status: 'populating' as const } : t
      );
      tablesRef.current = updated;
      return updated;
    });

    try {
      await api.setupPopulate(next.name);
      // Start polling fallback in case SSE doesn't arrive
      pollTimerRef.current = setTimeout(pollStatus, 3000);
    } catch {
      // If request fails, mark as done with error and continue
      setTables(prev => {
        const updated = prev.map(t =>
          t.name === next.name ? { ...t, status: 'done' as const, total: 0 } : t
        );
        tablesRef.current = updated;
        return updated;
      });
      setTimeout(() => populateNext(), 300);
    }
  }, [pollStatus]);

  // Keep ref updated
  useEffect(() => {
    populateNextRef.current = populateNext;
  }, [populateNext]);

  // Cleanup poll timer on unmount
  useEffect(() => {
    return () => clearPollTimer();
  }, []);

  const handleStart = () => {
    setRunning(true);
    populateNext();
  };

  const handleFinish = async () => {
    setFinishing(true);
    try {
      await api.setupComplete();
      onComplete();
    } catch {
      setFinishing(false);
    }
  };

  const doneCount = tables.filter(t => t.status === 'done').length;
  const totalCount = tables.length;
  const allDone = totalCount > 0 && doneCount === totalCount;
  const isPopulating = tables.some(t => t.status === 'populating');
  const hasPending = tables.some(t => t.status === 'pending');
  const progress = totalCount > 0 ? (doneCount / totalCount) * 100 : 0;

  // Show finish button when all done, OR when population ran and nothing is pending/populating anymore
  const canFinish = allDone || (hasRun && !running && !isPopulating && !hasPending);

  return (
    <div className="setup-page">
      <div className="setup-box">
        <div className="setup-header">
          <h1>Poblacion Inicial de Datos</h1>
          <p>Se leeran los archivos ISAM de Siigo y se cargaran en la base de datos local</p>
        </div>

        <div className="setup-progress">
          <div className="setup-progress-bar">
            <div className="setup-progress-fill" style={{ width: `${progress}%` }} />
          </div>
          <div className="setup-progress-text">{doneCount}/{totalCount} tablas</div>
        </div>

        <div className="setup-table-list">
          {tables.map((t, i) => (
            <div key={t.name} className={`setup-table-row ${t.status}`}>
              <div className="setup-table-num">{i + 1}</div>
              <div className={`setup-status-icon ${t.status}`}>
                {t.status === 'done' && <span>&#10003;</span>}
                {t.status === 'populating' && <span className="setup-spinner">&#9679;</span>}
                {t.status === 'pending' && <span>&#9675;</span>}
              </div>
              <div className="setup-table-name">{t.label}</div>
              <div className="setup-table-count">
                {t.status === 'done' && `${t.total.toLocaleString()} registros`}
                {t.status === 'populating' && 'Procesando...'}
                {t.status === 'pending' && 'Pendiente'}
              </div>
            </div>
          ))}
        </div>

        <div className="setup-actions">
          {!canFinish && !running && (
            <button className="setup-btn setup-btn-primary" onClick={handleStart}>
              {hasRun ? 'Reintentar Poblacion' : 'Iniciar Poblacion'}
            </button>
          )}
          {running && isPopulating && (
            <button className="setup-btn setup-btn-primary" disabled>
              Poblando...
            </button>
          )}
          {canFinish && (
            <button
              className="setup-btn setup-btn-success"
              onClick={handleFinish}
              disabled={finishing}
            >
              {finishing ? 'Finalizando...' : 'Finalizar Setup'}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
