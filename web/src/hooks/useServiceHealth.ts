import { useEffect, useState } from 'react';

import { apiRequest } from '../api/client';

interface HealthResponse {
  status: 'ready';
  version: string;
}

export type ServiceHealth = 'checking' | 'available' | 'unavailable' | 'offline';

export function useServiceHealth(isOnline: boolean): ServiceHealth {
  const [onlineHealth, setOnlineHealth] = useState<Exclude<ServiceHealth, 'offline'>>('checking');

  useEffect(() => {
    if (!isOnline) {
      return;
    }

    let active = true;
    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort(), 3_000);

    void apiRequest<HealthResponse>('/api/v1/health/ready', { signal: controller.signal })
      .then(() => {
        if (active) {
          setOnlineHealth('available');
        }
      })
      .catch(() => {
        if (active) {
          setOnlineHealth('unavailable');
        }
      })
      .finally(() => window.clearTimeout(timeout));

    return () => {
      active = false;
      window.clearTimeout(timeout);
      controller.abort();
    };
  }, [isOnline]);

  return isOnline ? onlineHealth : 'offline';
}
