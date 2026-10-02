import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import {
  listCustomers,
  listTables,
  type BillDetail as BillDetailData,
  type Customer,
  type Table,
} from '../api/tabs';
import { AppBottomNav } from '../components/AppBottomNav';
import { BillWorkspace } from '../components/BillWorkspace';
import { billLabel } from '../lib/billDisplay';
import { useSession } from '../lib/session';

// BillDetail is a bill's own full-page route (not a modal), reached from
// the Bills list -- it exists for browsing any bill, including settled/
// void history Sell.tsx's own open-bills pill strip never shows. All the
// actual content is BillWorkspace; this page just supplies the chrome
// (header, back-to-list, bottom nav) -- docs/PHASE_UNIFIED_SELL_BILLS.md.
export function BillDetail() {
  const { billId } = useParams<{ billId: string }>();
  const navigate = useNavigate();
  const { selectedBusinessId } = useSession();

  // Only fetched here for the header's label -- BillWorkspace owns the
  // real bill fetch and hands a fresh copy back via onDetailLoaded rather
  // than this page re-fetching the same bill itself.
  const [tables, setTables] = useState<Table[]>([]);
  const [customers, setCustomers] = useState<Customer[]>([]);
  const [title, setTitle] = useState('Bill');

  useEffect(() => {
    if (!selectedBusinessId) return;
    listTables(selectedBusinessId)
      .then(setTables)
      .catch(() => undefined);
    listCustomers(selectedBusinessId)
      .then(setCustomers)
      .catch(() => undefined);
  }, [selectedBusinessId]);

  function handleDetailLoaded(detail: BillDetailData) {
    setTitle(billLabel(detail, tables, customers));
  }

  if (!billId) return null;

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink pb-28">
      <div className="max-w-md mx-auto px-5 pt-6">
        <div className="flex items-center gap-4 mb-4">
          <button
            onClick={() => void navigate('/dashboard/tabs')}
            aria-label="Back to bills"
            className="text-jb-ink/40 hover:text-jb-ink transition-colors text-lg leading-none"
          >
            ←
          </button>
          <div>
            <div className="text-[11px] text-jb-ink/45">Bills</div>
            <h1 className="text-lg font-medium">{title}</h1>
          </div>
        </div>
      </div>

      <BillWorkspace billId={billId} onDetailLoaded={handleDetailLoaded} />

      <AppBottomNav />
    </div>
  );
}
