// Split out from DashboardTour.tsx purely so that component file only
// exports a component (react-refresh/only-export-components).
export const TOUR_STEPS = [
  {
    target: '#quick-sell',
    title: 'Record a sale in one tap',
    body: 'Tap Quick Sell whenever a drink goes out. It works even without a connection.',
  },
  {
    target: '#stock',
    title: 'Keep an eye on stock',
    body: "See what's running low before you run out, right from here.",
  },
  {
    target: '#bills',
    title: 'Track open tabs',
    body: 'Outstanding bills and running tabs for regulars live here.',
  },
  {
    target: '#more',
    title: 'Everything else',
    body: 'Expenses, reports, activity, and installing justbarme on your phone are all under More.',
  },
] as const;

const SEEN_KEY = 'jb_dashboard_tour_seen';

export function hasSeenDashboardTour(): boolean {
  try {
    return localStorage.getItem(SEEN_KEY) === 'true';
  } catch {
    // Private browsing or storage disabled -- show the tour every time
    // rather than crash; a repeated tour is a minor annoyance, not a bug.
    return false;
  }
}

export function markDashboardTourSeen(): void {
  try {
    localStorage.setItem(SEEN_KEY, 'true');
  } catch {
    // Nothing to do if storage is unavailable -- the tour will just show
    // again next time, which is an acceptable degradation.
  }
}
