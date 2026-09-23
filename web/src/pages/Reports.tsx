import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';

import {
  reportExpenses,
  reportGrossMargin,
  reportProducts,
  reportSales,
  reportStaffSales,
  reportStock,
  reportStockPurchases,
  type ExpensesByCategory,
  type GrossMargin,
  type ProductQuantity,
  type SalesDay,
  type StaffSales,
  type StockDiscrepancy,
  type StockPurchase,
} from '../api/reports';
import { listBills, type Bill } from '../api/tabs';
import { TimeGranularityView } from '../components/TimeGranularityView';
import { useConnectivity } from '../hooks/useConnectivity';
import { describeActionError } from '../lib/errors';
import type { DayValue } from '../lib/timeBuckets';
import { useSession } from '../lib/session';

function formatNaira(kobo: number): string {
  return (kobo / 100).toLocaleString();
}

type ReportKey = 'sales' | 'products' | 'margin' | 'staff' | 'expenses' | 'stock' | 'outstanding';

const REPORT_TABS: { key: ReportKey; label: string }[] = [
  { key: 'sales', label: 'Sales' },
  { key: 'products', label: 'Products' },
  { key: 'margin', label: 'Margin' },
  { key: 'staff', label: 'Staff' },
  { key: 'expenses', label: 'Expenses' },
  { key: 'stock', label: 'Stock' },
  { key: 'outstanding', label: 'Outstanding' },
];

