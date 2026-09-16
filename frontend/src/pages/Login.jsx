import { useState } from 'react';
import { Link, useNavigate, useLocation } from 'react-router-dom';
import { Landmark, LogIn } from 'lucide-react';
import { useAuth } from '../contexts/AuthContext';

export default function Login() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(false);

  const from = location.state?.from?.pathname || '/dashboard';

  async function handleSubmit(e) {
    e.preventDefault();
    setError(null);
    setLoading(true);

    try {
      await login(email, password);
      navigate(from, { replace: true });
    } catch (err) {
      const code = err.response?.data?.code;
      const message = err.response?.data?.message || 'Error al iniciar sesión';

      if (code === 'ACCOUNT_PENDING') {
        setError('Tu cuenta está siendo procesada. Intenta en unos minutos.');
      } else if (code === 'INVALID_CREDENTIALS') {
        setError('Credenciales inválidas');
      } else {
        setError(message);
      }
    } finally {
      setLoading(false);
    }
  }

  function fillDemo() {
    setEmail('demo@banco.com');
    setPassword('Demo1234!');
    setError(null);
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4">
      <div className="w-full max-w-md">
        {/* Logo */}
        <div className="flex flex-col items-center mb-8">
          <Landmark className="text-brand-600 mb-2" size={40} />
          <h1 className="text-2xl font-bold text-gray-900">Banking System</h1>
          <p className="text-sm text-gray-500 mt-1">
            Sistema de banca en línea
          </p>
        </div>

        {/* Card de login */}
        <div className="card p-8">
          <h2 className="text-xl font-semibold mb-6">Iniciar sesión</h2>

          {error && (
            <div className="error-box mb-4">{error}</div>
          )}

          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="label" htmlFor="email">Email</label>
              <input
                id="email"
                type="email"
                className="input"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="tu@email.com"
                required
                autoComplete="email"
                disabled={loading}
              />
            </div>

            <div>
              <label className="label" htmlFor="password">Contraseña</label>
              <input
                id="password"
                type="password"
                className="input"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••"
                required
                autoComplete="current-password"
                disabled={loading}
              />
            </div>

            <button
              type="submit"
              className="btn-primary w-full"
              disabled={loading}
            >
              <LogIn size={16} />
              {loading ? 'Ingresando...' : 'Ingresar'}
            </button>
          </form>

          <div className="mt-6 pt-6 border-t border-gray-100 text-center text-sm">
            <span className="text-gray-500">¿No tienes cuenta? </span>
            <Link to="/register" className="text-brand-600 font-medium hover:underline">
              Crear cuenta
            </Link>
          </div>
        </div>

        {/* Credenciales demo */}
        <div className="mt-6 card p-4 bg-brand-50 border-brand-100">
          <p className="text-xs font-medium text-brand-900 mb-2">
            Cuenta de prueba (demo)
          </p>
          <div className="text-xs text-brand-800 space-y-1">
            <p><span className="font-mono">demo@banco.com</span> / <span className="font-mono">Demo1234!</span></p>
            <p><span className="font-mono">demo2@banco.com</span> / <span className="font-mono">Demo1234!</span></p>
          </div>
          <button
            type="button"
            onClick={fillDemo}
            className="mt-3 text-xs text-brand-600 font-medium hover:underline"
          >
            Usar credenciales demo →
          </button>
        </div>
      </div>
    </div>
  );
}