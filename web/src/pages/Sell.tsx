import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import { listProducts, type CatalogueProduct } from '../api/catalogue';
import {
  addSaleRound,
  createCustomer,
  createTable,
  listBills,
  listCustomers,
  listTables,
  openBill,
  type Bill,
  type Customer,
  type Table,
} from '../api/tabs';
import { AppBottomNav } from '../components/AppBottomNav';
import { BillWorkspace } from '../components/BillWorkspace';
import { ProductGrid, type PickableVariant } from '../components/ProductGrid';
import { useConnectivity } from '../hooks/useConnectivity';
import { billLabel, formatNaira } from '../lib/billDisplay';
import { db } from '../lib/db';
import { describeActionError } from '../lib/errors';
import { useSession } from '../lib/session';

type PickerMode = 'table' | 'customer';

// Sell is the one screen for recording any sale -- walk-in or tab, no
// separate tools, no mode a non-technical user has to understand
// (docs/PHASE_UNIFIED_SELL_BILLS.md). A persistent pill strip of currently
// open bills sits at the top (jump back into any of them with one tap,
// same as the original Tabs.tsx always did); "+ New" reveals a two-way
// Table/Customer toggle -- never both selectable at once, which is what
// made the previous version of this screen confusing -- each defaulting to
// a plain pseudo-option ("No table" / "Walk-in customer") rather than a
// real row. Tapping the first drink is what starts the sale: it opens (or
// resumes, via the backend's find-or-open) the bill and posts that round
// immediately, live, exactly like every other round on a tab -- there is
// no separate "Start tab"/"Complete Sale" step, and walk-in is not a
// special case any more than Table or Customer is.
export function Sell() {
  const { csrfToken, selectedBusinessId } = useSession();
  const isOnline = useConnectivity();

  const [products, setProducts] = useState<CatalogueProduct[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  // True when `products` came from IndexedDB, not a fresh fetch -- shown
  // as an honest "may be out of date" note rather than presented as
  // current (docs/ARCHITECTURE.md §10.7's fixed, honest sync states).
  const [usingCachedCatalogue, setUsingCachedCatalogue] = useState(false);

  const [openBills, setOpenBills] = useState<Bill[]>([]);
  const [tables, setTables] = useState<Table[]>([]);
  const [customers, setCustomers] = useState<Customer[]>([]);

  // null = the "+ New" picker is showing; a real id = that bill's live
  // workspace is showing instead, inline, no navigation.
  const [activeBillId, setActiveBillId] = useState<string | null>(null);

  // Picker state -- only meaningful while activeBillId is null. pickerMode
  // decides which of the two is the active, visible selector; switching it
  // resets the other back to its default, so exactly one of
  // tableId/customerId can ever be a real id at a time.
  const [pickerMode, setPickerMode] = useState<PickerMode>('table');
  const [tableId, setTableId] = useState('');
  const [customerId, setCustomerId] = useState('');
  const [addingTable, setAddingTable] = useState(false);
  const [newTableLabel, setNewTableLabel] = useState('');
  const [addingCustomer, setAddingCustomer] = useState(false);
  const [customerSearch, setCustomerSearch] = useState('');
  const [newCustomerName, setNewCustomerName] = useState('');
  const [newCustomerPhone, setNewCustomerPhone] = useState('');

  const [resolving, setResolving] = useState(false);
  const [resolveError, setResolveError] = useState<string | null>(null);

  const refreshOpenBills = () => {
    if (!selectedBusinessId) return;
    listBills(selectedBusinessId, 'open')
      .then(setOpenBills)
      .catch(() => undefined);
  };

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
    refreshOpenBills();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedBusinessId]);

  function startNew() {
    setActiveBillId(null);
    setPickerMode('table');
    setTableId('');
    setCustomerId('');
    setAddingTable(false);
    setNewTableLabel('');
    setAddingCustomer(false);
    setCustomerSearch('');
    setResolveError(null);
  }

  function selectPickerMode(mode: PickerMode) {
    setPickerMode(mode);
    // Exactly one of table/customer can be a real selection at a time --
    // switching the toggle clears whichever side isn't active, rather than
    // letting both quietly stay selected underneath (the exact confusion
    // this toggle replaced two independently-selectable sections to fix).
    if (mode === 'table') setCustomerId('');
    else setTableId('');
  }

  async function handleAddTable() {
    if (!selectedBusinessId || !csrfToken || !newTableLabel.trim()) return;
    setResolving(true);
    setResolveError(null);
    try {
      const table = await createTable(newTableLabel.trim(), selectedBusinessId, csrfToken);
      setTables((t) => [...t, table]);
      setTableId(table.id);
      setAddingTable(false);
      setNewTableLabel('');
    } catch (err) {
      setResolveError(describeActionError(err, 'Could not add that table.'));
    } finally {
      setResolving(false);
    }
  }

  async function handleAddCustomer() {
    if (!selectedBusinessId || !csrfToken || !newCustomerName.trim()) return;
    setResolving(true);
    setResolveError(null);
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
      setResolveError(describeActionError(err, 'Could not add that customer.'));
    } finally {
      setResolving(false);
    }
  }

  // Tapping the first drink is what starts the sale: find-or-open the bill
  // for whatever's currently picked (both pickers at their defaults means
  // a plain walk-in), post this one item as a live round immediately, then
  // switch this same screen into that bill's own workspace -- no
  // "Start tab"/"Complete Sale" button in between.
  async function handleFirstItem(variant: PickableVariant) {
    if (!selectedBusinessId || !csrfToken || resolving || variant.outOfStock) return;
    setResolving(true);
    setResolveError(null);
    try {
      const bill = await openBill(
        { tableId: tableId || undefined, customerId: customerId || undefined },
        selectedBusinessId,
        csrfToken,
      );
      await addSaleRound(
        bill.id,
        {
          idempotencyKey: crypto.randomUUID(),
          occurredAt: new Date().toISOString(),
          items: [{ variantId: variant.variantId, quantity: 1, unitPriceKobo: variant.priceKobo }],
        },
        selectedBusinessId,
        csrfToken,
      );
      setActiveBillId(bill.id);
      refreshOpenBills();
    } catch (err) {
      setResolveError(describeActionError(err, 'Could not start this sale.'));
    } finally {
      setResolving(false);
    }
  }

  const variants: PickableVariant[] = (products ?? []).flatMap((p) =>
    p.variants
      .filter((v) => v.active)
      .map((v) => ({
        variantId: v.id,
        description: `${p.name} — ${v.name}`,
        priceKobo: v.currentPriceKobo,
        outOfStock: v.tracksInventory && v.currentStock <= 0,
      })),
  );

  const filteredCustomers = customerSearch.trim()
    ? customers.filter((c) => c.name.toLowerCase().includes(customerSearch.trim().toLowerCase()))
    : customers;

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
            <div className="text-[11px] text-jb-ink/45">Sell</div>
            <h1 className="text-lg font-medium">New sale</h1>
          </div>
        </div>

        {/* Persistent pill strip -- every open bill stays one tap away, no
            matter which one (or none) is currently active. */}
        <div className="flex flex-wrap gap-2 mb-4">
          {openBills.map((bill) => (
            <button
              key={bill.id}
              onClick={() => setActiveBillId(bill.id)}
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
            onClick={startNew}
            className={`shrink-0 rounded-full px-4 py-2 text-[13px] font-medium whitespace-nowrap border transition-colors ${
              activeBillId === null
                ? 'border-jb-ink text-jb-ink'
                : 'border-dashed border-jb-ink/25 text-jb-ink/50'
            }`}
          >
            + New
          </button>
        </div>

        {!isOnline && (
          <p className="text-[12px] text-jb-gold bg-jb-gold/10 rounded-xl px-4 py-2 mb-3">
            You&apos;re offline — sales need a connection right now.
          </p>
        )}
        {resolveError && <p className="text-[13px] text-red-700 mb-3">{resolveError}</p>}
      </div>

      {activeBillId !== null ? (
        <BillWorkspace billId={activeBillId} onBillChanged={refreshOpenBills} />
      ) : (
        <div className="max-w-md mx-auto px-5">
          <div className="rounded-xl border border-jb-ink/10 bg-white p-1 flex gap-1 mb-3">
            {(['table', 'customer'] as PickerMode[]).map((m) => (
              <button
                key={m}
                onClick={() => selectPickerMode(m)}
                className={`flex-1 rounded-lg py-2 text-[13px] font-medium capitalize transition-colors ${
                  pickerMode === m ? 'bg-jb-ink text-jb-cream' : 'text-jb-ink/55'
                }`}
              >
                {m}
              </button>
            ))}
          </div>

          {pickerMode === 'table' ? (
            <div className="mb-4">
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
                      disabled={resolving || !newTableLabel.trim()}
                      className="rounded-lg bg-jb-ink text-jb-cream text-[12.5px] font-medium px-3 disabled:opacity-40"
                    >
                      Add
                    </button>
                  </div>
                )}
              </div>
            </div>
          ) : (
            <div className="mb-4">
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
                  No specific customer — for someone paying now and leaving. Search below only if
                  you want to name who this sale belongs to.
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
                      disabled={resolving || !newCustomerName.trim()}
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
          )}

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
            cartQuantities={{}}
            inCartLabel="just added"
            onAdd={(v) => void handleFirstItem(v)}
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
      )}

      <AppBottomNav />
    </div>
  );
}
