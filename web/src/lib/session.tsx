import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';

import { apiRequest, ApiError } from '../api/client';
import { db } from './db';

export interface Membership {
  businessId: string;
  businessName: string;
  role: string;
}

export interface SessionUser {
  id: string;
  name: string;
  email: string;
}

interface MeResponse {
  user: { id: string; name: string; email: string };
  memberships: { business_id: string; business_name: string; role: string }[];
  csrf_token: string;
}

type SessionStatus = 'loading' | 'authenticated' | 'anonymous';

interface SessionState {
  status: SessionStatus;
  user: SessionUser | null;
  memberships: Membership[];
  csrfToken: string | null;
  selectedBusinessId: string | null;
}

interface SessionContextValue extends SessionState {
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
  selectBusiness: (businessId: string) => void;
  // Lets a screen that just called POST /businesses update session state
  // immediately, without a second GET /me round trip.
  addMembership: (membership: Membership) => Promise<void>;
  // Public signup (see SignUp.tsx): startSignup only triggers an email with
  // a code, it never touches session state. verifySignup is what actually
  // logs the person in -- its response is identical in shape to login's,
  // so it reuses the same applyMe path.
  startSignup: (email: string, name: string, password: string) => Promise<void>;
  verifySignup: (email: string, code: string) => Promise<void>;
}

const SessionContext = createContext<SessionContextValue | null>(null);

const emptyState: SessionState = {
  status: 'loading',
  user: null,
  memberships: [],
  csrfToken: null,
  selectedBusinessId: null,
};

function toMemberships(raw: MeResponse['memberships']): Membership[] {
  return raw.map((m) => ({ businessId: m.business_id, businessName: m.business_name, role: m.role }));
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<SessionState>(emptyState);

  const applyMe = useCallback(async (data: MeResponse) => {
    const memberships = toMemberships(data.memberships);
    const cached = await db.authMeta.get('current');
    const selectedBusinessId =
      cached?.selectedBusinessId && memberships.some((m) => m.businessId === cached.selectedBusinessId)
        ? cached.selectedBusinessId
        : (memberships[0]?.businessId ?? null);

    await db.authMeta.put({
      id: 'current',
      userId: data.user.id,
      name: data.user.name,
      email: data.user.email,
      memberships,
      selectedBusinessId,
      cachedAt: new Date().toISOString(),
    });

    setState({
      status: 'authenticated',
      user: { id: data.user.id, name: data.user.name, email: data.user.email },
      memberships,
      csrfToken: data.csrf_token,
      selectedBusinessId,
    });
  }, []);

  const refresh = useCallback(async () => {
    try {
      const data = await apiRequest<MeResponse>('/api/v1/me');
      await applyMe(data);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        await db.authMeta.delete('current');
        setState({ ...emptyState, status: 'anonymous' });
        return;
      }
      // A network/server error while checking is not the same as "logged
      // out" -- fall back to whatever identity is cached locally so an
      // offline reload doesn't bounce the user to the login screen.
      const cached = await db.authMeta.get('current');
      if (cached) {
        setState({
          status: 'authenticated',
          user: { id: cached.userId, name: cached.name, email: cached.email },
          memberships: cached.memberships,
          // No CSRF token available offline; mutating requests fail closed
          // (the server would reject them anyway) until back online.
          csrfToken: null,
          selectedBusinessId: cached.selectedBusinessId,
        });
        return;
      }
      setState({ ...emptyState, status: 'anonymous' });
    }
  }, [applyMe]);

  useEffect(() => {
    // refresh's setState calls only run after its internal await, i.e. in a
    // later microtask than this effect body -- not the synchronous
    // same-pass update the cascading-render rule guards against.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void refresh();
    // Only ever runs once on mount -- refresh is stable across re-renders.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const login = useCallback(
    async (email: string, password: string) => {
      const data = await apiRequest<MeResponse>('/api/v1/auth/login', {
        method: 'POST',
        body: JSON.stringify({ email, password }),
      });
      await applyMe(data);
    },
    [applyMe],
  );

  const startSignup = useCallback(async (email: string, name: string, password: string) => {
    await apiRequest('/api/v1/auth/signup/start', {
      method: 'POST',
      body: JSON.stringify({ email, name, password }),
    });
  }, []);

  const verifySignup = useCallback(
    async (email: string, code: string) => {
      const data = await apiRequest<MeResponse>('/api/v1/auth/signup/verify', {
        method: 'POST',
        body: JSON.stringify({ email, code }),
      });
      await applyMe(data);
    },
    [applyMe],
  );

  const logout = useCallback(async () => {
    if (state.csrfToken) {
      try {
        await apiRequest('/api/v1/auth/logout', {
          method: 'POST',
          headers: { 'X-CSRF-Token': state.csrfToken },
        });
      } catch {
        // Best-effort: still clear local state below even if the request
        // to revoke the session server-side failed (e.g. offline).
      }
    }
    await db.authMeta.delete('current');
    setState({ ...emptyState, status: 'anonymous' });
  }, [state.csrfToken]);

  const selectBusiness = useCallback((businessId: string) => {
    setState((s) => ({ ...s, selectedBusinessId: businessId }));
    void db.authMeta.update('current', { selectedBusinessId: businessId });
  }, []);

  const addMembership = useCallback(async (membership: Membership) => {
    setState((s) => ({
      ...s,
      memberships: [...s.memberships, membership],
      selectedBusinessId: membership.businessId,
    }));
    const cached = await db.authMeta.get('current');
    if (cached) {
      await db.authMeta.put({
        ...cached,
        memberships: [...cached.memberships, membership],
        selectedBusinessId: membership.businessId,
      });
    }
  }, []);

  const value = useMemo<SessionContextValue>(
    () => ({ ...state, login, logout, refresh, selectBusiness, addMembership, startSignup, verifySignup }),
    [state, login, logout, refresh, selectBusiness, addMembership, startSignup, verifySignup],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

// Pairing the Provider with its hook in one file is the standard context
// pattern; it costs a Fast Refresh boundary warning we accept here.
// eslint-disable-next-line react-refresh/only-export-components
export function useSession(): SessionContextValue {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error('useSession must be used within a SessionProvider');
  return ctx;
}
