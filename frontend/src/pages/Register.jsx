import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Landmark, UserPlus } from 'lucide-react';
import { useAuth } from '../contexts/AuthContext';

export default function Register() {
  const { register } = useAuth();
  const navigate = useNavigate();

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [fullName, setFullName] = useState('');
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(false);
  const [success, setSuccess] = useState(false);

  async function handleSubmit(e) {
    e.preventDefault();
    setError(null);
    setLoading(true);

    try {
      await register(email, password, fullName);
      setSuccess(true);
      setTimeout(() => {
        navigate('/login', { replace: true });
      }, 1500);
    } catch (err) {
      const code = err.response?.data?.code;
      const message = err.response?.data?.message || 'Error al crear la cuenta';

      if (code === 'EMAIL_EXISTS') {
        setError('Ya existe una cuenta con ese email');
      } else {
        setError(message);
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4 py-8">
      <div className="w-full max-w-md">
        <div className="flex flex-col items-center mb-8">
          <Landmark className="text-brand-600 mb-2" size={40} />
          <h1 className="text-2xl font-bold text-gray-900">Banking System</h1>
        </div>

        <div className="card p-8">
          <h2 className="text-xl font-semibold mb-6">Crear cuenta</h2>

          {error && <div className="error-box mb-4">{error}</div>}

          {success && (
            <div className="success-box mb-4">
              ✓ Cuenta creada. Redirigiendo al login...
            </div>
          )}

          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="label" htmlFor="full_name">Nombre completo</label>
              <input
                id="full_name"
                type="text"
                className="input"
                value={fullName}
                onChange={(e) => setFullName(e.target.value)}
                placeholder="Juan Pérez"
                required
                autoComplete="name"
                disabled={loading || success}
              />
            </div>

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
                disabled={loading || success}
              />
            </div>

            <div>
              <label className="label" htmlFor="password">
                Contraseña <span className="text-gray-400 font-normal">(mínimo 8 caracteres)</span>
              </label>
              <input
                id="password"
                type="password"
                className="input"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••"
                required
                minLength={8}
                autoComplete="new-password"
                disabled={loading || success}
              />
            </div>

            <button
              type="submit"
              className="btn-primary w-full"
              disabled={loading || success}
            >
              <UserPlus size={16} />
              {loading ? 'Creando...' : success ? 'Creada ✓' : 'Crear cuenta'}
            </button>
          </form>

          <div className="mt-6 pt-6 border-t border-gray-100 text-center text-sm">
            <span className="text-gray-500">¿Ya tienes cuenta? </span>
            <Link to="/login" className="text-brand-600 font-medium hover:underline">
              Iniciar sesión
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
}