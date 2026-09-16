import { useState } from 'react';
import { Check, Search, X } from 'lucide-react';
import api from '../api/client';

/**
 * Formulario de transacciones.
 *
 * Props:
 *   - type: 'deposit' | 'withdraw' | 'transfer'
 *   - onSuccess: callback que se llama después de una operación exitosa
 *                (usado para refrescar el saldo)
 */
export default function TransactionForm({ type, onSuccess }) {
  const [amount, setAmount] = useState('');
  const [toQuery, setToQuery] = useState('');
  const [lookupResult, setLookupResult] = useState(null);
  const [lookupLoading, setLookupLoading] = useState(false);
  const [lookupError, setLookupError] = useState(null);
  const [error, setError] = useState(null);
  const [success, setSuccess] = useState(null);
  const [loading, setLoading] = useState(false);

  const isTransfer = type === 'transfer';

  function resetForm() {
    setAmount('');
    setToQuery('');
    setLookupResult(null);
    setLookupError(null);
  }

  // Buscar destinatario por email o alias
  async function handleLookup(e) {
    e.preventDefault();
    if (!toQuery.trim()) return;

    setLookupLoading(true);
    setLookupError(null);
    setLookupResult(null);

    try {
      const query = toQuery.trim().toLowerCase();
      const isEmail = query.includes('@');
      const { data } = await api.get('/api/users/lookup', {
        params: isEmail ? { email: query } : { alias: query },
      });
      setLookupResult(data);
    } catch (err) {
      const code = err.response?.data?.code;
      if (code === 'USER_NOT_FOUND') {
        setLookupError('No se encontró ningún usuario con ese email o alias');
      } else {
        setLookupError(err.response?.data?.message || 'Error al buscar');
      }
    } finally {
      setLookupLoading(false);
    }
  }

  // Enviar la operación
  async function handleSubmit(e) {
    e.preventDefault();
    setError(null);
    setSuccess(null);

    const amountFloat = parseFloat(amount);
    if (!amountFloat || amountFloat <= 0) {
      setError('Ingresá un monto válido');
      return;
    }

    const amountCents = Math.round(amountFloat * 100);

    if (isTransfer && !lookupResult) {
      setError('Primero buscá al destinatario');
      return;
    }

    setLoading(true);

    try {
      let endpoint = '';
      let body = {};

      if (type === 'deposit') {
        endpoint = '/api/transactions/deposit';
        body = { amount_cents: amountCents };
      } else if (type === 'withdraw') {
        endpoint = '/api/transactions/withdraw';
        body = { amount_cents: amountCents };
      } else if (type === 'transfer') {
        endpoint = '/api/transactions/transfer';
        body = {
          to_account_id: lookupResult.tb_account_id,
          amount_cents: amountCents,
        };
      }

      // Idempotency-Key en el header
      const idempotencyKey = crypto.randomUUID();

      const { data } = await api.post(endpoint, body, {
        headers: { 'Idempotency-Key': idempotencyKey },
      });

      const label = {
        deposit: 'Depósito',
        withdraw: 'Retiro',
        transfer: 'Transferencia',
      }[type];

      setSuccess(`${label} de $${amountFloat.toFixed(2)} realizado correctamente`);
      resetForm();

      if (onSuccess) {
        await onSuccess();
      }
    } catch (err) {
      const code = err.response?.data?.code;
      const message = err.response?.data?.message || 'Error al procesar la operación';

      if (code === 'INSUFFICIENT_FUNDS') {
        setError('Saldo insuficiente para esta operación');
      } else if (code === 'DEST_NOT_FOUND') {
        setError('La cuenta destino no existe');
      } else if (code === 'SAME_ACCOUNT') {
        setError('No puedes transferir a tu propia cuenta');
      } else {
        setError(message);
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {/* Destinatario (solo transfer) */}
      {isTransfer && (
        <div>
          <label className="label">Destinatario</label>

          {lookupResult ? (
            <div className="flex items-center justify-between p-3 bg-green-50 border border-green-200 rounded-lg">
              <div className="flex items-center gap-2">
                <Check size={16} className="text-green-600" />
                <div>
                  <p className="text-sm font-medium text-green-900">
                    {lookupResult.full_name}
                  </p>
                  <p className="text-xs text-green-700">
                    @{lookupResult.alias} · {lookupResult.email}
                  </p>
                </div>
              </div>
              <button
                type="button"
                onClick={() => {
                  setLookupResult(null);
                  setToQuery('');
                }}
                className="text-green-700 hover:text-green-900"
                title="Cambiar destinatario"
              >
                <X size={16} />
              </button>
            </div>
          ) : (
            <div className="flex gap-2">
              <input
                type="text"
                className="input flex-1"
                value={toQuery}
                onChange={(e) => setToQuery(e.target.value)}
                placeholder="demo2@banco.com o @demo2"
                disabled={lookupLoading}
              />
              <button
                type="button"
                onClick={handleLookup}
                disabled={lookupLoading || !toQuery.trim()}
                className="btn-secondary px-4"
              >
                <Search size={16} />
                {lookupLoading ? 'Buscando...' : 'Buscar'}
              </button>
            </div>
          )}

          {lookupError && (
            <p className="text-xs text-red-600 mt-1">{lookupError}</p>
          )}
        </div>
      )}

      {/* Monto */}
      <div>
        <label className="label">Monto (USD)</label>
        <input
          type="number"
          step="0.01"
          min="0.01"
          className="input"
          value={amount}
          onChange={(e) => setAmount(e.target.value)}
          placeholder="10.50"
          disabled={loading}
          required
        />
        {amount && parseFloat(amount) > 0 && (
          <p className="text-xs text-gray-500 mt-1">
            Se procesarán {Math.round(parseFloat(amount) * 100)} centavos
          </p>
        )}
      </div>

      {/* Error / éxito */}
      {error && <div className="error-box">{error}</div>}
      {success && <div className="success-box">✓ {success}</div>}

      {/* Submit */}
      <button
        type="submit"
        disabled={loading || !amount}
        className={
          type === 'withdraw'
            ? 'btn-danger w-full'
            : 'btn-primary w-full'
        }
      >
        {loading
          ? 'Procesando...'
          : type === 'deposit'
          ? 'Depositar'
          : type === 'withdraw'
          ? 'Retirar'
          : 'Transferir'}
      </button>
    </form>
  );
}