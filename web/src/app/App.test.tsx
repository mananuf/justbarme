import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { App } from './App';

function renderApp(path = '/') {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  Object.defineProperty(navigator, 'onLine', { configurable: true, value: true });
});

describe('App', () => {
  it('renders the Phase 1 shell and reports a healthy service', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ data: { status: 'ready', version: 'test' } }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    );

    renderApp();
    expect(screen.getByRole('heading', { name: /run the shift/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /sell/i })).toBeDisabled();
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Connected'));
  });

  it('shows offline state without requesting readiness', () => {
    Object.defineProperty(navigator, 'onLine', { configurable: true, value: false });
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    renderApp();
    expect(screen.getByRole('status')).toHaveTextContent('Offline');
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('shows a controlled service unavailable notice', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('network failed')));
    renderApp();

    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('cannot be reached'));
  });

  it('renders a not-found route', () => {
    vi.stubGlobal('fetch', vi.fn());
    renderApp('/missing');
    expect(screen.getByRole('heading', { name: /page is not here/i })).toBeInTheDocument();
  });
});
