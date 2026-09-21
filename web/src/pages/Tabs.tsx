import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';

import { listProducts, type CatalogueProduct } from '../api/catalogue';
import {
  addSaleRound,
  closeBill,
  createCustomer,
  createOrRotateShareLink,
  createTable,
  getBillDetail,
  listBills,
  listCustomers,
  listTables,
  openBill,
  recordPayment,
  removeBillItem,
  voidBill,
  writeOffBill,
  type Bill,
  type BillDetail,
  type Customer,
  type Table,
} from '../api/tabs';
import { AppBottomNav } from '../components/AppBottomNav';
import { CartPanel, type CartLine } from '../components/CartPanel';
import { ProductGrid, type PickableVariant } from '../components/ProductGrid';
import { useConnectivity } from '../hooks/useConnectivity';
import { describeActionError } from '../lib/errors';
import { useSession } from '../lib/session';

type PaymentMethod = 'cash' | 'transfer' | 'card';

function formatNaira(kobo: number): string {
  return (kobo / 100).toLocaleString();
}

function billLabel(bill: Bill, tables: Table[], customers: Customer[]): string {
  if (bill.tableId) return tables.find((t) => t.id === bill.tableId)?.label ?? 'Table';
  if (bill.customerId)
    return customers.find((c) => c.id === bill.customerId)?.name ?? 'Customer tab';
  return 'Walk-in';
}

function statusLabel(status: Bill['status']): string {
  switch (status) {
    case 'open':
      return 'Open';
    case 'closed_unpaid':
      return 'Closed — balance due';
    case 'settled':
      return 'Settled';
    case 'void':
      return 'Void';
  }
}

