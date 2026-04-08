import { useState, useEffect } from 'react';
import { api } from '../api';
import PageHeader from '../components/PageHeader';

interface AuditEntry {
  id: number;
  username: string;
  action: string;
  table_name: string;
  record_id: string;
  details: string;
  created_at: string;
}

export default function AuditTrail() {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);

  useEffect(() => {
    api.getAuditTrail(page).then(res => {
      setEntries(res.entries || []);
      setTotal(res.total || 0);
    }).catch(() => {});
  }, [page]);

  return (
    <>
      <PageHeader title="Audit Trail">
        <span style={{ color: '#64748b', fontSize: 13 }}>{total} registros</span>
      </PageHeader>
      <div className="content">
        <table className="audit-table">
          <thead>
            <tr>
              <th>Fecha</th>
              <th>Usuario</th>
              <th>Accion</th>
              <th>Tabla</th>
              <th>Registro</th>
              <th>Detalles</th>
            </tr>
          </thead>
          <tbody>
            {entries.length === 0 && (
              <tr><td colSpan={6} style={{ textAlign: 'center', color: '#64748b', padding: 24 }}>No hay registros de auditoria</td></tr>
            )}
            {entries.map(e => (
              <tr key={e.id}>
                <td style={{ whiteSpace: 'nowrap', fontSize: 12 }}>{e.created_at?.replace('T', ' ').slice(0, 19)}</td>
                <td>{e.username}</td>
                <td><span className={`audit-action ${e.action}`}>{e.action}</span></td>
                <td>{e.table_name || '-'}</td>
                <td>{e.record_id || '-'}</td>
                <td style={{ maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis' }}>{e.details || '-'}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {total > 50 && (
          <div className="pagination" style={{ marginTop: 16 }}>
            <button disabled={page <= 1} onClick={() => setPage(p => p - 1)}>Anterior</button>
            <span style={{ color: '#94a3b8', fontSize: 13 }}>Pagina {page} de {Math.ceil(total / 50)}</span>
            <button disabled={page * 50 >= total} onClick={() => setPage(p => p + 1)}>Siguiente</button>
          </div>
        )}
      </div>
    </>
  );
}
