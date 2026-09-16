import AppLayout from '../components/AppLayout';
import BalanceCard from '../components/BalanceCard';

export default function Dashboard() {
  return (
    <AppLayout>
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-1">
          <BalanceCard
            balanceCents={106500}
            alias="demo"
            tbAccountID="c442931df9fcfd81bc6f7e3e3b2ecbb1"
          />
        </div>
        <div className="lg:col-span-2 card p-6">
          <p className="text-gray-500">Aquí va el chat (Batch 4)</p>
        </div>
      </div>
    </AppLayout>
  );
}