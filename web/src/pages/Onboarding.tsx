import { useState } from 'react';
import type { InputHTMLAttributes, ButtonHTMLAttributes } from 'react';
import { Link, Navigate } from 'react-router-dom';

import { apiRequest, ApiError } from '../api/client';
import { Logo } from '../components/Logo';
import { useSession } from '../lib/session';

const CATALOGUE_ITEMS = [
  'Guinness',
  'Star',
  'Heineken',
  'Trophy',
  'Jameson',
  'Hennessy',
  "Gordon's",
  'Coke',
  'Fanta',
  'Sprite',
  'Water',
];
const TOTAL_STEPS = 4;

type FormState = {
  businessName: string;
  products: string[];
  prices: Record<string, string>;
};

const DEFAULT_PRICES: Record<string, string> = {
  Guinness: '1500',
  Star: '900',
  Heineken: '1200',
  Coke: '500',
  Water: '300',
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

export function Onboarding() {
  const { csrfToken, addMembership, memberships } = useSession();

  // Captured once at mount, not the live value: creating a business from
  // step 1 calls addMembership before setStep(2), and those are two
  // separate state updates (session context vs. local step) that can
  // commit in separate renders. If this read the live `memberships` value,
  // that in-between render (memberships now non-empty, step still 1) would
  // incorrectly trigger the "already onboarded" redirect below and bounce a
  // user away from their own just-created business.
  const [alreadyHadBusiness] = useState(() => memberships.length > 0);

  const [step, setStep] = useState(1);
  const [form, setForm] = useState<FormState>({
    businessName: '',
    products: ['Guinness', 'Star', 'Heineken', 'Coke', 'Water'],
    prices: DEFAULT_PRICES,
  });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const update = (patch: Partial<FormState>) => setForm((f) => ({ ...f, ...patch }));
  const back = () => setStep((s) => Math.max(1, s - 1));

  // Skipping product selection makes a pricing step meaningless, so this
  // clears whatever was tapped and goes straight to the finish screen
  // rather than leaving step 3 with nothing to price.
  const skipProducts = () => {
    setForm((f) => ({ ...f, products: [] }));
    setStep(4);
  };

  const toggleProduct = (item: string) => {
    setForm((f) => {
      const has = f.products.includes(item);
      const products = has ? f.products.filter((p) => p !== item) : [...f.products, item];
      const prices = { ...f.prices };
      if (!has && !prices[item]) prices[item] = '';
      return { ...f, products, prices };
    });
  };

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
          <div className="flex-1 flex flex-col">
            <h1
              className="text-2xl font-light tracking-tight mb-1.5"
              style={{ fontFamily: 'var(--font-display)' }}
            >
              Choose what you sell
            </h1>
            <p className="text-[13px] text-jb-ink/45 mb-6">
              Tap the drinks you already sell. This is just a quick preview, and you&apos;ll be able
              to add or edit your full product list anytime once that update ships.
            </p>
            <div className="flex flex-wrap gap-2 content-start flex-1">
              {CATALOGUE_ITEMS.map((item) => {
                const active = form.products.includes(item);
                return (
                  <button
                    key={item}
                    onClick={() => toggleProduct(item)}
                    className={`px-4 py-2.5 rounded-full text-[14px] border transition-colors flex items-center gap-1.5 ${
                      active
                        ? 'bg-jb-ink border-jb-ink text-jb-cream'
                        : 'border-jb-ink/15 text-jb-ink/60 bg-white'
                    }`}
                  >
                    {active && <span className="text-[11px]">✓</span>}
                    {item}
                  </button>
                );
              })}
              <button className="px-4 py-2.5 rounded-full text-[14px] border border-dashed border-jb-ink/25 text-jb-ink/45">
                + Add custom product
              </button>
            </div>
            <div className="mt-6 flex flex-col gap-2.5">
              <PrimaryButton onClick={() => setStep(3)} disabled={form.products.length === 0}>
                Continue ({form.products.length} selected)
              </PrimaryButton>
              <div className="flex items-center justify-between">
                <button
                  onClick={back}
                  className="text-[13px] text-jb-ink/40 hover:text-jb-ink py-2 transition-colors"
                >
                  Back
                </button>
                <button
                  onClick={skipProducts}
                  className="text-[13px] text-jb-ink/40 hover:text-jb-ink py-2 transition-colors"
                >
                  Skip for now
                </button>
              </div>
            </div>
          </div>
        )}

        {step === 3 && (
          <div className="flex-1 flex flex-col">
            <h1
              className="text-2xl font-light tracking-tight mb-1.5"
              style={{ fontFamily: 'var(--font-display)' }}
            >
              Set your prices
            </h1>
            <p className="text-[13px] text-jb-ink/45 mb-6">
              Quick estimates are fine for now. You&apos;ll be able to fine-tune every price anytime
              once product setup ships.
            </p>
            <div className="space-y-2.5 flex-1 overflow-y-auto">
              {form.products.map((item) => (
                <div
                  key={item}
                  className="flex items-center justify-between rounded-xl border border-jb-ink/10 bg-white px-4 py-3"
                >
                  <span className="text-[14px] text-jb-ink/80">{item}</span>
                  <div className="flex items-center gap-1.5">
                    <span className="text-jb-ink/40 text-[13px]">₦</span>
                    <input
                      type="number"
                      inputMode="numeric"
                      placeholder="0"
                      value={form.prices[item] ?? ''}
                      onChange={(e) =>
                        update({ prices: { ...form.prices, [item]: e.target.value } })
                      }
                      className="w-20 text-right text-[14px] text-jb-ink bg-transparent focus:outline-none"
                    />
                  </div>
                </div>
              ))}
            </div>
            <div className="mt-6 flex flex-col gap-2.5">
              <PrimaryButton onClick={() => setStep(4)}>Finish Setup</PrimaryButton>
              <div className="flex items-center justify-between">
                <button
                  onClick={back}
                  className="text-[13px] text-jb-ink/40 hover:text-jb-ink py-2 transition-colors"
                >
                  Back
                </button>
                <button
                  onClick={() => setStep(4)}
                  className="text-[13px] text-jb-ink/40 hover:text-jb-ink py-2 transition-colors"
                >
                  Skip for now
                </button>
              </div>
            </div>
          </div>
        )}

        {step === 4 && (
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
              Your business is saved. Product setup and staff invites are coming soon, but you can
              install justbarme on this phone right now.
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
