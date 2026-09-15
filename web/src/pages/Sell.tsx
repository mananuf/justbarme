import { useState } from 'react';
import { Link } from 'react-router-dom';
import { AppBottomNav } from '../components/AppBottomNav';

const PRODUCTS = [
  { name: 'Guinness', size: '60cl', price: 1500 },
  { name: 'Heineken', size: '60cl', price: 1200 },
  { name: 'Star', size: '60cl', price: 900 },
  { name: 'Trophy', size: '60cl', price: 800 },
  { name: 'Coke', size: '50cl', price: 500 },
  { name: 'Water', size: '75cl', price: 300 },
];

type CartLine = { name: string; price: number; qty: number };

export function Sell() {
  const [cart, setCart] = useState<CartLine[]>([]);
  const [done, setDone] = useState(false);

  const addOne = (product: (typeof PRODUCTS)[number]) => {
    setCart((c) => {
      const existing = c.find((l) => l.name === product.name);
      if (existing) return c.map((l) => (l.name === product.name ? { ...l, qty: l.qty + 1 } : l));
      return [...c, { name: product.name, price: product.price, qty: 1 }];
    });
  };

  const changeQty = (name: string, delta: number) => {
    setCart((c) =>
      c.map((l) => (l.name === name ? { ...l, qty: l.qty + delta } : l)).filter((l) => l.qty > 0),
    );
  };

  const total = cart.reduce((sum, l) => sum + l.price * l.qty, 0);
  const itemCount = cart.reduce((sum, l) => sum + l.qty, 0);

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
            <h1 className="text-lg font-medium">Table 4</h1>
          </div>
        </div>

        <div className="grid grid-cols-2 gap-2.5">
          {PRODUCTS.map((p) => {
            const inCart = cart.find((l) => l.name === p.name);
            return (
              <button
                key={p.name}
                onClick={() => addOne(p)}
                className={`rounded-xl border p-4 text-left transition-colors active:scale-[0.97] ${
                  inCart
                    ? 'border-jb-ink bg-jb-ink text-jb-cream'
                    : 'border-jb-ink/10 bg-white text-jb-ink'
                }`}
              >
                <div className="text-[14px] font-medium">{p.name}</div>
                <div
                  className={`text-[11px] mt-0.5 ${inCart ? 'text-jb-cream/60' : 'text-jb-ink/45'}`}
                >
                  {p.size} · ₦{p.price}
                </div>
                {inCart && <div className="mt-2 text-[12px] font-medium">{inCart.qty} in sale</div>}
              </button>
            );
          })}
        </div>

        {cart.length > 0 && (
          <div className="mt-6">
            <div className="text-[11px] text-jb-ink/45 tracking-wide mb-2">THIS SALE</div>
            <div className="space-y-2">
              {cart.map((line) => (
                <div
                  key={line.name}
                  className="flex items-center justify-between rounded-xl border border-jb-ink/10 bg-white px-4 py-3"
                >
                  <div>
                    <div className="text-[13.5px] text-jb-ink/80">{line.name}</div>
                    <div className="text-[11px] text-jb-ink/40">₦{line.price} each</div>
                  </div>
                  <div className="flex items-center gap-3">
                    <button
                      onClick={() => changeQty(line.name, -1)}
                      className="w-7 h-7 rounded-full border border-jb-ink/20 flex items-center justify-center text-jb-ink"
                    >
                      −
                    </button>
                    <span className="text-[14px] font-medium w-4 text-center">{line.qty}</span>
                    <button
                      onClick={() => changeQty(line.name, 1)}
                      className="w-7 h-7 rounded-full border border-jb-ink/20 flex items-center justify-center text-jb-ink"
                    >
                      +
                    </button>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      {cart.length > 0 && (
        <div className="fixed bottom-[64px] inset-x-0 px-5 pb-3 bg-gradient-to-t from-jb-cream via-jb-cream to-transparent pt-6">
          <div className="max-w-md mx-auto">
            <div className="flex items-center justify-between text-[12px] text-jb-ink/55 mb-2 px-1">
              <span>
                {itemCount} item{itemCount === 1 ? '' : 's'}
              </span>
              <span className="text-jb-ink font-medium text-[15px]">₦{total.toLocaleString()}</span>
            </div>
            <button
              onClick={() => setDone(true)}
              className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 active:scale-[0.98] transition-transform"
            >
              Complete Sale
            </button>
          </div>
        </div>
      )}

      <AppBottomNav />
    </div>
  );
}
