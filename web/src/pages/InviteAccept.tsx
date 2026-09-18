import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';

import { ApiError, OfflineError } from '../api/client';
import {
  confirmIdentityVerification,
  getInvitationByToken,
  linkInvitationToExistingUser,
  type Invitation,
} from '../api/invitations';
import { Logo } from '../components/Logo';
import { describeActionError } from '../lib/errors';
import { useSession } from '../lib/session';

// Public landing page for an invitation link (docs/PHASE_INVITATIONS_
// WHATSAPP.md flows D and E). Not wrapped in RequireAuth -- an invitee is
// usually anonymous when they first open this, and the two flows fork on
// exactly that:
//   D) "I'm new here" -- name + password, no OTP (receiving the invitation
//      already proved channel ownership) -- useSession().acceptInvitation.
//   E) "I already have an account" -- sign in (or already signed in), then
//      link. If the invitation's identifier doesn't already match an
//      identifier on that account, a fresh verification code is required
//      before the membership is granted (see internal/verification).
type Mode = 'choose' | 'new' | 'existingLogin' | 'existingVerify';

export function InviteAccept() {
  const { token = '' } = useParams();
  const navigate = useNavigate();
  const { status, csrfToken, acceptInvitation, login, refresh } = useSession();

  const [invitation, setInvitation] = useState<Invitation | null>(null);
  const [loadingInvite, setLoadingInvite] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [mode, setMode] = useState<Mode>('choose');

  const [name, setName] = useState('');
  const [password, setPassword] = useState('');

  const [identifier, setIdentifier] = useState('');
  const [loginPassword, setLoginPassword] = useState('');

  const [verificationId, setVerificationId] = useState<string | null>(null);
  const [verifyChannel, setVerifyChannel] = useState<'email' | 'whatsapp' | null>(null);
  const [code, setCode] = useState('');

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const inv = await getInvitationByToken(token);
        if (!cancelled) setInvitation(inv);
      } catch (err) {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 404) {
          setLoadError('This invitation link is not valid.');
        } else if (err instanceof ApiError && err.status === 409) {
          setLoadError(err.message);
        } else {
          setLoadError(describeActionError(err, 'Could not load this invitation.'));
        }
      } finally {
        if (!cancelled) setLoadingInvite(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [token]);

  const attemptLink = useCallback(
    async (activeCsrfToken: string | null) => {
      if (!activeCsrfToken) return;
      setSubmitting(true);
      setError(null);
      try {
        const result = await linkInvitationToExistingUser(token, activeCsrfToken);
        if (!result.verificationRequired) {
          await refresh();
          void navigate('/dashboard', { replace: true });
          return;
        }
        setVerificationId(result.verificationId ?? null);
        setVerifyChannel(result.channel ?? null);
        setMode('existingVerify');
      } catch (err) {
        if (err instanceof ApiError && (err.status === 409 || err.status === 404)) {
          setError(err.message);
        } else {
          setError(describeActionError(err, 'Could not link this invitation.'));
        }
      } finally {
        setSubmitting(false);
      }
    },
    [navigate, refresh, token],
  );

  async function handleAcceptAsNew(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await acceptInvitation(token, name, password);
      void navigate('/dashboard', { replace: true });
    } catch (err) {
      if (err instanceof ApiError && (err.status === 409 || err.status === 422)) {
        setError(err.message);
      } else {
        setError(describeActionError(err, 'Could not accept this invitation.'));
      }
    } finally {
      setSubmitting(false);
    }
  }

  async function handleExistingLogin(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const freshCsrfToken = await login(identifier, loginPassword);
      await attemptLink(freshCsrfToken);
    } catch (err) {
      setSubmitting(false);
      if (err instanceof OfflineError) {
        setError(err.message);
      } else if (err instanceof ApiError && err.code === 'RATE_LIMITED') {
        setError('Too many attempts. Please wait a moment and try again.');
      } else {
        setError('Incorrect email/phone or password.');
      }
    }
  }

  async function handleVerify(e: React.FormEvent) {
    e.preventDefault();
    if (!verificationId || !csrfToken) return;
    setSubmitting(true);
    setError(null);
    try {
      await confirmIdentityVerification(verificationId, code, csrfToken, token);
      await refresh();
      void navigate('/dashboard', { replace: true });
    } catch (err) {
      if (err instanceof ApiError && (err.status === 409 || err.status === 404)) {
        setError(err.message);
      } else {
        setError(describeActionError(err, 'Could not verify that code.'));
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

        {(loadingInvite || status === 'loading') && (
          <div className="flex-1 flex items-center justify-center" aria-busy="true">
            <span className="w-2 h-2 rounded-full bg-jb-ink/30 animate-pulse" aria-hidden="true" />
          </div>
        )}

        {!loadingInvite && status !== 'loading' && loadError && (
          <div className="flex-1 flex flex-col items-center justify-center text-center gap-3">
            <p className="text-[15px] text-jb-ink/70">{loadError}</p>
            <Link to="/login" className="text-[13px] text-jb-ink/60 underline underline-offset-2">
              Go to sign in
            </Link>
          </div>
        )}

        {!loadingInvite && status !== 'loading' && !loadError && invitation && (
          <>
            {mode === 'choose' && (
              <div className="flex-1 flex flex-col">
                <h1
                  className="text-2xl font-light tracking-tight mb-1.5"
                  style={{ fontFamily: 'var(--font-display)' }}
                >
                  You&apos;re invited
                </h1>
                <p className="text-[13px] text-jb-ink/45 mb-8">
                  Join as <span className="text-jb-ink/70">{invitation.role}</span>.
                </p>

                {error && (
                  <p role="alert" className="text-[13px] text-red-700 mb-2">
                    {error}
                  </p>
                )}

                <div className="mt-auto flex flex-col gap-3">
                  {status === 'authenticated' ? (
                    <button
                      type="button"
                      onClick={() => void attemptLink(csrfToken)}
                      disabled={submitting}
                      className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 hover:bg-jb-green transition-colors active:scale-[0.98] disabled:opacity-40"
                    >
                      {submitting ? 'Accepting…' : 'Accept invitation'}
                    </button>
                  ) : (
                    <>
                      <button
                        type="button"
                        onClick={() => setMode('new')}
                        className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 hover:bg-jb-green transition-colors active:scale-[0.98]"
                      >
                        I&apos;m new here
                      </button>
                      <button
                        type="button"
                        onClick={() => setMode('existingLogin')}
                        className="w-full rounded-xl border border-jb-ink/15 text-jb-ink text-[15px] font-medium py-4 hover:bg-jb-ink/5 transition-colors active:scale-[0.98]"
                      >
                        I already have an account
                      </button>
                    </>
                  )}
                </div>
              </div>
            )}

            {mode === 'new' && (
              <form onSubmit={(e) => void handleAcceptAsNew(e)} className="flex-1 flex flex-col">
                <h1
                  className="text-2xl font-light tracking-tight mb-1.5"
                  style={{ fontFamily: 'var(--font-display)' }}
                >
                  Create your account
                </h1>
                <p className="text-[13px] text-jb-ink/45 mb-8">
                  Join as <span className="text-jb-ink/70">{invitation.role}</span>.
                </p>

                <label className="block mb-4">
                  <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">
                    Your name
                  </span>
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
                  <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">
                    Password
                  </span>
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
                    disabled={submitting || !name || password.length < 8}
                    className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 hover:bg-jb-green transition-colors active:scale-[0.98] disabled:opacity-40 disabled:active:scale-100"
                  >
                    {submitting ? 'Creating account…' : 'Create account and join'}
                  </button>
                  <button
                    type="button"
                    onClick={() => setMode('choose')}
                    disabled={submitting}
                    className="w-full text-[13px] text-jb-ink/45 hover:text-jb-ink py-3 transition-colors disabled:opacity-40"
                  >
                    Back
                  </button>
                </div>
              </form>
            )}

            {mode === 'existingLogin' && (
              <form onSubmit={(e) => void handleExistingLogin(e)} className="flex-1 flex flex-col">
                <h1
                  className="text-2xl font-light tracking-tight mb-1.5"
                  style={{ fontFamily: 'var(--font-display)' }}
                >
                  Sign in to accept
                </h1>
                <p className="text-[13px] text-jb-ink/45 mb-8">
                  Join as <span className="text-jb-ink/70">{invitation.role}</span>.
                </p>

                <label className="block mb-4">
                  <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">
                    Email or WhatsApp number
                  </span>
                  <input
                    type="text"
                    autoComplete="username"
                    required
                    placeholder="ada@example.com or 0803 123 4567"
                    value={identifier}
                    onChange={(e) => setIdentifier(e.target.value)}
                    className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3.5 text-[15px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none focus:border-jb-ink/40 transition-colors"
                  />
                </label>

                <label className="block mb-4">
                  <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">
                    Password
                  </span>
                  <input
                    type="password"
                    autoComplete="current-password"
                    required
                    placeholder="Your password"
                    value={loginPassword}
                    onChange={(e) => setLoginPassword(e.target.value)}
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
                    disabled={submitting || !identifier || !loginPassword}
                    className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 hover:bg-jb-green transition-colors active:scale-[0.98] disabled:opacity-40 disabled:active:scale-100"
                  >
                    {submitting ? 'Signing in…' : 'Sign in and continue'}
                  </button>
                  <button
                    type="button"
                    onClick={() => setMode('choose')}
                    disabled={submitting}
                    className="w-full text-[13px] text-jb-ink/45 hover:text-jb-ink py-3 transition-colors disabled:opacity-40"
                  >
                    Back
                  </button>
                </div>
              </form>
            )}

            {mode === 'existingVerify' && (
              <form onSubmit={(e) => void handleVerify(e)} className="flex-1 flex flex-col">
                <h1
                  className="text-2xl font-light tracking-tight mb-1.5"
                  style={{ fontFamily: 'var(--font-display)' }}
                >
                  {verifyChannel === 'whatsapp' ? 'Check WhatsApp' : 'Check your email'}
                </h1>
                <p className="text-[13px] text-jb-ink/45 mb-8">
                  This invitation was sent to a different{' '}
                  {verifyChannel === 'whatsapp' ? 'number' : 'address'} than the one on your
                  account. Enter the code we just sent there to link it.
                </p>

                <label className="block mb-4">
                  <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">
                    Verification code
                  </span>
                  <input
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

                <div className="mt-auto pt-6">
                  <button
                    type="submit"
                    disabled={submitting || code.length !== 6}
                    className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 hover:bg-jb-green transition-colors active:scale-[0.98] disabled:opacity-40 disabled:active:scale-100"
                  >
                    {submitting ? 'Verifying…' : 'Verify and join'}
                  </button>
                </div>
              </form>
            )}
          </>
        )}
      </div>
    </div>
  );
}
