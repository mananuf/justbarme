import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';

import { apiRequest } from '../api/client';

// A completely separate session context from src/lib/session.tsx's
// SessionProvider -- mirroring the backend's own separation of
// internal/platformadmin from internal/identity. Platform staff are not
// business members, so this must never share state, storage, or a code
// path with the business session: no IndexedDB caching (this is an
// always-online ops tool, not the offline-first business app), no
// memberships, no business selection.

export interface PlatformStaff {
  id: string;
  name: string;
  email: string;
  role: string;
}

interface PlatformMeResponse {
  staff: { id: string; name: string; email: string; role: string };
  csrf_token: string;
}

type PlatformSessionStatus = 'loading' | 'authenticated' | 'anonymous';

interface PlatformSessionState {
  status: PlatformSessionStatus;
  staff: PlatformStaff | null;
  csrfToken: string | null;
}

interface PlatformSessionContextValue extends PlatformSessionState {
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
}

const PlatformSessionContext = createContext<PlatformSessionContextValue | null>(null);

const emptyState: PlatformSessionState = {
  status: 'loading',
  staff: null,
  csrfToken: null,
};

function toStaff(raw: PlatformMeResponse['staff']): PlatformStaff {
  return { id: raw.id, name: raw.name, email: raw.email, role: raw.role };
}

export function PlatformSessionProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<PlatformSessionState>(emptyState);

  const applyMe = useCallback((data: PlatformMeResponse) => {
    setState({ status: 'authenticated', staff: toStaff(data.staff), csrfToken: data.csrf_token });
  }, []);

  const refresh = useCallback(async () => {
    try {
      const data = await apiRequest<PlatformMeResponse>('/api/v1/platform/me');
      applyMe(data);
    } catch {
      // No offline fallback here, unlike the business SessionProvider --
      // this tool has no offline requirement, so any failure (401, network
      // error) just means "not signed in right now."
      setState({ ...emptyState, status: 'anonymous' });
    }
  }, [applyMe]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const login = useCallback(
    async (email: string, password: string) => {
      const data = await apiRequest<PlatformMeResponse>('/api/v1/platform/auth/login', {
        method: 'POST',
        body: JSON.stringify({ email, password }),
      });
      applyMe(data);
    },
    [applyMe],
  );

  const logout = useCallback(async () => {
    if (state.csrfToken) {
      try {
        await apiRequest('/api/v1/platform/auth/logout', {
          method: 'POST',
          headers: { 'X-CSRF-Token': state.csrfToken },
        });
      } catch {
        // Best-effort, same reasoning as the business session's logout.
      }
    }
    setState({ ...emptyState, status: 'anonymous' });
  }, [state.csrfToken]);

  const value = useMemo<PlatformSessionContextValue>(
    () => ({ ...state, login, logout }),
    [state, login, logout],
  );

  return (
    <PlatformSessionContext.Provider value={value}>{children}</PlatformSessionContext.Provider>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function usePlatformSession(): PlatformSessionContextValue {
  const ctx = useContext(PlatformSessionContext);
  if (!ctx) throw new Error('usePlatformSession must be used within a PlatformSessionProvider');
  return ctx;
}
