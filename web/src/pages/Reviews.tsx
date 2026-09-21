import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import {
  approveAdjustment,
  listPendingAdjustments,
  rejectAdjustment,
  type AdjustmentRequest,
} from '../api/inventory';
import {
  listInventoryReviews,
  listSaleReviews,
  resolveInventoryReview,
  resolveSaleReview,
  type InventoryReview,
  type SaleReview,
} from '../api/reviews';
import { describeActionError } from '../lib/errors';
import { useSession } from '../lib/session';

function formatNaira(kobo: number): string {
  return (kobo / 100).toLocaleString();
}

function formatWhen(iso: string): string {
  if (!iso) return '';
  return new Date(iso).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  });
}

function reasonLabel(reason: SaleReview['reason']): string {
  switch (reason) {
    case 'deactivated_variant':
      return 'Item was deactivated after this sale';
    case 'price_mismatch':
      return "Charged price didn't match the price in effect at the time";
  }
}

const ADJUSTMENT_REASON_LABELS: Record<AdjustmentRequest['reasonCategory'], string> = {
  complimentary: 'Complimentary',
  broken: 'Broken',
  spoiled: 'Spoiled',
  staff_use: 'Staff use',
  manual: 'Manual correction',
  count_correction: 'Stock count correction',
};

function inventoryReviewLabel(review: InventoryReview): string {
  switch (review.type) {
    case 'negative_inventory':
      return 'Stock went negative';
    case 'stale_stock_count':
      return "Stock count didn't match current stock";
  }
}

