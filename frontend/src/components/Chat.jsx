import { useState, useRef, useEffect } from 'react';
import { Send, Check, X, Bot, User } from 'lucide-react';
import api from '../api/client';

const SUGGESTIONS = [
  '¿Cuánto tengo?',
  'Muéstrame mis últimas 5 transacciones',
  'Deposita $50 a mi cuenta',
];

export default function Chat({ onBalanceChange }) {
  const [messages, setMessages] = useState([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [pendingConfirm, setPendingConfirm] = useState(null);
  const [error, setError] = useState(null);
  const messagesEndRef = useRef(null);

  // Scroll al final cuando hay mensajes nuevos
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, pendingConfirm]);

  // Enviar mensaje al chat
  async function sendMessage(text) {
    if (!text.trim() || loading || pendingConfirm) return;

    const userMsg = { role: 'user', content: text };
    setMessages((prev) => [...prev, userMsg]);
    setInput('');
    setLoading(true);
    setError(null);

    // Historial completo (solo los últimos 20 mensajes)
    const history = [...messages, userMsg].slice(-20).map((m) => ({
      role: m.role,
      content: m.content,
    }));

    try {
      const { data } = await api.post('/api/chat', {
        message: text,
        history: history.slice(0, -1), // sin el último (es el message)
      });

      setMessages((prev) => [
        ...prev,
        { role: 'assistant', content: data.reply },
      ]);

      // ¿Hay confirmación pendiente?
      if (data.pending_confirmation) {
        setPendingConfirm(data.pending_confirmation);
      }
    } catch (err) {
      const msg = err.response?.data?.message || 'Error al procesar el mensaje';
      setError(msg);
      setMessages((prev) => [
        ...prev,
        { role: 'assistant', content: `❌ ${msg}` },
      ]);
    } finally {
      setLoading(false);
    }
  }

  // Confirmar la operación pendiente
  async function confirmOperation() {
    if (!pendingConfirm || loading) return;

    setLoading(true);
    setError(null);

    try {
      const { data } = await api.post('/api/chat/confirm', {
        confirmation_token: pendingConfirm.confirmation_token,
      });

      setMessages((prev) => [
        ...prev,
        {
          role: 'assistant',
          content: `✓ ${data.message}`,
        },
      ]);
      setPendingConfirm(null);

      // Notificar al padre para que refresque el saldo
      if (onBalanceChange) {
        await onBalanceChange();
      }
    } catch (err) {
      const msg = err.response?.data?.message || 'Error al confirmar';
      setError(msg);
      setMessages((prev) => [
        ...prev,
        { role: 'assistant', content: `❌ ${msg}` },
      ]);
      setPendingConfirm(null);
    } finally {
      setLoading(false);
    }
  }

  // Cancelar la operación pendiente
  function cancelOperation() {
    setMessages((prev) => [
      ...prev,
      { role: 'assistant', content: 'Operación cancelada.' },
    ]);
    setPendingConfirm(null);
    setError(null);
  }

  function handleSubmit(e) {
    e.preventDefault();
    sendMessage(input);
  }

  const inputDisabled = loading || !!pendingConfirm;

  return (
    <div className="card flex flex-col h-[600px]">
      {/* Header */}
      <div className="border-b border-gray-200 px-6 py-4">
        <div className="flex items-center gap-2">
          <Bot className="text-brand-600" size={20} />
          <h2 className="font-semibold">Asistente bancario</h2>
        </div>
        <p className="text-xs text-gray-500 mt-1">
          Pídele consultar saldo, ver historial o hacer operaciones
        </p>
      </div>

      {/* Mensajes */}
      <div className="flex-1 overflow-y-auto p-4 space-y-3">
        {messages.length === 0 && !loading && (
          <div className="text-center py-8">
            <Bot className="mx-auto text-gray-300 mb-3" size={40} />
            <p className="text-gray-500 text-sm mb-4">
              Escribe un mensaje o usa una sugerencia:
            </p>
            <div className="flex flex-wrap gap-2 justify-center">
              {SUGGESTIONS.map((s) => (
                <button
                  key={s}
                  onClick={() => sendMessage(s)}
                  disabled={loading}
                  className="text-xs px-3 py-1.5 bg-gray-100 hover:bg-gray-200 rounded-full transition disabled:opacity-50"
                >
                  {s}
                </button>
              ))}
            </div>
          </div>
        )}

        {messages.map((m, i) => (
          <div
            key={i}
            className={`flex gap-2 ${m.role === 'user' ? 'justify-end' : 'justify-start'}`}
          >
            {m.role === 'assistant' && (
              <div className="w-7 h-7 rounded-full bg-brand-100 flex items-center justify-center flex-shrink-0">
                <Bot size={14} className="text-brand-600" />
              </div>
            )}
            <div
              className={`max-w-[75%] px-4 py-2 rounded-2xl whitespace-pre-wrap text-sm ${
                m.role === 'user'
                  ? 'bg-brand-600 text-white rounded-br-sm'
                  : 'bg-gray-100 text-gray-900 rounded-bl-sm'
              }`}
            >
              {m.content}
            </div>
            {m.role === 'user' && (
              <div className="w-7 h-7 rounded-full bg-gray-200 flex items-center justify-center flex-shrink-0">
                <User size={14} className="text-gray-600" />
              </div>
            )}
          </div>
        ))}

        {loading && !pendingConfirm && (
          <div className="flex gap-2">
            <div className="w-7 h-7 rounded-full bg-brand-100 flex items-center justify-center">
              <Bot size={14} className="text-brand-600" />
            </div>
            <div className="bg-gray-100 px-4 py-2 rounded-2xl rounded-bl-sm">
              <div className="flex gap-1">
                <span className="w-1.5 h-1.5 bg-gray-400 rounded-full animate-bounce" style={{ animationDelay: '0ms' }}></span>
                <span className="w-1.5 h-1.5 bg-gray-400 rounded-full animate-bounce" style={{ animationDelay: '150ms' }}></span>
                <span className="w-1.5 h-1.5 bg-gray-400 rounded-full animate-bounce" style={{ animationDelay: '300ms' }}></span>
              </div>
            </div>
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>

      {/* Confirmación pendiente */}
      {pendingConfirm && (
        <div className="border-t-2 border-yellow-200 bg-yellow-50 px-4 py-3">
          <div className="flex items-start gap-2 mb-2">
            <span className="text-yellow-600 text-lg leading-none">⚠️</span>
            <div className="flex-1">
              <p className="text-sm font-semibold text-yellow-900">
                Operación pendiente
              </p>
              <p className="text-xs text-yellow-800 mt-0.5">
                {pendingConfirm.type === 'deposit' && 'Depósito'}
                {pendingConfirm.type === 'withdraw' && 'Retiro'}
                {pendingConfirm.type === 'transfer' && 'Transferencia'}
                {' de '}
                <span className="font-semibold">
                  ${(pendingConfirm.amount_cents / 100).toFixed(2)}
                </span>
                {pendingConfirm.type === 'transfer' && pendingConfirm.to_account_id && (
                  <> a la cuenta <span className="font-mono text-xs">{pendingConfirm.to_account_id.slice(0, 8)}…</span></>
                )}
              </p>
            </div>
          </div>
          <div className="flex gap-2">
            <button
              onClick={confirmOperation}
              disabled={loading}
              className="btn-success flex-1 text-sm py-2"
            >
              <Check size={16} />
              {loading ? 'Procesando...' : 'Confirmar'}
            </button>
            <button
              onClick={cancelOperation}
              disabled={loading}
              className="btn-secondary text-sm py-2"
            >
              <X size={16} />
              Cancelar
            </button>
          </div>
        </div>
      )}

      {/* Error */}
      {error && !pendingConfirm && (
        <div className="border-t border-red-200 bg-red-50 px-4 py-2 text-xs text-red-700">
          {error}
        </div>
      )}

      {/* Input */}
      <form onSubmit={handleSubmit} className="border-t border-gray-200 p-3 flex gap-2">
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder={
            pendingConfirm
              ? 'Confirma o cancela la operación pendiente'
              : 'Escribe un mensaje...'
          }
          className="input flex-1"
          disabled={inputDisabled}
        />
        <button
          type="submit"
          disabled={inputDisabled || !input.trim()}
          className="btn-primary px-4"
          title="Enviar"
        >
          <Send size={16} />
        </button>
      </form>
    </div>
  );
}