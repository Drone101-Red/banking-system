import { useCallback, useEffect, useState } from 'react';
import api from '../api/client';

/**
 * Hook centralizado para el saldo del usuario.
 *
 * - Carga el saldo al montar.
 * - Expone `refresh()` para forzar recarga después de una operación.
 *
 * Uso:
 *   const { balance, loading, error, refresh } = useBalance();
 *
 * Después de un depósito/retiro/transferencia/chat-confirm:
 *   await refresh();
 */
export function useBalance() {
  const [balance, setBalance] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const { data } = await api.get('/api/account/balance');
      setBalance(data.balance_cents);
    } catch (err) {
      setError(err.response?.data?.message || 'Error al cargar saldo');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return { balance, loading, error, refresh };
}