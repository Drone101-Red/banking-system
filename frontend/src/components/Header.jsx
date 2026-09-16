import { Link, NavLink, useNavigate } from 'react-router-dom';
import { Landmark, LogOut } from 'lucide-react';
import { useAuth } from '../contexts/AuthContext';

export default function Header() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  function handleLogout() {
    logout();
    navigate('/login', { replace: true });
  }

  const navLinkClass = ({ isActive }) =>
    `px-3 py-2 rounded-lg text-sm font-medium transition ${
      isActive
        ? 'bg-brand-50 text-brand-700'
        : 'text-gray-600 hover:bg-gray-100'
    }`;

  return (
    <header className="bg-white border-b border-gray-200 sticky top-0 z-10">
      <div className="max-w-6xl mx-auto px-4 h-16 flex items-center justify-between">
        {/* Logo + nav */}
        <div className="flex items-center gap-6">
          <Link to="/dashboard" className="flex items-center gap-2">
            <Landmark className="text-brand-600" size={24} />
            <span className="font-semibold text-gray-900 hidden sm:inline">
              Banking System
            </span>
          </Link>

          <nav className="hidden md:flex items-center gap-1">
            <NavLink to="/dashboard" className={navLinkClass}>
              Dashboard
            </NavLink>
            <NavLink to="/transactions" className={navLinkClass}>
              Transacciones
            </NavLink>
            <NavLink to="/history" className={navLinkClass}>
              Historial
            </NavLink>
          </nav>
        </div>

        {/* Usuario + logout */}
        <div className="flex items-center gap-3">
          {user && (
            <div className="hidden sm:block text-right">
              <p className="text-sm font-medium text-gray-900 leading-tight">
                {user.full_name}
              </p>
              <p className="text-xs text-gray-500 leading-tight">
                @{user.alias}
              </p>
            </div>
          )}

          <button
            onClick={handleLogout}
            className="btn-secondary text-sm"
            title="Cerrar sesión"
          >
            <LogOut size={16} />
            <span className="hidden sm:inline">Salir</span>
          </button>
        </div>
      </div>

      {/* Nav mobile */}
      <nav className="md:hidden border-t border-gray-100 px-4 py-2 flex items-center gap-1 overflow-x-auto">
        <NavLink to="/dashboard" className={navLinkClass}>
          Dashboard
        </NavLink>
        <NavLink to="/transactions" className={navLinkClass}>
          Transacciones
        </NavLink>
        <NavLink to="/history" className={navLinkClass}>
          Historial
        </NavLink>
      </nav>
    </header>
  );
}