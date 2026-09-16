import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import { listProducts, type CatalogueProduct } from '../api/catalogue';
import { AppBottomNav } from '../components/AppBottomNav';
import { useConnectivity } from '../hooks/useConnectivity';
import { flushPendingSales, queuePendingSale } from '../lib/salesSync';
import { useSession } from '../lib/session';

type PaymentMethod = 'cash' | 'transfer' | 'card';

type CartLine = {
  variantId: string;
  description: string;
  unitPriceKobo: number;
  quantity: number;
};

function formatNaira(kobo: number): string {
  return (kobo / 100).toLocaleString();
}

export function Sell() {
  const { csrfToken, selectedBusinessId } = useSession();
  const isOnline = useConnectivity();

  const [products, setProducts] = useState<CatalogueProduct[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [cart, setCart] = useState<CartLine[]>([]);
  const [method, setMethod] = useState<PaymentMethod>('cash');
  const [done, setDone] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!selectedBusinessId) return;
    listProducts(selectedBusinessId)
      .then(setProducts)
      .catch(() =>
        setLoadError('Could not load your products. Check your connection and try again.'),
      );
  }, [selectedBusinessId]);

  useEffect(() => {
    if (!isOnline || !selectedBusinessId || !csrfToken) return;
    // Catches up on anything queued from an earlier offline session, and
    // on regaining connectivity mid-session -- see src/lib/salesSync.ts.
    void flushPendingSales(selectedBusinessId, csrfToken);
  }, [isOnline, selectedBusinessId, csrfToken]);

  const addOne = (variantId: string, description: string, unitPriceKobo: number) => {
    setCart((c) => {
      const existing = c.find((l) => l.variantId === variantId);
      if (existing)
        return c.map((l) => (l.variantId === variantId ? { ...l, quantity: l.quantity + 1 } : l));
      return [...c, { variantId, description, unitPriceKobo, quantity: 1 }];
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
  const itemCount = cart.reduce((sum, l) => sum + l.quantity, 0);

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

  const variants = (products ?? []).flatMap((p) =>
    p.variants
      .filter((v) => v.active)
      .map((v) => ({
        variantId: v.id,
        description: `${p.name} — ${v.name}`,
        priceKobo: v.currentPriceKobo,
      })),
  );

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink pb-40">
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
            <div className="text-[11px] text-jb-ink/45">Quick Sell</div>
            <h1 className="text-lg font-medium">Walk-in sale</h1>
          </div>
        </div>

        {loadError && (
          <p role="alert" className="text-[13px] text-red-700 mb-4">
            {loadError}
          </p>
        )}
        {products === null && !loadError && (
          <p className="text-[13px] text-jb-ink/45">Loading products…</p>
        )}
        {products !== null && variants.length === 0 && (
          <p className="text-[13px] text-jb-ink/45">
            You haven&apos;t stocked anything yet.{' '}
            <Link to="/dashboard/stock" className="underline">
              Add stock
            </Link>{' '}
            to start selling.
          </p>
        )}

        <div className="grid grid-cols-2 gap-2.5">
          {variants.map((v) => {
            const inCart = cart.find((l) => l.variantId === v.variantId);
            return (
              <button
                key={v.variantId}
                onClick={() => addOne(v.variantId, v.description, v.priceKobo)}
                className={`rounded-xl border p-4 text-left transition-colors active:scale-[0.97] ${
                  inCart
                    ? 'border-jb-ink bg-jb-ink text-jb-cream'
                    : 'border-jb-ink/10 bg-white text-jb-ink'
                }`}
              >
                <div className="text-[14px] font-medium">{v.description}</div>
                <div
                  className={`text-[11px] mt-0.5 ${inCart ? 'text-jb-cream/60' : 'text-jb-ink/45'}`}
                >
                  ₦{formatNaira(v.priceKobo)}
                </div>
                {inCart && (
                  <div className="mt-2 text-[12px] font-medium">{inCart.quantity} in sale</div>
                )}
              </button>
            );
          })}
        </div>

        {cart.length > 0 && (
          <>
            <div className="mt-6">
              <div className="text-[11px] text-jb-ink/45 tracking-wide mb-2">THIS SALE</div>
              <div className="space-y-2">
                {cart.map((line) => (
                  <div
                    key={line.variantId}
                    className="flex items-center justify-between rounded-xl border border-jb-ink/10 bg-white px-4 py-3"
                  >
                    <div>
                      <div className="text-[13.5px] text-jb-ink/80">{line.description}</div>
                      <div className="text-[11px] text-jb-ink/40">
                        ₦{formatNaira(line.unitPriceKobo)} each
                      </div>
                    </div>
                    <div className="flex items-center gap-3">
                      <button
                        onClick={() => changeQty(line.variantId, -1)}
                        className="w-7 h-7 rounded-full border border-jb-ink/20 flex items-center justify-center text-jb-ink"
                      >
                        −
                      </button>
                      <span className="text-[14px] font-medium w-4 text-center">
                        {line.quantity}
                      </span>
                      <button
                        onClick={() => changeQty(line.variantId, 1)}
                        className="w-7 h-7 rounded-full border border-jb-ink/20 flex items-center justify-center text-jb-ink"
                      >
                        +
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            </div>

            <div className="mt-4">
              <div className="text-[11px] text-jb-ink/45 tracking-wide mb-2">PAID BY</div>
              <div className="flex gap-2 p-1 rounded-xl bg-jb-ink/[0.05]">
                {(['cash', 'transfer', 'card'] as PaymentMethod[]).map((m) => (
                  <button
                    key={m}
                    onClick={() => setMethod(m)}
                    className={`flex-1 rounded-lg py-2.5 text-[13px] font-medium capitalize transition-colors ${
                      method === m ? 'bg-white text-jb-ink shadow-sm' : 'text-jb-ink/45'
                    }`}
                  >
                    {m}
                  </button>
                ))}
              </div>
            </div>
          </>
        )}
      </div>

      {cart.length > 0 && (
        <div className="fixed bottom-[64px] inset-x-0 px-5 pb-3 bg-gradient-to-t from-jb-cream via-jb-cream to-transparent pt-6">
          <div className="max-w-md mx-auto">
            <div className="flex items-center justify-between text-[12px] text-jb-ink/55 mb-2 px-1">
              <span>
                {itemCount} item{itemCount === 1 ? '' : 's'}
              </span>
              <span className="text-jb-ink font-medium text-[15px]">₦{formatNaira(total)}</span>
            </div>
            <button
              onClick={() => void completeSale()}
              disabled={submitting}
              className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 active:scale-[0.98] transition-transform disabled:opacity-40"
            >
              {submitting ? 'Saving…' : 'Complete Sale'}
            </button>
          </div>
        </div>
      )}

      <AppBottomNav />
    </div>
  );
}
