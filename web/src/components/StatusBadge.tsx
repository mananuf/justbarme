import type { ServiceHealth } from '../hooks/useServiceHealth';

const labels: Record<ServiceHealth, string> = {
  checking: 'Checking service',
  available: 'Connected',
  unavailable: 'Service unavailable',
  offline: 'Offline',
};

export function StatusBadge({ status }: { status: ServiceHealth }) {
  return (
    <span className={`status-badge status-badge--${status}`} role="status">
      <span className="status-badge__dot" aria-hidden="true" />
      {labels[status]}
    </span>
  );
}
