// Split out from DashboardTour.tsx purely so that component file only
// exports a component (react-refresh/only-export-components).
//
// Deliberately longer than the original four-step version: a lot of real
// functionality has landed since that first pass (out-of-stock protection,
// negative-inventory/price reviews, standard sizing and quick pricing on
// Stock, per-drink profit margin, a restocking-spend report, staff
// invitations, and in-place product editing), and none of it is
// discoverable just by staring at the Dashboard. Owner-only steps
// (#more-reports, #more-settings) simply never resolve for a Staff viewer
// and get skipped automatically -- see DashboardTour's own polling logic.
export const TOUR_STEPS = [
  {
    target: '#quick-sell',
    title: 'Welcome — record a sale in one tap',
    body: "Tap Quick Sell whenever a drink goes out. It works even without a connection, syncing once you're back online — and if something's actually run out, we stop you right on the tile instead of letting it oversell.",
  },
  {
    target: '#alerts',
    title: 'Nothing slips through unnoticed',
    body: "A sale at a stale price, stock that's gone negative, or a staff adjustment waiting on a decision — all land here for you to review. Nothing gets silently changed or rejected on its own.",
  },
  {
    target: '#bills',
    title: 'Tabs and running credit',
    body: 'Open a tab for a table or a regular, add rounds as they order, and settle up later — partial payments and write-offs included.',
  },
  {
    target: '#nav-stock',
    title: 'Restock, count, and price — all in one place',
    body: "Pick a standard size (33cl to 1.5L) or enter your own, adjust a selling price right from the restock list, and tap the eye icon on any drink to see and edit its details. Something that isn't a drink, like a snooker table? Mark it as a service — no stock tracking needed.",
  },
  {
    target: '#more-reports',
    title: 'See your real numbers',
    body: 'Sales, top products, staff performance, and expenses — plus real profit margin per drink, tracked automatically as stock comes in and gets sold, and exactly how much you spent restocking, searchable by product and date.',
  },
  {
    target: '#more-team',
    title: 'Bring your staff in',
    body: 'Invite by WhatsApp or email — they get their own login, with only the access their role actually needs.',
  },
  {
    target: '#more-settings',
    title: 'Fine-tune anytime',
    body: 'Rename a product, adjust a price, change a size, or turn a drink into a service — none of it requires restocking first.',
  },
  {
    target: '#nav-more',
    title: 'Everything else lives here',
    body: 'Activity feed, expenses, and installing justbarme on your phone are all one tap away under More.',
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
