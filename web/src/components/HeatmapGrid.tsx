// A GitHub-style contribution heatmap -- a plain CSS/Tailwind grid, no
// charting library (docs/PHASE_EXPENSES_DASHBOARD_ACTIVITY_REPORTS.md §5,
// matching this codebase's minimal-dependency ethos: hand-rolled JWT
// verification, self-hosted fonts, no chart library anywhere else in
// web/package.json). Used by both the Activity page's daily-activity-count
// view and the Sales report's daily-revenue view -- built once, shared.
//
// Month labels above the columns and Mon/Wed/Fri labels on the left (both
// added after a design pass flagged the bare grid as illegible -- a wall
// of identical squares tells you nothing about *when* without them) give
// this the same at-a-glance orientation GitHub's own heatmap has.

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

interface HeatmapGridProps {
  // day -> value, YYYY-MM-DD keys already bucketed in the business's own
  // timezone by the backend -- this component does no timezone math.
  data: Record<string, number>;
  startDate: string; // YYYY-MM-DD, inclusive
  endDate: string; // YYYY-MM-DD, inclusive
  onDayClick?: (day: string) => void;
  formatValue?: (value: number) => string;
  showLegend?: boolean;
}

function parseDay(day: string): Date {
  const [y, m, d] = day.split('-').map(Number);
  return new Date(Date.UTC(y ?? 1970, (m ?? 1) - 1, d ?? 1));
}

function formatDay(date: Date): string {
  return date.toISOString().slice(0, 10);
}

// Buckets a value into one of 5 intensity levels relative to the range's
// own max -- the same relative-to-max approach GitHub's own heatmap uses,
// rather than fixed absolute thresholds that would break for a quiet
// single-bar pilot vs. a busy one.
function intensityClass(value: number, max: number): string {
  if (value <= 0) return 'bg-jb-ink/[0.05]';
  if (max <= 0) return 'bg-jb-ink/[0.05]';
  const ratio = value / max;
  if (ratio > 0.75) return 'bg-jb-green-dark';
  if (ratio > 0.5) return 'bg-jb-green/70';
  if (ratio > 0.25) return 'bg-jb-green/45';
  return 'bg-jb-green/25';
}

export function HeatmapGrid({
  data,
  startDate,
  endDate,
  onDayClick,
  formatValue,
  showLegend = true,
}: HeatmapGridProps) {
  const start = parseDay(startDate);
  const end = parseDay(endDate);

  // Pad to full weeks (Sunday-start, matching GitHub's own convention) so
  // the grid renders as clean columns.
  const gridStart = new Date(start);
  gridStart.setUTCDate(gridStart.getUTCDate() - gridStart.getUTCDay());

  const days: Date[] = [];
  for (let d = new Date(gridStart); d <= end; d.setUTCDate(d.getUTCDate() + 1)) {
    days.push(new Date(d));
  }

  const weeks: Date[][] = [];
  for (let i = 0; i < days.length; i += 7) {
    weeks.push(days.slice(i, i + 7));
  }

  const max = Math.max(0, ...Object.values(data));

  // A month label is shown above the first week-column whose Sunday falls
  // in a month different from the previous column's -- exactly GitHub's
  // own placement rule, so labels never repeat or crowd each other.
  let lastMonth = -1;
  const monthLabels = weeks.map((week) => {
    const firstDay = week[0];
    const month = firstDay?.getUTCMonth() ?? -1;
    if (month !== lastMonth) {
      lastMonth = month;
      return MONTH_NAMES[month];
    }
    return null;
  });

  return (
    <div className="overflow-x-auto">
      <div className="inline-flex gap-1">
        <div className="flex flex-col gap-1 pt-4 pr-1 shrink-0">
          {['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'].map((label, i) => (
            <span
              key={label}
              className="h-3 text-[9px] leading-3 text-jb-ink/35"
              style={{ visibility: i % 2 === 1 ? 'visible' : 'hidden' }}
            >
              {label.slice(0, 1)}
            </span>
          ))}
        </div>
        <div className="flex flex-col">
          <div className="flex gap-1 mb-0.5 h-3">
            {weeks.map((week, wi) => (
              <div key={wi} className="w-3 shrink-0 text-[9px] leading-3 text-jb-ink/40">
                {monthLabels[wi]}
              </div>
            ))}
          </div>
          <div className="flex gap-1 w-max">
            {weeks.map((week, wi) => (
              <div key={wi} className="flex flex-col gap-1">
                {week.map((date) => {
                  const key = formatDay(date);
                  const inRange = date >= start && date <= end;
                  const value = data[key] ?? 0;
                  return (
                    <button
                      key={key}
                      type="button"
                      disabled={!inRange || !onDayClick}
                      onClick={() => onDayClick?.(key)}
                      title={
                        inRange
                          ? `${date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })}: ${
                              formatValue ? formatValue(value) : value
                            }`
                          : undefined
                      }
                      className={`w-3 h-3 rounded-sm transition-transform ${
                        inRange ? intensityClass(value, max) : 'bg-transparent'
                      } ${onDayClick && inRange ? 'hover:scale-125 cursor-pointer' : ''}`}
                    />
                  );
                })}
              </div>
            ))}
          </div>
        </div>
      </div>
      {showLegend && (
        <div className="flex items-center gap-1.5 mt-2 pl-6 text-[10px] text-jb-ink/35">
          <span>Less</span>
          {[
            'bg-jb-ink/[0.05]',
            'bg-jb-green/25',
            'bg-jb-green/45',
            'bg-jb-green/70',
            'bg-jb-green-dark',
          ].map((cls) => (
            <span key={cls} className={`w-2.5 h-2.5 rounded-sm ${cls}`} />
          ))}
          <span>More</span>
        </div>
      )}
    </div>
  );
}
