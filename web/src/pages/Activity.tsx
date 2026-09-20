import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';

import {
  activityHeatmap,
  listActivity,
  type ActivityEntry,
  type ActivityType,
} from '../api/activity';
import { TimeGranularityView } from '../components/TimeGranularityView';
import { describeActionError } from '../lib/errors';
import type { DayValue } from '../lib/timeBuckets';
import { useSession } from '../lib/session';

function formatNaira(kobo: number): string {
  return (Math.abs(kobo) / 100).toLocaleString();
}

function formatWhen(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  });
}

const TYPE_LABELS: Record<ActivityType, string> = {
  sale: 'Sale',
  expense: 'Expense',
  inventory_adjustment: 'Stock adjustment',
  stock_receipt: 'Stock received',
};

const TYPE_FILTERS: { value: ActivityType | ''; label: string }[] = [
  { value: '', label: 'All' },
  { value: 'sale', label: 'Sales' },
  { value: 'expense', label: 'Expenses' },
  { value: 'inventory_adjustment', label: 'Adjustments' },
  { value: 'stock_receipt', label: 'Receipts' },
];

// Owner-facing feed of everything the team has done (docs/PHASE_EXPENSES_
// DASHBOARD_ACTIVITY_REPORTS.md §5) -- a day/week/month/year drillable
// heatmap up top (TimeGranularityView), filters and the full feed below.
// A whole year of daily counts is fetched once; every coarser granularity
// is derived from it client-side, so switching granularity is instant.
export function Activity() {
  const { selectedBusinessId } = useSession();

  const [dailyData, setDailyData] = useState<DayValue[]>([]);
  const [heatmapError, setHeatmapError] = useState<string | null>(null);

  const [typeFilter, setTypeFilter] = useState<ActivityType | ''>('');
  const [dayFilter, setDayFilter] = useState<string | null>(null);
  const [entries, setEntries] = useState<ActivityEntry[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  const range = useMemo(() => {
    const end = new Date();
    const start = new Date();
    start.setUTCDate(start.getUTCDate() - 364); // a full year, enough for a real Year view
    return { startAt: start.toISOString(), endAt: end.toISOString() };
  }, []);

  useEffect(() => {
    if (!selectedBusinessId) return;
    activityHeatmap(selectedBusinessId, range.startAt, range.endAt)
      .then((counts) => setDailyData(counts.map((c) => ({ day: c.day, value: c.count }))))
      .catch((err: unknown) =>
        setHeatmapError(describeActionError(err, 'Could not load the heatmap.')),
      );
  }, [selectedBusinessId, range]);

  useEffect(() => {
    if (!selectedBusinessId) return;
    const filter: { type?: ActivityType; startAt?: string; endAt?: string } = {};
    if (typeFilter) filter.type = typeFilter;
    if (dayFilter) {
      const start = new Date(dayFilter + 'T00:00:00Z');
      const end = new Date(start);
      end.setUTCDate(end.getUTCDate() + 1);
      filter.startAt = start.toISOString();
      filter.endAt = end.toISOString();
    }
    listActivity(selectedBusinessId, filter, 100)
      .then(setEntries)
      .catch((err: unknown) => setLoadError(describeActionError(err, 'Could not load activity.')));
  }, [selectedBusinessId, typeFilter, dayFilter]);

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink pb-16">
      <div className="max-w-md mx-auto px-5 pt-6">
        <div className="flex items-center gap-2.5 mb-6">
          <Link
            to="/dashboard"
            aria-label="Back to dashboard"
            className="text-jb-ink/40 hover:text-jb-ink text-lg leading-none"
          >
            ←
          </Link>
          <h1 className="text-lg font-medium">Activity</h1>
        </div>

        {heatmapError && <p className="text-[13px] text-red-700 mb-3">{heatmapError}</p>}
        <div className="mb-6">
          <TimeGranularityView
            dailyData={dailyData}
            defaultRangeDays={119}
            formatValue={(v) => `${v} event${v === 1 ? '' : 's'}`}
            onDaySelect={(day) => setDayFilter((prev) => (prev === day ? null : day))}
          />
        </div>

        {dayFilter && (
          <div className="flex items-center justify-between mb-3 text-[12.5px] text-jb-ink/50">
            <span>
              Filtered to{' '}
              {new Date(dayFilter + 'T00:00:00Z').toLocaleDateString(undefined, {
                month: 'short',
                day: 'numeric',
              })}
            </span>
            <button onClick={() => setDayFilter(null)} className="text-jb-ink/40 hover:text-jb-ink">
              Clear
            </button>
          </div>
        )}

        <div className="flex flex-wrap gap-2 mb-4">
          {TYPE_FILTERS.map((f) => (
            <button
              key={f.value}
              onClick={() => setTypeFilter(f.value)}
              className={`px-3.5 py-2 rounded-full text-[13px] border transition-colors ${
                typeFilter === f.value
                  ? 'bg-jb-ink text-jb-cream border-jb-ink'
                  : 'border-jb-ink/15 text-jb-ink/60 bg-white'
              }`}
            >
              {f.label}
            </button>
          ))}
        </div>

        {loadError && <p className="text-[13px] text-red-700 mb-3">{loadError}</p>}
        <div className="rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden">
          {entries === null && <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">Loading…</p>}
          {entries !== null && entries.length === 0 && (
            <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">Nothing here yet.</p>
          )}
          {(entries ?? []).map((e) => (
            <div key={e.id} className="px-4 py-3.5 flex items-center justify-between gap-3">
              <div className="min-w-0">
                <div className="text-[13px] text-jb-ink/80">{e.summary}</div>
                <div className="text-[11px] text-jb-ink/40 mt-0.5">
                  {TYPE_LABELS[e.type]} ·{' '}
                  {e.actorName && (
                    <span className="font-semibold text-jb-ink/55">{e.actorName}</span>
                  )}{' '}
                  {formatWhen(e.occurredAt)}
                </div>
              </div>
              <div
                className={`text-[13px] font-medium shrink-0 ${
                  e.amountKobo < 0 ? 'text-red-700/80' : 'text-jb-green-dark'
                }`}
              >
                {e.amountKobo > 0 ? '+' : e.amountKobo < 0 ? '−' : ''}₦{formatNaira(e.amountKobo)}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
