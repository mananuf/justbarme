import { useState } from 'react';

// ── Shared bits ────────────────────────────────────────────────────────────

function ScreenHeader({ title, sub }: { title: string; sub?: string }) {
  return (
    <div className="px-4 pt-3 pb-2">
      <div className="text-[15px] font-medium text-jb-ink">{title}</div>
      {sub && <div className="text-[11px] text-jb-ink/45 mt-0.5">{sub}</div>}
    </div>
  );
}

function SyncDot({ state = 'synced' }: { state?: 'synced' | 'pending' | 'offline' }) {
  const color =
    state === 'synced' ? 'bg-emerald-600' : state === 'pending' ? 'bg-jb-gold' : 'bg-jb-ink/30';
  return (
    <span className="relative flex h-2 w-2">
      {state !== 'offline' && (
        <span
          className={`animate-ping absolute inline-flex h-full w-full rounded-full ${color} opacity-50`}
        />
      )}
      <span className={`relative inline-flex rounded-full h-2 w-2 ${color}`} />
    </span>
  );
}

function BottomNav({ active = 'Home' }: { active?: string }) {
  const items = [
    { label: 'Home', icon: 'home' },
    { label: 'Sell', icon: 'sell' },
    { label: 'Stock', icon: 'stock' },
    { label: 'Bills', icon: 'bills' },
    { label: 'More', icon: 'more' },
  ] as const;
  return (
    <div className="mt-auto border-t border-jb-ink/[0.08] bg-jb-cream/95 px-1 pt-1.5 pb-2.5 flex items-stretch justify-between">
      {items.map((item) => {
        const isActive = item.label === active;
        const isSell = item.label === 'Sell';
        return (
          <div key={item.label} className="flex-1 flex flex-col items-center gap-1">
            <div
              className={
                isSell
                  ? 'w-9 h-9 rounded-full bg-jb-green-dark flex items-center justify-center -mt-3 shadow-[0_4px_10px_rgba(18,26,20,0.35)]'
                  : `w-6 h-6 rounded-md flex items-center justify-center ${isActive ? 'bg-jb-ink/10' : ''}`
              }
            >
              <NavGlyph icon={item.icon} active={isActive} onDark={isSell} />
            </div>
            <span
              className={`text-[9px] ${isActive ? 'text-jb-ink font-medium' : 'text-jb-ink/40'}`}
            >
              {item.label}
            </span>
          </div>
        );
      })}
    </div>
  );
}

function NavGlyph({ icon, active, onDark }: { icon: string; active?: boolean; onDark?: boolean }) {
  const stroke = onDark ? '#F4F1E8' : active ? '#16201A' : 'rgba(22,32,26,0.45)';
  const common = {
    width: 15,
    height: 15,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke,
    strokeWidth: 1.8,
  } as const;
  switch (icon) {
    case 'home':
      return (
        <svg {...common}>
          <path d="M3 11.5 12 4l9 7.5" />
          <path d="M5 10v9h14v-9" />
        </svg>
      );
    case 'sell':
      return (
        <svg {...common}>
          <path d="M12 5v14M5 12h14" />
        </svg>
      );
    case 'stock':
      return (
        <svg {...common}>
          <rect x="3" y="9" width="8" height="11" rx="1" />
          <rect x="13" y="4" width="8" height="16" rx="1" />
        </svg>
      );
    case 'bills':
      return (
        <svg {...common}>
          <rect x="5" y="3" width="14" height="18" rx="2" />
          <path d="M9 8h6M9 12h6M9 16h3" />
        </svg>
      );
    default:
      return (
        <svg {...common}>
          <circle cx="5" cy="12" r="1.4" fill={stroke} />
          <circle cx="12" cy="12" r="1.4" fill={stroke} />
          <circle cx="19" cy="12" r="1.4" fill={stroke} />
        </svg>
      );
  }
}

function Pill({
  children,
  tone = 'neutral',
}: {
  children: React.ReactNode;
  tone?: 'neutral' | 'warn' | 'good';
}) {
  const tones = {
    neutral: 'bg-jb-ink/[0.06] text-jb-ink/60',
    warn: 'bg-jb-gold/20 text-[#7a5b1f]',
    good: 'bg-emerald-600/10 text-emerald-700',
  };
  return (
    <span
      className={`text-[10px] px-2 py-0.5 rounded-full font-medium tracking-wide ${tones[tone]}`}
    >
      {children}
    </span>
  );
}

// ── 1. Dashboard preview (hero + features) ──────────────────────────────────

