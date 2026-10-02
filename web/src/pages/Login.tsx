import { useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';

import { ApiError, OfflineError } from '../api/client';
import { EmailIcon, WhatsAppIcon } from '../components/ChannelIcon';
import { GoogleSignInButton } from '../components/GoogleSignInButton';
import { Logo } from '../components/Logo';
import { PhoneInput } from '../components/PhoneInput';
import { useSession, type Channel } from '../lib/session';

export function Login() {
  const { login, memberships } = useSession();
  const navigate = useNavigate();
  const location = useLocation();

  // Which kind of identifier is being entered -- purely a UI choice, not
  // sent to the backend. identity.Service.Authenticate still detects
  // email vs. phone by format on its own; this only decides which input
  // to show, and specifically, for phone, routes entry through
  // PhoneInput so what gets submitted is always real E.164
  // (+234803...), never a bare local number (0803...) the backend's exact
  // phone lookup would never match -- the same bug PhoneInput was already
  // built to close for SignUp/Team (see its own doc comment).
  const [channel, setChannel] = useState<Channel>('email');
  const [identifier, setIdentifier] = useState('');
  const [password, setPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const from = (location.state as { from?: string } | null)?.from ?? null;

  function goPostSignIn() {
    // A brand-new account (created via cmd/seed, no business yet) goes
    // through onboarding; a returning owner/staff member goes straight in.
    void navigate(from ?? (memberships.length > 0 ? '/dashboard' : '/onboarding'), {
      replace: true,
    });
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await login(identifier, password);
      goPostSignIn();
    } catch (err) {
      if (err instanceof OfflineError) {
        setError(err.message);
      } else if (err instanceof ApiError) {
        if (err.code === 'RATE_LIMITED') {
          setError('Too many attempts. Please wait a moment and try again.');
        } else {
          setError('Incorrect email/phone or password.');
        }
      } else {
        setError('Could not reach justbarme. Check your connection and try again.');
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink flex flex-col">
      <div className="w-full max-w-sm mx-auto px-6 pt-8 pb-16 flex-1 flex flex-col">
        <Link
          to="/"
          className="flex items-center justify-center gap-1.5 mb-10"
          aria-label="justbarme home"
        >
          <Logo className="w-5 h-5 text-jb-ink/70" />
          <span className="font-pixel text-[10px] tracking-[0.2em] text-jb-ink/50">JUSTBARME</span>
        </Link>

        <form onSubmit={(e) => void handleSubmit(e)} className="flex-1 flex flex-col">
          <h1
            className="text-2xl font-light tracking-tight mb-1.5"
            style={{ fontFamily: 'var(--font-display)' }}
          >
            Welcome back
          </h1>
          <p className="text-[13px] text-jb-ink/45 mb-8">Sign in to run your bar.</p>

          <div className="flex gap-2 mb-4">
            <button
              type="button"
              onClick={() => {
                setChannel('email');
                setIdentifier('');
              }}
              className={`flex-1 flex items-center justify-center gap-1.5 rounded-xl border py-2.5 text-[13px] font-medium transition-colors ${
                channel === 'email'
                  ? 'border-jb-ink bg-jb-ink text-jb-cream'
                  : 'border-jb-ink/15 bg-white text-jb-ink/60'
              }`}
            >
              <EmailIcon className="w-4 h-4" />
              Email
            </button>
            <button
              type="button"
              onClick={() => {
                setChannel('whatsapp');
                setIdentifier('');
              }}
              className={`flex-1 flex items-center justify-center gap-1.5 rounded-xl border py-2.5 text-[13px] font-medium transition-colors ${
                channel === 'whatsapp'
                  ? 'border-jb-ink bg-jb-ink text-jb-cream'
                  : 'border-jb-ink/15 bg-white text-jb-ink/60'
              }`}
            >
              <WhatsAppIcon className="w-4 h-4" />
              WhatsApp
            </button>
          </div>

          <label className="block mb-4">
            <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">
              {channel === 'email' ? 'Email' : 'WhatsApp number'}
            </span>
            {channel === 'email' ? (
              <input
                type="email"
                autoComplete="username"
                required
                placeholder="ada@example.com"
                value={identifier}
                onChange={(e) => setIdentifier(e.target.value)}
                className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3.5 text-[15px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none focus:border-jb-ink/40 transition-colors"
              />
            ) : (
              <PhoneInput required onChange={setIdentifier} />
            )}
          </label>

          <label className="block mb-4">
            <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">Password</span>
            <input
              type="password"
              autoComplete="current-password"
              required
              placeholder="Your password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3.5 text-[15px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none focus:border-jb-ink/40 transition-colors"
            />
          </label>

          {error && (
            <p role="alert" className="text-[13px] text-red-700 mb-2">
              {error}
            </p>
          )}

          <div className="mt-auto pt-6">
            <button
              type="submit"
              disabled={submitting || !identifier || !password}
              className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 hover:bg-jb-green transition-colors active:scale-[0.98] disabled:opacity-40 disabled:active:scale-100"
            >
              {submitting ? 'Signing in…' : 'Sign in'}
            </button>

            <div className="flex items-center gap-3 my-5">
              <div className="h-px flex-1 bg-jb-ink/10" />
              <span className="text-[11px] text-jb-ink/35">or</span>
              <div className="h-px flex-1 bg-jb-ink/10" />
            </div>
            <GoogleSignInButton onSuccess={goPostSignIn} onError={setError} />

            <p className="text-center text-[12px] text-jb-ink/35 mt-4">
              New here?{' '}
              <Link to="/signup" className="text-jb-ink/60 underline underline-offset-2">
                Set up your bar
              </Link>
            </p>
          </div>
        </form>
      </div>
    </div>
  );
}
