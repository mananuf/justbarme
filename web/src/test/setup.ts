import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach, vi } from 'vitest';

afterEach(() => cleanup());

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
