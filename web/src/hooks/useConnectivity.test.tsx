import { act, renderHook } from '@testing-library/react';
import { afterEach, expect, it } from 'vitest';

import { useConnectivity } from './useConnectivity';

afterEach(() => {
  Object.defineProperty(navigator, 'onLine', { configurable: true, value: true });
});

it('reacts to browser connectivity events', () => {
  Object.defineProperty(navigator, 'onLine', { configurable: true, value: true });
  const { result } = renderHook(() => useConnectivity());
  expect(result.current).toBe(true);

  act(() => {
    Object.defineProperty(navigator, 'onLine', { configurable: true, value: false });
    window.dispatchEvent(new Event('offline'));
  });
  expect(result.current).toBe(false);
});
