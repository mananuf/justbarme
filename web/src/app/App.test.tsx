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

describe('App routing', () => {
  it('renders the landing page at /', () => {
    vi.stubGlobal('fetch', vi.fn());
    renderApp('/');
    expect(screen.getByRole('heading', { level: 1, name: /run your bar/i })).toBeInTheDocument();
  });

  it('renders onboarding at /onboarding', () => {
    vi.stubGlobal('fetch', vi.fn());
    renderApp('/onboarding');
    expect(screen.getByRole('heading', { name: /create your account/i })).toBeInTheDocument();
  });

  it('renders the install guide at /install', () => {
    vi.stubGlobal('fetch', vi.fn());
    renderApp('/install');
    expect(
      screen.getByRole('heading', { name: /install justbarme on your phone/i }),
    ).toBeInTheDocument();
  });

  it('renders a not-found route for unknown paths', () => {
    vi.stubGlobal('fetch', vi.fn());
    renderApp('/missing');
    expect(screen.getByRole('heading', { name: /page is not here/i })).toBeInTheDocument();
  });
});

describe('Dashboard connectivity', () => {
  it('reports a healthy service when the backend is reachable', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ data: { status: 'ready', version: 'test' } }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    );

    renderApp('/dashboard');
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Connected'));
  });

  it('shows offline state without requesting readiness', () => {
    Object.defineProperty(navigator, 'onLine', { configurable: true, value: false });
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    renderApp('/dashboard');
    expect(screen.getByRole('status')).toHaveTextContent('Offline');
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('shows a waiting-to-sync status when the service cannot be reached', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('network failed')));
    renderApp('/dashboard');

    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Waiting to sync'));
  });
});
