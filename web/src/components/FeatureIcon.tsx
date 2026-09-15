export type FeatureIconType =
  'sales' | 'stock' | 'tables' | 'payments' | 'expenses' | 'staff' | 'reports' | 'offline';

// Plain black outline glyphs for the core-features grid — a deliberately
// simpler, more legible alternative to the animated PixelIcon canvas marks,
// which read as near-invisible at this size.
export function FeatureIcon({
  type,
  className = 'w-5 h-5',
}: {
  type: FeatureIconType;
  className?: string;
}) {
  const common = {
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: 1.6,
    strokeLinecap: 'round' as const,
    strokeLinejoin: 'round' as const,
    className,
  };

  switch (type) {
    case 'sales':
      return (
        <svg {...common}>
          <path d="M7 3h10l-1.1 12.4a4 4 0 0 1-4 3.6c-2.1 0-3.8-1.6-4-3.6L7 3Z" />
          <path d="M9.5 21h5" />
          <path d="M12 19v2" />
        </svg>
      );
    case 'stock':
      return (
        <svg {...common}>
          <path d="M12 3.5 20 8l-8 4.5L4 8l8-4.5Z" />
          <path d="M4 8v8l8 4.5 8-4.5V8" />
          <path d="M12 12.5V21" />
        </svg>
      );
    case 'tables':
      return (
        <svg {...common}>
          <rect x="3" y="6.5" width="18" height="3" rx="1" />
          <path d="M5.5 9.5V19" />
          <path d="M18.5 9.5V19" />
        </svg>
      );
    case 'payments':
      return (
        <svg {...common}>
          <rect x="3" y="6" width="18" height="13" rx="2" />
          <path d="M3 10.5h18" />
          <circle cx="16.5" cy="14.5" r="1" fill="currentColor" stroke="none" />
        </svg>
      );
    case 'expenses':
      return (
        <svg {...common}>
          <path d="M6.5 3h11v17l-2-1.4-1.75 1.4-1.75-1.4-1.75 1.4L8.5 18.6l-2 1.4V3Z" />
          <path d="M9 8h6" />
          <path d="M9 12h6" />
        </svg>
      );
    case 'staff':
      return (
        <svg {...common}>
          <circle cx="9" cy="8" r="3" />
          <path d="M3.5 20c0-3.3 2.5-6 5.5-6s5.5 2.7 5.5 6" />
          <circle cx="17" cy="9" r="2.3" />
          <path d="M15.8 14.3c2.3.4 4.1 2.6 4.2 5.7" />
        </svg>
      );
    case 'reports':
      return (
        <svg {...common}>
          <path d="M3 20h18" />
          <path d="M6 20v-6" />
          <path d="M12 20V6" />
          <path d="M18 20v-9" />
        </svg>
      );
    case 'offline':
      return (
        <svg {...common}>
          <path d="M2.5 8.8c2.6-2.1 5.9-3.4 9.5-3.4s6.9 1.3 9.5 3.4" />
          <path d="M5.8 12.9a10 10 0 0 1 12.4 0" />
          <path d="M9 16.8a5 5 0 0 1 6 0" />
          <circle cx="12" cy="19.6" r="1" fill="currentColor" stroke="none" />
          <path d="M3 3l18 18" />
        </svg>
      );
  }
}
