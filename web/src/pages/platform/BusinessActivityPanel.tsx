import { useEffect, useState } from 'react';

import { getPlatformBusinessActivity, type PlatformBusinessActivity } from '../../api/platform';
import { describeActionError } from '../../lib/errors';

function naira(kobo: number): string {
  return `₦${(kobo / 100).toLocaleString(undefined, { maximumFractionDigits: 0 })}`;
}

function relativeTime(iso: string): string {
  const minutes = Math.round((Date.now() - new Date(iso).getTime()) / 60000);
  if (minutes < 1) return 'just now';
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 48) return `${hours}h ago`;
  return `${Math.round(hours / 24)}d ago`;
}

function Tile({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="rounded-lg border border-jb-ink/10 bg-white px-3 py-2.5">
      <div className="text-[11px] text-jb-ink/45 uppercase tracking-wide">{label}</div>
      <div className="text-[15px] font-medium tabular-nums">{value}</div>
      {hint && <div className="text-[11px] text-jb-ink/40 mt-0.5">{hint}</div>}
    </div>
  );
}

// Read-only, aggregate-only oversight view. Opening it is itself recorded in
// the platform audit log (server-side), for both roles.
export function BusinessActivityPanel({ businessId }: { businessId: string }) {
  const [data, setData] = useState<PlatformBusinessActivity | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const result = await getPlatformBusinessActivity(businessId);
        if (!cancelled) setData(result);
      } catch (err) {
        if (!cancelled) setError(describeActionError(err, 'Could not load this business.'));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [businessId]);

  if (error) {
    return (
      <p role="alert" className="text-[13px] text-red-700">
        {error}
      </p>
    );
  }
  if (!data) return <p className="text-[13px] text-jb-ink/45">Loading…</p>;

  return (
    <div>
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
        <Tile
          label="Sales today"
          value={naira(data.sales_today_kobo)}
          hint={`${data.sales_today_count} sales · ${data.items_sold_today} items`}
        />
        <Tile
          label="Last 7 days"
          value={naira(data.sales_7d_kobo)}
          hint={`${data.sales_7d_count} sales`}
        />
        <Tile
          label="Outstanding tabs"
          value={naira(data.outstanding_kobo)}
          hint={`${data.outstanding_bills} open`}
        />
        <Tile
          label="Needs review"
          value={String(data.alerts_count)}
          hint="alerts and pending decisions"
        />
        <Tile
          label="Stock health"
          value={`${data.out_of_stock} out`}
          hint={`of ${data.tracked_variants} tracked · ${data.negative_stock} negative`}
        />
        <Tile
          label="People"
          value={`${data.owners + data.staff}`}
          hint={`${data.owners} owner · ${data.staff} staff`}
        />
        <Tile
          label="Last activity"
          value={data.last_activity_at ? relativeTime(data.last_activity_at) : 'None yet'}
        />
      </div>

      <div className="mt-3">
        <div className="text-[11px] text-jb-ink/45 uppercase tracking-wide mb-1.5">
          Recent activity
        </div>
        {data.recent_activity.length === 0 ? (
          <p className="text-[13px] text-jb-ink/45">Nothing recorded yet.</p>
        ) : (
          <ul className="divide-y divide-jb-ink/[0.06] rounded-lg border border-jb-ink/10 bg-white">
            {data.recent_activity.map((a, i) => (
              <li key={i} className="px-3 py-2 flex items-center justify-between gap-3">
                <span className="text-[13px] text-jb-ink/80">{a.summary}</span>
                <span className="text-[11px] text-jb-ink/40 shrink-0">
                  {relativeTime(a.occurred_at)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
      <p className="text-[11px] text-jb-ink/35 mt-3">
        Read-only summary. Viewing this is recorded in the audit log.
      </p>
    </div>
  );
}