export function DashboardPreview() {
  return (
    <div className="flex flex-col h-full">
      <div className="px-4 pt-1 flex items-center justify-between">
        <div>
          <div className="text-[11px] text-jb-ink/45">The Place</div>
          <div className="text-[15px] font-medium text-jb-ink">Good evening 👋</div>
        </div>
        <div className="flex items-center gap-1.5 text-[10px] text-jb-ink/50">
          <SyncDot state="pending" />
          Waiting to sync
        </div>
      </div>

      <div className="px-4 mt-3">
        <div className="rounded-2xl bg-jb-green-dark text-jb-cream p-4">
          <div className="text-[11px] text-jb-cream/60 tracking-wide">TODAY&apos;S SALES</div>
          <div className="text-[26px] font-light mt-1">₦48,200</div>
          <div className="flex gap-4 mt-3 text-[11px] text-jb-cream/70">
            <span>32 items sold</span>
            <span>·</span>
            <span>4 tabs open</span>
          </div>
        </div>
      </div>

      <div className="px-4 mt-3">
        <button className="w-full rounded-xl bg-jb-ink text-jb-cream text-[13px] font-medium py-3 flex items-center justify-center gap-2 active:scale-[0.98] transition-transform">
          <span className="text-lg leading-none">+</span> Quick Sell
        </button>
      </div>

      <div className="px-4 mt-3 grid grid-cols-2 gap-2">
        <div className="rounded-xl bg-white/70 border border-jb-ink/[0.07] p-3">
          <div className="text-[10px] text-jb-ink/45">Stock alerts</div>
          <div className="text-[13px] font-medium text-jb-ink mt-1">3 items low</div>
          <div className="mt-1.5">
            <Pill tone="warn">Gordon&apos;s 75cl</Pill>
          </div>
        </div>
        <div className="rounded-xl bg-white/70 border border-jb-ink/[0.07] p-3">
          <div className="text-[10px] text-jb-ink/45">Outstanding</div>
          <div className="text-[13px] font-medium text-jb-ink mt-1">₦11,400</div>
          <div className="mt-1.5">
            <Pill>2 open tabs</Pill>
          </div>
        </div>
      </div>

      <div className="px-4 mt-3 flex-1">
        <div className="text-[10px] text-jb-ink/45 tracking-wide mb-1.5">RECENT ACTIVITY</div>
        <div className="space-y-1.5">
          {[
            { who: 'Tolu', what: 'Recorded a sale — Table 4', amt: '₦3,600' },
            { who: 'You', what: 'Received stock — Heineken', amt: '+24' },
            { who: 'Ada', what: 'Recorded payment — Chidi', amt: '₦5,000' },
          ].map((row) => (
            <div
              key={row.what}
              className="flex items-center justify-between rounded-lg px-2.5 py-2 bg-white/50"
            >
              <div>
                <div className="text-[11.5px] text-jb-ink/80">{row.what}</div>
                <div className="text-[10px] text-jb-ink/40">{row.who}</div>
              </div>
              <div className="text-[11.5px] text-jb-ink/60">{row.amt}</div>
            </div>
          ))}
        </div>
      </div>

      <BottomNav active="Home" />
    </div>
  );
}

// ── 2. Quick sell flow (select → quantity → done) ───────────────────────────

const DRINKS = [
  { name: 'Guinness', size: '60cl', price: 1500 },
  { name: 'Heineken', size: '60cl', price: 1200 },
  { name: 'Star', size: '60cl', price: 900 },
  { name: 'Coke', size: '50cl', price: 500 },
];

export function QuickSellPreview() {
  const [qty, setQty] = useState(2);
  const [selected, setSelected] = useState(0);
  return (
    <div className="flex flex-col h-full">
      <ScreenHeader title="Quick Sell" sub="Table 4" />
      <div className="px-4 grid grid-cols-2 gap-2">
        {DRINKS.map((d, i) => (
          <button
            key={d.name}
            onClick={() => setSelected(i)}
            className={`rounded-xl border p-2.5 text-left transition-colors ${
              i === selected
                ? 'border-jb-ink bg-jb-ink text-jb-cream'
                : 'border-jb-ink/[0.1] bg-white/70 text-jb-ink'
            }`}
          >
            <div className="text-[12px] font-medium">{d.name}</div>
            <div
              className={`text-[10px] ${i === selected ? 'text-jb-cream/60' : 'text-jb-ink/45'}`}
            >
              {d.size} · ₦{d.price}
            </div>
          </button>
        ))}
      </div>

      <div className="px-4 mt-4">
        <div className="rounded-xl border border-jb-ink/[0.1] bg-white/70 p-3 flex items-center justify-between">
          <span className="text-[12px] text-jb-ink/60">Quantity</span>
          <div className="flex items-center gap-3">
            <button
              onClick={() => setQty((q) => Math.max(1, q - 1))}
              className="w-7 h-7 rounded-full border border-jb-ink/20 flex items-center justify-center text-jb-ink"
            >
              −
            </button>
            <span className="text-[14px] font-medium w-4 text-center">{qty}</span>
            <button
              onClick={() => setQty((q) => q + 1)}
              className="w-7 h-7 rounded-full border border-jb-ink/20 flex items-center justify-center text-jb-ink"
            >
              +
            </button>
          </div>
        </div>
      </div>

      <div className="px-4 mt-auto pb-4 pt-4">
        <div className="flex items-center justify-between text-[12px] text-jb-ink/60 mb-2">
          <span>Total</span>
          <span className="text-jb-ink font-medium text-[14px]">
            ₦{(DRINKS[selected]!.price * qty).toLocaleString()}
          </span>
        </div>
        <button className="w-full rounded-xl bg-jb-ink text-jb-cream text-[13px] font-medium py-3 active:scale-[0.98] transition-transform">
          Complete Sale
        </button>
      </div>
    </div>
  );
}

