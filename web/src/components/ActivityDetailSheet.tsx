import { useEffect, useState } from 'react';

import { getAdjustmentRequest, getStockReceipt, reverseStockReceipt } from '../api/inventory';
import { getExpense, reverseExpense } from '../api/expenses';
import { getSale, reverseSale } from '../api/sales';
import { flagActivity, type ActivityEntry } from '../api/activity';
import { describeActionError } from '../lib/errors';

function naira(kobo: number): string {
  return `₦${(Math.abs(kobo) / 100).toLocaleString()}`;
}

function formatWhen(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  });
}

interface Row {
  label: string;
  value: string;
  negative?: boolean;
}

// The breakdown body for one activity entry -- shape depends entirely on
// entry.type, since each source table has its own fields. Returns null
// while loading or on error (the caller shows those states around it).
function useDetailRows(
  entry: ActivityEntry,
  businessId: string,
): {
  rows: Row[] | null;
  error: string | null;
  // Whether an owner's "Fix" button should show at all for this entry, and
  // why not when it shouldn't (shown as a small note instead of the
  // button).
  fixable: { yes: true } | { yes: false; reason: string };
} {
  const [rows, setRows] = useState<Row[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [fixable, setFixable] = useState<{ yes: true } | { yes: false; reason: string }>({
    yes: false,
    reason: '',
  });

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      // Reset inside the async callback, not the effect body itself --
      // these still run before the fetch below, just not in the same
      // synchronous pass React's cascading-render lint rule guards
      // against (see Stock.tsx's loadCatalogue for the same reasoning).
      setRows(null);
      setError(null);
      try {
        if (entry.type === 'sale') {
          const sale = await getSale(entry.id, businessId);
          if (cancelled) return;
          setRows([
            ...sale.items.map((i) => ({
              label: `${i.description} ×${i.quantity}`,
              value: naira(i.lineTotalKobo),
              negative: i.lineTotalKobo < 0,
            })),
            {
              label: 'Payment',
              value: `${sale.payment.method} · ${naira(sale.payment.amountKobo)}`,
            },
            { label: 'Total', value: naira(sale.totalKobo), negative: sale.totalKobo < 0 },
          ]);
          setFixable(
            sale.reversalOf
              ? { yes: false, reason: 'This is itself a correction of another sale.' }
              : { yes: true },
          );
        } else if (entry.type === 'expense') {
          const exp = await getExpense(entry.id, businessId);
          if (cancelled) return;
          setRows([
            { label: 'Category', value: exp.categoryName },
            { label: 'Description', value: exp.description },
            { label: 'Payment', value: exp.paymentMethod },
            { label: 'Amount', value: naira(exp.amountKobo), negative: exp.amountKobo < 0 },
          ]);
          setFixable(
            exp.reversalOf
              ? { yes: false, reason: 'This is itself a correction of another expense.' }
              : { yes: true },
          );
        } else if (entry.type === 'inventory_adjustment') {
          const adj = await getAdjustmentRequest(entry.id, businessId);
          if (cancelled) return;
          setRows([
            { label: 'Product', value: `${adj.productName} — ${adj.variantName}` },
            {
              label: 'Quantity',
              value: adj.quantityDelta > 0 ? `+${adj.quantityDelta}` : String(adj.quantityDelta),
            },
            { label: 'Reason', value: adj.reasonCategory },
            { label: 'Note', value: adj.reasonNote },
          ]);
          setFixable({
            yes: false,
            reason: 'To correct this, use Stock → Report issue to post a new adjustment.',
          });
        } else {
          const receipt = await getStockReceipt(entry.id, businessId);
          if (cancelled) return;
          setRows(
            receipt.lines.map((l) => ({
              label: `${l.productName} — ${l.variantName} ×${l.quantity}`,
              value: naira(l.totalCostKobo),
              negative: l.totalCostKobo < 0,
            })),
          );
          if (receipt.reversalOf) {
            setFixable({ yes: false, reason: 'This is itself a correction of another receipt.' });
          } else if (receipt.reversalOfThis) {
            setFixable({ yes: false, reason: 'This receipt has already been reversed.' });
          } else {
            setFixable({ yes: true });
          }
        }
      } catch (err) {
        if (!cancelled) setError(describeActionError(err, 'Could not load this entry.'));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [entry.id, entry.type, businessId]);

  return { rows, error, fixable };
}

export function ActivityDetailSheet({
  entry,
  isOwner,
  businessId,
  csrfToken,
  onClose,
  onChanged,
}: {
  entry: ActivityEntry;
  isOwner: boolean;
  businessId: string;
  csrfToken: string | null;
  onClose: () => void;
  onChanged: () => void;
}) {
  const { rows, error, fixable } = useDetailRows(entry, businessId);

  const [fixing, setFixing] = useState(false);
  const [fixError, setFixError] = useState<string | null>(null);
  const [flagging, setFlagging] = useState(false);
  const [flagReason, setFlagReason] = useState('');
  const [flagSubmitting, setFlagSubmitting] = useState(false);
  const [flagError, setFlagError] = useState<string | null>(null);
  const [flagDone, setFlagDone] = useState(false);

  async function handleFix() {
    if (!csrfToken) return;
    setFixing(true);
    setFixError(null);
    try {
      if (entry.type === 'sale') await reverseSale(entry.id, businessId, csrfToken);
      else if (entry.type === 'expense') await reverseExpense(entry.id, businessId, csrfToken);
      else if (entry.type === 'stock_receipt')
        await reverseStockReceipt(entry.id, businessId, csrfToken);
      onChanged();
      onClose();
    } catch (err) {
      setFixError(describeActionError(err, 'Could not fix this.'));
    } finally {
      setFixing(false);
    }
  }

  async function handleFlag() {
    if (!csrfToken || !flagReason.trim()) return;
    setFlagSubmitting(true);
    setFlagError(null);
    try {
      await flagActivity(entry.type, entry.id, flagReason.trim(), businessId, csrfToken);
      setFlagDone(true);
    } catch (err) {
      setFlagError(describeActionError(err, 'Could not send this to the owner.'));
    } finally {
      setFlagSubmitting(false);
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-end justify-center"
      role="dialog"
      aria-modal="true"
      aria-label="Activity detail"
    >
      <button
        aria-label="Close"
        onClick={onClose}
        className="absolute inset-0 bg-[rgba(18,26,20,0.55)]"
      />
      <div className="relative w-full max-w-md bg-jb-cream rounded-t-2xl px-5 pt-5 pb-[calc(env(safe-area-inset-bottom)+1.25rem)] max-h-[80vh] overflow-y-auto">
        <div className="flex items-start justify-between mb-1">
          <h2 className="text-[15px] font-medium text-jb-ink">{entry.summary}</h2>
          <button
            onClick={onClose}
            className="text-jb-ink/40 hover:text-jb-ink text-lg leading-none"
          >
            ×
          </button>
        </div>
        <p className="text-[12px] text-jb-ink/45 mb-4">
          {entry.actorName && <span className="font-medium text-jb-ink/60">{entry.actorName}</span>}{' '}
          {formatWhen(entry.occurredAt)}
        </p>

        {error && (
          <p role="alert" className="text-[13px] text-red-700 mb-3">
            {error}
          </p>
        )}
        {!error && rows === null && <p className="text-[13px] text-jb-ink/45 mb-3">Loading…</p>}
        {rows !== null && (
          <div className="rounded-xl border border-jb-ink/10 bg-white divide-y divide-jb-ink/[0.06] overflow-hidden mb-4">
            {rows.map((r, i) => (
              <div key={i} className="px-3.5 py-2.5 flex items-center justify-between gap-3">
                <span className="text-[13px] text-jb-ink/70 min-w-0 truncate">{r.label}</span>
                <span
                  className={`text-[13px] font-medium shrink-0 ${r.negative ? 'text-red-700/80' : 'text-jb-ink/85'}`}
                >
                  {r.value}
                </span>
              </div>
            ))}
          </div>
        )}

        {rows !== null && isOwner && (
          <div className="mb-3">
            {fixable.yes ? (
              <button
                onClick={() => void handleFix()}
                disabled={fixing}
                className="w-full rounded-xl border border-jb-ink/15 text-jb-ink text-[13.5px] font-medium py-2.5 disabled:opacity-40"
              >
                {fixing ? 'Working…' : 'Fix — undo this and redo it correctly'}
              </button>
            ) : (
              <p className="text-[12px] text-jb-ink/40">{fixable.reason}</p>
            )}
            {fixError && (
              <p role="alert" className="text-[12px] text-red-700 mt-1.5">
                {fixError}
              </p>
            )}
          </div>
        )}

        {rows !== null && !isOwner && (
          <div className="rounded-xl border border-jb-ink/10 bg-white p-3">
            {flagDone ? (
              <p className="text-[13px] text-jb-green-dark">Sent to the owner for review.</p>
            ) : !flagging ? (
              <button
                onClick={() => setFlagging(true)}
                className="w-full text-[13px] text-jb-ink/60"
              >
                Something&apos;s wrong — ask the owner to check
              </button>
            ) : (
              <div className="space-y-2">
                <textarea
                  autoFocus
                  value={flagReason}
                  onChange={(e) => setFlagReason(e.target.value)}
                  placeholder="What looks wrong?"
                  rows={2}
                  className="w-full rounded-lg border border-jb-ink/15 bg-white px-3 py-2 text-[13px] focus:outline-none focus:border-jb-ink/40"
                />
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => void handleFlag()}
                    disabled={flagSubmitting || !flagReason.trim()}
                    className="text-[13px] px-4 py-2 rounded-lg bg-jb-ink text-jb-cream disabled:opacity-40"
                  >
                    {flagSubmitting ? 'Sending…' : 'Send to owner'}
                  </button>
                  <button
                    onClick={() => setFlagging(false)}
                    className="text-[13px] px-3 py-2 text-jb-ink/50 hover:text-jb-ink"
                  >
                    Cancel
                  </button>
                </div>
                {flagError && (
                  <p role="alert" className="text-[12px] text-red-700">
                    {flagError}
                  </p>
                )}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
