import { useState, useEffect } from 'react';
import { api } from '../api';

interface Stats { [key: string]: number }
interface ISAMFile { file: string; record_size: number; records: number; num_keys: number; used_extfh: boolean; mod_time: string }

export default function Dashboard() {
  const [stats, setStats] = useState<Stats>({});
  const [isamInfo, setIsamInfo] = useState<ISAMFile[]>([]);

  useEffect(() => {
    api.getStats().then(setStats);
    api.getISAMInfo().then(setIsamInfo);
  }, []);

  const cards = [
    { label: 'Clientes', total: stats.clients_total || 0, synced: stats.clients_synced || 0, pending: stats.clients_pending || 0, errors: stats.clients_errors || 0, color: 'green' },
    { label: 'Productos', total: stats.products_total || 0, synced: stats.products_synced || 0, pending: stats.products_pending || 0, errors: stats.products_errors || 0, color: 'blue' },
    { label: 'Movimientos', total: stats.movements_total || 0, synced: stats.movements_synced || 0, pending: stats.movements_pending || 0, errors: stats.movements_errors || 0, color: 'yellow' },
    { label: 'Cartera', total: stats.cartera_total || 0, synced: stats.cartera_synced || 0, pending: stats.cartera_pending || 0, errors: stats.cartera_errors || 0, color: 'purple' },
  ];

  return (
    <>
      <div className="topbar"><h2>Dashboard</h2></div>
      <div className="content">
        <div className="stats-row">
          {cards.map(c => (
            <div key={c.label} className="stat-card">
              <div className="label">{c.label}</div>
              <div className={`value ${c.color}`}>{c.total}</div>
              <div className="stat-details">
                <span className="stat-synced">{c.synced} sincronizados</span>
                {c.pending > 0 && <span className="stat-pending">{c.pending} pendientes</span>}
                {c.errors > 0 && <span className="stat-errors">{c.errors} errores</span>}
              </div>
            </div>
          ))}
        </div>

        <h3 className="section-title">Archivos ISAM (Siigo)</h3>
        <div className="file-info">
          {isamInfo.map(f => (
            <div key={f.file} className="file-card">
              <h3>{f.file}</h3>
              <div className="info-row"><span className="label">Registros:</span><span>{f.records >= 0 ? f.records : 'Error'}</span></div>
              <div className="info-row"><span className="label">Tam. registro:</span><span>{f.record_size || '-'} bytes</span></div>
              <div className="info-row"><span className="label">EXTFH:</span><span>{f.used_extfh ? 'Si' : 'No'}</span></div>
              <div className="info-row"><span className="label">Modificado:</span><span>{f.mod_time || '-'}</span></div>
            </div>
          ))}
        </div>
      </div>
    </>
  );
}
