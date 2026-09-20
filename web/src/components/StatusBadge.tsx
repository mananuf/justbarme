import type { ServiceHealth } from '../hooks/useServiceHealth';

const labels: Record<ServiceHealth, string> = {
  checking: 'Checking service',
  available: 'Connected',
  unavailable: 'Waiting to sync',
  offline: 'Offline',
};

const dotColor: Record<ServiceHealth, string> = {
  checking: 'bg-jb-ink/30',
  available: 'bg-emerald-600',
  unavailable: 'bg-jb-gold',
  offline: 'bg-jb-ink/30',
};

const pulse: Record<ServiceHealth, boolean> = {
  checking: true,
  available: false,
  unavailable: true,
  offline: false,
};

export function StatusBadge({ status }: { status: ServiceHealth }) {
  return (
    <span
      role="status"
      className="inline-flex items-center gap-1.5 whitespace-nowrap text-[11px] text-jb-ink/50 bg-white/70 rounded-full px-3 py-1.5 border border-jb-ink/10"
    >
      <span className="relative flex h-2 w-2">
        {pulse[status] && (
          <span
            className={`animate-ping absolute inline-flex h-full w-full rounded-full opacity-50 ${dotColor[status]}`}
          />
        )}
        <span className={`relative inline-flex rounded-full h-2 w-2 ${dotColor[status]}`} />
      </span>
      {labels[status]}
    </span>
  );
}
