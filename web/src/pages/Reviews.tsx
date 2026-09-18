import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import { listSaleReviews, resolveSaleReview, type SaleReview } from '../api/reviews';
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

// Owner-facing queue for sale_reviews (docs/PHASE_OFFLINE_CATALOGUE_AND_
// REVIEWS.md) -- a sale is never rejected for a stale price or a
// deactivated variant (internal/sales posts it exactly as submitted), it
// just opens a review here instead. Resolving a review is purely
// acknowledgment: it never changes the underlying sale.
export function Reviews() {
  const { csrfToken, selectedBusinessId } = useSession();

  const [reviews, setReviews] = useState<SaleReview[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [resolvingId, setResolvingId] = useState<string | null>(null);
  const [note, setNote] = useState('');
  const [actionError, setActionError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!selectedBusinessId) return;
    listSaleReviews(selectedBusinessId)
      .then(setReviews)
      .catch((err: unknown) => setLoadError(describeActionError(err, 'Could not load reviews.')));
  }, [selectedBusinessId]);

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
