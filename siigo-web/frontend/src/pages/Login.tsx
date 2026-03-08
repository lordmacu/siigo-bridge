import { useState } from 'react';
import { api } from '../api';

export default function Login({ onLogin }: { onLogin: () => void }) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      const res = await api.login(username, password);
      if (res.token) {
        localStorage.setItem('siigo_token', res.token);
        onLogin();
      } else {
        setError(res.error || 'Error de autenticacion');
      }
    } catch {
      setError('Error de conexion');
    }
    setLoading(false);
  };

  return (
    <div className="login-page">
      <form className="login-box" onSubmit={handleSubmit}>
        <div className="login-header">
          <h1>Siigo Web</h1>
          <p>Ingresa tus credenciales</p>
        </div>
        {error && <div className="login-error">{error}</div>}
        <div className="form-group">
          <label>Usuario</label>
          <input
            value={username}
            onChange={e => setUsername(e.target.value)}
            placeholder="Usuario"
            autoFocus
          />
        </div>
        <div className="form-group">
          <label>Contrasena</label>
          <input
            type="password"
            value={password}
            onChange={e => setPassword(e.target.value)}
            placeholder="Contrasena"
          />
        </div>
        <button className="login-btn" type="submit" disabled={loading}>
          {loading ? 'Ingresando...' : 'Ingresar'}
        </button>
      </form>
    </div>
  );
}
