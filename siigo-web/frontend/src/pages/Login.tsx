import { useState } from 'react';
import { api, UserInfo } from '../api';

export default function Login({ onLogin }: { onLogin: (info: UserInfo, setupComplete?: boolean) => void }) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [showPass, setShowPass] = useState(false);
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
        onLogin({
          username: res.username || username,
          role: res.role || 'root',
          permissions: res.permissions || [],
        }, res.setup_complete as boolean);
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
          <div className="password-wrapper">
            <input
              type={showPass ? 'text' : 'password'}
              value={password}
              onChange={e => setPassword(e.target.value)}
              placeholder="Contrasena"
            />
            <button
              type="button"
              className="password-toggle"
              onClick={() => setShowPass(!showPass)}
              tabIndex={-1}
            >
              {showPass ? (
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94"/>
                  <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19"/>
                  <line x1="1" y1="1" x2="23" y2="23"/>
                </svg>
              ) : (
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
                  <circle cx="12" cy="12" r="3"/>
                </svg>
              )}
            </button>
          </div>
        </div>
        <button className="login-btn" type="submit" disabled={loading}>
          {loading ? 'Ingresando...' : 'Ingresar'}
        </button>
      </form>
    </div>
  );
}
