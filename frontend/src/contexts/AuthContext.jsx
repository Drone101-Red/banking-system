import { createContext, useContext, useState, useEffect } from 'react';
import api, { decodeJWT } from '../api/client';

const AuthContext = createContext(null);

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null);
  const [tbAccountID, setTbAccountID] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const token = localStorage.getItem('token');
    if (!token) {
      setLoading(false);
      return;
    }

    const claims = decodeJWT(token);
    setTbAccountID(claims.tb_account_id || null);

    api.get('/api/auth/me')
      .then(({ data }) => setUser(data))
      .catch(() => {
        localStorage.removeItem('token');
        setTbAccountID(null);
      })
      .finally(() => setLoading(false));
  }, []);

  async function login(email, password) {
    const { data } = await api.post('/api/auth/login', { email, password });
    const claims = decodeJWT(data.token);

    localStorage.setItem('token', data.token);

    setUser(data.user);
    setTbAccountID(claims.tb_account_id);
    return data.user;
  }

  async function register(email, password, fullName) {
    const { data } = await api.post('/api/auth/register', {
      email,
      password,
      full_name: fullName,
    });
    return data;
  }

  // Logout stateless.
  //
  // El backend expone POST /api/auth/logout pero no mantiene blacklist.
  // El cierre de sesión es responsabilidad del cliente: se borra el token
  // y el estado local. El endpoint existe por completitud del contrato.
  function logout() {
    localStorage.removeItem('token');
    setUser(null);
    setTbAccountID(null);
  }

  return (
    <AuthContext.Provider value={{ user, tbAccountID, loading, login, register, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth debe usarse dentro de AuthProvider');
  return ctx;
}