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

export interface ActivityFlag {
  id: string;
  sourceType: ActivityType;
  sourceId: string;
  flaggedBy: string;
  flaggedByName: string;
  reason: string;
  status: 'open' | 'resolved';
  resolutionNote: string;
  createdAt: string;
  resolvedAt: string | null;
}

interface RawActivityFlag {
  id: string;
  source_type: string;
  source_id: string;
  flagged_by: string;
  flagged_by_name?: string;
  reason: string;
  status: string;
  resolution_note?: string;
  created_at: string;
  resolved_at?: string;
}

function toFlag(raw: RawActivityFlag): ActivityFlag {
  return {
    id: raw.id,
    sourceType: raw.source_type as ActivityType,
    sourceId: raw.source_id,
    flaggedBy: raw.flagged_by,
    flaggedByName: raw.flagged_by_name ?? '',
    reason: raw.reason,
    status: raw.status as ActivityFlag['status'],
    resolutionNote: raw.resolution_note ?? '',
    createdAt: raw.created_at,
    resolvedAt: raw.resolved_at ?? null,
  };
}

// flagActivity wraps POST /api/v1/activity-flags (activity:read, both
// roles) -- "staff must request review," generalized across every
// activity type: anyone who can open an entry can flag it for the owner.
// It never changes the record being flagged.
export async function flagActivity(
  sourceType: ActivityType,
  sourceId: string,
  reason: string,
  businessId: string,
  csrfToken: string,
): Promise<ActivityFlag> {
  const raw = await apiRequest<RawActivityFlag>('/api/v1/activity-flags', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({ source_type: sourceType, source_id: sourceId, reason }),
  });
  return toFlag(raw);
}

// listActivityFlags wraps GET /api/v1/activity-flags (reviews:read, Owner
// only) -- open flags, surfaced in Reviews alongside sale/inventory
// reviews.
export async function listActivityFlags(businessId: string): Promise<ActivityFlag[]> {
  const raw = await apiRequest<RawActivityFlag[]>('/api/v1/activity-flags', {
    headers: { 'X-Business-ID': businessId },
  });
  return raw.map(toFlag);
}

// resolveActivityFlag wraps POST /api/v1/activity-flags/{id}/resolve
// (reviews:resolve, Owner only). Only marks the flag resolved -- the owner
// performs any actual fix separately, through the record's own correction
// mechanism.
export async function resolveActivityFlag(
  flagId: string,
  note: string,
  businessId: string,
  csrfToken: string,
): Promise<ActivityFlag> {
  const raw = await apiRequest<RawActivityFlag>(`/api/v1/activity-flags/${flagId}/resolve`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({ note }),
  });
  return toFlag(raw);
}
