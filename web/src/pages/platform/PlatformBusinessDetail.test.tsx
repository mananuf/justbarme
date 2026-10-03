import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { PlatformSessionProvider } from '../../lib/platformSession';
import { PlatformBusinessDetail } from './PlatformBusinessDetail';

function json(status: number, body: unknown) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

const BUSINESS = {
  id: 'b1',
  name: 'The Place',
  status: 'active',
  timezone: 'Africa/Lagos',
  currency: 'NGN',
  created_at: '2026-09-01T10:00:00Z',
};

const ACTIVITY = {
  business: BUSINESS,
  sales_today_kobo: 0,
  sales_today_count: 0,
  items_sold_today: 0,
  sales_7d_kobo: 0,
  sales_7d_count: 0,
  outstanding_kobo: 0,
  outstanding_bills: 0,
  alerts_count: 0,
  tracked_variants: 0,
  out_of_stock: 0,
  negative_stock: 0,
  owners: 1,
  staff: 0,
  recent_activity: [],
};

const SALES = [
  {
    id: 'sale-1',
    occurred_at: '2026-09-15T12:00:00Z',
    received_at: '2026-09-15T12:00:00Z',
    total_kobo: 150000,
    payment: { amount_kobo: 150000, method: 'cash' },
    seller_id: 'u1',
    bill_id: 'bill-1',
  },
];

function stub(role: 'superadmin' | 'support') {
  const me = {
    staff: { id: 's1', name: 'Ada Admin', email: 'ada@x.com', role },
    csrf_token: 'csrf',
  };
  const calls: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
      const path = new URL(url, 'http://localhost').pathname;
      calls.push(path);
      if (path.endsWith('/reverse') && init?.method === 'POST') {
        return Promise.resolve(
          json(201, {
            data: { ...SALES[0], id: 'sale-rev-1', total_kobo: -150000, reversal_of: 'sale-1' },
          }),
        );
      }
      const routes: Record<string, unknown> = {
        '/api/v1/platform/me': me,
        '/api/v1/platform/businesses/b1/activity': ACTIVITY,
        '/api/v1/platform/businesses/b1/sales': SALES,
      };
      if (!(path in routes)) throw new Error(`unexpected fetch to ${path} in test`);
      return Promise.resolve(json(200, { data: routes[path] }));
    }),
  );
  return calls;
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/platform/businesses/b1']}>
      <PlatformSessionProvider>
        <Routes>
          <Route path="/platform/businesses/:businessId" element={<PlatformBusinessDetail />} />
        </Routes>
      </PlatformSessionProvider>
    </MemoryRouter>,
  );
}

afterEach(() => vi.unstubAllGlobals());

describe('platform business deep-drill detail', () => {
  it('shows sales read-only for support, with no corrective-action buttons', async () => {
    stub('support');
    renderPage();

    expect(await screen.findByText('The Place')).toBeInTheDocument();
    await screen.findByText(/read-only \(support\)/i);
    expect(screen.queryByRole('button', { name: /^reverse$/i })).not.toBeInTheDocument();
  });

  it('lets a superadmin reverse a sale with a required reason', async () => {
    const calls = stub('superadmin');
    const user = userEvent.setup();
    renderPage();

    await screen.findByText('The Place');
    const reverseButton = await screen.findByRole('button', { name: /^reverse$/i });
    await user.click(reverseButton);

    const confirmButton = screen.getByRole('button', { name: /^confirm$/i });
    expect(confirmButton).toBeDisabled();

    await user.type(screen.getByPlaceholderText(/why is this being done/i), 'test dispute');
    expect(confirmButton).not.toBeDisabled();
    await user.click(confirmButton);

    await vi.waitFor(() =>
      expect(calls).toContain('/api/v1/platform/businesses/b1/sales/sale-1/reverse'),
    );
    // The prompt closes once the reversal succeeds.
    expect(screen.queryByPlaceholderText(/why is this being done/i)).not.toBeInTheDocument();
  });
});
