import type { ReactNode } from 'react';
import { Navigate } from 'react-router-dom';

import { usePlatformSession } from '../lib/platformSession';

// Mirrors RequireAuth, but for the platform session -- deliberately not a
// generalized "RequireAuth<TSession>" shared with the business guard, since
// a platform staff member and a business principal must never be treated
// as interchangeable even at the type level.
export function RequirePlatformAuth({ children }: { children: ReactNode }) {
  const { status } = usePlatformSession();

  if (status === 'loading') {
    return (
      <div className="min-h-screen bg-jb-cream flex items-center justify-center" aria-busy="true">
        <span className="w-2 h-2 rounded-full bg-jb-ink/30 animate-pulse" aria-hidden="true" />
      </div>
    );
  }

  if (status === 'anonymous') {
    return <Navigate to="/platform/login" replace />;
  }

  return <>{children}</>;
}
