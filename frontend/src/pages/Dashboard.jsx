import { useEffect, useState } from 'react';
import { ArrowRight, DollarSign, TrendingUp, TrendingDown } from 'lucide-react';
import { Link } from 'react-router-dom';
import AppLayout from '../components/AppLayout';
import BalanceCard from '../components/BalanceCard';
import { useAuth } from '../contexts/AuthContext';
import { useBalance } from '../hooks/useBalance';
import api from '../api/client';
import Chat from '../components/Chat';

export default function Dashboard() {
  const { user, tbAccountID } = useAuth();
  const { balance, loading: balanceLoading, refresh: refreshBalance } = useBalance();
  const [recentTxs, setRecentTxs] = useState([]);
  const [recentLoading, setRecentLoading] = useState(true);
  const [topupLoading, setTopupLoading] = useState(false);
  const [topupError, setTopupError] = useState(null);
  const [topupSuccess, setTopupSuccess] = useState(false);

  // Cargar saldo primero, luego transacciones (evita concurrencia en TB)
  useEffect(() => {
  let cancelled = false;

  async function load() {
    await refreshBalance();          // ← primero el saldo
    if (cancelled) return;

    setRecentLoading(true);
    try {
      const { data } = await api.get('/api/transactions/history', {
        params: { page: 1, limit: 5 },
      });
      if (!cancelled) setRecentTxs(data.transactions || []);
    } catch {
      if (!cancelled) setRecentTxs([]);
    } finally {
      if (!cancelled) setRecentLoading(false);
    }
  }

  load();
  return () => { cancelled = true; };
}, [refreshBalance]);

  async function handleDemoTopup() {
    setTopupLoading(true);
    setTopupError(null);
    setTopupSuccess(false);

    try {
      await api.post('/api/transactions/demo-topup');
      await refreshBalance();
      setTopupSuccess(true);

      // Recargar transacciones recientes
      const { data } = await api.get('/api/transactions/history', {
        params: { page: 1, limit: 5 },
      });
      setRecentTxs(data.transactions || []);

      setTimeout(() => setTopupSuccess(false), 3000);
    } catch (err) {
      const code = err.response?.data?.code;
      if (code === 'DEMO_DISABLED') {
        setTopupError('El crédito de demo no está disponible en este entorno');
      } else {
        setTopupError(err.response?.data?.message || 'Error al cargar el crédito');
      }
    } finally {
      setTopupLoading(false);
    }
  }

  function formatCents(cents) {
    return `$${(cents / 100).toFixed(2)}`;
  }

  function formatDate(iso) {
    return new Date(iso).toLocaleString('es-ES', {
      day: '2-digit',
      month: 'short',
      hour: '2-digit',
      minute: '2-digit',
    });
  }

  function codeLabel(code) {
    if (code === 1) return 'Depósito';
    if (code === 2) return 'Retiro';
    if (code === 3) return 'Transferencia';
    return '—';
  }

  function codeBadgeClass(code) {
    if (code === 1) return 'bg-green-100 text-green-800';
    if (code === 2) return 'bg-red-100 text-red-800';
    if (code === 3) return 'bg-blue-100 text-blue-800';
    return 'bg-gray-100 text-gray-800';
  }

  return (
    <AppLayout>
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">
          Hola, {user?.full_name?.split(' ')[0] || 'usuario'} 👋
        </h1>
        <p className="text-sm text-gray-500 mt-1">
          Aquí tienes el resumen de tu cuenta
        </p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Columna izquierda */}
        <div className="lg:col-span-1 space-y-4">
          {/* Balance */}
          {balanceLoading && balance === null ? (
            <div className="card p-6">
              <div className="text-gray-400 text-sm mb-1">Saldo disponible</div>
              <div className="text-4xl font-bold text-gray-300 mb-4">$ ——</div>
            </div>
          ) : (
            <BalanceCard
              balanceCents={balance}
              alias={user?.alias}
              tbAccountID={tbAccountID}
            />
          )}

          {/* Botón demo topup */}
          <div className="card p-4">
            <p className="text-xs text-gray-500 mb-2">
              ¿Necesitas fondos para probar?
            </p>
            <button
              onClick={handleDemoTopup}
              disabled={topupLoading}
              className="btn-primary w-full text-sm"
            >
              <DollarSign size={16} />
              {topupLoading ? 'Cargando...' : 'Cargar $1,000 de demo'}
            </button>

            {topupError && (
              <div className="error-box mt-3 text-xs">{topupError}</div>
            )}
            {topupSuccess && (
              <div className="success-box mt-3 text-xs">
                ✓ $1,000 acreditados
              </div>
            )}
          </div>

          {/* Acciones rápidas */}
          <div className="card p-4">
            <p className="text-xs text-gray-500 mb-2">Operaciones</p>
            <Link
              to="/transactions"
              className="btn-secondary w-full text-sm"
            >
              Ver transacciones
              <ArrowRight size={14} />
            </Link>
          </div>
        </div>

        {/* Columna derecha */}
        <div className="lg:col-span-2 space-y-6">
          {/* Chat (placeholder hasta Batch 4b) */}
          <Chat onBalanceChange={refreshBalance} />

          {/* Últimas transacciones */}
          <div className="card">
            <div className="flex items-center justify-between p-6 pb-4">
              <h2 className="font-semibold">Últimas transacciones</h2>
              <Link
                to="/history"
                className="text-sm text-brand-600 font-medium hover:underline"
              >
                Ver todas →
              </Link>
            </div>

            {recentLoading ? (
              <div className="px-6 pb-6 text-center text-gray-400 text-sm">
                Cargando...
              </div>
            ) : recentTxs.length === 0 ? (
              <div className="px-6 pb-6 text-center text-gray-400 text-sm">
                No tienes transacciones todavía.
                <br />
                <span className="text-xs">
                  Cargá fondos con el botón de demo o hacé tu primer depósito.
                </span>
              </div>
            ) : (
              <div className="divide-y divide-gray-100">
                {recentTxs.map((tx) => (
                  <div
                    key={tx.id}
                    className="px-6 py-3 flex items-center justify-between hover:bg-gray-50"
                  >
                    <div className="flex items-center gap-3">
                      <span
                        className={`inline-block px-2 py-1 rounded text-xs font-medium ${codeBadgeClass(tx.code)}`}
                      >
                        {codeLabel(tx.code)}
                      </span>
                      <span className="text-xs text-gray-500">
                        {formatDate(tx.created_at)}
                      </span>
                    </div>
                    <span className="font-mono text-sm font-medium">
                      {formatCents(tx.amount_cents)}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </AppLayout>
  );
}