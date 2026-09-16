import { useState } from 'react';
import { Navigate, useNavigate } from 'react-router-dom';

import { ApiError } from '../../api/client';
import { Logo } from '../../components/Logo';
import { usePlatformSession } from '../../lib/platformSession';

// Deliberately not styled or worded like the consumer-facing Login page --
// this is an internal ops tool, not part of the justbarme product a bar
// owner ever sees, so it should never be mistaken for one (no "sign in to
// run your bar" copy, no marketing chrome).
export function PlatformLogin() {
  const { status, login } = usePlatformSession();
  const navigate = useNavigate();

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (status === 'authenticated') {
    return <Navigate to="/platform" replace />;
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await login(email, password);
      void navigate('/platform', { replace: true });
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.code === 'RATE_LIMITED') {
          setError('Too many attempts. Please wait before trying again.');
        } else {
          setError('Incorrect email or password.');
        }
      } else {
        setError('Could not reach the platform admin API.');
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="min-h-screen bg-jb-ink text-jb-cream flex flex-col items-center justify-center px-6">
      <div className="w-full max-w-sm">
        <div className="flex items-center gap-2 mb-8 text-jb-cream/60">
          <Logo className="w-5 h-5" />
          <span className="font-pixel text-[10px] tracking-[0.2em]">JUSTBARME PLATFORM</span>
        </div>

        <h1 className="text-2xl font-light tracking-tight mb-1.5" style={{ fontFamily: 'var(--font-display)' }}>
          Platform admin
        </h1>
        <p className="text-[13px] text-jb-cream/45 mb-8">Internal access only. Every action here is logged.</p>

        <form onSubmit={(e) => void handleSubmit(e)}>
          <label className="block mb-4">
            <span className="block text-[13px] font-medium text-jb-cream/70 mb-1.5">Email</span>
            <input
              type="email"
              autoComplete="username"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full rounded-xl border border-jb-cream/15 bg-jb-cream/5 px-4 py-3.5 text-[15px] text-jb-cream placeholder:text-jb-cream/30 focus:outline-none focus:border-jb-cream/40 transition-colors"
            />
          </label>

          <label className="block mb-4">
            <span className="block text-[13px] font-medium text-jb-cream/70 mb-1.5">Password</span>
            <input
              type="password"
              autoComplete="current-password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full rounded-xl border border-jb-cream/15 bg-jb-cream/5 px-4 py-3.5 text-[15px] text-jb-cream placeholder:text-jb-cream/30 focus:outline-none focus:border-jb-cream/40 transition-colors"
            />
          </label>

          {error && (
            <p role="alert" className="text-[13px] text-red-300 mb-2">
              {error}
            </p>
          )}

          <button
            type="submit"
            disabled={submitting || !email || !password}
            className="w-full mt-4 rounded-xl bg-jb-cream text-jb-ink text-[15px] font-medium py-4 hover:bg-white transition-colors active:scale-[0.98] disabled:opacity-40 disabled:active:scale-100"
          >
            {submitting ? 'Signing in…' : 'Sign in'}
          </button>
        </form>
      </div>
    </div>
  );
}
