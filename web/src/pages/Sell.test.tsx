import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { db } from '../lib/db';
import { SessionProvider } from '../lib/session';
import { Sell } from './Sell';

function jsonResponse(status: number, body: unknown) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

const ME_OK = () =>
  jsonResponse(200, {
    data: {
      user: { id: 'user-1', name: 'Ada Owner', email: 'ada@example.com' },
      memberships: [{ business_id: 'biz-1', business_name: 'The Place', role: 'owner' }],
      csrf_token: 'test-csrf-token',
    },
  });

const PRODUCTS_OK = () =>
  jsonResponse(200, {
    data: [
      {
        id: 'product-1',
        name: 'Trophy Lager',
        category_id: null,
        variants: [
          {
            id: 'variant-1',
            name: '50cl Bottle',
            active: true,
            current_price: { amount_kobo: 100000 },
            current_stock: 10,
          },
        ],
      },
    ],
  });

/** Routes a stubbed fetch by URL path, like a tiny fake backend -- same
 * helper shape as src/app/App.test.tsx. */
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

function renderSell() {
  return render(
    <MemoryRouter>
      <SessionProvider>
        <Sell />
      </SessionProvider>
    </MemoryRouter>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('Sell offline catalogue cache', () => {
  it('caches the catalogue in IndexedDB after a successful load', async () => {
    stubFetch({ '/api/v1/me': ME_OK, '/api/v1/products': PRODUCTS_OK });
    renderSell();

    await waitFor(() => expect(screen.getByText(/Trophy Lager/i)).toBeInTheDocument());

    const cached = await db.catalogueCache.get('biz-1');
    expect(cached?.products[0]?.name).toBe('Trophy Lager');
    expect(screen.queryByText(/showing saved products/i)).not.toBeInTheDocument();
  });

  it('falls back to the cached catalogue, with an honest note, when the fetch fails', async () => {
    await db.catalogueCache.put({
      businessId: 'biz-1',
      products: [
        {
          id: 'product-1',
          name: 'Trophy Lager',
          categoryId: null,
          variants: [
            {
              id: 'variant-1',
              name: '50cl Bottle',
              active: true,
              currentPriceKobo: 100000,
              currentStock: 10,
              tracksInventory: true,
            },
          ],
        },
      ],
      cachedAt: new Date().toISOString(),
    });
    stubFetch({
      '/api/v1/me': ME_OK,
      '/api/v1/products': () => Promise.reject(new TypeError('Failed to fetch')),
    });

    renderSell();

    await waitFor(() => expect(screen.getByText(/Trophy Lager/i)).toBeInTheDocument());
    expect(screen.getByText(/showing saved products/i)).toBeInTheDocument();
  });

  it('shows a real load error when the fetch fails and nothing is cached', async () => {
    stubFetch({
      '/api/v1/me': ME_OK,
      '/api/v1/products': () => Promise.reject(new TypeError('Failed to fetch')),
    });

    renderSell();

    await waitFor(() => expect(screen.getByText(/you're offline/i)).toBeInTheDocument());
  });
});
