import { useState } from 'react';
import type { InputHTMLAttributes, ButtonHTMLAttributes } from 'react';
import { Link, Navigate } from 'react-router-dom';

import { apiRequest, ApiError } from '../api/client';
import { Logo } from '../components/Logo';
import { useSession } from '../lib/session';

const TOTAL_STEPS = 2;

type FormState = {
  businessName: string;
};

interface CreateBusinessResponse {
  business: { id: string; name: string; timezone: string; currency: string };
  membership: { business_id: string; role: string; status: string };
}

function ProgressBar({ step }: { step: number }) {
  return (
    <div className="flex items-center gap-4 mb-10">
      <Link
        to="/"
        aria-label="Back to homepage"
        className="text-jb-ink/40 hover:text-jb-ink transition-colors text-lg leading-none"
      >
        ←
      </Link>
      <div className="flex-1 h-1 rounded-full bg-jb-ink/10 overflow-hidden">
        <div
          className="h-full bg-jb-ink rounded-full transition-all duration-500 ease-out"
          style={{ width: `${(step / TOTAL_STEPS) * 100}%` }}
        />
      </div>
      <span className="font-pixel text-[10px] tracking-widest text-jb-ink/35 shrink-0">
        {step}/{TOTAL_STEPS}
      </span>
    </div>
  );
}

function Field({ label, ...props }: { label: string } & InputHTMLAttributes<HTMLInputElement>) {
  return (
    <label className="block mb-4">
      <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">{label}</span>
      <input
        {...props}
        className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3.5 text-[15px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none focus:border-jb-ink/40 transition-colors"
      />
    </label>
  );
}

function PrimaryButton({ children, ...props }: ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      {...props}
      className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 hover:bg-jb-green transition-colors active:scale-[0.98] disabled:opacity-40 disabled:active:scale-100"
    >
      {children}
    </button>
  );
}

// Onboarding is deliberately just business name -> done (see
// docs/PHASE_STOCK_RECEIVING.md's rationale). What you sell and its price
// used to be forced onboarding steps here; that job now belongs to the
// "Add Stock" screen (/dashboard/stock), triggered by an actual purchase
// rather than a mandatory signup step -- the first real stock receipt an
// owner logs is what gives their catalogue its first entry.
export function Onboarding() {
  const { csrfToken, addMembership, memberships } = useSession();

  // Captured once at mount, not the live value -- see the redirect guard
  // below for why: a live read would race addMembership's own state update.
  const [alreadyHadBusiness] = useState(() => memberships.length > 0);

  const [step, setStep] = useState(1);
  const [form, setForm] = useState<FormState>({ businessName: '' });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const update = (patch: Partial<FormState>) => setForm((f) => ({ ...f, ...patch }));

  async function createBusiness() {
    setSubmitting(true);
    setError(null);
    try {
      const result = await apiRequest<CreateBusinessResponse>('/api/v1/businesses', {
        method: 'POST',
        headers: csrfToken ? { 'X-CSRF-Token': csrfToken } : {},
        body: JSON.stringify({ name: form.businessName }),
      });
      await addMembership({
        businessId: result.business.id,
        businessName: result.business.name,
        role: result.membership.role,
      });
      setStep(2);
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : 'Could not reach justbarme. Check your connection and try again.',
      );
    } finally {
      setSubmitting(false);
    }
  }

  // A user can land here having already onboarded a first business in an
  // earlier session (e.g. reopening /onboarding directly) -- send them
  // straight to the dashboard rather than letting them create a duplicate.
  if (alreadyHadBusiness && step === 1) {
    return <Navigate to="/dashboard" replace />;
  }

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink flex flex-col">
      <div className="w-full max-w-sm mx-auto px-6 pt-8 pb-16 flex-1 flex flex-col">
        <Link
          to="/"
          className="flex items-center justify-center gap-1.5 mb-8"
          aria-label="justbarme home"
        >
          <Logo className="w-5 h-5 text-jb-ink/70" />
          <span className="font-pixel text-[10px] tracking-[0.2em] text-jb-ink/50">JUSTBARME</span>
        </Link>
        {step < TOTAL_STEPS && <ProgressBar step={step} />}

        {step === 1 && (
          <div className="flex-1 flex flex-col">
            <h1
              className="text-2xl font-light tracking-tight mb-1.5"
              style={{ fontFamily: 'var(--font-display)' }}
            >
              Create your business
            </h1>
            <p className="text-[13px] text-jb-ink/45 mb-8">
              Branding, address and receipt details can be added later in Settings.
            </p>
            <Field
              label="Business name"
              placeholder="e.g. The Place"
              value={form.businessName}
              onChange={(e) => update({ businessName: e.target.value })}
            />
            {error && (
              <p role="alert" className="text-[13px] text-red-700 mb-2">
                {error}
              </p>
            )}
            <div className="mt-auto pt-6">
              <PrimaryButton
                onClick={() => void createBusiness()}
                disabled={!form.businessName || submitting}
              >
                {submitting ? 'Creating…' : 'Create Business'}
              </PrimaryButton>
            </div>
          </div>
        )}

        {step === 2 && (
          <div className="flex-1 flex flex-col items-center text-center justify-center">
            <div
              className="w-16 h-16 rounded-full bg-jb-ink text-jb-cream flex items-center justify-center text-2xl mb-6"
              style={{ animation: 'pop-in 0.5s cubic-bezier(0.16,1,0.3,1) both' }}
            >
              ✓
            </div>
            <h1
              className="text-2xl font-light tracking-tight mb-2"
              style={{ fontFamily: 'var(--font-display)' }}
            >
              {form.businessName || 'Your bar'} is ready.
            </h1>
            <p className="text-[13px] text-jb-ink/45 mb-10 max-w-xs">
              Add what you sell the first time you stock it — tap Stock from the dashboard whenever
              you're ready.
            </p>
            <div className="w-full flex flex-col gap-2.5">
              <Link
                to="/install"
                className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 hover:bg-jb-green transition-colors text-center"
              >
                Install justbarme on your phone
              </Link>
              <Link
                to="/dashboard"
                className="text-[13px] text-jb-ink/45 hover:text-jb-ink py-2 transition-colors"
              >
                Maybe later, open justbarme
              </Link>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
