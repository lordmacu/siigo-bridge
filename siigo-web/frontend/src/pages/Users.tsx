import { useState, useEffect } from 'react';
import { api } from '../api';
import { showToast } from '../components/Toast';

const MODULE_LABELS: Record<string, string> = {
  dashboard: 'Dashboard',
  clients: 'Clientes',
  products: 'Productos',
  movements: 'Movimientos',
  cartera: 'Cartera',
  'field-mappings': 'Mapeo Campos',
  errors: 'Errores',
  logs: 'Logs',
  explorer: 'SQL Explorer',
  config: 'Configuracion',
  users: 'Usuarios',
};

const ROLE_LABELS: Record<string, string> = {
  admin: 'Administrador',
  editor: 'Editor',
  viewer: 'Solo Lectura',
};

interface AppUser {
  id: number;
  username: string;
  role: string;
  permissions: string[];
  active: boolean;
  created_at: string;
  updated_at: string;
}

export default function Users() {
  const [users, setUsers] = useState<AppUser[]>([]);
  const [allModules, setAllModules] = useState<string[]>([]);
  const [showCreate, setShowCreate] = useState(false);
  const [editUser, setEditUser] = useState<AppUser | null>(null);

  // Create form
  const [newUsername, setNewUsername] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newRole, setNewRole] = useState('viewer');
  const [newPerms, setNewPerms] = useState<string[]>(['dashboard']);

  // Edit form
  const [editRole, setEditRole] = useState('');
  const [editPerms, setEditPerms] = useState<string[]>([]);
  const [editActive, setEditActive] = useState(true);
  const [editPassword, setEditPassword] = useState('');

  const load = () => {
    api.getUsers().then(res => {
      setUsers(res.users || []);
      setAllModules(res.all_modules || []);
    }).catch(() => {});
  };

  useEffect(load, []);

  const handleCreate = async () => {
    if (!newUsername.trim() || !newPassword.trim()) {
      showToast('error', 'Usuario y contrasena son requeridos');
      return;
    }
    const res = await api.createUser({
      username: newUsername,
      password: newPassword,
      role: newRole,
      permissions: newPerms,
    });
    if (res.status === 'ok') {
      showToast('success', 'Usuario creado');
      setShowCreate(false);
      setNewUsername(''); setNewPassword(''); setNewRole('viewer'); setNewPerms(['dashboard']);
      load();
    } else {
      showToast('error', res.error || 'Error al crear');
    }
  };

  const handleEdit = (user: AppUser) => {
    setEditUser(user);
    setEditRole(user.role);
    setEditPerms([...user.permissions]);
    setEditActive(user.active);
    setEditPassword('');
  };

  const handleSaveEdit = async () => {
    if (!editUser) return;
    const data: Record<string, unknown> = {
      role: editRole,
      permissions: editPerms,
      active: editActive,
    };
    if (editPassword.trim()) data.password = editPassword;
    const res = await api.updateUser(editUser.id, data);
    if (res.status === 'ok') {
      showToast('success', 'Usuario actualizado');
      setEditUser(null);
      load();
    } else {
      showToast('error', res.error || 'Error al actualizar');
    }
  };

  const handleDelete = async (user: AppUser) => {
    if (!confirm(`Eliminar usuario "${user.username}"?`)) return;
    const res = await api.deleteUser(user.id);
    if (res.status === 'ok') {
      showToast('success', 'Usuario eliminado');
      load();
    } else {
      showToast('error', res.error || 'Error al eliminar');
    }
  };

  const togglePerm = (perms: string[], perm: string, setter: (p: string[]) => void) => {
    if (perms.includes(perm)) {
      setter(perms.filter(p => p !== perm));
    } else {
      setter([...perms, perm]);
    }
  };

  const renderPermToggles = (perms: string[], setter: (p: string[]) => void) => (
    <div className="perm-toggles">
      {allModules.filter(m => m !== 'users').map(mod => (
        <div key={mod} className="send-toggle-row" style={{ marginBottom: 4 }}>
          <label className="toggle-switch">
            <input
              type="checkbox"
              checked={perms.includes(mod)}
              onChange={() => togglePerm(perms, mod, setter)}
            />
            <span className="toggle-slider"></span>
          </label>
          <span className={`send-toggle-label ${perms.includes(mod) ? 'active' : 'inactive'}`}>
            {MODULE_LABELS[mod] || mod}
          </span>
        </div>
      ))}
    </div>
  );

  return (
    <>
      <div className="topbar">
        <h2>Usuarios</h2>
        <button className="btn-save" onClick={() => setShowCreate(true)}>+ Nuevo Usuario</button>
      </div>
      <div className="content">
        <div className="config-msg warning" style={{ marginBottom: 16 }}>
          El usuario root (configurado en config.json) siempre tiene acceso total y no aparece aqui.
        </div>

        <table className="data-table">
          <thead>
            <tr>
              <th>Usuario</th>
              <th>Rol</th>
              <th>Permisos</th>
              <th>Estado</th>
              <th>Acciones</th>
            </tr>
          </thead>
          <tbody>
            {users.length === 0 && (
              <tr><td colSpan={5} style={{ textAlign: 'center', color: '#64748b', padding: 24 }}>No hay usuarios creados</td></tr>
            )}
            {users.map(user => (
              <tr key={user.id}>
                <td className="col-name">{user.username}</td>
                <td className="col-type">{ROLE_LABELS[user.role] || user.role}</td>
                <td>
                  <div className="perm-badges">
                    {user.role === 'admin' ? (
                      <span className="perm-badge all">Todos</span>
                    ) : (
                      user.permissions.map(p => (
                        <span key={p} className="perm-badge">{MODULE_LABELS[p] || p}</span>
                      ))
                    )}
                  </div>
                </td>
                <td>
                  <span className={`status-badge ${user.active ? 'active' : 'inactive'}`}>
                    {user.active ? 'Activo' : 'Inactivo'}
                  </span>
                </td>
                <td>
                  <div style={{ display: 'flex', gap: 6 }}>
                    <button className="btn-edit" onClick={() => handleEdit(user)}>Editar</button>
                    <button className="btn-danger-sm" onClick={() => handleDelete(user)}>Eliminar</button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>

        {/* CREATE MODAL */}
        {showCreate && (
          <div className="modal-overlay" onClick={() => setShowCreate(false)}>
            <div className="user-modal" onClick={e => e.stopPropagation()}>
              <div className="user-modal-header">
                <h3>Nuevo Usuario</h3>
                <button className="user-modal-close" onClick={() => setShowCreate(false)}>&times;</button>
              </div>
              <div className="user-modal-body">
                <div className="form-group">
                  <label>Username</label>
                  <input value={newUsername} onChange={e => setNewUsername(e.target.value)} placeholder="nombre de usuario" autoFocus />
                </div>
                <div className="form-group">
                  <label>Password</label>
                  <input type="password" value={newPassword} onChange={e => setNewPassword(e.target.value)} placeholder="contrasena" />
                </div>
                <div className="form-group">
                  <label>Rol</label>
                  <select value={newRole} onChange={e => setNewRole(e.target.value)} className="form-select">
                    <option value="viewer">Solo Lectura</option>
                    <option value="editor">Editor</option>
                    <option value="admin">Administrador</option>
                  </select>
                </div>
                {newRole !== 'admin' && (
                  <div className="form-group">
                    <label>Modulos permitidos</label>
                    {renderPermToggles(newPerms, setNewPerms)}
                  </div>
                )}
                {newRole === 'admin' && (
                  <div className="config-msg" style={{ color: '#6ee7b7', marginTop: 8 }}>
                    Los administradores tienen acceso a todos los modulos.
                  </div>
                )}
              </div>
              <div className="user-modal-footer">
                <button className="btn-cancel" onClick={() => setShowCreate(false)}>Cancelar</button>
                <button className="btn-save" onClick={handleCreate}>Crear Usuario</button>
              </div>
            </div>
          </div>
        )}

        {/* EDIT MODAL */}
        {editUser && (
          <div className="modal-overlay" onClick={() => setEditUser(null)}>
            <div className="user-modal" onClick={e => e.stopPropagation()}>
              <div className="user-modal-header">
                <h3>Editar: {editUser.username}</h3>
                <button className="user-modal-close" onClick={() => setEditUser(null)}>&times;</button>
              </div>
              <div className="user-modal-body">
                <div className="form-group">
                  <label>Nueva contrasena</label>
                  <input type="password" value={editPassword} onChange={e => setEditPassword(e.target.value)} placeholder="dejar vacio para no cambiar" />
                </div>
                <div className="form-group">
                  <label>Rol</label>
                  <select value={editRole} onChange={e => setEditRole(e.target.value)} className="form-select">
                    <option value="viewer">Solo Lectura</option>
                    <option value="editor">Editor</option>
                    <option value="admin">Administrador</option>
                  </select>
                </div>
                {editRole !== 'admin' && (
                  <div className="form-group">
                    <label>Modulos permitidos</label>
                    {renderPermToggles(editPerms, setEditPerms)}
                  </div>
                )}
                {editRole === 'admin' && (
                  <div className="config-msg" style={{ color: '#6ee7b7', marginTop: 8 }}>
                    Los administradores tienen acceso a todos los modulos.
                  </div>
                )}
                <div className="send-toggle-row" style={{ marginTop: 16 }}>
                  <label className="toggle-switch">
                    <input type="checkbox" checked={editActive} onChange={() => setEditActive(!editActive)} />
                    <span className="toggle-slider"></span>
                  </label>
                  <span className={`send-toggle-label ${editActive ? 'active' : 'inactive'}`}>
                    {editActive ? 'Usuario ACTIVO' : 'Usuario INACTIVO'}
                  </span>
                </div>
              </div>
              <div className="user-modal-footer">
                <button className="btn-cancel" onClick={() => setEditUser(null)}>Cancelar</button>
                <button className="btn-save" onClick={handleSaveEdit}>Guardar Cambios</button>
              </div>
            </div>
          </div>
        )}
      </div>
    </>
  );
}
