import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { App } from './App';

function jsonResponse(status: number, body: unknown) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

const UNAUTHENTICATED = () =>
  jsonResponse(401, {
    error: { code: 'AUTHENTICATION_REQUIRED', message: 'You must be signed in to do that.', request_id: '', details: {} },
  });

const ME_OK = () =>
  jsonResponse(200, {
    data: {
      user: { id: 'user-1', name: 'Ada Owner', email: 'ada@example.com' },
      memberships: [{ business_id: 'biz-1', business_name: 'The Place', role: 'owner' }],
      csrf_token: 'test-csrf-token',
    },
  });

// A freshly seeded account: authenticated, but with no business yet -- this
// is the only case that should actually see onboarding step 1.
const ME_OK_NO_BUSINESS = () =>
  jsonResponse(200, {
    data: {
      user: { id: 'user-1', name: 'Ada Owner', email: 'ada@example.com' },
      memberships: [],
      csrf_token: 'test-csrf-token',
    },
  });

const HEALTH_READY = () => jsonResponse(200, { data: { status: 'ready', version: 'test' } });

/** Routes a stubbed fetch by URL path, like a tiny fake backend. */
function stubFetch(handlers: Record<string, () => Response | Promise<Response>>) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
      const path = new URL(url, 'http://localhost').pathname;
      const handler = handlers[path];
      if (!handler) {
        throw new Error(`unexpected fetch to ${path} in test`);
      }
      return Promise.resolve(handler());
    }),
  );
}

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

describe('public routes', () => {
  it('renders the landing page at / without requiring auth', () => {
    stubFetch({ '/api/v1/me': UNAUTHENTICATED });
    renderApp('/');
    expect(screen.getByRole('heading', { level: 1, name: /run your bar/i })).toBeInTheDocument();
  });

  it('renders the login page at /login', () => {
    stubFetch({ '/api/v1/me': UNAUTHENTICATED });
    renderApp('/login');
    expect(screen.getByRole('heading', { name: /welcome back/i })).toBeInTheDocument();
  });

  it('renders a not-found route for unknown paths', () => {
    stubFetch({ '/api/v1/me': UNAUTHENTICATED });
    renderApp('/missing');
    expect(screen.getByRole('heading', { name: /page is not here/i })).toBeInTheDocument();
  });
});

describe('authenticated routes', () => {
  it('redirects /onboarding to /login when signed out', async () => {
    stubFetch({ '/api/v1/me': UNAUTHENTICATED });
    renderApp('/onboarding');
    await waitFor(() => expect(screen.getByRole('heading', { name: /welcome back/i })).toBeInTheDocument());
  });

  it('renders /onboarding step 1 for a signed-in user with no business yet', async () => {
    stubFetch({ '/api/v1/me': ME_OK_NO_BUSINESS });
    renderApp('/onboarding');
    await waitFor(() => expect(screen.getByRole('heading', { name: /create your business/i })).toBeInTheDocument());
  });

  it('redirects /onboarding to /dashboard for a user who already has a business', async () => {
    stubFetch({ '/api/v1/me': ME_OK, '/api/v1/health/ready': HEALTH_READY });
    renderApp('/onboarding');
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /good evening/i })).toBeInTheDocument(),
    );
  });

  it('renders the install guide at /install once signed in', async () => {
    stubFetch({ '/api/v1/me': ME_OK });
    renderApp('/install');
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /install justbarme on your phone/i })).toBeInTheDocument(),
    );
  });
});

describe('dashboard connectivity', () => {
  it('reports a healthy service when the backend is reachable', async () => {
    stubFetch({ '/api/v1/me': ME_OK, '/api/v1/health/ready': HEALTH_READY });
    renderApp('/dashboard');
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Connected'));
  });

  it('shows offline state without requesting readiness', async () => {
    Object.defineProperty(navigator, 'onLine', { configurable: true, value: false });
    stubFetch({ '/api/v1/me': ME_OK });
    renderApp('/dashboard');
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Offline'));
  });

  it('shows a waiting-to-sync status when the service cannot be reached', async () => {
    stubFetch({
      '/api/v1/me': ME_OK,
      '/api/v1/health/ready': () => Promise.reject(new TypeError('network failed')),
    });
    renderApp('/dashboard');
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Waiting to sync'));
  });
});
