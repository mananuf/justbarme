import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import {
  listBills,
  listCustomers,
  listTables,
  type Bill,
  type BillStatusFilter,
  type Customer,
  type Table,
} from '../api/tabs';
import { AppBottomNav } from '../components/AppBottomNav';
import { useSession } from '../lib/session';
import { billLabel, formatNaira, statusLabel } from '../lib/billDisplay';
import { describeActionError } from '../lib/errors';

const FILTERS: { value: BillStatusFilter; label: string }[] = [
  { value: 'open', label: 'Open' },
  { value: 'closed_unpaid', label: 'Closed — unpaid' },
  { value: 'settled', label: 'Settled' },
  { value: 'void', label: 'Void' },
  { value: 'all', label: 'All' },
];

// Tabs is the Bills screen: every bill, filterable by status, each one
// opening into its own full page rather than a modal
// (docs/PHASE_UNIFIED_SELL_BILLS.md). Starting a new sale lives on its own
// screen (Sell.tsx) reachable from here or from the bottom nav directly --
// this page is purely for looking up and resuming bills, current or past.
export function Tabs() {
  const navigate = useNavigate();
  const { selectedBusinessId } = useSession();

  const [filter, setFilter] = useState<BillStatusFilter>('open');
  const [bills, setBills] = useState<Bill[] | null>(null);
  const [tables, setTables] = useState<Table[]>([]);
  const [customers, setCustomers] = useState<Customer[]>([]);
  const [loadError, setLoadError] = useState<string | null>(null);

  useEffect(() => {
    if (!selectedBusinessId) return;
    listTables(selectedBusinessId)
      .then(setTables)
      .catch(() => undefined);
    listCustomers(selectedBusinessId)
      .then(setCustomers)
      .catch(() => undefined);
  }, [selectedBusinessId]);

  useEffect(() => {
    if (!selectedBusinessId) return;
    // Reset inside the async callback, not the effect body itself -- these
    // still run before the fetch below, just not in the same synchronous
    // pass React's cascading-render lint rule guards against (see
    // Stock.tsx's loadCatalogue for the same reasoning).
    void (async () => {
      setBills(null);
      setLoadError(null);
      try {
        setBills(await listBills(selectedBusinessId, filter));
      } catch (err) {
        setLoadError(describeActionError(err, 'Could not load bills.'));
      }
    })();
  }, [selectedBusinessId, filter]);

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink pb-28">
      <div className="max-w-md mx-auto px-5 pt-6">
        <div className="flex items-center gap-4 mb-4">
          <Link
            to="/dashboard"
            aria-label="Back"
            className="text-jb-ink/40 hover:text-jb-ink transition-colors text-lg leading-none"
          >
            ←
          </Link>
          <div>
            <div className="text-[11px] text-jb-ink/45">Tabs &amp; credit</div>
            <h1 className="text-lg font-medium">Bills</h1>
          </div>
        </div>

        <button
          onClick={() => void navigate('/dashboard/sell')}
          className="w-full rounded-xl bg-jb-ink text-jb-cream text-[14.5px] font-medium py-3.5 mb-4 active:scale-[0.98] transition-transform"
        >
          + New sale
        </button>

        <div className="flex flex-wrap gap-2 mb-4">
          {FILTERS.map((f) => (
            <button
              key={f.value}
              onClick={() => setFilter(f.value)}
              className={`rounded-full px-3.5 py-1.5 text-[12.5px] font-medium whitespace-nowrap transition-colors ${
                filter === f.value
                  ? 'bg-jb-ink text-jb-cream'
                  : 'bg-white border border-jb-ink/15 text-jb-ink/60'
              }`}
            >
              {f.label}
            </button>
          ))}
        </div>

        {loadError && <p className="text-[13px] text-red-700 mb-3">{loadError}</p>}
        {bills === null && !loadError && <p className="text-[13px] text-jb-ink/45">Loading…</p>}

        {bills !== null && bills.length === 0 && !loadError && (
          <p className="text-[13px] text-jb-ink/45">No bills in this filter yet.</p>
        )}

        {bills !== null && bills.length > 0 && (
          <div className="flex flex-col gap-2">
            {bills.map((bill) => (
              <button
                key={bill.id}
                onClick={() => void navigate(`/dashboard/tabs/${bill.id}`)}
                className="text-left rounded-xl border border-jb-ink/10 bg-white p-3.5 hover:bg-jb-ink/[0.02] transition-colors"
              >
                <div className="flex items-center justify-between mb-0.5">
                  <span className="text-[14px] font-medium text-jb-ink">
                    {billLabel(bill, tables, customers)}
                  </span>
                  <span className="text-[14px] font-medium text-jb-ink">
                    {bill.balanceKobo !== 0 ? `₦${formatNaira(bill.balanceKobo)}` : '—'}
                  </span>
                </div>
                <div className="flex items-center justify-between text-[11.5px] text-jb-ink/45">
                  <span>{statusLabel(bill.status)}</span>
                  <span>{new Date(bill.openedAt).toLocaleString()}</span>
                </div>
              </button>
            ))}
          </div>
        )}
      </div>

      <AppBottomNav />
    </div>
  );
}
