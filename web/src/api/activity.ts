import { apiRequest } from './client';

export type ActivityType = 'sale' | 'expense' | 'inventory_adjustment' | 'stock_receipt';

export interface ActivityEntry {
  id: string;
  type: ActivityType;
  actorId: string;
  actorName: string;
  occurredAt: string;
  summary: string;
  amountKobo: number;
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

function toEntry(raw: RawActivityEntry): ActivityEntry {
  return {
    id: raw.id,
    type: raw.type as ActivityType,
    actorId: raw.actor_id,
    actorName: raw.actor_name ?? '',
    occurredAt: raw.occurred_at,
    summary: raw.summary,
    amountKobo: raw.amount_kobo,
  };
}

export interface ActivityFilter {
  actorId?: string;
  type?: ActivityType;
  startAt?: string;
  endAt?: string;
}

// listActivity wraps GET /api/v1/activity (activity:read) -- the unified
// feed (docs/PHASE_EXPENSES_DASHBOARD_ACTIVITY_REPORTS.md §3) combining
// sales, expenses, approved adjustments, and stock receipts.
export async function listActivity(
  businessId: string,
  filter: ActivityFilter = {},
  limit = 50,
): Promise<ActivityEntry[]> {
  const params = new URLSearchParams({ limit: String(limit) });
  if (filter.actorId) params.set('actor_id', filter.actorId);
  if (filter.type) params.set('type', filter.type);
  if (filter.startAt) params.set('start_at', filter.startAt);
  if (filter.endAt) params.set('end_at', filter.endAt);

  const raw = await apiRequest<RawActivityEntry[]>(`/api/v1/activity?${params.toString()}`, {
    headers: { 'X-Business-ID': businessId },
  });
  return raw.map(toEntry);
}

export interface ActivityDayCount {
  day: string; // YYYY-MM-DD, already bucketed in the business's own timezone
  count: number;
}

interface RawActivityDayCount {
  day: string;
  count: number;
}

// activityHeatmap wraps GET /api/v1/activity/heatmap (activity:read) --
// per-day activity counts backing the Activity page's GitHub-style
// heatmap.
export async function activityHeatmap(
  businessId: string,
  startAt: string,
  endAt: string,
): Promise<ActivityDayCount[]> {
  const params = new URLSearchParams({ start_at: startAt, end_at: endAt });
  const raw = await apiRequest<RawActivityDayCount[]>(
    `/api/v1/activity/heatmap?${params.toString()}`,
    {
      headers: { 'X-Business-ID': businessId },
    },
  );
  return raw.map((r) => ({ day: r.day, count: r.count }));
}
