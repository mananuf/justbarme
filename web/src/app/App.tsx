import { Link, Route, Routes } from 'react-router-dom';

import { RequireAuth } from '../components/RequireAuth';
import { SessionProvider } from '../lib/session';
import { Landing } from '../pages/Landing';
import { Login } from '../pages/Login';
import { Onboarding } from '../pages/Onboarding';
import { Install } from '../pages/Install';
import { Dashboard } from '../pages/Dashboard';
import { Sell } from '../pages/Sell';

function NotFound() {
  return (
    <main className="min-h-screen bg-jb-cream text-jb-ink flex flex-col items-center justify-center px-6 text-center gap-4">
      <p className="text-xs tracking-widest text-jb-ink/40">404</p>
      <h1 className="text-2xl font-light tracking-tight">That page is not here.</h1>
      <Link
        to="/"
        className="px-6 py-3 rounded-xl bg-jb-ink text-jb-cream text-sm font-medium hover:bg-jb-green transition-colors"
      >
        Return home
      </Link>
    </main>
  );
}

export function App() {
  return (
    <SessionProvider>
      <Routes>
        <Route path="/" element={<Landing />} />
        <Route path="/login" element={<Login />} />
        <Route
          path="/onboarding"
          element={
            <RequireAuth>
              <Onboarding />
            </RequireAuth>
          }
        />
        <Route
          path="/install"
          element={
            <RequireAuth>
              <Install />
            </RequireAuth>
          }
        />
        <Route
          path="/dashboard"
          element={
            <RequireAuth requireBusiness>
              <Dashboard />
            </RequireAuth>
          }
        />
        <Route
          path="/dashboard/sell"
          element={
            <RequireAuth requireBusiness>
              <Sell />
            </RequireAuth>
          }
        />
        <Route path="*" element={<NotFound />} />
      </Routes>
    </SessionProvider>
  );
}