export function Tabs() {
  const { csrfToken, selectedBusinessId, memberships } = useSession();
  const isOnline = useConnectivity();
  const isOwner = memberships.find((m) => m.businessId === selectedBusinessId)?.role === 'owner';

  const [bills, setBills] = useState<Bill[] | null>(null);
  const [tables, setTables] = useState<Table[]>([]);
  const [customers, setCustomers] = useState<Customer[]>([]);
  const [products, setProducts] = useState<CatalogueProduct[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  // null means "no explicit choice yet" (falls back to the first open
  // bill once loaded); 'new' means the user tapped "+ New" and explicitly
  // wants the new-tab chooser even though other tabs are open.
  // `activeBillId` below derives the actual bill to show, falling back to
  // another open bill if the raw selection drops out of the open list.
  const [selectedBillId, setSelectedBillId] = useState<string | null>(null);
  const [billDetails, setBillDetails] = useState<Record<string, BillDetail>>({});
  const [actionError, setActionError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [newTabMode, setNewTabMode] = useState<'walkin' | 'table' | 'customer'>('walkin');
  const [newTableLabel, setNewTableLabel] = useState('');
  const [customerSearch, setCustomerSearch] = useState('');
  const [newCustomerName, setNewCustomerName] = useState('');
  const [newCustomerPhone, setNewCustomerPhone] = useState('');

  const [paymentAmount, setPaymentAmount] = useState('');
  const [paymentMethod, setPaymentMethod] = useState<PaymentMethod>('cash');
  const [showWriteOff, setShowWriteOff] = useState(false);
  const [writeOffAmount, setWriteOffAmount] = useState('');
  const [writeOffReason, setWriteOffReason] = useState('');
  const [sharing, setSharing] = useState(false);
  const [shareLinkUrl, setShareLinkUrl] = useState<string | null>(null);

  const refreshBillDetail = async (billId: string) => {
    if (!selectedBusinessId) return;
    const detail = await getBillDetail(billId, selectedBusinessId);
    setBillDetails((d) => ({ ...d, [billId]: detail }));
  };

  const refreshBills = () => {
    if (!selectedBusinessId) return;
    listBills(selectedBusinessId)
      .then(setBills)
      .catch((err: unknown) => setLoadError(describeActionError(err, 'Could not load tabs.')));
  };

  useEffect(() => {
    if (!selectedBusinessId) return;
    refreshBills();
    listTables(selectedBusinessId)
      .then(setTables)
      .catch(() => undefined);
    listCustomers(selectedBusinessId)
      .then(setCustomers)
      .catch(() => undefined);
    listProducts(selectedBusinessId)
      .then(setProducts)
      .catch(() => undefined);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedBusinessId]);

  // Falls back to another open bill (or null) if the raw selection drops
  // out of the open list (e.g. it just got settled or voided) -- computed
  // at render rather than synced back into state via an effect, so there's
  // never a stale-then-corrected render in between.
  const activeBillId: string | null =
    selectedBillId === 'new'
      ? null
      : selectedBillId && bills?.some((b) => b.id === selectedBillId)
        ? selectedBillId
        : (bills?.[0]?.id ?? null);

  useEffect(() => {
    if (!activeBillId || !selectedBusinessId) return;
    let cancelled = false;
    void (async () => {
      try {
        const detail = await getBillDetail(activeBillId, selectedBusinessId);
        if (!cancelled) setBillDetails((d) => ({ ...d, [activeBillId]: detail }));
      } catch (err) {
        if (!cancelled) setActionError(describeActionError(err, 'Could not load this tab.'));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [activeBillId, selectedBusinessId]);

  const variants: PickableVariant[] = useMemo(
    () =>
      (products ?? []).flatMap((p) =>
        p.variants
          .filter((v) => v.active)
          .map((v) => ({
            variantId: v.id,
            description: `${p.name} — ${v.name}`,
            priceKobo: v.currentPriceKobo,
          })),
      ),
    [products],
  );

  const filteredCustomers = customerSearch.trim()
    ? customers.filter((c) => c.name.toLowerCase().includes(customerSearch.trim().toLowerCase()))
    : customers;

  function resetNewTabForm() {
    setNewTabMode('walkin');
    setNewTableLabel('');
    setCustomerSearch('');
    setNewCustomerName('');
    setNewCustomerPhone('');
  }

  async function handleOpenTab(tableId?: string, customerId?: string) {
    if (!selectedBusinessId || !csrfToken) return;
    setBusy(true);
    setActionError(null);
    try {
      const bill = await openBill({ tableId, customerId }, selectedBusinessId, csrfToken);
      resetNewTabForm();
      refreshBills();
      setSelectedBillId(bill.id);
    } catch (err) {
      setActionError(describeActionError(err, 'Could not open the tab.'));
    } finally {
      setBusy(false);
    }
  }

  async function handleCreateTableAndOpen() {
    if (!selectedBusinessId || !csrfToken || !newTableLabel.trim()) return;
    setBusy(true);
    setActionError(null);
    try {
      const table = await createTable(newTableLabel.trim(), selectedBusinessId, csrfToken);
      setTables((t) => [...t, table]);
      await handleOpenTab(table.id, undefined);
    } catch (err) {
      setActionError(describeActionError(err, 'Could not create the table.'));
      setBusy(false);
    }
  }

  async function handleCreateCustomerAndOpen() {
    if (!selectedBusinessId || !csrfToken || !newCustomerName.trim()) return;
    setBusy(true);
    setActionError(null);
    try {
      const customer = await createCustomer(
        { name: newCustomerName.trim(), phone: newCustomerPhone.trim() || undefined },
        selectedBusinessId,
        csrfToken,
      );
      setCustomers((c) => [...c, customer]);
      await handleOpenTab(undefined, customer.id);
    } catch (err) {
      setActionError(describeActionError(err, 'Could not create the customer.'));
      setBusy(false);
    }
  }

  // Adjusts one drink's net quantity on a still-editable bill by posting
  // directly -- no local draft to "submit" separately, so a plus/minus tap
  // takes effect immediately, same as changing your mind about an order
  // out loud. delta > 0 posts another round (AddSaleRound); delta < 0
  // posts a compensating negative round (RemoveBillItem) -- never edits or
  // deletes a previously posted round, matching every other correction in
  // this codebase. `busy` guards against double-submitting on a fast
  // double-tap while the previous request is still in flight.
  async function handleAdjustQuantity(
    billId: string,
    variant: { variantId: string; description: string; priceKobo: number },
    delta: number,
  ) {
    if (!selectedBusinessId || !csrfToken || busy || delta === 0) return;
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
      await refreshBillDetail(billId);
      refreshBills();
    } catch (err) {
      setActionError(describeActionError(err, 'Could not update this item.'));
    } finally {
      setBusy(false);
    }
  }

  async function handleRecordPayment(billId: string) {
    if (!selectedBusinessId || !csrfToken) return;
    const amountKobo = Math.round(parseFloat(paymentAmount) * 100);
    if (!amountKobo || amountKobo <= 0) return;
    setBusy(true);
    setActionError(null);
    try {
      await recordPayment(billId, amountKobo, paymentMethod, selectedBusinessId, csrfToken);
      setPaymentAmount('');
      await refreshBillDetail(billId);
      refreshBills();
    } catch (err) {
      setActionError(describeActionError(err, 'Could not record this payment.'));
    } finally {
      setBusy(false);
    }
  }

  async function handleCloseBill(billId: string) {
    if (!selectedBusinessId || !csrfToken) return;
    setBusy(true);
    setActionError(null);
    try {
      await closeBill(billId, selectedBusinessId, csrfToken);
      await refreshBillDetail(billId);
      refreshBills();
    } catch (err) {
      setActionError(describeActionError(err, 'Could not close this tab.'));
    } finally {
      setBusy(false);
    }
  }

  async function handleVoidBill(billId: string) {
    if (!selectedBusinessId || !csrfToken) return;
    setBusy(true);
    setActionError(null);
    try {
      await voidBill(billId, selectedBusinessId, csrfToken);
      await refreshBillDetail(billId);
      refreshBills();
    } catch (err) {
      setActionError(describeActionError(err, 'Could not void this tab.'));
    } finally {
      setBusy(false);
    }
  }

  // handleShare creates (or rotates) the bill's public share link, then
  // hands it to the Web Share API when available -- WhatsApp/email/etc.
  // all show up as native share-sheet targets on a phone -- falling back
  // to just showing/copying the URL in-page on a browser without it (most
  // desktop browsers) per docs/PHASE_PILOT_RELEASE.md §4.
  async function handleShare(billId: string) {
    if (!selectedBusinessId || !csrfToken) return;
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

  async function handleWriteOff(billId: string) {
    if (!selectedBusinessId || !csrfToken) return;
    const amountKobo = Math.round(parseFloat(writeOffAmount) * 100);
    if (!amountKobo || amountKobo <= 0 || !writeOffReason.trim()) return;
    setBusy(true);
    setActionError(null);
    try {
      await writeOffBill(billId, amountKobo, writeOffReason.trim(), selectedBusinessId, csrfToken);
      setWriteOffAmount('');
      setWriteOffReason('');
      setShowWriteOff(false);
      await refreshBillDetail(billId);
      refreshBills();
    } catch (err) {
      setActionError(describeActionError(err, 'Could not write off this balance.'));
    } finally {
      setBusy(false);
    }
  }

  const activeBill = bills?.find((b) => b.id === activeBillId) ?? null;
  const activeDetail = activeBillId ? billDetails[activeBillId] : undefined;
  const canEditItems = activeDetail?.status === 'open' || activeDetail?.status === 'closed_unpaid';

  // Net quantity per variant across every round posted so far -- a
  // removal round's negative quantity nets against the rounds that added
  // it, so this is always "what's actually on the tab right now," not a
  // list of individual rounds. Description/price shown come from the most
  // recently posted round for that variant.
  const currentOrderLines: CartLine[] = useMemo(() => {
    if (!activeDetail) return [];
    const byVariant = new Map<string, CartLine>();
    for (const round of activeDetail.sales) {
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
  }, [activeDetail]);
  const currentOrderQuantities = Object.fromEntries(
    currentOrderLines.map((l) => [l.variantId, l.quantity]),
  );

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

        {!isOnline && (
          <p className="text-[12.5px] text-jb-gold bg-jb-gold/10 rounded-xl px-4 py-2 mb-3">
            You&apos;re offline. Tabs need a connection — actions here won&apos;t go through until
            you&apos;re back online.
          </p>
        )}
        {loadError && <p className="text-[13px] text-red-700 mb-3">{loadError}</p>}
        {actionError && <p className="text-[13px] text-red-700 mb-3">{actionError}</p>}

        {/* Persistent tab switcher -- every open bill stays one tap away,
            no matter which one you're currently working on. Switching
            never discards an unsubmitted round for whichever tab you
            leave (see roundDrafts). */}
        <div className="flex flex-wrap gap-2 mb-4">
          {(bills ?? []).map((bill) => (
            <button
              key={bill.id}
              onClick={() => setSelectedBillId(bill.id)}
              className={`rounded-full px-4 py-2 text-[13px] font-medium whitespace-nowrap transition-colors ${
                activeBillId === bill.id
                  ? 'bg-jb-ink text-jb-cream'
                  : 'bg-white border border-jb-ink/15 text-jb-ink/70'
              }`}
            >
              {billLabel(bill, tables, customers)}
              {bill.balanceKobo > 0 && ` · ₦${formatNaira(bill.balanceKobo)}`}
            </button>
          ))}
          <button
            onClick={() => setSelectedBillId('new')}
            className={`shrink-0 rounded-full px-4 py-2 text-[13px] font-medium whitespace-nowrap border transition-colors ${
              activeBillId === null
                ? 'border-jb-ink text-jb-ink'
                : 'border-dashed border-jb-ink/25 text-jb-ink/50'
            }`}
          >
            + New
          </button>
        </div>

        {bills === null && !loadError && <p className="text-[13px] text-jb-ink/45">Loading…</p>}

        {bills !== null && activeBillId === null && (
          <div className="rounded-xl border border-jb-ink/10 bg-white p-4">
            <div className="flex gap-2 p-1 rounded-xl bg-jb-ink/[0.05] mb-3">
              {(['walkin', 'table', 'customer'] as const).map((m) => (
                <button
                  key={m}
                  onClick={() => setNewTabMode(m)}
                  className={`flex-1 rounded-lg py-2 text-[12.5px] font-medium capitalize transition-colors ${
                    newTabMode === m ? 'bg-white text-jb-ink shadow-sm' : 'text-jb-ink/45'
                  }`}
                >
                  {m === 'walkin' ? 'Walk-in' : m}
                </button>
              ))}
            </div>

            {newTabMode === 'walkin' && (
              <button
                onClick={() => void handleOpenTab()}
                disabled={busy}
                className="w-full rounded-xl bg-jb-ink text-jb-cream text-[14px] font-medium py-3 disabled:opacity-40"
              >
                Open walk-in tab
              </button>
            )}

            {newTabMode === 'table' && (
              <div>
                {tables.length > 0 && (
                  <div className="flex flex-wrap gap-2 mb-3">
                    {tables.map((t) => (
                      <button
                        key={t.id}
                        onClick={() => void handleOpenTab(t.id, undefined)}
                        disabled={busy}
                        className="rounded-full border border-jb-ink/15 px-4 py-2 text-[13px] disabled:opacity-40"
                      >
                        {t.label}
                      </button>
                    ))}
                  </div>
                )}
                <div className="flex gap-2">
                  <input
                    type="text"
                    placeholder="New table label (e.g. T4)"
                    value={newTableLabel}
                    onChange={(e) => setNewTableLabel(e.target.value)}
                    className="flex-1 rounded-xl border border-jb-ink/15 bg-white px-4 py-2.5 text-[13.5px] focus:outline-none focus:border-jb-ink/40"
                  />
                  <button
                    onClick={() => void handleCreateTableAndOpen()}
                    disabled={busy || !newTableLabel.trim()}
                    className="rounded-xl bg-jb-ink text-jb-cream text-[13.5px] font-medium px-4 disabled:opacity-40"
                  >
                    Add
                  </button>
                </div>
              </div>
            )}

            {newTabMode === 'customer' && (
              <div>
                <input
                  type="text"
                  placeholder="Search customers…"
                  value={customerSearch}
                  onChange={(e) => setCustomerSearch(e.target.value)}
                  className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-2.5 text-[13.5px] focus:outline-none focus:border-jb-ink/40 mb-3"
                />
                {filteredCustomers.length > 0 && (
                  <div className="flex flex-col gap-1.5 mb-3">
                    {filteredCustomers.map((c) => (
                      <button
                        key={c.id}
                        onClick={() => void handleOpenTab(undefined, c.id)}
                        disabled={busy}
                        className="text-left rounded-xl border border-jb-ink/15 px-4 py-2.5 text-[13.5px] disabled:opacity-40"
                      >
                        {c.name}
                      </button>
                    ))}
                  </div>
                )}
                <div className="border-t border-jb-ink/10 pt-3">
                  <div className="text-[11px] text-jb-ink/45 mb-2">New customer</div>
                  <input
                    type="text"
                    placeholder="Name (required)"
                    value={newCustomerName}
                    onChange={(e) => setNewCustomerName(e.target.value)}
                    className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-2.5 text-[13.5px] focus:outline-none focus:border-jb-ink/40 mb-2"
                  />
                  <input
                    type="tel"
                    placeholder="Phone (optional)"
                    value={newCustomerPhone}
                    onChange={(e) => setNewCustomerPhone(e.target.value)}
                    className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-2.5 text-[13.5px] focus:outline-none focus:border-jb-ink/40 mb-2"
                  />
                  <button
                    onClick={() => void handleCreateCustomerAndOpen()}
                    disabled={busy || !newCustomerName.trim()}
                    className="w-full rounded-xl bg-jb-ink text-jb-cream text-[13.5px] font-medium py-2.5 disabled:opacity-40"
                  >
                    Create &amp; open tab
                  </button>
                </div>
              </div>
            )}
          </div>
        )}

        {activeBillId !== null && (
          <>
            {!activeDetail || !activeBill ? (
              <p className="text-[13px] text-jb-ink/45">Loading…</p>
            ) : (
              <>
                <div className="rounded-xl bg-jb-ink text-jb-cream p-4 mb-4">
                  <div className="flex items-center justify-between mb-1">
                    <span className="text-[12px] text-jb-cream/60">
                      {statusLabel(activeDetail.status)}
                    </span>
                    <span className="text-[15px] font-medium">
                      ₦{formatNaira(activeDetail.balanceKobo)} due
                    </span>
                  </div>
                  <div className="text-[11px] text-jb-cream/45">
                    Opened {new Date(activeDetail.openedAt).toLocaleString()}
                  </div>
                </div>

                {canEditItems && (
                  <div className="mb-4">
                    <div className="text-[11px] text-jb-ink/45 tracking-wide mb-2">
                      ADD OR ADJUST
                    </div>
                    <ProductGrid
                      variants={variants}
                      isLoading={products === null}
                      cartQuantities={currentOrderQuantities}
                      inCartLabel="on this tab"
                      onAdd={(v) => void handleAdjustQuantity(activeBillId, v, 1)}
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
          </>
        )}
      </div>

      {/* The live current order lives outside the centered column above so
          CartPanel's own bottom-sheet styling (full-bleed background,
          bounded height) matches Sell.tsx exactly. Each +/- tap posts
          immediately -- see handleAdjustQuantity -- so there's no separate
          "submit this round" step here, unlike Sell.tsx's one-shot cart. */}
      {activeBillId !== null && activeDetail && activeBill && canEditItems && (
        <CartPanel
          label="CURRENT ORDER"
          lines={currentOrderLines}
          emptyMessage="Tap a drink above to add it here."
          onChangeQty={(variantId, delta) => {
            const line = currentOrderLines.find((l) => l.variantId === variantId);
            const live = variants.find((v) => v.variantId === variantId);
            void handleAdjustQuantity(
              activeBillId,
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

      {activeBillId !== null && activeDetail && (
        <div className="max-w-md mx-auto px-5">
          {activeDetail.payments.length > 0 && (
            <div className="mb-4 mt-4">
              <div className="text-[11px] text-jb-ink/45 tracking-wide mb-2">PAYMENTS</div>
              <div className="space-y-1.5">
                {activeDetail.payments.map((p, i) => (
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

          {(activeDetail.status === 'open' || activeDetail.status === 'closed_unpaid') &&
            activeDetail.balanceKobo > 0 && (
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
                    placeholder={`Up to ₦${formatNaira(activeDetail.balanceKobo)}`}
                    value={paymentAmount}
                    onChange={(e) => setPaymentAmount(e.target.value)}
                    className="flex-1 rounded-xl border border-jb-ink/15 bg-white px-4 py-3 text-[14px] focus:outline-none focus:border-jb-ink/40"
                  />
                  <button
                    onClick={() => void handleRecordPayment(activeBillId)}
                    disabled={busy || !paymentAmount}
                    className="rounded-xl bg-jb-ink text-jb-cream text-[14px] font-medium px-5 disabled:opacity-40"
                  >
                    Pay
                  </button>
                </div>
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
              onClick={() => void handleShare(activeBillId)}
              disabled={sharing}
              className="w-full rounded-xl border border-jb-ink/15 text-jb-ink text-[14px] font-medium py-3 disabled:opacity-40"
            >
              {sharing ? 'Preparing link…' : 'Share bill'}
            </button>
            {activeDetail.status === 'open' && (
              <button
                onClick={() => void handleCloseBill(activeBillId)}
                disabled={busy}
                className="w-full rounded-xl border border-jb-ink/15 text-jb-ink text-[14px] font-medium py-3 disabled:opacity-40"
              >
                Close tab
              </button>
            )}
            {isOwner &&
              (activeDetail.status === 'open' || activeDetail.status === 'closed_unpaid') && (
                <button
                  onClick={() => void handleVoidBill(activeBillId)}
                  disabled={busy}
                  className="w-full rounded-xl border border-jb-ink/15 text-jb-ink/60 text-[13px] py-2.5 disabled:opacity-40"
                >
                  Void tab (only if nothing effective remains)
                </button>
              )}
            {isOwner && activeDetail.status === 'closed_unpaid' && (
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
                      placeholder={`Amount, up to ₦${formatNaira(activeDetail.balanceKobo)}`}
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
                      onClick={() => void handleWriteOff(activeBillId)}
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

          {activeDetail.sales.length > 0 && (
            <div className="mb-6">
              <div className="text-[11px] text-jb-ink/45 tracking-wide mb-2">ROUNDS</div>
              <div className="space-y-2">
                {activeDetail.sales.map((round) => (
                  <div key={round.id} className="rounded-xl border border-jb-ink/10 bg-white p-3">
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
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      <AppBottomNav />
    </div>
  );
}
