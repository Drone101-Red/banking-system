import { useEffect, useState } from 'react';
import { ChevronLeft, ChevronRight, ArrowDownLeft, ArrowUpRight, Inbox } from 'lucide-react';
import AppLayout from '../components/AppLayout';
import { useAuth } from '../contexts/AuthContext';
import api from '../api/client';

const ITEMS_PER_PAGE = 10;

export default function History() {
  const { tbAccountID } = useAuth();
  const [transactions, setTransactions] = useState([]);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    api
      .get('/api/transactions/history', {
        params: { page, limit: ITEMS_PER_PAGE },
      })
      .then(({ data }) => {
        if (cancelled) return;
        setTransactions(data.transactions || []);
        setTotalPages(data.total_pages || 1);
        setTotal(data.total || 0);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(err.response?.data?.message || 'Error al cargar el historial');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [page]);

  function computeDirection(tx) {
    if (!tbAccountID) return 'unknown';
    if (tx.credit_account_id === tbAccountID) return 'in';
    if (tx.debit_account_id === tbAccountID) return 'out';
    return 'unknown';
  }

  function codeLabel(code) {
    if (code === 1) return 'Depósito';
    if (code === 2) return 'Retiro';
    if (code === 3) return 'Transferencia';
    return '—';
  }

  function formatCents(cents) {
    return `$${(cents / 100).toFixed(2)}`;
  }

  function formatDate(iso) {
    return new Date(iso).toLocaleString('es-ES', {
      day: '2-digit',
      month: 'short',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  }

  return (
    <AppLayout>
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">Historial</h1>
        <p className="text-sm text-gray-500 mt-1">
          {total > 0
            ? `${total} transacción${total !== 1 ? 'es' : ''} en total`
            : 'Tus movimientos aparecerán acá'}
        </p>
      </div>

      {/* Estado: cargando */}
      {loading && (
        <div className="card p-12 text-center text-gray-400">
          Cargando...
        </div>
      )}

      {/* Estado: error */}
      {!loading && error && (
        <div className="card p-6">
          <div className="error-box">{error}</div>
        </div>
      )}

      {/* Estado: vacío */}
      {!loading && !error && transactions.length === 0 && (
        <div className="card p-12 text-center">
          <Inbox className="mx-auto text-gray-300 mb-3" size={48} />
          <p className="text-gray-500 font-medium">No tienes transacciones todavía</p>
          <p className="text-sm text-gray-400 mt-1">
            Hacé tu primer depósito desde la página de Transacciones.
          </p>
        </div>
      )}

      {/* Estado: con transacciones */}
      {!loading && !error && transactions.length > 0 && (
        <>
          <div className="card overflow-hidden">
            <table className="w-full">
              <thead>
                <tr className="bg-gray-50 border-b border-gray-200">
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Tipo
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider hidden sm:table-cell">
                    Fecha
                  </th>
                  <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Monto
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {transactions.map((tx) => {
                  const direction = computeDirection(tx);
                  const isIn = direction === 'in';

                  return (
                    <tr key={tx.id} className="hover:bg-gray-50 transition">
                      {/* Tipo + dirección */}
                      <td className="px-6 py-4">
                        <div className="flex items-center gap-3">
                          <div
                            className={`w-9 h-9 rounded-full flex items-center justify-center flex-shrink-0 ${
                              isIn
                                ? 'bg-green-100 text-green-700'
                                : 'bg-red-100 text-red-700'
                            }`}
                          >
                            {isIn ? <ArrowDownLeft size={16} /> : <ArrowUpRight size={16} />}
                          </div>
                          <div>
                            <p className="text-sm font-medium text-gray-900">
                              {codeLabel(tx.code)}
                            </p>
                            <p className="text-xs text-gray-500">
                              {isIn ? 'Recibido' : 'Enviado'}
                            </p>
                          </div>
                        </div>
                      </td>

                      {/* Fecha */}
                      <td className="px-6 py-4 text-sm text-gray-500 hidden sm:table-cell">
                        {formatDate(tx.created_at)}
                      </td>

                      {/* Monto */}
                      <td
                        className={`px-6 py-4 text-right font-mono text-sm font-medium ${
                          isIn ? 'text-green-700' : 'text-red-700'
                        }`}
                      >
                        {isIn ? '+' : '−'}
                        {formatCents(tx.amount_cents)}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          {/* Paginación */}
          {totalPages > 1 && (
            <div className="mt-6 flex items-center justify-between">
              <button
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={page === 1}
                className="btn-secondary text-sm"
              >
                <ChevronLeft size={16} />
                Anterior
              </button>

              <span className="text-sm text-gray-600">
                Página <span className="font-medium">{page}</span> de{' '}
                <span className="font-medium">{totalPages}</span>
              </span>

              <button
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                disabled={page === totalPages}
                className="btn-secondary text-sm"
              >
                Siguiente
                <ChevronRight size={16} />
              </button>
            </div>
          )}
        </>
      )}
    </AppLayout>
  );
}