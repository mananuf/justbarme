import { useCallback, useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import { getPlatformBusinessActivity, type PlatformBusiness } from '../../api/platform';
import {
  approvePlatformAdjustmentRequest,
  closePlatformBill,
  correctPlatformBalance,
  forceReversePlatformStockReceipt,
  getPlatformBillDetail,
  getPlatformSale,
  getPlatformStockHistory,
  getPlatformStockReceipt,
  listPlatformActivityFeed,
  listPlatformAdjustmentRequests,
  listPlatformBills,
  listPlatformExpenses,
  listPlatformInventoryReviews,
  listPlatformMembers,
  listPlatformSaleReviews,
  listPlatformSales,
  rejectPlatformAdjustmentRequest,
  resolvePlatformInventoryReview,
  resolvePlatformSaleReview,
  reversePlatformExpense,
  reversePlatformSale,
  reversePlatformStockReceipt,
  voidPlatformBill,
  type ActivityEntry,
  type AdjustmentRequest,
  type Bill,
  type BillDetail,
  type Expense,
  type HistoryEntry,
  type InventoryReview,
  type Member,
  type Review,
  type Sale,
  type StockReceipt,
} from '../../api/platformDetail';
import { Logo } from '../../components/Logo';
import { describeActionError } from '../../lib/errors';
import { usePlatformSession } from '../../lib/platformSession';

// docs/PHASE_PLATFORM_ADMIN_DEEP_DRILL.md: full line-item drill-down into
// one business plus superadmin-only corrective actions, widening
// PlatformDashboard's aggregate-only BusinessActivityPanel. Deliberately
// no customer PII anywhere on this page -- the API never returns it (see
// the plan doc's "protected by omission" note), so there is nothing here
// to accidentally render.

function naira(kobo: number): string {
  const sign = kobo < 0 ? '−' : '';
  return `${sign}₦${(Math.abs(kobo) / 100).toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
}

function when(iso?: string): string {
  if (!iso) return '';
  return new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
}

type Tab = 'sales' | 'expenses' | 'bills' | 'stock' | 'reviews' | 'members' | 'activity';

const TABS: { key: Tab; label: string }[] = [
  { key: 'sales', label: 'Sales' },
  { key: 'expenses', label: 'Expenses' },
  { key: 'bills', label: 'Bills' },
  { key: 'stock', label: 'Stock' },
  { key: 'reviews', label: 'Reviews' },
  { key: 'members', label: 'Members' },
  { key: 'activity', label: 'Activity' },
];

// pendingAction drives the one shared inline "reason" prompt every
// corrective-action button on this page opens -- matching
// PlatformDashboard's existing suspend/reactivate prompt shape, so the
// interaction feels the same whether you're suspending a business or
// reversing one of its sales.
interface PendingAction {
  label: string;
  run: (reason: string) => Promise<void>;
}

function ActionButton({
  label,
  tone = 'default',
  onClick,
}: {
  label: string;
  tone?: 'default' | 'danger';
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      className={`text-[12px] px-2.5 py-1 rounded-lg border transition-colors ${
        tone === 'danger'
          ? 'border-red-200 text-red-700 hover:bg-red-50'
          : 'border-jb-ink/15 text-jb-ink/70 hover:bg-jb-ink/[0.04]'
      }`}
    >
      {label}
    </button>
  );
}

// CorrectBalancePrompt is its own small form (target quantity + reason)
// rather than reusing ActionPrompt's single-reason-field shape -- a
// balance correction needs a target value, not just a reason.
function CorrectBalancePrompt({
  variantName,
  onConfirm,
  onCancel,
}: {
  variantName?: string;
  onConfirm: (targetQuantity: number, reason: string) => Promise<void>;
  onCancel: () => void;
}) {
  const [target, setTarget] = useState('');
  const [reason, setReason] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const targetNum = Number(target);
  const canSubmit = target.trim() !== '' && !Number.isNaN(targetNum) && reason.trim() !== '';

  async function confirm() {
    setSubmitting(true);
    setError(null);
    try {
      await onConfirm(targetNum, reason);
    } catch (err) {
      setError(describeActionError(err, 'Could not correct this balance.'));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rounded-lg border border-jb-ink/10 bg-jb-ink/[0.03] px-3 py-2.5 mt-2">
      <p className="text-[12.5px] text-jb-ink/70 mb-2">
        Set {variantName ?? 'this variant'}&apos;s balance to a new value. Posted as a real,
        auto-approved adjustment through the normal ledger -- never a raw overwrite.
      </p>
      <div className="flex items-center gap-2 flex-wrap">
        <input
          type="number"
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          placeholder="New balance"
          className="w-28 rounded-lg border border-jb-ink/15 bg-white px-3 py-1.5 text-[13px] focus:outline-none focus:border-jb-ink/40"
        />
        <input
          type="text"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder="Why is this being corrected?"
          className="flex-1 min-w-[12rem] rounded-lg border border-jb-ink/15 bg-white px-3 py-1.5 text-[13px] focus:outline-none focus:border-jb-ink/40"
        />
        <button
          onClick={() => void confirm()}
          disabled={submitting || !canSubmit}
          className="text-[13px] px-3 py-1.5 rounded-lg bg-jb-ink text-jb-cream disabled:opacity-40 transition-colors"
        >
          {submitting ? 'Working…' : 'Confirm'}
        </button>
        <button
          onClick={onCancel}
          className="text-[13px] px-2 py-1.5 text-jb-ink/50 hover:text-jb-ink"
        >
          Cancel
        </button>
      </div>
      {error && (
        <p role="alert" className="text-[12px] text-red-700 mt-2">
          {error}
        </p>
      )}
    </div>
  );
}

function ActionPrompt({ pending, onCancel }: { pending: PendingAction; onCancel: () => void }) {
  const [reason, setReason] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function confirm() {
    setSubmitting(true);
    setError(null);
    try {
      await pending.run(reason);
    } catch (err) {
      setError(describeActionError(err, 'Could not complete this action.'));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rounded-lg border border-jb-ink/10 bg-jb-ink/[0.03] px-3 py-2.5 mt-2">
      <p className="text-[12.5px] text-jb-ink/70 mb-2">
        {pending.label}. A reason is required, and it&apos;s recorded in the audit log and shown in
        the business&apos;s own activity feed.
      </p>
      <div className="flex items-center gap-2">
        <input
          type="text"
          autoFocus
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder="Why is this being done?"
          className="flex-1 rounded-lg border border-jb-ink/15 bg-white px-3 py-1.5 text-[13px] focus:outline-none focus:border-jb-ink/40"
        />
        <button
          onClick={() => void confirm()}
          disabled={submitting || !reason.trim()}
          className="text-[13px] px-3 py-1.5 rounded-lg bg-jb-ink text-jb-cream disabled:opacity-40 transition-colors"
        >
          {submitting ? 'Working…' : 'Confirm'}
        </button>
        <button
          onClick={onCancel}
          className="text-[13px] px-2 py-1.5 text-jb-ink/50 hover:text-jb-ink"
        >
          Cancel
        </button>
      </div>
      {error && (
        <p role="alert" className="text-[12px] text-red-700 mt-2">
          {error}
        </p>
      )}
    </div>
  );
}

export function PlatformBusinessDetail() {
  const { businessId } = useParams<{ businessId: string }>();
  const { staff, csrfToken } = usePlatformSession();
  const canAdjust = staff?.role === 'superadmin' && !!csrfToken;

  const [business, setBusiness] = useState<PlatformBusiness | null>(null);
  const [tab, setTab] = useState<Tab>('sales');
  const [pending, setPending] = useState<{ key: string; action: PendingAction } | null>(null);

  const [sales, setSales] = useState<Sale[] | null>(null);
  const [openSale, setOpenSale] = useState<Sale | null>(null);
  const [expenses, setExpenses] = useState<Expense[] | null>(null);
  const [bills, setBills] = useState<Bill[] | null>(null);
  const [openBill, setOpenBill] = useState<BillDetail | null>(null);
  const [receiptCache, setReceiptCache] = useState<Record<string, StockReceipt>>({});
  const [adjustments, setAdjustments] = useState<AdjustmentRequest[] | null>(null);
  const [history, setHistory] = useState<{ variantId: string; entries: HistoryEntry[] } | null>(
    null,
  );
  const [correctingVariant, setCorrectingVariant] = useState<{
    variantId: string;
    variantName?: string;
  } | null>(null);
  const [saleReviews, setSaleReviews] = useState<Review[] | null>(null);
  const [inventoryReviews, setInventoryReviews] = useState<InventoryReview[] | null>(null);
  const [members, setMembers] = useState<Member[] | null>(null);
  const [activity, setActivity] = useState<ActivityEntry[] | null>(null);
  const [listError, setListError] = useState<string | null>(null);

  useEffect(() => {
    if (!businessId) return;
    void (async () => {
      try {
        const result = await getPlatformBusinessActivity(businessId);
        setBusiness(result.business);
      } catch {
        // The header is secondary -- the tabs below still work without it.
      }
    })();
  }, [businessId]);

  const load = useCallback(
    async (t: Tab) => {
      if (!businessId) return;
      setListError(null);
      try {
        switch (t) {
          case 'sales':
            setSales(await listPlatformSales(businessId));
            break;
          case 'expenses':
            setExpenses(await listPlatformExpenses(businessId));
            break;
          case 'bills':
            setBills(await listPlatformBills(businessId, 'all'));
            break;
          case 'stock':
            setAdjustments(await listPlatformAdjustmentRequests(businessId));
            break;
          case 'reviews':
            setSaleReviews(await listPlatformSaleReviews(businessId));
            setInventoryReviews(await listPlatformInventoryReviews(businessId));
            break;
          case 'members':
            setMembers(await listPlatformMembers(businessId));
            break;
          case 'activity':
            setActivity(await listPlatformActivityFeed(businessId));
            break;
        }
      } catch (err) {
        setListError(describeActionError(err, 'Could not load this.'));
      }
    },
    [businessId],
  );

  useEffect(() => {
    void (async () => {
      await load(tab);
    })();
  }, [tab, load]);

  if (!businessId) return null;

  async function openSaleDetail(saleId: string) {
    try {
      setOpenSale(await getPlatformSale(businessId!, saleId));
    } catch (err) {
      setListError(describeActionError(err, 'Could not load this sale.'));
    }
  }

  async function openBillDetail(billId: string) {
    try {
      setOpenBill(await getPlatformBillDetail(businessId!, billId));
    } catch (err) {
      setListError(describeActionError(err, 'Could not load this bill.'));
    }
  }

  async function loadReceiptInline(receiptId: string) {
    if (receiptCache[receiptId]) return;
    try {
      const r = await getPlatformStockReceipt(businessId!, receiptId);
      setReceiptCache((prev) => ({ ...prev, [receiptId]: r }));
    } catch (err) {
      setListError(describeActionError(err, 'Could not load this receipt.'));
    }
  }

  async function loadVariantHistory(variantId: string) {
    try {
      const entries = await getPlatformStockHistory(businessId!, variantId);
      setHistory({ variantId, entries });
    } catch (err) {
      setListError(describeActionError(err, 'Could not load stock history.'));
    }
  }

  function startAction(key: string, label: string, run: (reason: string) => Promise<void>) {
    setPending({ key, action: { label, run } });
  }

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink">
      <header className="border-b border-jb-ink/10 bg-white/60">
        <div className="max-w-5xl mx-auto px-6 py-4 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Link to="/platform" className="text-jb-ink/40 hover:text-jb-ink text-lg leading-none">
              ←
            </Link>
            <Logo className="w-5 h-5 text-jb-ink/70" />
            <div>
              <div className="text-[14px] font-medium">{business?.name ?? 'Business'}</div>
              <div className="text-[11px] text-jb-ink/40 uppercase tracking-wide">
                Deep drill-down
              </div>
            </div>
          </div>
          {!canAdjust && (
            <span className="text-[11px] text-jb-ink/40">
              Read-only ({staff?.role ?? 'support'})
            </span>
          )}
        </div>
      </header>

      <main className="max-w-5xl mx-auto px-6 py-8">
        <div className="flex gap-1 mb-5 border-b border-jb-ink/10 overflow-x-auto" role="tablist">
          {TABS.map((t) => (
            <button
              key={t.key}
              role="tab"
              aria-selected={tab === t.key}
              onClick={() => {
                setTab(t.key);
                setPending(null);
              }}
              className={`text-[13px] px-4 py-2 -mb-px border-b-2 whitespace-nowrap transition-colors ${
                tab === t.key
                  ? 'border-jb-ink text-jb-ink font-medium'
                  : 'border-transparent text-jb-ink/45 hover:text-jb-ink'
              }`}
            >
              {t.label}
            </button>
          ))}
        </div>

        {listError && (
          <p role="alert" className="text-[13px] text-red-700 mb-4">
            {listError}
          </p>
        )}

        {tab === 'sales' && (
          <div className="rounded-xl border border-jb-ink/10 bg-white/70 divide-y divide-jb-ink/[0.06] overflow-hidden">
            {sales === null && <p className="px-4 py-3 text-[13px] text-jb-ink/45">Loading…</p>}
            {sales?.length === 0 && (
              <p className="px-4 py-3 text-[13px] text-jb-ink/45">No sales yet.</p>
            )}
            {sales?.map((s) => (
              <div key={s.id} className="px-4 py-3">
                <div className="flex items-center justify-between gap-3">
                  <button
                    onClick={() => void openSaleDetail(s.id)}
                    className="text-left text-[13px] text-jb-ink/85 hover:underline underline-offset-2"
                  >
                    {when(s.occurred_at)} {s.reversal_of && '(reversal)'}
                  </button>
                  <div className="flex items-center gap-2 shrink-0">
                    <span
                      className={`text-[13px] font-medium ${s.total_kobo < 0 ? 'text-jb-green-dark' : ''}`}
                    >
                      {naira(s.total_kobo)}
                    </span>
                    {canAdjust && !s.reversal_of && (
                      <ActionButton
                        label="Reverse"
                        tone="danger"
                        onClick={() =>
                          startAction(
                            `sale-${s.id}`,
                            `Reverse this sale (${naira(s.total_kobo)})`,
                            async (reason) => {
                              await reversePlatformSale(businessId, s.id, reason, csrfToken);
                              setPending(null);
                              void load('sales');
                            },
                          )
                        }
                      />
                    )}
                  </div>
                </div>
                {openSale?.id === s.id && (
                  <div className="mt-2 text-[12.5px] text-jb-ink/60 space-y-1">
                    {openSale.items?.map((it, i) => (
                      <div key={i} className="flex justify-between">
                        <span>
                          {it.quantity} × {it.description}
                        </span>
                        <span>{naira(it.line_total_kobo)}</span>
                      </div>
                    ))}
                    <div>
                      Payment: {naira(openSale.payment.amount_kobo)} ({openSale.payment.method})
                    </div>
                    <Link
                      to={`#bill-${openSale.bill_id}`}
                      onClick={(e) => {
                        e.preventDefault();
                        setTab('bills');
                        void openBillDetail(openSale.bill_id);
                      }}
                      className="underline underline-offset-2"
                    >
                      View bill →
                    </Link>
                  </div>
                )}
                {pending?.key === `sale-${s.id}` && (
                  <ActionPrompt pending={pending.action} onCancel={() => setPending(null)} />
                )}
              </div>
            ))}
          </div>
        )}

        {tab === 'expenses' && (
          <div className="rounded-xl border border-jb-ink/10 bg-white/70 divide-y divide-jb-ink/[0.06] overflow-hidden">
            {expenses === null && <p className="px-4 py-3 text-[13px] text-jb-ink/45">Loading…</p>}
            {expenses?.length === 0 && (
              <p className="px-4 py-3 text-[13px] text-jb-ink/45">No expenses yet.</p>
            )}
            {expenses?.map((e) => (
              <div key={e.id} className="px-4 py-3">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <div className="text-[13px] text-jb-ink/85">{e.description}</div>
                    <div className="text-[11px] text-jb-ink/40">
                      {e.category_name} · {when(e.occurred_at)}
                      {e.reversal_of && ' · reversal'}
                    </div>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <span
                      className={`text-[13px] font-medium ${e.amount_kobo < 0 ? 'text-jb-green-dark' : ''}`}
                    >
                      {naira(e.amount_kobo)}
                    </span>
                    {canAdjust && !e.reversal_of && (
                      <ActionButton
                        label="Reverse"
                        tone="danger"
                        onClick={() =>
                          startAction(
                            `expense-${e.id}`,
                            `Reverse this expense (${naira(e.amount_kobo)})`,
                            async (reason) => {
                              await reversePlatformExpense(businessId, e.id, reason, csrfToken);
                              setPending(null);
                              void load('expenses');
                            },
                          )
                        }
                      />
                    )}
                  </div>
                </div>
                {pending?.key === `expense-${e.id}` && (
                  <ActionPrompt pending={pending.action} onCancel={() => setPending(null)} />
                )}
              </div>
            ))}
          </div>
        )}

        {tab === 'bills' && (
          <div className="rounded-xl border border-jb-ink/10 bg-white/70 divide-y divide-jb-ink/[0.06] overflow-hidden">
            {bills === null && <p className="px-4 py-3 text-[13px] text-jb-ink/45">Loading…</p>}
            {bills?.length === 0 && (
              <p className="px-4 py-3 text-[13px] text-jb-ink/45">No bills yet.</p>
            )}
            {bills?.map((b) => (
              <div key={b.id} id={`bill-${b.id}`} className="px-4 py-3">
                <div className="flex items-center justify-between gap-3">
                  <button
                    onClick={() => void openBillDetail(b.id)}
                    className="text-left text-[13px] text-jb-ink/85 hover:underline underline-offset-2"
                  >
                    {b.status} · opened {when(b.opened_at)}
                  </button>
                  <div className="flex items-center gap-2 shrink-0">
                    <span className="text-[13px] font-medium">{naira(b.balance_kobo)}</span>
                    {canAdjust && (b.status === 'open' || b.status === 'closed_unpaid') && (
                      <>
                        <ActionButton
                          label="Close"
                          onClick={() =>
                            startAction(`bill-close-${b.id}`, 'Close this bill', async (reason) => {
                              await closePlatformBill(businessId, b.id, reason, csrfToken);
                              setPending(null);
                              void load('bills');
                            })
                          }
                        />
                        <ActionButton
                          label="Void"
                          tone="danger"
                          onClick={() =>
                            startAction(`bill-void-${b.id}`, 'Void this bill', async (reason) => {
                              await voidPlatformBill(businessId, b.id, reason, csrfToken);
                              setPending(null);
                              void load('bills');
                            })
                          }
                        />
                      </>
                    )}
                  </div>
                </div>
                {openBill?.id === b.id && (
                  <div className="mt-2 text-[12.5px] text-jb-ink/60 space-y-2">
                    {openBill.sales?.map((s) => (
                      <div key={s.id} className="flex justify-between">
                        <span>
                          {when(s.occurred_at)} {s.seller_name && `· ${s.seller_name}`}
                        </span>
                        <span className={s.total_kobo < 0 ? 'text-jb-green-dark' : ''}>
                          {naira(s.total_kobo)}
                        </span>
                      </div>
                    ))}
                    {openBill.payments?.map((p, i) => (
                      <div key={i} className="flex justify-between text-jb-ink/45">
                        <span>Payment ({p.method})</span>
                        <span>{naira(p.amount_kobo)}</span>
                      </div>
                    ))}
                  </div>
                )}
                {(pending?.key === `bill-close-${b.id}` ||
                  pending?.key === `bill-void-${b.id}`) && (
                  <ActionPrompt pending={pending.action} onCancel={() => setPending(null)} />
                )}
              </div>
            ))}
          </div>
        )}

        {tab === 'stock' && (
          <div>
            <div className="text-[11px] text-jb-ink/45 uppercase tracking-wide mb-2">
              Adjustment requests (all, including decided)
            </div>
            <div className="rounded-xl border border-jb-ink/10 bg-white/70 divide-y divide-jb-ink/[0.06] overflow-hidden mb-6">
              {adjustments === null && (
                <p className="px-4 py-3 text-[13px] text-jb-ink/45">Loading…</p>
              )}
              {adjustments?.length === 0 && (
                <p className="px-4 py-3 text-[13px] text-jb-ink/45">No adjustment requests yet.</p>
              )}
              {adjustments?.map((a) => (
                <div key={a.id} className="px-4 py-3">
                  <div className="flex items-center justify-between gap-3">
                    <button
                      onClick={() => void loadVariantHistory(a.variant_id)}
                      className="text-left text-[13px] text-jb-ink/85 hover:underline underline-offset-2"
                    >
                      {a.product_name} — {a.variant_name} ({a.quantity_delta > 0 ? '+' : ''}
                      {a.quantity_delta}) · {a.reason_category}
                    </button>
                    <div className="flex items-center gap-2 shrink-0">
                      <span className="text-[11px] text-jb-ink/40 uppercase">{a.status}</span>
                      {canAdjust && a.status === 'pending' && (
                        <>
                          <ActionButton
                            label="Approve"
                            onClick={() =>
                              startAction(
                                `adj-approve-${a.id}`,
                                'Approve this adjustment request',
                                async (reason) => {
                                  await approvePlatformAdjustmentRequest(
                                    businessId,
                                    a.id,
                                    reason,
                                    csrfToken,
                                  );
                                  setPending(null);
                                  void load('stock');
                                },
                              )
                            }
                          />
                          <ActionButton
                            label="Reject"
                            tone="danger"
                            onClick={() =>
                              startAction(
                                `adj-reject-${a.id}`,
                                'Reject this adjustment request',
                                async (reason) => {
                                  await rejectPlatformAdjustmentRequest(
                                    businessId,
                                    a.id,
                                    reason,
                                    csrfToken,
                                  );
                                  setPending(null);
                                  void load('stock');
                                },
                              )
                            }
                          />
                        </>
                      )}
                    </div>
                  </div>
                  <div className="text-[11px] text-jb-ink/40 mt-0.5">
                    {a.reason_note} · requested by {a.requested_by_name ?? 'unknown'}
                    {a.decided_by_name && ` · decided by ${a.decided_by_name}`}
                  </div>
                  {(pending?.key === `adj-approve-${a.id}` ||
                    pending?.key === `adj-reject-${a.id}`) && (
                    <ActionPrompt pending={pending.action} onCancel={() => setPending(null)} />
                  )}
                  {history?.variantId === a.variant_id && (
                    <div className="mt-2 rounded-lg bg-jb-ink/[0.03] p-2.5 text-[12px] text-jb-ink/60 space-y-1">
                      <div className="flex items-center justify-between">
                        <span className="font-medium text-jb-ink/70">Stock history</span>
                        {canAdjust && (
                          <ActionButton
                            label="Correct balance…"
                            onClick={() =>
                              setCorrectingVariant({
                                variantId: a.variant_id,
                                variantName: a.variant_name,
                              })
                            }
                          />
                        )}
                      </div>
                      {correctingVariant?.variantId === a.variant_id && (
                        <CorrectBalancePrompt
                          variantName={correctingVariant.variantName}
                          onCancel={() => setCorrectingVariant(null)}
                          onConfirm={async (target, reason) => {
                            if (!csrfToken) return;
                            await correctPlatformBalance(
                              businessId,
                              a.variant_id,
                              target,
                              reason,
                              csrfToken,
                            );
                            setCorrectingVariant(null);
                            void load('stock');
                            void loadVariantHistory(a.variant_id);
                          }}
                        />
                      )}
                      {history.entries.map((h) => (
                        <div key={h.id} className="flex justify-between">
                          <span>
                            {h.event_type}
                            {h.adjustment_reason_category && ` (${h.adjustment_reason_category})`}
                            {h.actor_name && ` · ${h.actor_name}`}
                          </span>
                          <span
                            className={h.quantity_delta < 0 ? 'text-red-700' : 'text-jb-green-dark'}
                          >
                            {h.quantity_delta > 0 ? '+' : ''}
                            {h.quantity_delta}
                          </span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>

            <p className="text-[11px] text-jb-ink/35">
              Stock receipts can be reversed or force-reversed from a sale&apos;s detail or a
              receipt&apos;s own history entry above -- open a receipt via the activity feed for the
              reverse/force-reverse actions.
            </p>
          </div>
        )}

        {tab === 'reviews' && (
          <div>
            <div className="text-[11px] text-jb-ink/45 uppercase tracking-wide mb-2">
              Sale reviews
            </div>
            <div className="rounded-xl border border-jb-ink/10 bg-white/70 divide-y divide-jb-ink/[0.06] overflow-hidden mb-6">
              {saleReviews === null && (
                <p className="px-4 py-3 text-[13px] text-jb-ink/45">Loading…</p>
              )}
              {saleReviews?.length === 0 && (
                <p className="px-4 py-3 text-[13px] text-jb-ink/45">No open sale reviews.</p>
              )}
              {saleReviews?.map((r) => (
                <div key={r.id} className="px-4 py-3">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <div className="text-[13px] text-jb-ink/85">{r.reason}</div>
                      <div className="text-[11px] text-jb-ink/40">
                        {r.item_description} · {r.seller_name} · {when(r.sale_occurred_at)}
                      </div>
                    </div>
                    {canAdjust && (
                      <ActionButton
                        label="Resolve"
                        onClick={() =>
                          startAction(
                            `salerev-${r.id}`,
                            'Resolve this sale review',
                            async (reason) => {
                              await resolvePlatformSaleReview(businessId, r.id, reason, csrfToken);
                              setPending(null);
                              void load('reviews');
                            },
                          )
                        }
                      />
                    )}
                  </div>
                  {pending?.key === `salerev-${r.id}` && (
                    <ActionPrompt pending={pending.action} onCancel={() => setPending(null)} />
                  )}
                </div>
              ))}
            </div>

            <div className="text-[11px] text-jb-ink/45 uppercase tracking-wide mb-2">
              Inventory reviews
            </div>
            <div className="rounded-xl border border-jb-ink/10 bg-white/70 divide-y divide-jb-ink/[0.06] overflow-hidden">
              {inventoryReviews === null && (
                <p className="px-4 py-3 text-[13px] text-jb-ink/45">Loading…</p>
              )}
              {inventoryReviews?.length === 0 && (
                <p className="px-4 py-3 text-[13px] text-jb-ink/45">No open inventory reviews.</p>
              )}
              {inventoryReviews?.map((r) => (
                <div key={r.id} className="px-4 py-3">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <div className="text-[13px] text-jb-ink/85">
                        {r.type} — {r.product_name} / {r.variant_name}
                      </div>
                      <div className="text-[11px] text-jb-ink/40">{when(r.created_at)}</div>
                    </div>
                    {canAdjust && (
                      <ActionButton
                        label="Resolve"
                        onClick={() =>
                          startAction(
                            `invrev-${r.id}`,
                            'Resolve this inventory review',
                            async (reason) => {
                              await resolvePlatformInventoryReview(
                                businessId,
                                r.id,
                                reason,
                                csrfToken,
                              );
                              setPending(null);
                              void load('reviews');
                            },
                          )
                        }
                      />
                    )}
                  </div>
                  {pending?.key === `invrev-${r.id}` && (
                    <ActionPrompt pending={pending.action} onCancel={() => setPending(null)} />
                  )}
                </div>
              ))}
            </div>
          </div>
        )}

        {tab === 'members' && (
          <div className="rounded-xl border border-jb-ink/10 bg-white/70 divide-y divide-jb-ink/[0.06] overflow-hidden">
            {members === null && <p className="px-4 py-3 text-[13px] text-jb-ink/45">Loading…</p>}
            {members?.map((m, i) => (
              <div key={i} className="px-4 py-3 flex items-center justify-between">
                <span className="text-[13px] text-jb-ink/85">{m.name}</span>
                <span className="text-[11px] text-jb-ink/40">
                  {m.role} · joined {when(m.joined_at)}
                </span>
              </div>
            ))}
          </div>
        )}

        {tab === 'activity' && (
          <div className="rounded-xl border border-jb-ink/10 bg-white/70 divide-y divide-jb-ink/[0.06] overflow-hidden">
            {activity === null && <p className="px-4 py-3 text-[13px] text-jb-ink/45">Loading…</p>}
            {activity?.length === 0 && (
              <p className="px-4 py-3 text-[13px] text-jb-ink/45">Nothing recorded yet.</p>
            )}
            {activity?.map((e) => (
              <div key={e.id} className="px-4 py-3">
                <div className="flex items-center justify-between gap-3">
                  <button
                    onClick={() => {
                      if (e.type === 'stock_receipt') void loadReceiptInline(e.id);
                    }}
                    disabled={e.type !== 'stock_receipt'}
                    className="text-left flex-1 disabled:cursor-default"
                  >
                    <div className="text-[13px] text-jb-ink/85">{e.summary}</div>
                    <div className="text-[11px] text-jb-ink/40">
                      {e.actor_name} · {when(e.occurred_at)}
                    </div>
                  </button>
                  <span
                    className={`text-[13px] font-medium shrink-0 ${
                      e.amount_kobo < 0 ? 'text-red-700' : 'text-jb-green-dark'
                    }`}
                  >
                    {naira(e.amount_kobo)}
                  </span>
                </div>
                <div>
                  {(() => {
                    const receipt = receiptCache[e.id];
                    if (!receipt) return null;
                    return (
                      <div className="mt-1.5 text-[12px] text-jb-ink/60 space-y-1">
                        {receipt.lines.map((l, i) => (
                          <div key={i} className="flex justify-between">
                            <span>
                              {l.quantity} × {l.product_name} — {l.variant_name}
                            </span>
                            <span>{naira(l.total_cost_kobo)}</span>
                          </div>
                        ))}
                        {canAdjust && !receipt.reversal_of_this && (
                          <div className="flex gap-2 pt-1">
                            <ActionButton
                              label="Reverse"
                              tone="danger"
                              onClick={() =>
                                startAction(
                                  `receipt-${e.id}`,
                                  'Reverse this stock receipt (blocked if any of it has already sold)',
                                  async (reason) => {
                                    await reversePlatformStockReceipt(
                                      businessId,
                                      e.id,
                                      reason,
                                      csrfToken,
                                    );
                                    setPending(null);
                                    void loadReceiptInline(e.id);
                                    void load('activity');
                                  },
                                )
                              }
                            />
                            <ActionButton
                              label="Force reverse…"
                              onClick={() =>
                                startAction(
                                  `receipt-force-${e.id}`,
                                  'Force-reverse this receipt, giving back only what’s still unsold',
                                  async (reason) => {
                                    await forceReversePlatformStockReceipt(
                                      businessId,
                                      e.id,
                                      reason,
                                      csrfToken,
                                    );
                                    setPending(null);
                                    void loadReceiptInline(e.id);
                                    void load('activity');
                                  },
                                )
                              }
                            />
                          </div>
                        )}
                        {(pending?.key === `receipt-${e.id}` ||
                          pending?.key === `receipt-force-${e.id}`) && (
                          <ActionPrompt
                            pending={pending.action}
                            onCancel={() => setPending(null)}
                          />
                        )}
                      </div>
                    );
                  })()}
                </div>
              </div>
            ))}
          </div>
        )}
      </main>
    </div>
  );
}