// Owner-facing reports (docs/PHASE_EXPENSES_DASHBOARD_ACTIVITY_REPORTS.md
// §5): Sales gets a daily-revenue heatmap (the one other genuinely
// daily-continuous dataset besides Activity's); everything else is a
// ranked table/bar list, deliberately not a heatmap -- categorical/ranked
// data reads better sorted than gridded. Outstanding reuses the existing
// GET /bills?status=outstanding data, no new report call.
export function Reports() {
  const { selectedBusinessId } = useSession();
  const isOnline = useConnectivity();
  const [tab, setTab] = useState<ReportKey>('sales');
  const [error, setError] = useState<string | null>(null);

  const range = useMemo(() => {
    const end = new Date();
    const start = new Date();
    start.setUTCDate(start.getUTCDate() - 29); // last 30 days
    return { startAt: start.toISOString(), endAt: end.toISOString() };
  }, []);

  // Sales alone fetches a full year -- its heatmap needs enough history
  // for a real Week/Month/Year drill-down (docs/PHASE_EXPENSES_DASHBOARD_
  // ACTIVITY_REPORTS.md's day/week/month/year explorer); every other tab
  // stays a simple 30-day range.
  const salesRange = useMemo(() => {
    const end = new Date();
    const start = new Date();
    start.setUTCDate(start.getUTCDate() - 364);
    return { startAt: start.toISOString(), endAt: end.toISOString() };
  }, []);

  const [salesDays, setSalesDays] = useState<SalesDay[] | null>(null);
  const [products, setProducts] = useState<ProductQuantity[] | null>(null);
  const [margins, setMargins] = useState<GrossMargin[] | null>(null);
  const [staffSales, setStaffSales] = useState<StaffSales[] | null>(null);
  const [expensesByCategory, setExpensesByCategory] = useState<ExpensesByCategory[] | null>(null);
  const [discrepancies, setDiscrepancies] = useState<StockDiscrepancy[] | null>(null);
  const [stockPurchases, setStockPurchases] = useState<StockPurchase[] | null>(null);
  const [outstandingBills, setOutstandingBills] = useState<Bill[] | null>(null);

  // Stock tab gets its own adjustable date range (plain YYYY-MM-DD, native
  // <input type="date">) rather than the fixed "last 30 days" every other
  // non-Sales tab uses -- unlike Sales, this isn't a daily-continuous
  // heatmap dataset, so a simple start/end picker is the better fit than
  // pulling in TimeGranularityView. Defaults match the same 30-day window
  // so nothing changes until the owner actually adjusts it.
  const todayStr = useMemo(() => new Date().toISOString().slice(0, 10), []);
  const defaultStockStartStr = useMemo(() => {
    const d = new Date();
    d.setUTCDate(d.getUTCDate() - 29);
    return d.toISOString().slice(0, 10);
  }, []);
  const [stockRangeStart, setStockRangeStart] = useState(defaultStockStartStr);
  const [stockRangeEnd, setStockRangeEnd] = useState(todayStr);
  // A plain product/variant name filter -- the catalogue is already
  // searchable everywhere else in the app (Stock.tsx, ProductGrid), so
  // this matches that existing pattern rather than inventing a new one.
  const [stockSearch, setStockSearch] = useState('');

  const stockRange = useMemo(() => {
    const start = new Date(`${stockRangeStart}T00:00:00.000Z`);
    // end is exclusive in the query, so add one day to make the picked
    // end date's own receipts included.
    const end = new Date(`${stockRangeEnd}T00:00:00.000Z`);
    end.setUTCDate(end.getUTCDate() + 1);
    return { startAt: start.toISOString(), endAt: end.toISOString() };
  }, [stockRangeStart, stockRangeEnd]);

  useEffect(() => {
    if (!selectedBusinessId) return;
    const { startAt, endAt } = range;
    switch (tab) {
      case 'sales':
        reportSales(selectedBusinessId, salesRange.startAt, salesRange.endAt)
          .then((d) => {
            setSalesDays(d);
            setError(null);
          })
          .catch((err: unknown) =>
            setError(describeActionError(err, 'Could not load the sales report.')),
          );
        break;
      case 'products':
        reportProducts(selectedBusinessId, startAt, endAt)
          .then((d) => {
            setProducts(d);
            setError(null);
          })
          .catch((err: unknown) =>
            setError(describeActionError(err, 'Could not load the products report.')),
          );
        break;
      case 'margin':
        reportGrossMargin(selectedBusinessId, startAt, endAt)
          .then((d) => {
            setMargins(d);
            setError(null);
          })
          .catch((err: unknown) =>
            setError(describeActionError(err, 'Could not load the margin report.')),
          );
        break;
      case 'staff':
        reportStaffSales(selectedBusinessId, startAt, endAt)
          .then((d) => {
            setStaffSales(d);
            setError(null);
          })
          .catch((err: unknown) =>
            setError(describeActionError(err, 'Could not load the staff report.')),
          );
        break;
      case 'expenses':
        reportExpenses(selectedBusinessId, startAt, endAt)
          .then((d) => {
            setExpensesByCategory(d);
            setError(null);
          })
          .catch((err: unknown) =>
            setError(describeActionError(err, 'Could not load the expenses report.')),
          );
        break;
      case 'stock':
        reportStock(selectedBusinessId, stockRange.startAt, stockRange.endAt)
          .then((d) => {
            setDiscrepancies(d);
            setError(null);
          })
          .catch((err: unknown) =>
            setError(describeActionError(err, 'Could not load the stock report.')),
          );
        reportStockPurchases(selectedBusinessId, stockRange.startAt, stockRange.endAt)
          .then((d) => setStockPurchases(d))
          .catch((err: unknown) =>
            setError(describeActionError(err, 'Could not load the stock report.')),
          );
        break;
      case 'outstanding':
        listBills(selectedBusinessId, 'outstanding')
          .then((d) => {
            setOutstandingBills(d);
            setError(null);
          })
          .catch((err: unknown) =>
            setError(describeActionError(err, 'Could not load outstanding bills.')),
          );
        break;
    }
  }, [selectedBusinessId, tab, range, salesRange, stockRange]);

  const salesDailyValues: DayValue[] = useMemo(
    () => (salesDays ?? []).map((d) => ({ day: d.day, value: d.totalKobo })),
    [salesDays],
  );

  // The heatmap/drill-down above answers "when" at a glance; this list is
  // the same "recent detail below the picture" the Activity page's feed
  // gives its own heatmap -- capped to the last 30 days (salesDays itself
  // covers a full year, for the heatmap's Week/Month/Year views) so it
  // doesn't turn into a 365-row wall.
  const recentSalesDays = useMemo(() => {
    if (!salesDays) return [];
    const cutoff = new Date();
    cutoff.setUTCDate(cutoff.getUTCDate() - 29);
    const cutoffDay = cutoff.toISOString().slice(0, 10);
    return salesDays
      .filter((d) => d.day >= cutoffDay)
      .slice()
      .reverse();
  }, [salesDays]);

  const maxExpense = Math.max(1, ...(expensesByCategory ?? []).map((c) => c.totalKobo));
  const maxProduct = Math.max(1, ...(products ?? []).map((p) => p.unitsSold));

  const stockQuery = stockSearch.trim().toLowerCase();
  const matchesStockQuery = (productName: string, variantName: string) =>
    !stockQuery ||
    productName.toLowerCase().includes(stockQuery) ||
    variantName.toLowerCase().includes(stockQuery);
  const filteredStockPurchases = (stockPurchases ?? []).filter((p) =>
    matchesStockQuery(p.productName, p.variantName),
  );
  const filteredDiscrepancies = (discrepancies ?? []).filter((d) =>
    matchesStockQuery(d.productName, d.variantName),
  );
  const maxStockPurchase = Math.max(1, ...filteredStockPurchases.map((p) => p.totalKobo));
  const totalStockSpend = filteredStockPurchases.reduce((sum, p) => sum + p.totalKobo, 0);

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink pb-16">
      <div className="max-w-md mx-auto px-5 pt-6">
        <div className="flex items-center gap-2.5 mb-6">
          <Link
            to="/dashboard"
            aria-label="Back to dashboard"
            className="text-jb-ink/40 hover:text-jb-ink text-lg leading-none"
          >
            ←
          </Link>
          <h1 className="text-lg font-medium">Reports</h1>
        </div>

        {!isOnline && (
          <p className="text-[12px] text-jb-ink/40 mb-3">
            Offline — figures shown are from your last successful load, may be out of date.
          </p>
        )}

        <div className="flex flex-wrap gap-2 mb-5">
          {REPORT_TABS.map((t) => (
            <button
              key={t.key}
              onClick={() => setTab(t.key)}
              className={`px-3.5 py-2 rounded-full text-[13px] border transition-colors ${
                tab === t.key
                  ? 'bg-jb-ink text-jb-cream border-jb-ink'
                  : 'border-jb-ink/15 text-jb-ink/60 bg-white'
              }`}
            >
              {t.label}
            </button>
          ))}
        </div>

        {error && <p className="text-[13px] text-red-700 mb-3">{error}</p>}
        {tab !== 'sales' && tab !== 'stock' && (
          <p className="text-[11px] text-jb-ink/40 mb-3">Last 30 days</p>
        )}

        {tab === 'sales' &&
          (salesDays === null ? (
            <p className="text-[13px] text-jb-ink/40 px-1">Loading…</p>
          ) : (
            <div>
              <TimeGranularityView
                dailyData={salesDailyValues}
                defaultRangeDays={119}
                formatValue={(v) => `₦${formatNaira(v)}`}
              />
              <p className="text-[11px] text-jb-ink/40 mb-2 mt-5">Last 30 days</p>
              <div className="rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden">
                {recentSalesDays.length === 0 && (
                  <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">No sales in this range.</p>
                )}
                {recentSalesDays.map((d) => (
                  <div key={d.day} className="px-4 py-3 flex items-center justify-between">
                    <span className="text-[13px] text-jb-ink/70">
                      {new Date(d.day + 'T00:00:00Z').toLocaleDateString(undefined, {
                        month: 'short',
                        day: 'numeric',
                      })}
                    </span>
                    <span className="text-[13px] text-jb-ink/80">
                      ₦{formatNaira(d.totalKobo)} · {d.saleCount} sale{d.saleCount === 1 ? '' : 's'}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          ))}

        {tab === 'products' && (
          <div className="rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden">
            {products === null && (
              <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">Loading…</p>
            )}
            {products !== null && products.length === 0 && (
              <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">No units sold in this range.</p>
            )}
            {(products ?? []).map((p) => (
              <div key={p.variantId} className="px-4 py-3">
                <div className="flex items-center justify-between mb-1.5">
                  <span className="text-[13px] text-jb-ink/80">
                    {p.productName} — {p.variantName}
                  </span>
                  <span className="text-[13px] text-jb-ink/60">{p.unitsSold}</span>
                </div>
                <div className="h-1.5 rounded-full bg-jb-ink/[0.06] overflow-hidden">
                  <div
                    className="h-full bg-jb-green-dark rounded-full"
                    style={{ width: `${(p.unitsSold / maxProduct) * 100}%` }}
                  />
                </div>
              </div>
            ))}
          </div>
        )}

        {tab === 'margin' && (
          <div className="rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden">
            {margins === null && <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">Loading…</p>}
            {margins !== null && margins.length === 0 && (
              <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">No units sold in this range.</p>
            )}
            {(margins ?? []).map((m) => (
              <div key={m.variantId} className="px-4 py-3">
                <div className="flex items-center justify-between">
                  <span className="text-[13px] text-jb-ink/80">
                    {m.productName} — {m.variantName}
                  </span>
                  <span className="text-[13px] font-medium text-jb-ink/80">
                    ₦{formatNaira(m.grossMarginKobo)}
                  </span>
                </div>
                <div className="text-[11.5px] text-jb-ink/40 mt-0.5">
                  Revenue ₦{formatNaira(m.revenueKobo)} − cost ₦{formatNaira(m.resolvedCogsKobo)}
                </div>
                {m.unresolvedUnits > 0 && (
                  <div className="text-[11.5px] text-jb-gold mt-1">
                    {m.unresolvedUnits} unit{m.unresolvedUnits === 1 ? '' : 's'} sold with cost not
                    yet known — margin above is a lower bound, not final. Resolve the matching
                    review to record the real profit.
                  </div>
                )}
              </div>
            ))}
          </div>
        )}

        {tab === 'staff' && (
          <div className="rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden">
            {staffSales === null && (
              <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">Loading…</p>
            )}
            {staffSales !== null && staffSales.length === 0 && (
              <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">No sales in this range.</p>
            )}
            {(staffSales ?? []).map((s) => (
              <div key={s.sellerId} className="px-4 py-3.5 flex items-center justify-between">
                <span className="text-[13px] text-jb-ink/80">{s.sellerName || 'Unknown'}</span>
                <span className="text-[13px] text-jb-ink/60">
                  ₦{formatNaira(s.totalKobo)} · {s.saleCount} sale{s.saleCount === 1 ? '' : 's'}
                </span>
              </div>
            ))}
          </div>
        )}

        {tab === 'expenses' && (
          <div className="rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden">
            {expensesByCategory === null && (
              <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">Loading…</p>
            )}
            {expensesByCategory !== null && expensesByCategory.length === 0 && (
              <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">No expenses in this range.</p>
            )}
            {(expensesByCategory ?? []).map((c) => (
              <div key={c.categoryId} className="px-4 py-3">
                <div className="flex items-center justify-between mb-1.5">
                  <span className="text-[13px] text-jb-ink/80">{c.categoryName}</span>
                  <span className="text-[13px] text-jb-ink/60">₦{formatNaira(c.totalKobo)}</span>
                </div>
                <div className="h-1.5 rounded-full bg-jb-ink/[0.06] overflow-hidden">
                  <div
                    className="h-full bg-jb-gold rounded-full"
                    style={{ width: `${(c.totalKobo / maxExpense) * 100}%` }}
                  />
                </div>
              </div>
            ))}
          </div>
        )}

        {tab === 'stock' && (
          <>
            <div className="flex gap-2 mb-3">
              <label className="flex-1">
                <span className="block text-[11px] text-jb-ink/40 mb-1">From</span>
                <input
                  type="date"
                  value={stockRangeStart}
                  max={stockRangeEnd}
                  onChange={(e) => setStockRangeStart(e.target.value)}
                  className="w-full rounded-lg border border-jb-ink/15 bg-white px-3 py-2 text-[13px] text-jb-ink focus:outline-none focus:border-jb-ink/40"
                />
              </label>
              <label className="flex-1">
                <span className="block text-[11px] text-jb-ink/40 mb-1">To</span>
                <input
                  type="date"
                  value={stockRangeEnd}
                  min={stockRangeStart}
                  max={todayStr}
                  onChange={(e) => setStockRangeEnd(e.target.value)}
                  className="w-full rounded-lg border border-jb-ink/15 bg-white px-3 py-2 text-[13px] text-jb-ink focus:outline-none focus:border-jb-ink/40"
                />
              </label>
            </div>
            <input
              type="text"
              inputMode="search"
              placeholder="Filter by product…"
              value={stockSearch}
              onChange={(e) => setStockSearch(e.target.value)}
              className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-2.5 text-[13.5px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none focus:border-jb-ink/40 transition-colors mb-4"
            />

            <div className="rounded-xl bg-jb-ink text-jb-cream p-4 mb-4">
              <div className="text-[11px] text-jb-cream/60 mb-1">Spent restocking, this range</div>
              <div className="text-[20px] font-medium">₦{formatNaira(totalStockSpend)}</div>
            </div>

            <div className="text-[11px] font-medium text-jb-ink/40 uppercase tracking-wide mb-2">
              Restocking by product
            </div>
            <div className="rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden mb-6">
              {stockPurchases === null && (
                <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">Loading…</p>
              )}
              {stockPurchases !== null && filteredStockPurchases.length === 0 && (
                <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">
                  {stockQuery
                    ? `No restocking matches "${stockSearch.trim()}" in this range.`
                    : 'No stock purchases in this range.'}
                </p>
              )}
              {filteredStockPurchases.map((p) => (
                <div key={p.variantId} className="px-4 py-3">
                  <div className="flex items-center justify-between mb-1.5">
                    <span className="text-[13px] text-jb-ink/80">
                      {p.productName} — {p.variantName}
                    </span>
                    <span className="text-[13px] text-jb-ink/60">₦{formatNaira(p.totalKobo)}</span>
                  </div>
                  <div className="text-[11px] text-jb-ink/40 mb-1.5">
                    {p.quantityReceived} unit{p.quantityReceived === 1 ? '' : 's'} ·{' '}
                    {p.receiptCount} receipt{p.receiptCount === 1 ? '' : 's'}
                  </div>
                  <div className="h-1.5 rounded-full bg-jb-ink/[0.06] overflow-hidden">
                    <div
                      className="h-full bg-jb-gold rounded-full"
                      style={{ width: `${(p.totalKobo / maxStockPurchase) * 100}%` }}
                    />
                  </div>
                </div>
              ))}
            </div>

            <div className="text-[11px] font-medium text-jb-ink/40 uppercase tracking-wide mb-2">
              Count discrepancies
            </div>
            <div className="rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden">
              {discrepancies === null && (
                <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">Loading…</p>
              )}
              {discrepancies !== null && filteredDiscrepancies.length === 0 && (
                <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">
                  {stockQuery
                    ? `No discrepancies match "${stockSearch.trim()}" in this range.`
                    : 'No discrepancies in this range.'}
                </p>
              )}
              {filteredDiscrepancies.map((d) => (
                <div key={d.id} className="px-4 py-3.5">
                  <div className="flex items-center justify-between">
                    <span className="text-[13px] text-jb-ink/80">
                      {d.productName} — {d.variantName}
                    </span>
                    <span
                      className={`text-[13px] font-medium ${d.variance < 0 ? 'text-red-700/80' : 'text-jb-green-dark'}`}
                    >
                      {d.variance > 0 ? '+' : ''}
                      {d.variance}
                    </span>
                  </div>
                  <div className="text-[11px] text-jb-ink/40 mt-0.5">
                    Expected {d.expectedQuantity}, counted {d.physicalQuantity}
                    {d.isStale && ' · stale'} · {d.countedByName || 'Unknown'}
                  </div>
                </div>
              ))}
            </div>
          </>
        )}

        {tab === 'outstanding' && (
          <div className="rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden">
            {outstandingBills === null && (
              <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">Loading…</p>
            )}
            {outstandingBills !== null && outstandingBills.length === 0 && (
              <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">No outstanding bills.</p>
            )}
            {(outstandingBills ?? []).map((b) => (
              <div key={b.id} className="px-4 py-3.5 flex items-center justify-between">
                <span className="text-[13px] text-jb-ink/80">
                  Opened{' '}
                  {new Date(b.openedAt).toLocaleDateString(undefined, {
                    month: 'short',
                    day: 'numeric',
                  })}
                </span>
                <span className="text-[13px] text-jb-ink/60">₦{formatNaira(b.balanceKobo)}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
