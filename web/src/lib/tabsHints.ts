// Two small, one-time explainers on the Tabs screen, each shown until
// dismissed once and never again -- same localStorage "seen" pattern as
// dashboardTour.ts, just two independent flags since each teaches a
// different habit at a different moment:
//
// - The payment hint: shown next to "RECORD A PAYMENT" whenever an open
//   bill has never had a payment recorded, explaining that entering what
//   was paid is what makes "Close tab" appear (see Tabs.tsx's own gating
//   of that button on payments.length > 0).
// - The void hint: pops once, the first time a new tab is ever opened,
//   explaining what "Void tab" is for -- a usability fix requested after
//   staff conflated "closing" a tab with "voiding" one.

function hasSeen(key: string): boolean {
  try {
    return localStorage.getItem(key) === 'true';
  } catch {
    // Private browsing or storage disabled -- show the hint every time
    // rather than crash; a repeated hint is a minor annoyance, not a bug.
    return false;
  }
}

function markSeen(key: string): void {
  try {
    localStorage.setItem(key, 'true');
  } catch {
    // Nothing to do if storage is unavailable -- the hint will just show
    // again next time, which is an acceptable degradation.
  }
}

const PAYMENT_HINT_KEY = 'jb_tabs_payment_hint_seen';
const VOID_HINT_KEY = 'jb_tabs_void_hint_seen';

export function hasSeenPaymentHint(): boolean {
  return hasSeen(PAYMENT_HINT_KEY);
}

export function markPaymentHintSeen(): void {
  markSeen(PAYMENT_HINT_KEY);
}

export function hasSeenVoidHint(): boolean {
  return hasSeen(VOID_HINT_KEY);
}

export function markVoidHintSeen(): void {
  markSeen(VOID_HINT_KEY);
}
