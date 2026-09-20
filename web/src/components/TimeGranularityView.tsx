import { useMemo, useState } from 'react';

import { HeatmapGrid } from './HeatmapGrid';
import {
  bucketByMonth,
  bucketByWeek,
  bucketByYear,
  formatDayRangeLabel,
  type Bucket,
  type DayValue,
} from '../lib/timeBuckets';

export type Granularity = 'day' | 'week' | 'month' | 'year';

const GRANULARITIES: { value: Granularity; label: string }[] = [
  { value: 'day', label: 'Day' },
  { value: 'week', label: 'Week' },
  { value: 'month', label: 'Month' },
  { value: 'year', label: 'Year' },
];

interface TimeGranularityViewProps {
  // Day-bucketed source data over the whole available range -- every
  // coarser granularity is derived from this client-side, so switching
  // granularity needs no extra request (docs/PHASE_EXPENSES_DASHBOARD_
  // ACTIVITY_REPORTS.md's Activity/Sales-report heatmap, made properly
  // descriptive and drillable after a design pass flagged the original
  // bare, ungrouped grid as confusing).
  dailyData: DayValue[];
  defaultRangeDays?: number;
  formatValue?: (value: number) => string;
  onDaySelect?: (day: string) => void;
}

function toDayRecord(
  data: DayValue[],
  start?: string,
  endExclusive?: string,
): Record<string, number> {
  const out: Record<string, number> = {};
  for (const { day, value } of data) {
    if (start && day < start) continue;
    if (endExclusive && day >= endExclusive) continue;
    out[day] = (out[day] ?? 0) + value;
  }
  return out;
}

function toDateOnly(d: Date): string {
  return d.toISOString().slice(0, 10);
}

// Day/week/month/year time-series explorer: a tab row makes the current
// granularity explicit, and clicking any week/month/year bar drills into
// the next finer granularity scoped to that period -- clicking a year
// shows its months, clicking a month shows its weeks, clicking a week
// shows its days as a heatmap. Shared by the Activity page and the Sales
// report's Sales tab.
export function TimeGranularityView({
  dailyData,
  defaultRangeDays = 120,
  formatValue,
  onDaySelect,
}: TimeGranularityViewProps) {
  const [granularity, setGranularity] = useState<Granularity>('day');
  const [scope, setScope] = useState<{ start: string; endExclusive: string } | null>(null);

  function selectGranularity(g: Granularity) {
    setGranularity(g);
    setScope(null);
  }

  function drillInto(bucket: Bucket, next: Granularity) {
    setScope({ start: bucket.start, endExclusive: bucket.endExclusive });
    setGranularity(next);
  }

  const scoped = useMemo(() => {
    if (!scope) return dailyData;
    return dailyData.filter((d) => d.day >= scope.start && d.day < scope.endExclusive);
  }, [dailyData, scope]);

  const weekBuckets = useMemo(() => bucketByWeek(scoped), [scoped]);
  const monthBuckets = useMemo(() => bucketByMonth(scoped), [scoped]);
  const yearBuckets = useMemo(() => bucketByYear(scoped), [scoped]);

  const dayRange = useMemo(() => {
    if (scope) {
      // scope.endExclusive is exclusive (the day after the period's last
      // real day, per lib/timeBuckets.ts), but HeatmapGrid's endDate is
      // inclusive -- step back one day or a drilled-into week/month would
      // render with one extra, out-of-period day tacked on the end.
      const inclusiveEnd = new Date(scope.endExclusive + 'T00:00:00Z');
      inclusiveEnd.setUTCDate(inclusiveEnd.getUTCDate() - 1);
      return { start: scope.start, end: toDateOnly(inclusiveEnd) };
    }
    const end = new Date();
    const start = new Date();
    start.setUTCDate(start.getUTCDate() - defaultRangeDays);
    return { start: toDateOnly(start), end: toDateOnly(end) };
  }, [scope, defaultRangeDays]);

  function renderBars(buckets: Bucket[], onClick?: (b: Bucket) => void) {
    const max = Math.max(1, ...buckets.map((b) => b.value));
    const ordered = buckets.slice().reverse();
    return (
      <div className="rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden">
        {ordered.length === 0 && (
          <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">Nothing in this range.</p>
        )}
        {ordered.map((b) => (
          <button
            key={b.key}
            type="button"
            disabled={!onClick}
            onClick={() => onClick?.(b)}
            className={`w-full text-left px-4 py-3 ${onClick ? 'hover:bg-jb-ink/[0.03] transition-colors cursor-pointer' : ''}`}
          >
            <div className="flex items-center justify-between mb-1.5">
              <span className="text-[13px] text-jb-ink/75">{b.label}</span>
              <span className="text-[13px] text-jb-ink/60">
                {formatValue ? formatValue(b.value) : b.value}
              </span>
            </div>
            <div className="h-1.5 rounded-full bg-jb-ink/[0.06] overflow-hidden">
              <div
                className="h-full bg-jb-green-dark rounded-full"
                style={{ width: `${(b.value / max) * 100}%` }}
              />
            </div>
          </button>
        ))}
      </div>
    );
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <div className="flex gap-2">
          {GRANULARITIES.map((g) => (
            <button
              key={g.value}
              onClick={() => selectGranularity(g.value)}
              className={`px-3 py-1.5 rounded-full text-[12.5px] border transition-colors ${
                granularity === g.value
                  ? 'bg-jb-ink text-jb-cream border-jb-ink'
                  : 'border-jb-ink/15 text-jb-ink/60 bg-white'
              }`}
            >
              {g.label}
            </button>
          ))}
        </div>
        {scope && (
          <button
            onClick={() => setScope(null)}
            className="text-[12px] text-jb-ink/40 hover:text-jb-ink transition-colors shrink-0"
          >
            ‹ All time
          </button>
        )}
      </div>

      {scope && (
        <p className="text-[11.5px] text-jb-ink/45 mb-2">
          Showing {formatDayRangeLabel(scope.start, scope.endExclusive)}
        </p>
      )}

      {granularity === 'day' && (
        <HeatmapGrid
          data={toDayRecord(dailyData, dayRange.start, dayRange.end)}
          startDate={dayRange.start}
          endDate={dayRange.end}
          onDayClick={onDaySelect}
          formatValue={formatValue}
        />
      )}
      {granularity === 'week' && renderBars(weekBuckets, (b) => drillInto(b, 'day'))}
      {granularity === 'month' && renderBars(monthBuckets, (b) => drillInto(b, 'week'))}
      {granularity === 'year' && renderBars(yearBuckets, (b) => drillInto(b, 'month'))}
    </div>
  );
}
