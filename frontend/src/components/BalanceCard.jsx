import { useState } from 'react';
import { Copy, Check, Wallet } from 'lucide-react';

export default function BalanceCard({ balanceCents, alias, tbAccountID }) {
  const [copied, setCopied] = useState(false);

  function formatCents(cents) {
    if (cents === null || cents === undefined) return '—';
    return `$${(cents / 100).toFixed(2)}`;
  }

  async function handleCopy() {
    if (!tbAccountID) return;
    try {
      await navigator.clipboard.writeText(tbAccountID);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // clipboard puede fallar en HTTP (no HTTPS) en algunos navegadores.
      // Fallback: seleccionar texto. Aceptable para la prueba.
    }
  }

  return (
    <div className="card p-6">
      <div className="flex items-center gap-2 text-gray-500 text-sm mb-1">
        <Wallet size={16} />
        <span>Saldo disponible</span>
      </div>

      <div className="text-4xl font-bold text-gray-900 mb-4">
        {formatCents(balanceCents)}
      </div>

      <div className="flex items-center justify-between pt-4 border-t border-gray-100">
        <div>
          <p className="text-xs text-gray-500">Tu alias</p>
          <p className="text-sm font-medium text-gray-900">
            {alias ? `@${alias}` : '—'}
          </p>
        </div>

        {tbAccountID && (
          <button
            onClick={handleCopy}
            className="btn-secondary text-xs py-1.5 px-3"
            title="Copiar ID de cuenta"
          >
            {copied ? (
              <>
                <Check size={14} className="text-green-600" />
                Copiado
              </>
            ) : (
              <>
                <Copy size={14} />
                Copiar ID
              </>
            )}
          </button>
        )}
      </div>
    </div>
  );
}