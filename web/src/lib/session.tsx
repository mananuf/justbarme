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
  phone: string;
}

interface MeResponse {
  user: { id: string; name: string; email?: string; phone?: string };
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

// A signup/verification identifier is either an email address or a phone
// number (docs/PHASE_INVITATIONS_WHATSAPP.md's dual-identity design) --
// callers pick which channel they're using and pass the matching value.
export type Channel = 'email' | 'whatsapp';

interface SessionContextValue extends SessionState {
  // identifier is either an email address or a phone number -- the backend
  // detects which by format (identity.Service.Authenticate). Resolves to
  // the fresh CSRF token straight from the response, for a caller that
  // needs to make an authenticated request in the same handler (see
  // InviteAccept.tsx).
  login: (identifier: string, password: string) => Promise<string>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
  selectBusiness: (businessId: string) => void;
  // Lets a screen that just called POST /businesses update session state
  // immediately, without a second GET /me round trip.
  addMembership: (membership: Membership) => Promise<void>;
  // Public signup (see SignUp.tsx): startSignup only triggers a code over
  // email or WhatsApp, it never touches session state. verifySignup is what
  // actually logs the person in -- its response is identical in shape to
  // login's, so it reuses the same applyMe path.
  startSignup: (
    channel: Channel,
    identifier: string,
    name: string,
    password: string,
  ) => Promise<void>;
  verifySignup: (channel: Channel, identifier: string, code: string) => Promise<void>;
  // "Sign in with Google" (see GoogleSignInButton.tsx). idToken is the raw
  // JWT Google Identity Services hands back client-side; the backend
  // verifies it and returns a session in the same shape login/verifySignup
  // do, whether this is a brand new account or a returning one.
  continueWithGoogle: (idToken: string) => Promise<void>;
  // Invitation "I'm new here" path (see InviteAccept.tsx, flow D): no OTP
  // challenge -- receiving the invitation already proved channel
  // ownership. Response shape matches login/verifySignup, so it also
  // reuses applyMe. The "I already have an account" path (flow E) doesn't
  // need a session-context method: it's just POST /invitations/{token}/link
  // and POST /identity-verifications/{id}/confirm (see src/api/invitations.ts),
  // followed by a plain refresh() to pick up the new membership.
  acceptInvitation: (token: string, name: string, password: string) => Promise<void>;
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
  return raw.map((m) => ({
    businessId: m.business_id,
    businessName: m.business_name,
    role: m.role,
  }));
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<SessionState>(emptyState);

  const applyMe = useCallback(async (data: MeResponse) => {
    const memberships = toMemberships(data.memberships);
    const cached = await db.authMeta.get('current');
    const selectedBusinessId =
      cached?.selectedBusinessId &&
      memberships.some((m) => m.businessId === cached.selectedBusinessId)
        ? cached.selectedBusinessId
        : (memberships[0]?.businessId ?? null);

    await db.authMeta.put({
      id: 'current',
      userId: data.user.id,
      name: data.user.name,
      email: data.user.email ?? '',
      phone: data.user.phone ?? '',
      memberships,
      selectedBusinessId,
      cachedAt: new Date().toISOString(),
    });

    setState({
      status: 'authenticated',
      user: {
        id: data.user.id,
        name: data.user.name,
        email: data.user.email ?? '',
        phone: data.user.phone ?? '',
      },
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
          user: { id: cached.userId, name: cached.name, email: cached.email, phone: cached.phone },
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
    async (identifier: string, password: string) => {
      const data = await apiRequest<MeResponse>('/api/v1/auth/login', {
        method: 'POST',
        body: JSON.stringify({ identifier, password }),
      });
      await applyMe(data);
      // Returned directly (not read back off context) so a caller that
      // immediately needs to make an authenticated request -- e.g.
      // InviteAccept's "sign in, then link this invitation" flow -- doesn't
      // have to wait for a second render to see the new csrfToken.
      return data.csrf_token;
    },
    [applyMe],
  );

  const startSignup = useCallback(
    async (channel: Channel, identifier: string, name: string, password: string) => {
      await apiRequest('/api/v1/auth/signup/start', {
        method: 'POST',
        body: JSON.stringify({
          channel,
          email: channel === 'email' ? identifier : undefined,
          phone: channel === 'whatsapp' ? identifier : undefined,
          name,
          password,
        }),
      });
    },
    [],
  );

  const verifySignup = useCallback(
    async (channel: Channel, identifier: string, code: string) => {
      const data = await apiRequest<MeResponse>('/api/v1/auth/signup/verify', {
        method: 'POST',
        body: JSON.stringify({
          channel,
          email: channel === 'email' ? identifier : undefined,
          phone: channel === 'whatsapp' ? identifier : undefined,
          code,
        }),
      });
      await applyMe(data);
    },
    [applyMe],
  );

  const continueWithGoogle = useCallback(
    async (idToken: string) => {
      const data = await apiRequest<MeResponse>('/api/v1/auth/google', {
        method: 'POST',
        body: JSON.stringify({ id_token: idToken }),
      });
      await applyMe(data);
    },
    [applyMe],
  );

  const acceptInvitation = useCallback(
    async (token: string, name: string, password: string) => {
      const data = await apiRequest<MeResponse>(
        `/api/v1/invitations/${encodeURIComponent(token)}/accept`,
        {
          method: 'POST',
          body: JSON.stringify({ name, password }),
        },
      );
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
    () => ({
      ...state,
      login,
      logout,
      refresh,
      selectBusiness,
      addMembership,
      startSignup,
      verifySignup,
      continueWithGoogle,
      acceptInvitation,
    }),
    [
      state,
      login,
      logout,
      refresh,
      selectBusiness,
      addMembership,
      startSignup,
      verifySignup,
      continueWithGoogle,
      acceptInvitation,
    ],
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
