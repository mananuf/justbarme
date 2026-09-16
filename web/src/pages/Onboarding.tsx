import { useEffect, useMemo, useState } from 'react';
import type { InputHTMLAttributes, ButtonHTMLAttributes } from 'react';
import { Link, Navigate } from 'react-router-dom';

import {
  applyCatalogueTemplates,
  listCatalogueTemplates,
  setVariantPrice,
  type CatalogueTemplate,
} from '../api/catalogue';
import { apiRequest, ApiError } from '../api/client';
import { Logo } from '../components/Logo';
import { useSession } from '../lib/session';

const TOTAL_STEPS = 4;

type FormState = {
  businessName: string;
  selectedTemplateIds: string[];
  // Naira strings (not kobo) keyed by template ID, edited directly by the
  // input in step 3 -- converted to kobo only when actually submitted.
  prices: Record<string, string>;
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

function nairaToKobo(naira: string): number {
  const parsed = Number(naira);
  return Number.isFinite(parsed) ? Math.round(parsed * 100) : 0;
}

export function Onboarding() {
  const { csrfToken, selectedBusinessId, addMembership, memberships } = useSession();

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
    selectedTemplateIds: [],
    prices: {},
  });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [templates, setTemplates] = useState<CatalogueTemplate[] | null>(null);
  const [templatesError, setTemplatesError] = useState<string | null>(null);
  const [search, setSearch] = useState('');

  useEffect(() => {
    let cancelled = false;
    listCatalogueTemplates()
      .then((data) => {
        if (!cancelled) setTemplates(data);
      })
      .catch(() => {
        if (!cancelled) setTemplatesError('Could not load the product list. You can still skip and add products later.');
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const filteredTemplates = useMemo(() => {
    if (!templates) return [];
    const q = search.trim().toLowerCase();
    return q ? templates.filter((t) => t.name.toLowerCase().includes(q)) : templates;
  }, [templates, search]);

  // templates is already sorted by category sort order server-side, so
  // grouping in encounter order keeps Beer & Stout / Gin / Whiskey /
  // Spirits clustered without an extra sort here.
  const templatesByCategory = useMemo(() => {
    const groups: { category: string; items: CatalogueTemplate[] }[] = [];
    const groupByCategory = new Map<string, { category: string; items: CatalogueTemplate[] }>();
    for (const t of filteredTemplates) {
      let group = groupByCategory.get(t.categoryName);
      if (!group) {
        group = { category: t.categoryName, items: [] };
        groupByCategory.set(t.categoryName, group);
        groups.push(group);
      }
      group.items.push(t);
    }
    return groups;
  }, [filteredTemplates]);

  const update = (patch: Partial<FormState>) => setForm((f) => ({ ...f, ...patch }));
  const back = () => setStep((s) => Math.max(1, s - 1));

  const toggleTemplate = (t: CatalogueTemplate) => {
    setForm((f) => {
      const has = f.selectedTemplateIds.includes(t.id);
      const selectedTemplateIds = has
        ? f.selectedTemplateIds.filter((id) => id !== t.id)
        : [...f.selectedTemplateIds, t.id];
      const prices = { ...f.prices };
      if (!has && prices[t.id] === undefined) {
        const suggested = t.variants[0]?.suggestedPriceKobo ?? 0;
        prices[t.id] = suggested > 0 ? String(suggested / 100) : '';
      }
      return { ...f, selectedTemplateIds, prices };
    });
  };

  // Skipping product selection makes a pricing step meaningless, so this
  // clears whatever was tapped and goes straight to the finish screen
  // rather than leaving step 3 with nothing to price.
  const skipProducts = () => {
    setForm((f) => ({ ...f, selectedTemplateIds: [], prices: {} }));
    setStep(4);
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

  // Applies the selected templates at their suggested prices, skipping the
  // per-item price edits below -- used by both "Skip for now" (owner never
  // touched a price field) and as the first step of a real submit.
  async function applySelectedTemplates(): Promise<ReturnType<typeof applyCatalogueTemplates>> {
    if (!selectedBusinessId || !csrfToken) {
      throw new Error('missing business context');
    }
    return applyCatalogueTemplates(form.selectedTemplateIds, selectedBusinessId, csrfToken);
  }

  async function skipPrices() {
    setSubmitting(true);
    setError(null);
    try {
      await applySelectedTemplates();
      setStep(4);
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : 'Could not save your product setup. Check your connection and try again.',
      );
    } finally {
      setSubmitting(false);
    }
  }

  async function finishWithPrices() {
    if (!selectedBusinessId || !csrfToken || !templates) return;
    setSubmitting(true);
    setError(null);
    try {
      const applied = await applySelectedTemplates();
      const priceUpdates = form.selectedTemplateIds.flatMap((templateId) => {
        const template = templates.find((t) => t.id === templateId);
        const product = applied.find((p) => p.name === template?.name);
        const variant = product?.variants[0];
        if (!template || !variant) return [];
        const enteredKobo = nairaToKobo(form.prices[templateId] ?? '0');
        if (enteredKobo === variant.currentPriceKobo) return [];
        return [setVariantPrice(variant.id, enteredKobo, selectedBusinessId, csrfToken)];
      });
      await Promise.all(priceUpdates);
      setStep(4);
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : 'Could not save your product setup. Check your connection and try again.',
      );
    } finally {
      setSubmitting(false);
    }
  }

  const pricesIncomplete = form.selectedTemplateIds.some((id) => !form.prices[id]?.trim());

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
            <p className="text-[13px] text-jb-ink/45 mb-4">
              Search or browse to pick your drinks. You&apos;ll be able to add or edit your product
              list anytime later in Settings.
            </p>
            <input
              type="text"
              inputMode="search"
              placeholder="Search drinks…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3 text-[14px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none focus:border-jb-ink/40 transition-colors mb-4"
            />
            {templatesError && (
              <p role="alert" className="text-[13px] text-red-700 mb-2">
                {templatesError}
              </p>
            )}
            <div className="flex-1 overflow-y-auto space-y-4">
              {templates === null && !templatesError && (
                <p className="text-[13px] text-jb-ink/45">Loading products…</p>
              )}
              {templatesByCategory.map(({ category, items }) => (
                <div key={category}>
                  <div className="text-[11px] font-medium text-jb-ink/40 uppercase tracking-wide mb-2">
                    {category}
                  </div>
                  <div className="flex flex-wrap gap-2">
                    {items.map((t) => {
                      const active = form.selectedTemplateIds.includes(t.id);
                      return (
                        <button
                          key={t.id}
                          onClick={() => toggleTemplate(t)}
                          className={`px-4 py-2.5 rounded-full text-[14px] border transition-colors flex items-center gap-1.5 ${
                            active
                              ? 'bg-jb-ink border-jb-ink text-jb-cream'
                              : 'border-jb-ink/15 text-jb-ink/60 bg-white'
                          }`}
                        >
                          {active && <span className="text-[11px]">✓</span>}
                          {t.name}
                        </button>
                      );
                    })}
                  </div>
                </div>
              ))}
              {templates !== null && filteredTemplates.length === 0 && (
                <p className="text-[13px] text-jb-ink/40">No drinks match &ldquo;{search}&rdquo;.</p>
              )}
            </div>
            <div className="mt-6 flex flex-col gap-2.5">
              <PrimaryButton
                onClick={() => setStep(3)}
                disabled={form.selectedTemplateIds.length === 0}
              >
                Continue ({form.selectedTemplateIds.length} selected)
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
              later in Settings.
            </p>
            <div className="space-y-2.5 flex-1 overflow-y-auto">
              {form.selectedTemplateIds.map((id) => {
                const t = templates?.find((x) => x.id === id);
                if (!t) return null;
                return (
                  <div
                    key={id}
                    className="flex items-center justify-between rounded-xl border border-jb-ink/10 bg-white px-4 py-3"
                  >
                    <span className="text-[14px] text-jb-ink/80">{t.name}</span>
                    <div className="flex items-center gap-1.5">
                      <span className="text-jb-ink/40 text-[13px]">₦</span>
                      <input
                        type="number"
                        inputMode="numeric"
                        placeholder="0"
                        value={form.prices[id] ?? ''}
                        onChange={(e) => update({ prices: { ...form.prices, [id]: e.target.value } })}
                        className="w-20 text-right text-[14px] text-jb-ink bg-transparent focus:outline-none"
                      />
                    </div>
                  </div>
                );
              })}
            </div>
            {error && (
              <p role="alert" className="text-[13px] text-red-700 mt-2">
                {error}
              </p>
            )}
            <div className="mt-6 flex flex-col gap-2.5">
              <PrimaryButton
                onClick={() => void finishWithPrices()}
                disabled={submitting || pricesIncomplete}
              >
                {submitting ? 'Saving…' : 'Finish Setup'}
              </PrimaryButton>
              <div className="flex items-center justify-between">
                <button
                  onClick={back}
                  className="text-[13px] text-jb-ink/40 hover:text-jb-ink py-2 transition-colors"
                >
                  Back
                </button>
                <button
                  onClick={() => void skipPrices()}
                  disabled={submitting}
                  className="text-[13px] text-jb-ink/40 hover:text-jb-ink py-2 transition-colors disabled:opacity-40"
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
              Your business and product list are saved. Staff invites are coming soon, but you can
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
