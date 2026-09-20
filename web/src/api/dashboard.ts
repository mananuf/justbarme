import { apiRequest } from './client';
import type { ActivityEntry } from './activity';

// DashboardData mirrors GET /api/v1/dashboard's capability-aware response
// exactly: every field is optional because the server omits a section the
// caller's role can't see (rather than 403ing the whole endpoint) --
// docs/PHASE_EXPENSES_DASHBOARD_ACTIVITY_REPORTS.md §3. A Staff viewer
// today only ever gets outstandingKobo populated.
export interface DashboardData {
  todayTotalKobo: number | null;
  todaySaleCount: number | null;
  itemsSoldToday: number | null;
  todayExpensesKobo: number | null;
  outstandingKobo: number | null;
  alertsCount: number | null;
  recentActivity: ActivityEntry[];
}

interface RawActivityEntry {
  id: string;
  type: string;
  actor_id: string;
  actor_name?: string;
  occurred_at: string;
  summary: string;
  amount_kobo: number;
}

interface RawDashboard {
  today_total_kobo?: number;
  today_sale_count?: number;
  items_sold_today?: number;
  today_expenses_kobo?: number;
  outstanding_kobo?: number;
  alerts_count?: number;
  recent_activity?: RawActivityEntry[];
}

// getDashboard wraps GET /api/v1/dashboard -- one round trip replacing
// the previous separate sales-summary/sales-list/outstanding-bills calls.
export async function getDashboard(businessId: string): Promise<DashboardData> {
  const raw = await apiRequest<RawDashboard>('/api/v1/dashboard', {
    headers: { 'X-Business-ID': businessId },
  });
  return {
    todayTotalKobo: raw.today_total_kobo ?? null,
    todaySaleCount: raw.today_sale_count ?? null,
    itemsSoldToday: raw.items_sold_today ?? null,
    todayExpensesKobo: raw.today_expenses_kobo ?? null,
    outstandingKobo: raw.outstanding_kobo ?? null,
    alertsCount: raw.alerts_count ?? null,
    recentActivity: (raw.recent_activity ?? []).map((e) => ({
      id: e.id,
      type: e.type as ActivityEntry['type'],
      actorId: e.actor_id,
      actorName: e.actor_name ?? '',
      occurredAt: e.occurred_at,
      summary: e.summary,
      amountKobo: e.amount_kobo,
    })),
  };
}
