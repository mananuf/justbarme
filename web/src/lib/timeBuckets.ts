// Client-side aggregation from a day-bucketed series into week/month/year
// buckets, backing the Activity/Sales-report heatmap's drill-down (day ->
// week -> month -> year). No new backend endpoint -- the day-level data
// already returned by GET /activity/heatmap and GET /reports/sales is
// enough to derive every coarser granularity from, so switching
// granularity is instant and needs no extra request.

export interface DayValue {
  day: string; // YYYY-MM-DD
  value: number;
}

export interface Bucket {
  key: string;
  label: string;
  start: string; // YYYY-MM-DD, inclusive
  endExclusive: string; // YYYY-MM-DD, exclusive
  value: number;
}

function parseDay(day: string): Date {
  const [y, m, d] = day.split('-').map(Number);
  return new Date(Date.UTC(y ?? 1970, (m ?? 1) - 1, d ?? 1));
}

function fmt(date: Date): string {
  return date.toISOString().slice(0, 10);
}

function startOfWeek(date: Date): Date {
  const d = new Date(date);
  d.setUTCDate(d.getUTCDate() - d.getUTCDay());
  return d;
}

const MONTH_NAMES = [
  'Jan',
  'Feb',
  'Mar',
  'Apr',
  'May',
  'Jun',
  'Jul',
  'Aug',
  'Sep',
  'Oct',
  'Nov',
  'Dec',
];

export function bucketByWeek(data: DayValue[]): Bucket[] {
  const map = new Map<string, Bucket>();
  for (const { day, value } of data) {
    const date = parseDay(day);
    const start = startOfWeek(date);
    const end = new Date(start);
    end.setUTCDate(end.getUTCDate() + 7);
    const key = fmt(start);
    const existing = map.get(key);
    if (existing) {
      existing.value += value;
    } else {
      map.set(key, {
        key,
        label: `${MONTH_NAMES[start.getUTCMonth()]} ${start.getUTCDate()}`,
        start: key,
        endExclusive: fmt(end),
        value,
      });
    }
  }
  return [...map.values()].sort((a, b) => a.start.localeCompare(b.start));
}

export function bucketByMonth(data: DayValue[]): Bucket[] {
  const map = new Map<string, Bucket>();
  for (const { day, value } of data) {
    const date = parseDay(day);
    const y = date.getUTCFullYear();
    const m = date.getUTCMonth();
    const key = `${y}-${String(m + 1).padStart(2, '0')}`;
    const start = new Date(Date.UTC(y, m, 1));
    const end = new Date(Date.UTC(y, m + 1, 1));
    const existing = map.get(key);
    if (existing) {
      existing.value += value;
    } else {
      map.set(key, {
        key,
        label: `${MONTH_NAMES[m]} ${y}`,
        start: fmt(start),
        endExclusive: fmt(end),
        value,
      });
    }
  }
  return [...map.values()].sort((a, b) => a.start.localeCompare(b.start));
}

export function bucketByYear(data: DayValue[]): Bucket[] {
  const map = new Map<string, Bucket>();
  for (const { day, value } of data) {
    const date = parseDay(day);
    const y = date.getUTCFullYear();
    const key = String(y);
    const start = new Date(Date.UTC(y, 0, 1));
    const end = new Date(Date.UTC(y + 1, 0, 1));
    const existing = map.get(key);
    if (existing) {
      existing.value += value;
    } else {
      map.set(key, { key, label: key, start: fmt(start), endExclusive: fmt(end), value });
    }
  }
  return [...map.values()].sort((a, b) => a.start.localeCompare(b.start));
}

export function formatDayRangeLabel(start: string, endExclusive: string): string {
  const s = parseDay(start);
  const e = parseDay(endExclusive);
  e.setUTCDate(e.getUTCDate() - 1);
  if (s.getUTCMonth() === e.getUTCMonth()) {
    return `${MONTH_NAMES[s.getUTCMonth()]} ${s.getUTCDate()}–${e.getUTCDate()}`;
  }
  return `${MONTH_NAMES[s.getUTCMonth()]} ${s.getUTCDate()} – ${MONTH_NAMES[e.getUTCMonth()]} ${e.getUTCDate()}`;
}
