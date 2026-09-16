import { useState } from 'react';
import { ArrowDownToLine, ArrowUpFromLine, Send } from 'lucide-react';
import AppLayout from '../components/AppLayout';
import TransactionForm from '../components/TransactionForm';
import BalanceCard from '../components/BalanceCard';
import { useAuth } from '../contexts/AuthContext';
import { useBalance } from '../hooks/useBalance';

const TABS = [
  { key: 'deposit', label: 'Depositar', icon: ArrowDownToLine },
  { key: 'withdraw', label: 'Retirar', icon: ArrowUpFromLine },
  { key: 'transfer', label: 'Transferir', icon: Send },
];

export default function Transactions() {
  const { user, tbAccountID } = useAuth();
  const { balance, refresh: refreshBalance } = useBalance();
  const [activeTab, setActiveTab] = useState('deposit');

  return (
    <AppLayout>
      <h1 className="text-2xl font-bold mb-6">Transacciones</h1>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Sidebar con saldo */}
        <div className="lg:col-span-1">
          <BalanceCard
            balanceCents={balance}
            alias={user?.alias}
            tbAccountID={tbAccountID}
          />
        </div>

        {/* Formularios */}
        <div className="lg:col-span-2">
          <div className="card">
            {/* Tabs */}
            <div className="flex border-b border-gray-200">
              {TABS.map(({ key, label, icon: Icon }) => (
                <button
                  key={key}
                  onClick={() => setActiveTab(key)}
                  className={`flex-1 flex items-center justify-center gap-2 px-4 py-3 text-sm font-medium transition border-b-2 ${
                    activeTab === key
                      ? 'border-brand-600 text-brand-700'
                      : 'border-transparent text-gray-500 hover:text-gray-700'
                  }`}
                >
                  <Icon size={16} />
                  {label}
                </button>
              ))}
            </div>

            {/* Form */}
            <div className="p-6">
              <TransactionForm
                key={activeTab}
                type={activeTab}
                onSuccess={refreshBalance}
              />
            </div>
          </div>
        </div>
      </div>
    </AppLayout>
  );
}