// ── 3. Stock receipt ─────────────────────────────────────────────────────────

export function StockReceivePreview() {
  return (
    <div className="flex flex-col h-full">
      <ScreenHeader title="Receive Stock" sub="From: Main distributor" />
      <div className="px-4 space-y-2">
        {[
          { name: 'Heineken 60cl', qty: 24, cost: '9,600' },
          { name: 'Star 60cl', qty: 48, cost: '16,800' },
        ].map((row) => (
          <div key={row.name} className="rounded-xl border border-jb-ink/[0.1] bg-white/70 p-3">
            <div className="flex items-center justify-between">
              <span className="text-[12.5px] font-medium text-jb-ink">{row.name}</span>
              <span className="text-[11px] text-jb-ink/45">+{row.qty} units</span>
            </div>
            <div className="text-[10.5px] text-jb-ink/40 mt-1">Cost: ₦{row.cost}</div>
          </div>
        ))}
        <button className="w-full rounded-xl border border-dashed border-jb-ink/20 text-jb-ink/50 text-[12px] py-2.5">
          + Add another product
        </button>
      </div>
      <div className="px-4 mt-auto pb-4 pt-4">
        <button className="w-full rounded-xl bg-jb-ink text-jb-cream text-[13px] font-medium py-3">
          Save Receipt
        </button>
      </div>
    </div>
  );
}

// ── 4. Outstanding bill / tab ────────────────────────────────────────────────

export function OutstandingBillPreview() {
  return (
    <div className="flex flex-col h-full">
      <ScreenHeader title="Chidi — Table 2" sub="Open tab · started 6:40pm" />
      <div className="px-4 space-y-1.5">
        {[
          { item: 'Hennessy VS (2x)', amt: '₦18,000' },
          { item: 'Sprite (3x)', amt: '₦1,500' },
          { item: 'Payment received', amt: '− ₦8,100' },
        ].map((row) => (
          <div key={row.item} className="flex items-center justify-between px-1 py-1.5 text-[12px]">
            <span className="text-jb-ink/70">{row.item}</span>
            <span className="text-jb-ink/60">{row.amt}</span>
          </div>
        ))}
      </div>
      <div className="px-4 mt-3">
        <div className="rounded-xl bg-jb-gold/15 border border-jb-gold/30 p-3 flex items-center justify-between">
          <span className="text-[12px] text-[#7a5b1f] font-medium">Outstanding balance</span>
          <span className="text-[16px] font-semibold text-[#7a5b1f]">₦11,400</span>
        </div>
      </div>
      <div className="px-4 mt-auto pb-4 pt-4 flex gap-2">
        <button className="flex-1 rounded-xl border border-jb-ink/15 text-jb-ink text-[12.5px] font-medium py-3">
          Share Bill
        </button>
        <button className="flex-1 rounded-xl bg-jb-ink text-jb-cream text-[12.5px] font-medium py-3">
          Record Payment
        </button>
      </div>
    </div>
  );
}

// ── 5. Branded bill / receipt ────────────────────────────────────────────────

