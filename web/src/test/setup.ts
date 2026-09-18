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
