import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { PlatformSessionProvider } from '../../lib/platformSession';
import { PlatformDashboard } from './PlatformDashboard';

function json(status: number, body: unknown) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

const BUSINESSES = [
  {
    id: 'b1',
    name: 'The Place',
    status: 'active',
    timezone: 'Africa/Lagos',
    currency: 'NGN',
    created_at: '2026-09-01T10:00:00Z',
  },
  {
    id: 'b2',
    name: 'Blue Lounge',
    status: 'active',
    timezone: 'Africa/Lagos',
    currency: 'NGN',
    created_at: '2026-09-10T10:00:00Z',
  },
];

const ACTIVITY = {
  business: BUSINESSES[0],
  sales_today_kobo: 1250000,
  sales_today_count: 14,
  items_sold_today: 31,
  sales_7d_kobo: 8400000,
  sales_7d_count: 90,
  outstanding_kobo: 300000,
  outstanding_bills: 2,
  alerts_count: 3,
  tracked_variants: 20,
  out_of_stock: 4,
  negative_stock: 1,
  owners: 1,
  staff: 3,
  last_activity_at: new Date().toISOString(),
  recent_activity: [
    {
      type: 'sale',
      occurred_at: new Date().toISOString(),
      summary: 'Recorded a sale',
      amount_kobo: 100000,
    },
  ],
};

const STAFF = [
  {
    id: 's1',
    name: 'Ada Admin',
    email: 'ada@x.com',
    role: 'superadmin',
    status: 'active',
    created_at: '2026-09-01T10:00:00Z',
  },
  {
    id: 's2',
    name: 'Sam Support',
    email: 'sam@x.com',
    role: 'support',
    status: 'active',
    created_at: '2026-09-02T10:00:00Z',
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
    vi.fn((input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
      const path = new URL(url, 'http://localhost').pathname;
      calls.push(path);
      const routes: Record<string, unknown> = {
        '/api/v1/platform/me': me,
        '/api/v1/platform/businesses': BUSINESSES,
        '/api/v1/platform/audit-log': [],
        '/api/v1/platform/stats': {
          total_businesses: 2,
          active_businesses: 2,
          suspended_businesses: 0,
          new_businesses_last_7d: 1,
          total_users: 7,
        },
        '/api/v1/platform/businesses/b1/activity': ACTIVITY,
        '/api/v1/platform/staff': STAFF,
      };
      if (!(path in routes)) throw new Error(`unexpected fetch to ${path} in test`);
      return Promise.resolve(json(200, { data: routes[path] }));
    }),
  );
  return calls;
}

function renderDashboard() {
  return render(
    <MemoryRouter>
      <PlatformSessionProvider>
        <PlatformDashboard />
      </PlatformSessionProvider>
    </MemoryRouter>,
  );
}

afterEach(() => vi.unstubAllGlobals());

describe('platform admin dashboard', () => {
  it('shows platform stats and filters businesses by name', async () => {
    stub('support');
    const user = userEvent.setup();
    renderDashboard();

    await screen.findByText('The Place');
    expect(screen.getByText('Blue Lounge')).toBeInTheDocument();
    expect(await screen.findByText('New this week')).toBeInTheDocument();

    await user.type(screen.getByRole('searchbox', { name: /search businesses/i }), 'blue');
    expect(screen.queryByText('The Place')).not.toBeInTheDocument();
    expect(screen.getByText('Blue Lounge')).toBeInTheDocument();

    await user.clear(screen.getByRole('searchbox', { name: /search businesses/i }));
    await user.type(screen.getByRole('searchbox', { name: /search businesses/i }), 'zzz');
    expect(screen.getByText(/no business matches/i)).toBeInTheDocument();
  });

  it('opens a read-only activity summary for either role, without suspend controls for support', async () => {
    const calls = stub('support');
    const user = userEvent.setup();
    renderDashboard();

    await screen.findByText('The Place');
    expect(screen.queryByRole('button', { name: /^suspend$/i })).not.toBeInTheDocument();

    expect(calls).not.toContain('/api/v1/platform/businesses/b1/activity');
    await user.click(screen.getAllByRole('button', { name: /^activity$/i })[0]!);

    expect(await screen.findByText('Sales today')).toBeInTheDocument();
    expect(screen.getByText('₦12,500')).toBeInTheDocument();
    expect(screen.getByText(/1 owner · 3 staff/i)).toBeInTheDocument();
    expect(screen.getByText(/viewing this is recorded in the audit log/i)).toBeInTheDocument();
    expect(calls).toContain('/api/v1/platform/businesses/b1/activity');
  });

  it('lets a superadmin add and revoke staff but hides those controls from support', async () => {
    stub('superadmin');
    const user = userEvent.setup();
    const view = renderDashboard();

    await screen.findByText('The Place');
    await user.click(screen.getByRole('tab', { name: /team/i }));
    expect(await screen.findByRole('button', { name: /add staff/i })).toBeInTheDocument();
    // Cannot revoke yourself; can revoke the other account.
    const samRow = screen.getByText('Sam Support').closest('div.px-4')!;
    expect(
      within(samRow as HTMLElement).getByRole('button', { name: /revoke/i }),
    ).toBeInTheDocument();
    const adaRow = screen.getByText(/\(you\)/).closest('div.px-4')!;
    expect(
      within(adaRow as HTMLElement).queryByRole('button', { name: /revoke/i }),
    ).not.toBeInTheDocument();
    view.unmount();

    vi.unstubAllGlobals();
    stub('support');
    renderDashboard();
    await waitFor(() => expect(screen.getByRole('tab', { name: /team/i })).toBeInTheDocument());
    await user.click(screen.getByRole('tab', { name: /team/i }));
    expect(
      await screen.findByText(/only a superadmin can add or revoke staff/i),
    ).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /add staff/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /revoke/i })).not.toBeInTheDocument();
  });
});
