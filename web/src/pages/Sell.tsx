import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';

import { listProducts, type CatalogueProduct } from '../api/catalogue';
import { AppBottomNav } from '../components/AppBottomNav';
import { CartPanel, type CartLine } from '../components/CartPanel';
import { ProductGrid, type PickableVariant } from '../components/ProductGrid';
import { useConnectivity } from '../hooks/useConnectivity';
import { db } from '../lib/db';
import { describeActionError } from '../lib/errors';
import { flushPendingSales, queuePendingSale } from '../lib/salesSync';
import { useSession } from '../lib/session';

type PaymentMethod = 'cash' | 'transfer' | 'card';

function formatNaira(kobo: number): string {
  return (kobo / 100).toLocaleString();
}

export function Sell() {
  const { csrfToken, selectedBusinessId } = useSession();
  const isOnline = useConnectivity();

  const [products, setProducts] = useState<CatalogueProduct[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  // True when `products` came from IndexedDB, not a fresh fetch -- shown
  // as an honest "may be out of date" note rather than presented as
  // current (docs/ARCHITECTURE.md §10.7's fixed, honest sync states).
  const [usingCachedCatalogue, setUsingCachedCatalogue] = useState(false);
  const [cart, setCart] = useState<CartLine[]>([]);
  const [method, setMethod] = useState<PaymentMethod>('cash');
  const [done, setDone] = useState(false);
  const [submitting, setSubmitting] = useState(false);

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
  }, [selectedBusinessId]);

  useEffect(() => {
    if (!isOnline || !selectedBusinessId || !csrfToken) return;
    // Catches up on anything queued from an earlier offline session, and
    // on regaining connectivity mid-session -- see src/lib/salesSync.ts.
    void flushPendingSales(selectedBusinessId, csrfToken);
  }, [isOnline, selectedBusinessId, csrfToken]);

  const addOne = (variant: PickableVariant) => {
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
    setCart((c) =>
      c
        .map((l) => (l.variantId === variantId ? { ...l, quantity: l.quantity + delta } : l))
        .filter((l) => l.quantity > 0),
    );
  };

  const total = cart.reduce((sum, l) => sum + l.unitPriceKobo * l.quantity, 0);
  const cartQuantities = Object.fromEntries(cart.map((l) => [l.variantId, l.quantity]));

  async function completeSale() {
    if (!selectedBusinessId || cart.length === 0) return;
    setSubmitting(true);
    try {
      const idempotencyKey = crypto.randomUUID();
      // Written to IndexedDB first, before any network call -- this is
      // what makes "sale recorded" true even fully offline. Confirmation
      // below never waits on the network.
      await queuePendingSale({
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
      });
      setDone(true);
      if (isOnline && csrfToken) {
        void flushPendingSales(selectedBusinessId, csrfToken);
      }
    } finally {
      setSubmitting(false);
    }
  }

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

  if (done) {
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
          Saved on this device{isOnline ? ' and synced' : ''}. It will sync automatically once
          you&apos;re back online.
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
            <div className="text-[11px] text-jb-ink/45">Quick Sell</div>
            <h1 className="text-lg font-medium">Walk-in sale</h1>
          </div>
        </div>
      </div>

      {/* Top half: the product picker -- scrolls independently */}
      <div className="flex-1 min-h-0 overflow-y-auto">
        <div className="max-w-md mx-auto w-full px-5 pb-3">
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

      {/* Bottom half: what's been added for this customer -- its own
          scroll region, always visible so the sale-in-progress never
          disappears behind the product list. */}
      <CartPanel
        label="THIS SALE"
        lines={cart}
        emptyMessage="Tap a drink above to add it here."
        onChangeQty={changeQty}
        footer={
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
              disabled={submitting}
              className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-3.5 active:scale-[0.98] transition-transform disabled:opacity-40"
            >
              {submitting ? 'Saving…' : 'Complete Sale'}
            </button>
          </>
        }
      />

      {/* Reserves room for the fixed bottom nav below, matching the
          clearance every other app-shell page uses. */}
      <div className="h-28 shrink-0" />
      <AppBottomNav />
    </div>
  );
}
