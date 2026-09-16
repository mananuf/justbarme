import { useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import { ApiError } from '../api/client';
import { Logo } from '../components/Logo';
import { useSession } from '../lib/session';

type Step = 'details' | 'code';

// Public signup: anyone can create an account here, unlike Login which only
// signs in an account someone else already set up. Two steps -- email is
// verified with a one-time code before the account actually exists (see
// internal/signup) -- but presented as one continuous flow, not a
// multi-page wizard, since it's a handful of seconds between them.
export function SignUp() {
  const { startSignup, verifySignup } = useSession();
  const navigate = useNavigate();

  const [step, setStep] = useState<Step>('details');
  const [email, setEmail] = useState('');
  const [name, setName] = useState('');
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [resent, setResent] = useState(false);
  const codeInputRef = useRef<HTMLInputElement | null>(null);

  async function handleStart(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await startSignup(email, name, password);
      setStep('code');
      setTimeout(() => codeInputRef.current?.focus(), 0);
    } catch (err) {
      setError(describeError(err, 'start'));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleVerify(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await verifySignup(email, code);
      // A brand-new account never has a business yet -- straight into
      // onboarding, same destination Login sends a business-less account to.
      void navigate('/onboarding', { replace: true });
    } catch (err) {
      setError(describeError(err, 'verify'));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleResend() {
    setSubmitting(true);
    setError(null);
    setResent(false);
    try {
      await startSignup(email, name, password);
      setResent(true);
    } catch (err) {
      setError(describeError(err, 'start'));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink flex flex-col">
      <div className="w-full max-w-sm mx-auto px-6 pt-8 pb-16 flex-1 flex flex-col">
        <Link to="/" className="flex items-center justify-center gap-1.5 mb-10" aria-label="justbarme home">
          <Logo className="w-5 h-5 text-jb-ink/70" />
          <span className="font-pixel text-[10px] tracking-[0.2em] text-jb-ink/50">JUSTBARME</span>
        </Link>

        {step === 'details' && (
          <form onSubmit={(e) => void handleStart(e)} className="flex-1 flex flex-col">
            <h1
              className="text-2xl font-light tracking-tight mb-1.5"
              style={{ fontFamily: 'var(--font-display)' }}
            >
              Set up your bar
            </h1>
            <p className="text-[13px] text-jb-ink/45 mb-8">Takes about a minute. No card, no waiting.</p>

            <label className="block mb-4">
              <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">Your name</span>
              <input
                type="text"
                autoComplete="name"
                required
                placeholder="Ada Obi"
                value={name}
                onChange={(e) => setName(e.target.value)}
                className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3.5 text-[15px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none focus:border-jb-ink/40 transition-colors"
              />
            </label>

            <label className="block mb-4">
              <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">Email</span>
              <input
                type="email"
                autoComplete="username"
                required
                placeholder="ada@example.com"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3.5 text-[15px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none focus:border-jb-ink/40 transition-colors"
              />
            </label>

            <label className="block mb-4">
              <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">Password</span>
              <input
                type="password"
                autoComplete="new-password"
                required
                minLength={8}
                placeholder="At least 8 characters"
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
                disabled={submitting || !email || !name || password.length < 8}
                className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 hover:bg-jb-green transition-colors active:scale-[0.98] disabled:opacity-40 disabled:active:scale-100"
              >
                {submitting ? 'Sending code…' : 'Continue'}
              </button>
              <p className="text-center text-[12px] text-jb-ink/35 mt-4">
                Already have an account?{' '}
                <Link to="/login" className="text-jb-ink/60 underline underline-offset-2">
                  Sign in
                </Link>
              </p>
            </div>
          </form>
        )}

        {step === 'code' && (
          <form onSubmit={(e) => void handleVerify(e)} className="flex-1 flex flex-col">
            <h1
              className="text-2xl font-light tracking-tight mb-1.5"
              style={{ fontFamily: 'var(--font-display)' }}
            >
              Check your email
            </h1>
            <p className="text-[13px] text-jb-ink/45 mb-8">
              We sent a 6-digit code to <span className="text-jb-ink/70">{email}</span>.
            </p>

            <label className="block mb-4">
              <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">Verification code</span>
              <input
                ref={codeInputRef}
                type="text"
                inputMode="numeric"
                autoComplete="one-time-code"
                required
                maxLength={6}
                placeholder="000000"
                value={code}
                onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3.5 text-[20px] tracking-[0.3em] text-center text-jb-ink placeholder:text-jb-ink/20 focus:outline-none focus:border-jb-ink/40 transition-colors"
              />
            </label>

            {error && (
              <p role="alert" className="text-[13px] text-red-700 mb-2">
                {error}
              </p>
            )}
            {resent && !error && (
              <p className="text-[13px] text-jb-ink/45 mb-2">A new code is on its way.</p>
            )}

            <div className="mt-auto pt-6">
              <button
                type="submit"
                disabled={submitting || code.length !== 6}
                className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 hover:bg-jb-green transition-colors active:scale-[0.98] disabled:opacity-40 disabled:active:scale-100"
              >
                {submitting ? 'Verifying…' : 'Verify and continue'}
              </button>
              <button
                type="button"
                onClick={() => void handleResend()}
                disabled={submitting}
                className="w-full text-[13px] text-jb-ink/45 hover:text-jb-ink py-3 transition-colors disabled:opacity-40"
              >
                Didn&apos;t get it? Resend code
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  );
}

function describeError(err: unknown, phase: 'start' | 'verify'): string {
  if (err instanceof ApiError) {
    if (err.code === 'EMAIL_ALREADY_REGISTERED') return 'This email is already registered. Sign in instead.';
    if (err.code === 'RATE_LIMITED') return 'Too many attempts. Please wait a moment and try again.';
    if (err.code === 'INVALID_CODE') return "That code is wrong or has expired. You can request a new one.";
    if (err.code === 'VALIDATION_FAILED') return 'Please check your details and try again.';
  }
  return phase === 'start'
    ? 'Could not reach justbarme. Check your connection and try again.'
    : 'Could not verify that code. Check your connection and try again.';
}
