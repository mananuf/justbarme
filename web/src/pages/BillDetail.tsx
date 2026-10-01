import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';

import { listProducts, type CatalogueProduct } from '../api/catalogue';
import {
  addSaleRound,
  closeBill,
  createOrRotateShareLink,
  getBillDetail,
  listCustomers,
  listTables,
  recordPayment,
  removeBillItem,
  voidBill,
  writeOffBill,
  type BillDetail as BillDetailData,
  type BillRound,
  type Customer,
  type Table,
} from '../api/tabs';
import { ActivityDetailSheet } from '../components/ActivityDetailSheet';
import { AppBottomNav } from '../components/AppBottomNav';
import { CartPanel, type CartLine } from '../components/CartPanel';
import { ProductGrid, type PickableVariant } from '../components/ProductGrid';
import { useConnectivity } from '../hooks/useConnectivity';
import { billLabel, formatNaira, statusLabel } from '../lib/billDisplay';
import { describeActionError } from '../lib/errors';
import { useSession } from '../lib/session';
import {
  hasSeenPaymentHint,
  hasSeenVoidHint,
  markPaymentHintSeen,
  markVoidHintSeen,
} from '../lib/tabsHints';

type PaymentMethod = 'cash' | 'transfer' | 'card';

// BillDetail is the receipt-style, full-page view for one bill -- a real
// route rather than a modal (docs/PHASE_UNIFIED_SELL_BILLS.md), so it gets
// normal back-button/deep-link behavior and room for the breakdown below.
// Every sale lands here once recorded, whether it came from the unified
// Sell flow's one-shot walk-in path or its table/customer tab path -- "a
// sale is always a sale, one view" extends to what you see right after
// recording one, not just how you record it.
export function BillDetail() {
  const { billId } = useParams<{ billId: string }>();
  const navigate = useNavigate();
  const { csrfToken, selectedBusinessId, memberships } = useSession();
  const isOnline = useConnectivity();
  const isOwner = memberships.find((m) => m.businessId === selectedBusinessId)?.role === 'owner';

  const [detail, setDetail] = useState<BillDetailData | null>(null);
  const [tables, setTables] = useState<Table[]>([]);
  const [customers, setCustomers] = useState<Customer[]>([]);
  const [products, setProducts] = useState<CatalogueProduct[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [paymentAmount, setPaymentAmount] = useState('');
  const [paymentMethod, setPaymentMethod] = useState<PaymentMethod>('cash');
  const [showWriteOff, setShowWriteOff] = useState(false);
  const [writeOffAmount, setWriteOffAmount] = useState('');
  const [writeOffReason, setWriteOffReason] = useState('');
  const [sharing, setSharing] = useState(false);
  const [shareLinkUrl, setShareLinkUrl] = useState<string | null>(null);
  const [paymentHintDismissed, setPaymentHintDismissed] = useState(() => hasSeenPaymentHint());
  const [voidHintSeen, setVoidHintSeen] = useState(() => hasSeenVoidHint());
  const [openRound, setOpenRound] = useState<BillRound | null>(null);

  const refreshDetail = async () => {
    if (!selectedBusinessId || !billId) return;
    try {
      const d = await getBillDetail(billId, selectedBusinessId);
      setDetail(d);
    } catch (err) {
      setLoadError(describeActionError(err, 'Could not load this bill.'));
    }
  };

  const refreshProducts = () => {
    if (!selectedBusinessId) return;
    listProducts(selectedBusinessId)
      .then(setProducts)
      .catch(() => undefined);
  };

  useEffect(() => {
    if (!selectedBusinessId || !billId) return;
    // Wrapped so the lint rule sees this as "a callback that happens to
    // call setState," not a direct setState statement in the effect body
    // itself -- see Tabs.tsx's own listBills effect for the same pattern.
    void (async () => {
      await refreshDetail();
    })();
    listTables(selectedBusinessId)
      .then(setTables)
      .catch(() => undefined);
    listCustomers(selectedBusinessId)
      .then(setCustomers)
      .catch(() => undefined);
    refreshProducts();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedBusinessId, billId]);

  const canEditItems = detail?.status === 'open' || detail?.status === 'closed_unpaid';

  const variants: PickableVariant[] = useMemo(
    () =>
      (products ?? []).flatMap((p) =>
        p.variants
          .filter((v) => v.active)
          .map((v) => ({
            variantId: v.id,
            description: `${p.name} — ${v.name}`,
            priceKobo: v.currentPriceKobo,
            outOfStock: v.tracksInventory && v.currentStock <= 0,
          })),
      ),
    [products],
  );

  // Net quantity per variant across every round posted so far -- a
  // removal round's negative quantity nets against the rounds that added
  // it, so this is always "what's actually on the bill right now."
  const currentOrderLines: CartLine[] = useMemo(() => {
    if (!detail) return [];
    const byVariant = new Map<string, CartLine>();
    for (const round of detail.sales) {
      for (const item of round.items) {
        const existing = byVariant.get(item.variantId);
        byVariant.set(item.variantId, {
          variantId: item.variantId,
          description: item.description,
          unitPriceKobo: item.unitPriceKobo,
          quantity: (existing?.quantity ?? 0) + item.quantity,
        });
      }
    }
    return Array.from(byVariant.values()).filter((l) => l.quantity > 0);
  }, [detail]);
  const currentOrderQuantities = Object.fromEntries(
    currentOrderLines.map((l) => [l.variantId, l.quantity]),
  );

  async function handleAdjustQuantity(
    variant: { variantId: string; description: string; priceKobo: number },
    delta: number,
  ) {
    if (!selectedBusinessId || !csrfToken || !billId || busy || delta === 0) return;
    if (delta > 0 && variants.find((v) => v.variantId === variant.variantId)?.outOfStock) return;
    setBusy(true);
    setActionError(null);
    try {
      if (delta > 0) {
        await addSaleRound(
          billId,
          {
            idempotencyKey: crypto.randomUUID(),
            occurredAt: new Date().toISOString(),
            items: [
              { variantId: variant.variantId, quantity: delta, unitPriceKobo: variant.priceKobo },
            ],
          },
          selectedBusinessId,
          csrfToken,
        );
      } else {
        await removeBillItem(
          billId,
          {
            idempotencyKey: crypto.randomUUID(),
            occurredAt: new Date().toISOString(),
            variantId: variant.variantId,
            quantity: -delta,
            unitPriceKobo: variant.priceKobo,
          },
          selectedBusinessId,
          csrfToken,
        );
      }
      await refreshDetail();
      refreshProducts();
    } catch (err) {
      setActionError(describeActionError(err, 'Could not update this item.'));
    } finally {
      setBusy(false);
    }
  }

  async function handleRecordPayment() {
    if (!selectedBusinessId || !csrfToken || !billId) return;
    const amountKobo = Math.round(parseFloat(paymentAmount) * 100);
    if (!amountKobo || amountKobo <= 0) return;
    setBusy(true);
    setActionError(null);
    try {
      await recordPayment(billId, amountKobo, paymentMethod, selectedBusinessId, csrfToken);
      setPaymentAmount('');
      await refreshDetail();
    } catch (err) {
      setActionError(describeActionError(err, 'Could not record this payment.'));
    } finally {
      setBusy(false);
    }
  }

  async function handleCloseBill() {
    if (!selectedBusinessId || !csrfToken || !billId) return;
    setBusy(true);
    setActionError(null);
    try {
      await closeBill(billId, selectedBusinessId, csrfToken);
      await refreshDetail();
    } catch (err) {
      setActionError(describeActionError(err, 'Could not close this tab.'));
    } finally {
      setBusy(false);
    }
  }

  async function handleVoidBill() {
    if (!selectedBusinessId || !csrfToken || !billId) return;
    setBusy(true);
    setActionError(null);
    try {
      await voidBill(billId, selectedBusinessId, csrfToken);
      await refreshDetail();
    } catch (err) {
      setActionError(describeActionError(err, 'Could not void this tab.'));
    } finally {
      setBusy(false);
    }
  }

  async function handleShare() {
    if (!selectedBusinessId || !csrfToken || !billId) return;
    setSharing(true);
    setActionError(null);
    try {
      const { url } = await createOrRotateShareLink(billId, selectedBusinessId, csrfToken);
      if (navigator.share) {
        try {
          await navigator.share({ title: 'Your bill', url });
          return;
        } catch {
          // User cancelled the share sheet, or the platform rejected it --
          // fall through to the copy-link fallback rather than treating
          // this as an error.
        }
      }
      if (navigator.clipboard) {
        try {
          await navigator.clipboard.writeText(url);
        } catch {
          // Clipboard access can be denied by the browser -- the URL is
          // still shown on-page below, so this is not fatal.
        }
      }
      setShareLinkUrl(url);
    } catch (err) {
      setActionError(describeActionError(err, 'Could not create a share link.'));
    } finally {
      setSharing(false);
    }
  }

  async function handleWriteOff() {
    if (!selectedBusinessId || !csrfToken || !billId) return;
    const amountKobo = Math.round(parseFloat(writeOffAmount) * 100);
    if (!amountKobo || amountKobo <= 0 || !writeOffReason.trim()) return;
    setBusy(true);
    setActionError(null);
    try {
      await writeOffBill(billId, amountKobo, writeOffReason.trim(), selectedBusinessId, csrfToken);
      setWriteOffAmount('');
      setWriteOffReason('');
      setShowWriteOff(false);
      await refreshDetail();
    } catch (err) {
      setActionError(describeActionError(err, 'Could not write off this balance.'));
    } finally {
      setBusy(false);
    }
  }

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
            <h1 className="text-lg font-medium">
              {detail ? billLabel(detail, tables, customers) : 'Bill'}
            </h1>
          </div>
        </div>

        {!isOnline && (
          <p className="text-[12.5px] text-jb-gold bg-jb-gold/10 rounded-xl px-4 py-2 mb-3">
            You&apos;re offline. Tab actions need a connection — they won&apos;t go through until
            you&apos;re back online.
          </p>
        )}
        {loadError && <p className="text-[13px] text-red-700 mb-3">{loadError}</p>}
        {actionError && <p className="text-[13px] text-red-700 mb-3">{actionError}</p>}

        {!detail ? (
          !loadError && <p className="text-[13px] text-jb-ink/45">Loading…</p>
        ) : (
          <>
            <div className="rounded-xl bg-jb-ink text-jb-cream p-4 mb-4">
              <div className="flex items-center justify-between mb-1">
                <span className="text-[12px] text-jb-cream/60">{statusLabel(detail.status)}</span>
                <span className="text-[15px] font-medium">
                  ₦{formatNaira(detail.balanceKobo)} due
                </span>
              </div>
              <div className="text-[11px] text-jb-cream/45">
                Opened {new Date(detail.openedAt).toLocaleString()}
              </div>
            </div>

            {canEditItems && (
              <div className="mb-4">
                <div className="text-[11px] text-jb-ink/45 tracking-wide mb-2">ADD OR ADJUST</div>
                <ProductGrid
                  variants={variants}
                  isLoading={products === null}
                  cartQuantities={currentOrderQuantities}
                  inCartLabel="on this bill"
                  onAdd={(v) => void handleAdjustQuantity(v, 1)}
                  scrollMaxHeightClassName="max-h-[38vh]"
                  emptyMessage={
                    <p className="text-[13px] text-jb-ink/45">
                      You haven&apos;t stocked anything yet.{' '}
                      <Link to="/dashboard/stock" className="underline">
                        Add stock
                      </Link>
                      .
                    </p>
                  }
                />
              </div>
            )}
          </>
        )}
      </div>

      {/* Outside the centered column so CartPanel's own bottom-sheet
          styling matches the rest of the app. Each +/- tap posts
          immediately -- there's no separate "submit this round" step. */}
      {detail && canEditItems && (
        <CartPanel
          label="CURRENT ORDER"
          lines={currentOrderLines.map((l) => ({
            ...l,
            atStockLimit: variants.find((v) => v.variantId === l.variantId)?.outOfStock,
          }))}
          emptyMessage="Tap a drink above to add it here."
          onChangeQty={(variantId, delta) => {
            const line = currentOrderLines.find((l) => l.variantId === variantId);
            const live = variants.find((v) => v.variantId === variantId);
            void handleAdjustQuantity(
              {
                variantId,
                description: live?.description ?? line?.description ?? '',
                priceKobo: live?.priceKobo ?? line?.unitPriceKobo ?? 0,
              },
              delta,
            );
          }}
        />
      )}

      {detail && (
        <div className="max-w-md mx-auto px-5">
          {detail.payments.length > 0 && (
            <div className="mb-4 mt-4">
              <div className="text-[11px] text-jb-ink/45 tracking-wide mb-2">PAYMENTS</div>
              <div className="space-y-1.5">
                {detail.payments.map((p, i) => (
                  <div
                    key={i}
                    className="flex items-center justify-between text-[13px] text-jb-ink/70"
                  >
                    <span className="capitalize">{p.method}</span>
                    <span className={p.amountKobo < 0 ? 'text-red-700' : ''}>
                      {p.amountKobo < 0 ? '−' : ''}₦{formatNaira(Math.abs(p.amountKobo))}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {(detail.status === 'open' || detail.status === 'closed_unpaid') &&
            detail.balanceKobo > 0 && (
              <div className="mb-4">
                <div className="text-[11px] text-jb-ink/45 tracking-wide mb-2">
                  RECORD A PAYMENT
                </div>
                <div className="flex gap-2 p-1 rounded-xl bg-jb-ink/[0.05] mb-2">
                  {(['cash', 'transfer', 'card'] as PaymentMethod[]).map((m) => (
                    <button
                      key={m}
                      onClick={() => setPaymentMethod(m)}
                      className={`flex-1 rounded-lg py-2 text-[12.5px] font-medium capitalize transition-colors ${
                        paymentMethod === m ? 'bg-white text-jb-ink shadow-sm' : 'text-jb-ink/45'
                      }`}
                    >
                      {m}
                    </button>
                  ))}
                </div>
                <div className="flex gap-2">
                  <input
                    type="number"
                    inputMode="decimal"
                    placeholder={`Up to ₦${formatNaira(detail.balanceKobo)}`}
                    value={paymentAmount}
                    onChange={(e) => setPaymentAmount(e.target.value)}
                    className="flex-1 rounded-xl border border-jb-ink/15 bg-white px-4 py-3 text-[14px] focus:outline-none focus:border-jb-ink/40"
                  />
                  <button
                    onClick={() => void handleRecordPayment()}
                    disabled={busy || !paymentAmount}
                    className="rounded-xl bg-jb-ink text-jb-cream text-[14px] font-medium px-5 disabled:opacity-40"
                  >
                    Paid
                  </button>
                </div>
                {!paymentHintDismissed &&
                  detail.status === 'open' &&
                  detail.payments.length === 0 && (
                    <div className="mt-2 flex items-start justify-between gap-2 rounded-lg bg-jb-ink/[0.04] px-3 py-2">
                      <p className="text-[12px] text-jb-ink/60">
                        Enter what was paid and tap Paid — that&apos;s what marks this bill Paid
                        instead of still owing, and is what makes Close tab appear below.
                      </p>
                      <button
                        onClick={() => {
                          markPaymentHintSeen();
                          setPaymentHintDismissed(true);
                        }}
                        className="shrink-0 text-[11px] text-jb-ink/40 underline underline-offset-2"
                      >
                        Dismiss
                      </button>
                    </div>
                  )}
              </div>
            )}

          {shareLinkUrl && (
            <div className="mb-4 rounded-xl border border-jb-ink/10 bg-white p-3 text-[13px] space-y-1.5">
              <div className="text-jb-ink/60">Link copied — share it with the customer:</div>
              <div className="break-all text-jb-ink font-medium">{shareLinkUrl}</div>
              <button
                onClick={() => setShareLinkUrl(null)}
                className="text-jb-ink/45 text-[12px] underline underline-offset-2"
              >
                Dismiss
              </button>
            </div>
          )}

          <div className="flex flex-col gap-2 mb-6">
            <button
              onClick={() => void handleShare()}
              disabled={sharing}
              className="w-full rounded-xl border border-jb-ink/15 text-jb-ink text-[14px] font-medium py-3 disabled:opacity-40"
            >
              {sharing ? 'Preparing link…' : 'Share bill'}
            </button>
            {detail.status === 'open' && detail.payments.length > 0 && (
              <button
                onClick={() => void handleCloseBill()}
                disabled={busy}
                className="w-full rounded-xl border border-jb-ink/15 text-jb-ink text-[14px] font-medium py-3 disabled:opacity-40"
              >
                Close tab
              </button>
            )}
            {isOwner && (detail.status === 'open' || detail.status === 'closed_unpaid') && (
              <button
                onClick={() => void handleVoidBill()}
                disabled={busy}
                className="w-full rounded-xl border border-jb-ink/15 text-jb-ink/60 text-[13px] py-2.5 disabled:opacity-40"
              >
                Void tab (only if nothing effective remains)
              </button>
            )}
            {!voidHintSeen && (detail.status === 'open' || detail.status === 'closed_unpaid') && (
              <div className="flex items-start justify-between gap-2 rounded-lg bg-jb-ink/[0.04] px-3 py-2">
                <p className="text-[12px] text-jb-ink/60">
                  Void tab is for a bill that should never have been opened — nothing was
                  effectively ordered or everything on it was removed. If drinks were served and
                  money is owed or paid, use Close tab instead.
                </p>
                <button
                  onClick={() => {
                    markVoidHintSeen();
                    setVoidHintSeen(true);
                  }}
                  className="shrink-0 text-[11px] text-jb-ink/40 underline underline-offset-2"
                >
                  Dismiss
                </button>
              </div>
            )}
            {isOwner && detail.status === 'closed_unpaid' && (
              <div className="rounded-xl border border-jb-ink/10 bg-white p-3">
                {!showWriteOff ? (
                  <button
                    onClick={() => setShowWriteOff(true)}
                    className="w-full text-[13px] text-jb-ink/60"
                  >
                    Write off debt (owner only)
                  </button>
                ) : (
                  <div className="space-y-2">
                    <input
                      type="number"
                      inputMode="decimal"
                      placeholder={`Amount, up to ₦${formatNaira(detail.balanceKobo)}`}
                      value={writeOffAmount}
                      onChange={(e) => setWriteOffAmount(e.target.value)}
                      className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-2.5 text-[13.5px] focus:outline-none focus:border-jb-ink/40"
                    />
                    <input
                      type="text"
                      placeholder="Reason (required)"
                      value={writeOffReason}
                      onChange={(e) => setWriteOffReason(e.target.value)}
                      className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-2.5 text-[13.5px] focus:outline-none focus:border-jb-ink/40"
                    />
                    <button
                      onClick={() => void handleWriteOff()}
                      disabled={busy || !writeOffAmount || !writeOffReason.trim()}
                      className="w-full rounded-xl bg-jb-ink text-jb-cream text-[13.5px] font-medium py-2.5 disabled:opacity-40"
                    >
                      Confirm write-off
                    </button>
                  </div>
                )}
              </div>
            )}
          </div>

          {detail.sales.length > 0 && (
            <div className="mb-6">
              <div className="text-[11px] text-jb-ink/45 tracking-wide mb-2">ROUNDS</div>
              <div className="space-y-2">
                {detail.sales.map((round) => (
                  <button
                    key={round.id}
                    onClick={() => setOpenRound(round)}
                    className="w-full text-left rounded-xl border border-jb-ink/10 bg-white p-3 hover:bg-jb-ink/[0.02] transition-colors"
                  >
                    <div className="flex items-center justify-between mb-1">
                      <span className="text-[11px] text-jb-ink/40">
                        {new Date(round.occurredAt).toLocaleTimeString()}
                        {round.sellerName && (
                          <span className="ml-1.5 font-semibold text-jb-ink/60">
                            {round.sellerName}
                          </span>
                        )}
                        {round.totalKobo < 0 && (
                          <span className="ml-2 text-[10px] uppercase tracking-wide text-jb-ink/35">
                            Correction
                          </span>
                        )}
                      </span>
                      <span
                        className={`text-[13px] font-medium ${round.totalKobo < 0 ? 'text-red-700' : ''}`}
                      >
                        {round.totalKobo < 0 ? '−' : ''}₦{formatNaira(Math.abs(round.totalKobo))}
                      </span>
                    </div>
                    {round.items.map((i) => (
                      <div key={i.variantId} className="text-[12.5px] text-jb-ink/70">
                        {i.quantity < 0 ? '−' : ''}
                        {Math.abs(i.quantity)}× {i.description}
                      </div>
                    ))}
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      <AppBottomNav />

      {openRound && selectedBusinessId && (
        <ActivityDetailSheet
          entry={{
            id: openRound.id,
            type: 'sale',
            actorId: '',
            actorName: openRound.sellerName,
            occurredAt: openRound.occurredAt,
            summary: openRound.totalKobo < 0 ? 'Correction' : 'Round',
            amountKobo: openRound.totalKobo,
          }}
          isOwner={isOwner}
          businessId={selectedBusinessId}
          csrfToken={csrfToken}
          onClose={() => setOpenRound(null)}
          onChanged={() => void refreshDetail()}
          forceNotFixable={
            canEditItems
              ? 'This is part of an open tab — use the +/- controls above to adjust it instead.'
              : "This tab is already settled. Correcting a round on a settled tab isn't supported yet — contact support."
          }
        />
      )}
    </div>
  );
}