// Owner-facing queue combining three related, but distinct, flags:
// pending adjustment requests (approve/reject, from Stock.tsx's "Report
// issue" flow), inventory reviews (negative balance or a stale stock
// count, docs/PHASE_INVENTORY_COUNTS_AND_ADJUSTMENTS.md), and sale_reviews
// (docs/PHASE_OFFLINE_CATALOGUE_AND_REVIEWS.md). A sale/adjustment is never
// silently rejected for a stale price, deactivated variant, or unexpected
// balance -- each opens one of these instead, for the owner to look at.
export function Reviews() {
  const { csrfToken, selectedBusinessId } = useSession();

  const [adjustments, setAdjustments] = useState<AdjustmentRequest[] | null>(null);
  const [adjustmentsError, setAdjustmentsError] = useState<string | null>(null);
  const [decidingId, setDecidingId] = useState<string | null>(null);
  const [decidingAction, setDecidingAction] = useState<'approve' | 'reject' | null>(null);
  const [decisionNote, setDecisionNote] = useState('');
  const [decisionError, setDecisionError] = useState<string | null>(null);
  const [decisionBusy, setDecisionBusy] = useState(false);

  const [inventoryReviews, setInventoryReviews] = useState<InventoryReview[] | null>(null);
  const [inventoryReviewsError, setInventoryReviewsError] = useState<string | null>(null);
  const [resolvingInventoryId, setResolvingInventoryId] = useState<string | null>(null);
  const [inventoryNote, setInventoryNote] = useState('');
  // Only meaningful for a negative_inventory review that traces back to a
  // sale's oversell (docs/PHASE_FIFO_COSTING.md §5) -- left blank, the
  // pending FIFO cost allocation (if any) just stays pending.
  const [resolvedUnitCostNaira, setResolvedUnitCostNaira] = useState('');
  const [inventoryActionError, setInventoryActionError] = useState<string | null>(null);
  const [inventoryBusy, setInventoryBusy] = useState(false);

  const [reviews, setReviews] = useState<SaleReview[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [resolvingId, setResolvingId] = useState<string | null>(null);
  const [note, setNote] = useState('');
  const [actionError, setActionError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const loadAdjustments = useCallback(() => {
    if (!selectedBusinessId) return;
    listPendingAdjustments(selectedBusinessId)
      .then(setAdjustments)
      .catch((err: unknown) =>
        setAdjustmentsError(describeActionError(err, 'Could not load pending adjustments.')),
      );
  }, [selectedBusinessId]);

  useEffect(() => {
    loadAdjustments();
  }, [loadAdjustments]);

  useEffect(() => {
    if (!selectedBusinessId) return;
    listInventoryReviews(selectedBusinessId)
      .then(setInventoryReviews)
      .catch((err: unknown) =>
        setInventoryReviewsError(describeActionError(err, 'Could not load inventory reviews.')),
      );
  }, [selectedBusinessId]);

  useEffect(() => {
    if (!selectedBusinessId) return;
    listSaleReviews(selectedBusinessId)
      .then(setReviews)
      .catch((err: unknown) => setLoadError(describeActionError(err, 'Could not load reviews.')));
  }, [selectedBusinessId]);

  function startDeciding(id: string, action: 'approve' | 'reject') {
    setDecidingId(id);
    setDecidingAction(action);
    setDecisionNote('');
    setDecisionError(null);
  }

  async function handleDecision(id: string) {
    if (!selectedBusinessId || !csrfToken || !decisionNote.trim() || !decidingAction) return;
    setDecisionBusy(true);
    setDecisionError(null);
    try {
      if (decidingAction === 'approve') {
        await approveAdjustment(id, decisionNote.trim(), selectedBusinessId, csrfToken);
      } else {
        await rejectAdjustment(id, decisionNote.trim(), selectedBusinessId, csrfToken);
      }
      setAdjustments((prev) => (prev ?? []).filter((a) => a.id !== id));
      setDecidingId(null);
      setDecidingAction(null);
      // An approval can drive a balance negative and open a new inventory
      // review -- refresh that list too rather than leaving it stale.
      if (decidingAction === 'approve' && selectedBusinessId) {
        listInventoryReviews(selectedBusinessId)
          .then(setInventoryReviews)
          .catch(() => {
            /* non-fatal -- the reviews list just stays as it was */
          });
      }
    } catch (err) {
      setDecisionError(describeActionError(err, 'Could not save this decision.'));
    } finally {
      setDecisionBusy(false);
    }
  }

  function startResolvingInventory(id: string) {
    setResolvingInventoryId(id);
    setInventoryNote('');
    setResolvedUnitCostNaira('');
    setInventoryActionError(null);
  }

  async function handleResolveInventory(id: string) {
    if (!selectedBusinessId || !csrfToken || !inventoryNote.trim()) return;
    setInventoryBusy(true);
    setInventoryActionError(null);
    try {
      const resolvedUnitCostKobo = resolvedUnitCostNaira.trim()
        ? Math.round(Number(resolvedUnitCostNaira) * 100)
        : undefined;
      await resolveInventoryReview(
        id,
        inventoryNote.trim(),
        selectedBusinessId,
        csrfToken,
        resolvedUnitCostKobo,
      );
      setInventoryReviews((prev) => (prev ?? []).filter((r) => r.id !== id));
      setResolvingInventoryId(null);
    } catch (err) {
      setInventoryActionError(describeActionError(err, 'Could not resolve this review.'));
    } finally {
      setInventoryBusy(false);
    }
  }

  function startResolving(id: string) {
    setResolvingId(id);
    setNote('');
    setActionError(null);
  }

  async function handleResolve(id: string) {
    if (!selectedBusinessId || !csrfToken || !note.trim()) return;
    setBusy(true);
    setActionError(null);
    try {
      await resolveSaleReview(id, note.trim(), selectedBusinessId, csrfToken);
      setReviews((prev) => (prev ?? []).filter((r) => r.id !== id));
      setResolvingId(null);
    } catch (err) {
      setActionError(describeActionError(err, 'Could not resolve this review.'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink pb-16">
      <div className="max-w-md mx-auto px-5 pt-6">
        <div className="flex items-center gap-4 mb-6">
          <Link
            to="/dashboard"
            aria-label="Back"
            className="text-jb-ink/40 hover:text-jb-ink transition-colors text-lg leading-none"
          >
            ←
          </Link>
          <div>
            <div className="text-[11px] text-jb-ink/45">Dashboard</div>
            <h1 className="text-lg font-medium">Reviews</h1>
          </div>
        </div>

        <div className="text-[11px] font-medium text-jb-ink/40 uppercase tracking-wide mb-2">
          Pending adjustments
        </div>
        {adjustmentsError && <p className="text-[13px] text-red-700 mb-3">{adjustmentsError}</p>}
        <div className="rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden mb-6">
          {adjustments === null && (
            <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">Loading…</p>
          )}
          {adjustments !== null && adjustments.length === 0 && (
            <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">Nothing waiting on you.</p>
          )}
          {(adjustments ?? []).map((a) => (
            <div key={a.id} className="px-4 py-3.5">
              <div className="min-w-0">
                <div className="text-[13px] text-jb-ink/80">
                  {a.quantityDelta > 0 ? '+' : ''}
                  {a.quantityDelta} ·{' '}
                  {a.variantName ? `${a.productName} — ${a.variantName}` : 'Item'}
                </div>
                <div className="text-[11.5px] text-jb-ink/50 mt-0.5">
                  {ADJUSTMENT_REASON_LABELS[a.reasonCategory]} — {a.reasonNote}
                </div>
                <div className="text-[11px] text-jb-ink/40 mt-1">
                  {a.requestedByName && (
                    <span className="font-semibold text-jb-ink/55">{a.requestedByName}</span>
                  )}{' '}
                  {formatWhen(a.createdAt)}
                </div>
              </div>

              {decidingId !== a.id && (
                <div className="flex gap-3 mt-2">
                  <button
                    onClick={() => startDeciding(a.id, 'approve')}
                    className="text-[12.5px] text-jb-ink/60 hover:text-jb-ink transition-colors"
                  >
                    Approve
                  </button>
                  <button
                    onClick={() => startDeciding(a.id, 'reject')}
                    className="text-[12.5px] text-jb-ink/60 hover:text-jb-ink transition-colors"
                  >
                    Reject
                  </button>
                </div>
              )}

              {decidingId === a.id && (
                <div className="mt-3">
                  <textarea
                    value={decisionNote}
                    onChange={(e) => setDecisionNote(e.target.value)}
                    placeholder={
                      decidingAction === 'approve'
                        ? 'Why is this approved? e.g. confirmed with staff.'
                        : 'Why is this rejected?'
                    }
                    rows={2}
                    className="w-full rounded-lg border border-jb-ink/15 bg-white px-3 py-2 text-[13px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none focus:border-jb-ink/40 transition-colors mb-2"
                  />
                  {decisionError && (
                    <p className="text-[12.5px] text-red-700 mb-2">{decisionError}</p>
                  )}
                  <div className="flex gap-2">
                    <button
                      onClick={() => void handleDecision(a.id)}
                      disabled={decisionBusy || !decisionNote.trim()}
                      className="rounded-lg bg-jb-ink text-jb-cream text-[12.5px] font-medium px-4 py-2 hover:bg-jb-green transition-colors disabled:opacity-40"
                    >
                      {decisionBusy
                        ? 'Saving…'
                        : decidingAction === 'approve'
                          ? 'Confirm approval'
                          : 'Confirm rejection'}
                    </button>
                    <button
                      onClick={() => {
                        setDecidingId(null);
                        setDecidingAction(null);
                      }}
                      disabled={decisionBusy}
                      className="text-[12.5px] text-jb-ink/45 hover:text-jb-ink transition-colors disabled:opacity-40"
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>

        <div className="text-[11px] font-medium text-jb-ink/40 uppercase tracking-wide mb-2">
          Inventory reviews
        </div>
        {inventoryReviewsError && (
          <p className="text-[13px] text-red-700 mb-3">{inventoryReviewsError}</p>
        )}
        <div className="rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden mb-6">
          {inventoryReviews === null && (
            <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">Loading…</p>
          )}
          {inventoryReviews !== null && inventoryReviews.length === 0 && (
            <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">
              Nothing flagged — no negative balances or stale counts.
            </p>
          )}
          {(inventoryReviews ?? []).map((review) => (
            <div key={review.id} className="px-4 py-3.5">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="text-[13px] text-jb-ink/80">
                    {review.variantName ? `${review.productName} — ${review.variantName}` : 'Item'}
                  </div>
                  <div className="text-[11.5px] text-jb-ink/50 mt-0.5">
                    {inventoryReviewLabel(review)}
                    {review.type === 'stale_stock_count' &&
                      ` — expected ${review.countExpectedQuantity}, counted ${review.countPhysicalQuantity}`}
                  </div>
                  <div className="text-[11px] text-jb-ink/40 mt-1">
                    {formatWhen(review.createdAt)}
                  </div>
                </div>
                {resolvingInventoryId !== review.id && (
                  <button
                    onClick={() => startResolvingInventory(review.id)}
                    className="shrink-0 text-[12.5px] text-jb-ink/60 hover:text-jb-ink transition-colors"
                  >
                    Resolve
                  </button>
                )}
              </div>

              {resolvingInventoryId === review.id && (
                <div className="mt-3">
                  <textarea
                    value={inventoryNote}
                    onChange={(e) => setInventoryNote(e.target.value)}
                    placeholder="What did you check? e.g. re-counted, submitted a corrected count."
                    rows={2}
                    className="w-full rounded-lg border border-jb-ink/15 bg-white px-3 py-2 text-[13px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none focus:border-jb-ink/40 transition-colors mb-2"
                  />
                  {review.type === 'negative_inventory' && (
                    <label className="block mb-2">
                      <span className="block text-[11.5px] text-jb-ink/50 mb-1">
                        If this was a sale that oversold before the stock was logged, enter what it
                        actually cost per unit to record the real profit (optional).
                      </span>
                      <div className="flex items-center gap-1">
                        <span className="text-jb-ink/40 text-[13px]">₦</span>
                        <input
                          type="number"
                          inputMode="decimal"
                          placeholder="Cost per unit"
                          value={resolvedUnitCostNaira}
                          onChange={(e) => setResolvedUnitCostNaira(e.target.value)}
                          className="flex-1 rounded-lg border border-jb-ink/15 bg-white px-3 py-2 text-[13px] focus:outline-none focus:border-jb-ink/40"
                        />
                      </div>
                    </label>
                  )}
                  {inventoryActionError && (
                    <p className="text-[12.5px] text-red-700 mb-2">{inventoryActionError}</p>
                  )}
                  <div className="flex gap-2">
                    <button
                      onClick={() => void handleResolveInventory(review.id)}
                      disabled={inventoryBusy || !inventoryNote.trim()}
                      className="rounded-lg bg-jb-ink text-jb-cream text-[12.5px] font-medium px-4 py-2 hover:bg-jb-green transition-colors disabled:opacity-40"
                    >
                      {inventoryBusy ? 'Resolving…' : 'Mark resolved'}
                    </button>
                    <button
                      onClick={() => setResolvingInventoryId(null)}
                      disabled={inventoryBusy}
                      className="text-[12.5px] text-jb-ink/45 hover:text-jb-ink transition-colors disabled:opacity-40"
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>

        <div className="text-[11px] font-medium text-jb-ink/40 uppercase tracking-wide mb-2">
          Sale reviews
        </div>
        {loadError && <p className="text-[13px] text-red-700 mb-3">{loadError}</p>}

        <div className="rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden">
          {reviews === null && <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">Loading…</p>}
          {reviews !== null && reviews.length === 0 && (
            <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">
              Nothing flagged — every sale matched a current price and an active product.
            </p>
          )}
          {(reviews ?? []).map((review) => (
            <div key={review.id} className="px-4 py-3.5">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="text-[13px] text-jb-ink/80">
                    {review.itemQuantity}× {review.itemDescription || 'Item'}
                  </div>
                  <div className="text-[11.5px] text-jb-ink/50 mt-0.5">
                    {reasonLabel(review.reason)}
                  </div>
                  <div className="text-[11px] text-jb-ink/40 mt-1">
                    {review.sellerName && (
                      <span className="font-semibold text-jb-ink/55">{review.sellerName}</span>
                    )}{' '}
                    {formatWhen(review.saleOccurredAt)} · charged ₦
                    {formatNaira(review.itemLineTotalKobo)}
                  </div>
                </div>
                {resolvingId !== review.id && (
                  <button
                    onClick={() => startResolving(review.id)}
                    className="shrink-0 text-[12.5px] text-jb-ink/60 hover:text-jb-ink transition-colors"
                  >
                    Resolve
                  </button>
                )}
              </div>

              {resolvingId === review.id && (
                <div className="mt-3">
                  <textarea
                    value={note}
                    onChange={(e) => setNote(e.target.value)}
                    placeholder="What did you check? e.g. confirmed with staff, price was correct at the time."
                    rows={2}
                    className="w-full rounded-lg border border-jb-ink/15 bg-white px-3 py-2 text-[13px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none focus:border-jb-ink/40 transition-colors mb-2"
                  />
                  {actionError && <p className="text-[12.5px] text-red-700 mb-2">{actionError}</p>}
                  <div className="flex gap-2">
                    <button
                      onClick={() => void handleResolve(review.id)}
                      disabled={busy || !note.trim()}
                      className="rounded-lg bg-jb-ink text-jb-cream text-[12.5px] font-medium px-4 py-2 hover:bg-jb-green transition-colors disabled:opacity-40"
                    >
                      {busy ? 'Resolving…' : 'Mark resolved'}
                    </button>
                    <button
                      onClick={() => setResolvingId(null)}
                      disabled={busy}
                      className="text-[12.5px] text-jb-ink/45 hover:text-jb-ink transition-colors disabled:opacity-40"
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
