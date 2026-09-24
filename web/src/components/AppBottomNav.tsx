import { Link, useLocation } from 'react-router-dom';

const NAV_ITEMS = [
  { label: 'Home', href: '/dashboard', icon: 'home', id: 'nav-home' },
  { label: 'Sell', href: '/dashboard/sell', icon: 'sell', id: 'nav-sell' },
  { label: 'Stock', href: '/dashboard/stock', icon: 'stock', id: 'nav-stock' },
  { label: 'Bills', href: '/dashboard/tabs', icon: 'bills', id: 'nav-bills' },
  { label: 'More', href: '/dashboard#more', icon: 'more', id: 'nav-more' },
] as const;

// Every page the More accordion links to -- landing on any of these (not
// just the accordion being open on /dashboard itself) should show "More"
// as the active tab, the same way "Sell" stays active for the whole
// /dashboard/sell subtree.
const MORE_PATHS = [
  '/dashboard/team',
  '/dashboard/reviews',
  '/dashboard/expenses',
  '/dashboard/activity',
  '/dashboard/reports',
  '/dashboard/settings',
];

function Glyph({ icon, active, onDark }: { icon: string; active?: boolean; onDark?: boolean }) {
  const stroke = onDark ? '#F4F1E8' : active ? '#16201A' : 'rgba(22,32,26,0.45)';
  const common = {
    width: 19,
    height: 19,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke,
    strokeWidth: 1.7,
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
          <circle cx="5" cy="12" r="1.5" fill={stroke} />
          <circle cx="12" cy="12" r="1.5" fill={stroke} />
          <circle cx="19" cy="12" r="1.5" fill={stroke} />
        </svg>
      );
  }
}

export function AppBottomNav() {
  const { pathname } = useLocation();
  return (
    <nav className="fixed bottom-0 inset-x-0 z-40 border-t border-jb-ink/[0.08] bg-jb-cream/95 backdrop-blur px-2 pt-2 pb-[calc(env(safe-area-inset-bottom)+0.5rem)]">
      <div className="max-w-md mx-auto flex items-stretch justify-between">
        {NAV_ITEMS.map((item) => {
          const active =
            item.label === 'Home'
              ? pathname === '/dashboard'
              : item.label === 'Sell'
                ? pathname.startsWith('/dashboard/sell')
                : item.label === 'Stock'
                  ? pathname.startsWith('/dashboard/stock')
                  : item.label === 'Bills'
                    ? pathname.startsWith('/dashboard/tabs')
                    : item.label === 'More'
                      ? MORE_PATHS.some((p) => pathname.startsWith(p))
                      : false;
          return (
            <Link
              key={item.label}
              id={item.id}
              to={item.href}
              className="flex-1 flex flex-col items-center gap-1 py-1"
            >
              <div
                className={
                  active
                    ? 'w-11 h-11 rounded-full bg-jb-ink flex items-center justify-center -mt-4 shadow-[0_6px_14px_rgba(18,26,20,0.35)]'
                    : 'w-8 h-8 rounded-lg flex items-center justify-center'
                }
              >
                <Glyph icon={item.icon} active={active} onDark={active} />
              </div>
              <span
                className={`text-[10px] ${active ? 'text-jb-ink font-medium' : 'text-jb-ink/40'}`}
              >
                {item.label}
              </span>
            </Link>
          );
        })}
      </div>
    </nav>
  );
}
