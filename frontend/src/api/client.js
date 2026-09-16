import axios from 'axios';

const api = axios.create({
  baseURL: '',
  headers: { 'Content-Type': 'application/json' },
});

// Request: agrega el JWT
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Response: maneja 401
//
// Nota: si varias requests concurrentes reciben 401 a la vez, se disparan
// varios redirects a /login. Es aceptable para esta prueba. Una versión
// más robusta delegaría el manejo al AuthContext con un flag de "logging out".
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token');
      if (window.location.pathname !== '/login' && window.location.pathname !== '/register') {
        window.location.href = '/login';
      }
    }
    return Promise.reject(error);
  }
);

export default api;

// Decodifica el payload de un JWT (sin verificar firma).
//
// El payload usa Base64URL, no Base64 estándar. Los caracteres `-` y `_`
// se reemplazan por `+` y `/`, y se agrega el padding `=` necesario.
//
// IMPORTANTE: esto NO verifica la firma. Solo se usa para leer `tb_account_id`
// como dato auxiliar. Nunca debe usarse para autorizar operaciones.
export function decodeJWT(token) {
  try {
    const base64Url = token.split('.')[1];
    const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/');
    const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4);

    const json = decodeURIComponent(
      atob(padded)
        .split('')
        .map((c) => '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2))
        .join('')
    );
    return JSON.parse(json);
  } catch {
    return {};
  }
}