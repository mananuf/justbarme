// Per-device convenience: remembers the last "bottles per crate" entered
// for a given variant, so restocking the same product doesn't require
// retyping it every time. Not synced anywhere and not worth a backend
// column for this -- same reasoning as dashboardTour.ts's "seen" flag.
const KEY_PREFIX = 'jb_crate_size_';

export function getRememberedCrateSize(variantId: string): string {
  try {
    return localStorage.getItem(KEY_PREFIX + variantId) ?? '';
  } catch {
    return '';
  }
}

export function setRememberedCrateSize(variantId: string, crateSize: string): void {
  try {
    localStorage.setItem(KEY_PREFIX + variantId, crateSize);
  } catch {
    // Best-effort convenience only -- a private-mode browser or a full
    // storage quota just means the next visit re-asks, which is harmless.
  }
}
