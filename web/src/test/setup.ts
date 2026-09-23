import '@testing-library/jest-dom/vitest';
// jsdom does not implement IndexedDB. Dexie (src/lib/db.ts) needs one to
// exist at all -- this polyfills window.indexedDB/IDBKeyRange globally.
import 'fake-indexeddb/auto';
import { cleanup } from '@testing-library/react';
import { afterEach, vi } from 'vitest';

import { db } from '../lib/db';

afterEach(async () => {
  cleanup();
  // fake-indexeddb persists across tests in the same file by default --
  // without this, a cached identity from one test leaks into the next.
  await db.authMeta.clear();
  await db.device.clear();
  await db.pendingSales.clear();
  await db.catalogueCache.clear();
  // Same reasoning for jsdom's localStorage (e.g. the dashboard tour's
  // "seen" flag).
  localStorage.clear();
});

// jsdom does not implement IntersectionObserver. Scroll-triggered reveal
// components (RevealText, the landing page's BentoCard) only need it to
// exist and be inert under tests — real behavior is exercised by Playwright.
class MockIntersectionObserver implements IntersectionObserver {
  readonly root: Element | Document | null = null;
  readonly rootMargin: string = '';
  readonly scrollMargin: string = '';
  readonly thresholds: ReadonlyArray<number> = [];
  observe = () => undefined;
  unobserve = () => undefined;
  disconnect = () => undefined;
  takeRecords = (): IntersectionObserverEntry[] => [];
}

vi.stubGlobal('IntersectionObserver', MockIntersectionObserver);

// jsdom does not implement scrollIntoView. DashboardTour calls it (inside
// a requestAnimationFrame callback, so it can fire after the test that
// triggered it has already finished) to center the spotlighted element --
// only needs to exist and be inert under tests, same reasoning as
// IntersectionObserver above. Without this, that later callback throws an
// unhandled exception that fails the whole run even though every test
// itself already passed.
if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => undefined;
}
