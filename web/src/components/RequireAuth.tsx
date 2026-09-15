import type { ReactNode } from 'react';
import { Navigate, useLocation } from 'react-router-dom';

import { useSession } from '../lib/session';

interface RequireAuthProps {
  children: ReactNode;
  // Some screens (dashboard, sell, settings) assume a business already
  // exists. A freshly seeded/signed-up user with no memberships yet must be
  // sent to onboarding instead, even if they reach the URL directly (e.g. a
  // stale "from" redirect, a bookmark, or the back button) rather than
  // through Login's own post-login routing.
  requireBusiness?: boolean;
}

export function RequireAuth({ children, requireBusiness = false }: RequireAuthProps) {
  const { status, memberships } = useSession();
  const location = useLocation();

  if (status === 'loading') {
    return (
      <div className="min-h-screen bg-jb-cream flex items-center justify-center" aria-busy="true">
        <span className="w-2 h-2 rounded-full bg-jb-ink/30 animate-pulse" aria-hidden="true" />
      </div>
    );
  }

  if (status === 'anonymous') {
    return <Navigate to="/login" state={{ from: location.pathname }} replace />;
  }

  if (requireBusiness && memberships.length === 0) {
    return <Navigate to="/onboarding" replace />;
  }

  return <>{children}</>;
}
