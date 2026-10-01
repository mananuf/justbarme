import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import { listProducts, type CatalogueProduct } from '../api/catalogue';
import {
  addSaleRound,
  createCustomer,
  createTable,
  listCustomers,
  listTables,
  openBill,
  type Customer,
  type Table,
} from '../api/tabs';
import { AppBottomNav } from '../components/AppBottomNav';
import { CartPanel, type CartLine } from '../components/CartPanel';
import { ProductGrid, type PickableVariant } from '../components/ProductGrid';
import { useConnectivity } from '../hooks/useConnectivity';
import { db, type PendingSale } from '../lib/db';
import { describeActionError } from '../lib/errors';
import { flushPendingSales, queuePendingSale, trySyncPendingSale } from '../lib/salesSync';
import { useSession } from '../lib/session';

type PaymentMethod = 'cash' | 'transfer' | 'card';

function formatNaira(kobo: number): string {
  return (kobo / 100).toLocaleString();
}

// Sell is the one screen for recording any sale -- walk-in or tab, no
// separate tools or modes to choose between
// (docs/PHASE_UNIFIED_SELL_BILLS.md). Table and customer pickers are
// always shown, each defaulting to a plain pseudo-option ("No table" /
// "Walk-in customer") rather than a real row -- picking neither is a
// one-shot walk-in sale (still fully offline-safe, the existing
// pendingSales queue); picking either turns this into a real, nameable
// bill and hands off to the normal online tab flow from there, landing on
// its detail page either way.
export function Sell() {
  const navigate = useNavigate();
  const { csrfToken, selectedBusinessId } = useSession();
  const isOnline = useConnectivity();

  const [products, setProducts] = useState<CatalogueProduct[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  // True when `products` came from IndexedDB, not a fresh fetch -- shown
  // as an honest "may be out of date" note rather than presented as
  // current (docs/ARCHITECTURE.md §10.7's fixed, honest sync states).
  const [usingCachedCatalogue, setUsingCachedCatalogue] = useState(false);

  const [tables, setTables] = useState<Table[]>([]);
  const [customers, setCustomers] = useState<Customer[]>([]);
  // '' is the default pseudo-option in both pickers -- never a real
  // table/customer row. Only a real id turns this sale into a persisted,
  // nameable bill.
  const [tableId, setTableId] = useState('');
  const [customerId, setCustomerId] = useState('');
  const [addingTable, setAddingTable] = useState(false);
  const [newTableLabel, setNewTableLabel] = useState('');
  const [addingCustomer, setAddingCustomer] = useState(false);
  const [customerSearch, setCustomerSearch] = useState('');
  const [newCustomerName, setNewCustomerName] = useState('');
  const [newCustomerPhone, setNewCustomerPhone] = useState('');

  const [cart, setCart] = useState<CartLine[]>([]);
  const [method, setMethod] = useState<PaymentMethod>('cash');
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  // Set only for the walk-in path when the sale was queued but couldn't be
  // posted right away (offline, or a momentary failure) -- there's no
  // server-side bill yet to navigate to, so this is the same plain local
  // confirmation Sell.tsx has always shown in that case.
  const [queuedOffline, setQueuedOffline] = useState(false);

  const isWalkIn = tableId === '' && customerId === '';

  useEffect(() => {
    if (!selectedBusinessId) return;
    listProducts(selectedBusinessId)
      .then((fetched) => {
        setProducts(fetched);
        setUsingCachedCatalogue(false);
        void db.catalogueCache.put({
          businessId: selectedBusinessId,
          products: fetched,
          cachedAt: new Date().toISOString(),
        });
      })
      .catch((err: unknown) => {
        void db.catalogueCache.get(selectedBusinessId).then((cached) => {
          if (cached) {
            setProducts(cached.products);
            setUsingCachedCatalogue(true);
            return;
          }
          setLoadError(describeActionError(err, 'Could not load your products.'));
        });
      });
    listTables(selectedBusinessId)
      .then(setTables)
      .catch(() => undefined);
    listCustomers(selectedBusinessId)
      .then(setCustomers)
      .catch(() => undefined);
  }, [selectedBusinessId]);

  useEffect(() => {
    if (!isOnline || !selectedBusinessId || !csrfToken) return;
    // Catches up on anything queued from an earlier offline session, and
    // on regaining connectivity mid-session -- see src/lib/salesSync.ts.
    void flushPendingSales(selectedBusinessId, csrfToken);
  }, [isOnline, selectedBusinessId, csrfToken]);

  const addOne = (variant: PickableVariant) => {
    if (variant.outOfStock) return;
    setCart((c) => {
      const existing = c.find((l) => l.variantId === variant.variantId);
      if (existing)
        return c.map((l) =>
          l.variantId === variant.variantId ? { ...l, quantity: l.quantity + 1 } : l,
        );
      return [
        ...c,
        {
          variantId: variant.variantId,
          description: variant.description,
          unitPriceKobo: variant.priceKobo,
          quantity: 1,
        },
      ];
    });
  };

  const changeQty = (variantId: string, delta: number) => {
    if (delta > 0 && variants.find((v) => v.variantId === variantId)?.outOfStock) return;
    setCart((c) =>
      c
        .map((l) => (l.variantId === variantId ? { ...l, quantity: l.quantity + delta } : l))
        .filter((l) => l.quantity > 0),
    );
  };

  const total = cart.reduce((sum, l) => sum + l.unitPriceKobo * l.quantity, 0);
  const cartQuantities = Object.fromEntries(cart.map((l) => [l.variantId, l.quantity]));

  async function handleAddTable() {
    if (!selectedBusinessId || !csrfToken || !newTableLabel.trim()) return;
    setSubmitting(true);
    setSubmitError(null);
    try {
      const table = await createTable(newTableLabel.trim(), selectedBusinessId, csrfToken);
      setTables((t) => [...t, table]);
      setTableId(table.id);
      setAddingTable(false);
      setNewTableLabel('');
    } catch (err) {
      setSubmitError(describeActionError(err, 'Could not add that table.'));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleAddCustomer() {
    if (!selectedBusinessId || !csrfToken || !newCustomerName.trim()) return;
    setSubmitting(true);
    setSubmitError(null);
    try {
      const customer = await createCustomer(
        { name: newCustomerName.trim(), phone: newCustomerPhone.trim() || undefined },
        selectedBusinessId,
        csrfToken,
      );
      setCustomers((c) => [...c, customer]);
      setCustomerId(customer.id);
      setAddingCustomer(false);
      setNewCustomerName('');
      setNewCustomerPhone('');
    } catch (err) {
      setSubmitError(describeActionError(err, 'Could not add that customer.'));
    } finally {
      setSubmitting(false);
    }
  }

  async function completeSale() {
    if (!selectedBusinessId) return;
    // A one-shot walk-in sale can't record nothing; a tab can be opened
    // empty and ordered into later from its own detail page.
    if (isWalkIn && cart.length === 0) return;
    setSubmitting(true);
    setSubmitError(null);
    try {
      if (isWalkIn) {
        const idempotencyKey = crypto.randomUUID();
        const pending: PendingSale = {
          idempotencyKey,
          businessId: selectedBusinessId,
          occurredAt: new Date().toISOString(),
          items: cart.map((l) => ({
            variantId: l.variantId,
            description: l.description,
            quantity: l.quantity,
            unitPriceKobo: l.unitPriceKobo,
          })),
          payment: { amountKobo: total, method },
          totalKobo: total,
          status: 'pending',
          createdAt: new Date().toISOString(),
        };
        // Written to IndexedDB first, before any network call -- this is
        // what makes "sale recorded" true even fully offline.
        await queuePendingSale(pending);
        if (isOnline && csrfToken) {
          const posted = await trySyncPendingSale(pending, selectedBusinessId, csrfToken);
          if (posted) {
            void navigate(`/dashboard/tabs/${posted.billId}`);
            return;
          }
        }
        setQueuedOffline(true);
        return;
      }

      // A table and/or named customer was chosen -- this is a real,
      // nameable bill from here, the normal online-only tab flow
      // (docs/PHASE_TABS_CREDIT.md). Find-or-open: picking an
      // already-occupied table/customer resumes its existing tab rather
      // than opening a second one.
      if (!csrfToken) throw new Error('offline');
      const bill = await openBill(
        { tableId: tableId || undefined, customerId: customerId || undefined },
        selectedBusinessId,
        csrfToken,
      );
      if (cart.length > 0) {
        await addSaleRound(
          bill.id,
          {
            idempotencyKey: crypto.randomUUID(),
            occurredAt: new Date().toISOString(),
            items: cart.map((l) => ({
              variantId: l.variantId,
              quantity: l.quantity,
              unitPriceKobo: l.unitPriceKobo,
            })),
          },
          selectedBusinessId,
          csrfToken,
        );
      }
      void navigate(`/dashboard/tabs/${bill.id}`);
    } catch (err) {
      setSubmitError(describeActionError(err, 'Could not start this sale.'));
    } finally {
      setSubmitting(false);
    }
  }

  // A drink's price is fetched fresh, but this uncommitted local cart
  // hasn't posted yet -- currentStock still reflects the server's real
  // stock, so outOfStock has to subtract what's already sitting in the
  // cart itself. Not memoized -- it has to recompute whenever `cart`
  // changes anyway, and this catalogue is small enough that recomputing on
  // every render costs nothing worth guarding against.
  const inCart = new Map(cart.map((l) => [l.variantId, l.quantity]));
  const variants: PickableVariant[] = (products ?? []).flatMap((p) =>
    p.variants
      .filter((v) => v.active)
      .map((v) => ({
        variantId: v.id,
        description: `${p.name} — ${v.name}`,
        priceKobo: v.currentPriceKobo,
        outOfStock: v.tracksInventory && v.currentStock - (inCart.get(v.id) ?? 0) <= 0,
      })),
  );

  const filteredCustomers = customerSearch.trim()
    ? customers.filter((c) => c.name.toLowerCase().includes(customerSearch.trim().toLowerCase()))
    : customers;

  if (queuedOffline) {
    return (
      <div className="min-h-screen bg-jb-cream text-jb-ink flex flex-col items-center justify-center px-6 text-center pb-28">
        <div
          className="w-16 h-16 rounded-full bg-jb-ink text-jb-cream flex items-center justify-center text-2xl mb-6"
          style={{ animation: 'pop-in 0.5s cubic-bezier(0.16,1,0.3,1) both' }}
        >
          ✓
        </div>
        <h1
          className="text-2xl font-light tracking-tight mb-2"
          style={{ fontFamily: 'var(--font-display)' }}
        >
          Sale recorded
        </h1>
        <p className="text-[13px] text-jb-ink/45 mb-8 max-w-xs">
          Saved on this device. It will sync automatically once you&apos;re back online.
        </p>
        <Link
          to="/dashboard"
          className="rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 px-8 hover:bg-jb-green transition-colors"
        >
          Back to Dashboard
        </Link>
        <AppBottomNav />
      </div>
    );
  }

  return (
    <div className="h-screen flex flex-col bg-jb-cream text-jb-ink">
      {/* Fixed header -- never scrolls */}
      <div className="max-w-md mx-auto w-full px-5 pt-6 shrink-0">
        <div className="flex items-center gap-4 mb-4">
          <Link
            to="/dashboard"
            aria-label="Back"
            className="text-jb-ink/40 hover:text-jb-ink transition-colors text-lg leading-none"
          >
            ←
          </Link>
          <div>
            <div className="text-[11px] text-jb-ink/45">Sell</div>
            <h1 className="text-lg font-medium">New sale</h1>
          </div>
        </div>
      </div>

      {/* Top half: table/customer pickers, then the product grid -- all
          scroll together as one picker, not separate screens. */}
      <div className="flex-1 min-h-0 overflow-y-auto">
        <div className="max-w-md mx-auto w-full px-5 pb-3">
          {!isOnline && !isWalkIn && (
            <p className="text-[12px] text-jb-gold bg-jb-gold/10 rounded-xl px-4 py-2 mb-3">
              You&apos;re offline — a table or named tab needs a connection. A plain walk-in sale
              still works offline.
            </p>
          )}
          {submitError && <p className="text-[13px] text-red-700 mb-3">{submitError}</p>}

          <div className="mb-3">
            <div className="text-[11px] text-jb-ink/45 tracking-wide mb-1.5">TABLE</div>
            <div className="flex flex-wrap gap-2">
              <button
                onClick={() => setTableId('')}
                className={`rounded-full px-3.5 py-1.5 text-[12.5px] font-medium transition-colors ${
                  tableId === ''
                    ? 'bg-jb-ink text-jb-cream'
                    : 'bg-white border border-jb-ink/15 text-jb-ink/60'
                }`}
              >
                No table
              </button>
              {tables.map((t) => (
                <button
                  key={t.id}
                  onClick={() => setTableId(t.id)}
                  className={`rounded-full px-3.5 py-1.5 text-[12.5px] font-medium transition-colors ${
                    tableId === t.id
                      ? 'bg-jb-ink text-jb-cream'
                      : 'bg-white border border-jb-ink/15 text-jb-ink/60'
                  }`}
                >
                  {t.label}
                </button>
              ))}
              {!addingTable ? (
                <button
                  onClick={() => setAddingTable(true)}
                  className="rounded-full px-3.5 py-1.5 text-[12.5px] border border-dashed border-jb-ink/25 text-jb-ink/45"
                >
                  + Add table
                </button>
              ) : (
                <div className="flex gap-1.5 w-full mt-1">
                  <input
                    autoFocus
                    type="text"
                    placeholder="Table label (e.g. T4)"
                    value={newTableLabel}
                    onChange={(e) => setNewTableLabel(e.target.value)}
                    className="flex-1 rounded-lg border border-jb-ink/15 bg-white px-3 py-1.5 text-[12.5px] focus:outline-none focus:border-jb-ink/40"
                  />
                  <button
                    onClick={() => void handleAddTable()}
                    disabled={submitting || !newTableLabel.trim()}
                    className="rounded-lg bg-jb-ink text-jb-cream text-[12.5px] font-medium px-3 disabled:opacity-40"
                  >
                    Add
                  </button>
                </div>
              )}
            </div>
          </div>

          <div className="mb-4">
            <div className="text-[11px] text-jb-ink/45 tracking-wide mb-1.5">CUSTOMER</div>
            <div className="flex flex-wrap gap-2 mb-1.5">
              <button
                onClick={() => setCustomerId('')}
                className={`rounded-full px-3.5 py-1.5 text-[12.5px] font-medium transition-colors ${
                  customerId === ''
                    ? 'bg-jb-ink text-jb-cream'
                    : 'bg-white border border-jb-ink/15 text-jb-ink/60'
                }`}
              >
                Walk-in customer
              </button>
              {customers
                .filter((c) => c.id === customerId)
                .map((c) => (
                  <button
                    key={c.id}
                    className="rounded-full px-3.5 py-1.5 text-[12.5px] font-medium bg-jb-ink text-jb-cream"
                  >
                    {c.name}
                  </button>
                ))}
            </div>
            {customerId === '' && (
              <p className="text-[11.5px] text-jb-ink/40 mb-2">
                No specific customer — for someone paying now and leaving. Search below only if you
                want to name who this sale (or tab) belongs to.
              </p>
            )}
            <input
              type="text"
              placeholder="Search customers…"
              value={customerSearch}
              onChange={(e) => setCustomerSearch(e.target.value)}
              className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-2 text-[13px] focus:outline-none focus:border-jb-ink/40 mb-1.5"
            />
            {customerSearch.trim() && filteredCustomers.length > 0 && (
              <div className="flex flex-col gap-1 mb-1.5">
                {filteredCustomers.map((c) => (
                  <button
                    key={c.id}
                    onClick={() => {
                      setCustomerId(c.id);
                      setCustomerSearch('');
                    }}
                    className="text-left rounded-lg border border-jb-ink/15 px-3 py-1.5 text-[12.5px]"
                  >
                    {c.name}
                  </button>
                ))}
              </div>
            )}
            {!addingCustomer ? (
              <button
                onClick={() => setAddingCustomer(true)}
                className="text-[12px] text-jb-ink/45 underline underline-offset-2"
              >
                + New customer
              </button>
            ) : (
              <div className="rounded-xl border border-jb-ink/10 bg-white p-3 mt-1.5 space-y-2">
                <input
                  autoFocus
                  type="text"
                  placeholder="Name (required)"
                  value={newCustomerName}
                  onChange={(e) => setNewCustomerName(e.target.value)}
                  className="w-full rounded-lg border border-jb-ink/15 bg-white px-3 py-2 text-[12.5px] focus:outline-none focus:border-jb-ink/40"
                />
                <input
                  type="tel"
                  placeholder="Phone (optional)"
                  value={newCustomerPhone}
                  onChange={(e) => setNewCustomerPhone(e.target.value)}
                  className="w-full rounded-lg border border-jb-ink/15 bg-white px-3 py-2 text-[12.5px] focus:outline-none focus:border-jb-ink/40"
                />
                <div className="flex gap-2">
                  <button
                    onClick={() => void handleAddCustomer()}
                    disabled={submitting || !newCustomerName.trim()}
                    className="flex-1 rounded-lg bg-jb-ink text-jb-cream text-[12.5px] font-medium py-2 disabled:opacity-40"
                  >
                    Add customer
                  </button>
                  <button
                    onClick={() => setAddingCustomer(false)}
                    className="text-[12.5px] text-jb-ink/45 px-3"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            )}
          </div>

          {usingCachedCatalogue && (
            <p className="text-[12px] text-jb-gold bg-jb-gold/10 rounded-xl px-4 py-2 mb-3">
              Showing saved products — may be out of date. Prices and stock will refresh once
              you&apos;re back online.
            </p>
          )}
          <ProductGrid
            variants={variants}
            isLoading={products === null && !loadError}
            loadError={loadError}
            cartQuantities={cartQuantities}
            inCartLabel="in sale"
            onAdd={addOne}
            emptyMessage={
              <p className="text-[13px] text-jb-ink/45">
                You haven&apos;t stocked anything yet.{' '}
                <Link to="/dashboard/stock" className="underline">
                  Add stock
                </Link>{' '}
                to start selling.
              </p>
            }
          />
        </div>
      </div>

      {/* Bottom half: what's been added so far -- its own scroll region,
          always visible. The footer changes with the picker above it (full
          payment entry for a plain walk-in, a plain "Start tab" button
          once a table/customer is chosen) but this stays one screen, not
          two different tools. */}
      <CartPanel
        label="THIS SALE"
        lines={cart.map((l) => ({
          ...l,
          atStockLimit: variants.find((v) => v.variantId === l.variantId)?.outOfStock,
        }))}
        emptyMessage="Tap a drink above to add it here."
        onChangeQty={changeQty}
        showFooterWhenEmpty={!isWalkIn}
        footer={
          isWalkIn ? (
            <>
              <div className="flex gap-2 p-1 rounded-xl bg-jb-ink/[0.05] mb-2">
                {(['cash', 'transfer', 'card'] as PaymentMethod[]).map((m) => (
                  <button
                    key={m}
                    onClick={() => setMethod(m)}
                    className={`flex-1 rounded-lg py-2 text-[12.5px] font-medium capitalize transition-colors ${
                      method === m ? 'bg-white text-jb-ink shadow-sm' : 'text-jb-ink/45'
                    }`}
                  >
                    {m}
                  </button>
                ))}
              </div>
              <div className="flex items-center justify-between text-[12px] text-jb-ink/55 mb-2 px-1">
                <span>Total</span>
                <span className="text-jb-ink font-medium text-[15px]">₦{formatNaira(total)}</span>
              </div>
              <button
                onClick={() => void completeSale()}
                disabled={submitting || cart.length === 0}
                className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-3.5 active:scale-[0.98] transition-transform disabled:opacity-40"
              >
                {submitting ? 'Saving…' : 'Complete Sale'}
              </button>
            </>
          ) : (
            <>
              <div className="flex items-center justify-between text-[12px] text-jb-ink/55 mb-2 px-1">
                <span>So far</span>
                <span className="text-jb-ink font-medium text-[15px]">₦{formatNaira(total)}</span>
              </div>
              <button
                onClick={() => void completeSale()}
                disabled={submitting || !isOnline}
                className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-3.5 active:scale-[0.98] transition-transform disabled:opacity-40"
              >
                {submitting ? 'Starting…' : 'Start tab'}
              </button>
              <p className="text-[11px] text-jb-ink/40 text-center mt-1.5">
                You can add more and take payment on the next screen.
              </p>
            </>
          )
        }
      />

      {/* Reserves room for the fixed bottom nav below, matching the
          clearance every other app-shell page uses. */}
      <div className="h-28 shrink-0" />
      <AppBottomNav />
    </div>
  );
}