export function BrandedBillPreview() {
  return (
    <div className="flex flex-col h-full items-stretch px-4 pt-4 pb-4">
      <div className="rounded-2xl border border-jb-ink/[0.1] bg-white p-5 flex-1 flex flex-col">
        <div className="text-center">
          <div className="font-pixel text-[13px] tracking-[0.2em] text-jb-ink">THE PLACE</div>
          <div className="text-[10px] text-jb-ink/40 mt-1">
            14 Adeola Street, Lekki · 0803 000 0000
          </div>
        </div>
        <div className="mt-4 border-t border-dashed border-jb-ink/15 pt-3 space-y-1.5 text-[11.5px]">
          <div className="flex justify-between text-jb-ink/70">
            <span>Hennessy VS ×2</span>
            <span>₦18,000</span>
          </div>
          <div className="flex justify-between text-jb-ink/70">
            <span>Sprite ×3</span>
            <span>₦1,500</span>
          </div>
          <div className="flex justify-between text-jb-ink/50">
            <span>Paid</span>
            <span>− ₦8,100</span>
          </div>
        </div>
        <div className="mt-4 border-t border-dashed border-jb-ink/15 pt-3 text-center">
          <div className="text-[10px] text-jb-ink/45 tracking-wide">OUTSTANDING BALANCE</div>
          <div className="text-[24px] font-light text-jb-ink mt-1">₦11,400</div>
        </div>
        <div className="mt-auto pt-4 text-center text-[9.5px] text-jb-ink/35">
          Pay via transfer — GTBank · 0123456789 · The Place Ltd
        </div>
      </div>
      <button className="mt-3 w-full rounded-xl bg-jb-ink text-jb-cream text-[13px] font-medium py-3">
        Share Bill
      </button>
    </div>
  );
}

// ── 6. Catalogue / product setup ─────────────────────────────────────────────

const CATALOGUE_ITEMS = [
  'Guinness',
  'Star',
  'Heineken',
  'Trophy',
  'Jameson',
  'Hennessy',
  "Gordon's",
  'Coke',
  'Fanta',
  'Sprite',
  'Water',
];

export function CatalogueSetupPreview() {
  const [picked, setPicked] = useState<string[]>(['Guinness', 'Star', 'Heineken', 'Coke', 'Water']);
  const toggle = (name: string) =>
    setPicked((p) => (p.includes(name) ? p.filter((x) => x !== name) : [...p, name]));
  return (
    <div className="flex flex-col h-full">
      <ScreenHeader title="Choose what you sell" sub={`${picked.length} selected`} />
      <div className="px-4 flex flex-wrap gap-2 content-start flex-1">
        {CATALOGUE_ITEMS.map((name) => {
          const active = picked.includes(name);
          return (
            <button
              key={name}
              onClick={() => toggle(name)}
              className={`px-3 py-2 rounded-full text-[12px] border transition-colors flex items-center gap-1.5 ${
                active
                  ? 'bg-jb-ink border-jb-ink text-jb-cream'
                  : 'border-jb-ink/15 text-jb-ink/60 bg-white/60'
              }`}
            >
              {active && <span className="text-[10px]">✓</span>}
              {name}
            </button>
          );
        })}
        <button className="px-3 py-2 rounded-full text-[12px] border border-dashed border-jb-ink/25 text-jb-ink/45">
          + Custom
        </button>
      </div>
      <div className="px-4 mt-auto pb-4 pt-4">
        <button className="w-full rounded-xl bg-jb-ink text-jb-cream text-[13px] font-medium py-3">
          Continue
        </button>
      </div>
    </div>
  );
}

// ── 7. Activity timeline ─────────────────────────────────────────────────────

export function ActivityPreview() {
  const rows = [
    { who: 'Ada', what: 'closed Table 3', time: '7:42pm', tone: 'neutral' as const },
    { who: 'Tolu', what: 'recorded a ₦3,600 sale', time: '7:38pm', tone: 'good' as const },
    {
      who: 'System',
      what: 'flagged a stock count mismatch',
      time: '7:20pm',
      tone: 'warn' as const,
    },
    { who: 'You', what: 'received 24 units of Heineken', time: '6:55pm', tone: 'neutral' as const },
  ];
  return (
    <div className="flex flex-col h-full">
      <ScreenHeader title="Activity" sub="Today" />
      <div className="px-4 space-y-2">
        {rows.map((r) => (
          <div key={r.what} className="flex items-start gap-2.5">
            <div className="w-1.5 h-1.5 rounded-full bg-jb-ink/25 mt-1.5 shrink-0" />
            <div className="flex-1">
              <div className="text-[11.5px] text-jb-ink/75">
                <span className="font-medium text-jb-ink">{r.who}</span> {r.what}
              </div>
              <div className="text-[10px] text-jb-ink/35 mt-0.5">{r.time}</div>
            </div>
            {r.tone === 'warn' && <Pill tone="warn">Review</Pill>}
          </div>
        ))}
      </div>
    </div>
  );
}

// ── 8. Offline / sync status badge (used inline in copy, not phone-framed) ──

export function OfflineSyncBadge() {
  return (
    <div className="inline-flex items-center gap-2 rounded-full border border-jb-ink/10 bg-white/70 px-3.5 py-2 text-[12px] text-jb-ink/60">
      <SyncDot state="pending" />
      Waiting to sync · 3 sales · reconnecting…
    </div>
  );
}
