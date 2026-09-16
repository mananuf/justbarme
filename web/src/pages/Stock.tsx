import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ButtonHTMLAttributes } from 'react';
import { Link } from 'react-router-dom';

import {
  applyCatalogueTemplates,
  createProduct,
  createVariant,
  listCatalogueTemplates,
  listProducts,
  setVariantPrice,
  type CatalogueProduct,
  type CatalogueTemplate,
} from '../api/catalogue';
import { ApiError } from '../api/client';
import { receiveStock } from '../api/inventory';
import { Logo } from '../components/Logo';
import { getRememberedCrateSize, setRememberedCrateSize } from '../lib/stockPreferences';
import { useSession } from '../lib/session';

// Default variant name for a brand-new custom product -- asking for a
// selling price is the one field this flow needs; a bottle-size name can
// be renamed later in Settings, and defaulting it silently keeps this to
// one field instead of two.
const DEFAULT_VARIANT_NAME = 'Bottle';

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

function formatNaira(kobo: number): string {
  return (kobo / 100).toLocaleString(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  });
}

interface ExistingTarget {
  kind: 'existing';
  variantId: string;
  label: string;
}
interface TemplateTarget {
  kind: 'template';
  template: CatalogueTemplate;
}
interface CustomTarget {
  kind: 'custom';
  name: string;
}
type Target = ExistingTarget | TemplateTarget | CustomTarget;

type Phase = 'search' | 'price' | 'receive' | 'done';

export function Stock() {
  const { csrfToken, selectedBusinessId } = useSession();

  const [phase, setPhase] = useState<Phase>('search');
  const [search, setSearch] = useState('');
  const [ownProducts, setOwnProducts] = useState<CatalogueProduct[] | null>(null);
  const [templates, setTemplates] = useState<CatalogueTemplate[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [target, setTarget] = useState<Target | null>(null);
  const [variantId, setVariantId] = useState<string | null>(null);
  const [sellingPriceNaira, setSellingPriceNaira] = useState('');

  const [unitMode, setUnitMode] = useState<'crate' | 'bottle'>('crate');
  const [crateSize, setCrateSize] = useState('');
  const [crateCount, setCrateCount] = useState('');
  const [bottleCount, setBottleCount] = useState('');
  const [totalCostNaira, setTotalCostNaira] = useState('');

  const [lastResult, setLastResult] = useState<{ label: string; newBalance: number } | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadCatalogue = useCallback(async () => {
    if (!selectedBusinessId) return;
    try {
      const [products, templateList] = await Promise.all([
        listProducts(selectedBusinessId),
        listCatalogueTemplates(),
      ]);
      setOwnProducts(products);
      setTemplates(templateList);
    } catch {
      setLoadError('Could not load your products. Check your connection and try again.');
    }
  }, [selectedBusinessId]);

  useEffect(() => {
    // loadCatalogue's setState calls only run after its internal await,
    // i.e. in a later microtask than this effect body -- not the
    // synchronous same-pass update the cascading-render rule guards
    // against (see src/lib/session.tsx's identical refresh() pattern).
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadCatalogue();
  }, [loadCatalogue]);

  const ownProductNames = useMemo(
    () => new Set((ownProducts ?? []).map((p) => p.name.toLowerCase())),
    [ownProducts],
  );

  const query = search.trim().toLowerCase();

  const matchingOwnVariants = useMemo(() => {
    if (!ownProducts) return [];
    const rows: { variantId: string; label: string; priceKobo: number }[] = [];
    for (const product of ownProducts) {
      if (query && !product.name.toLowerCase().includes(query)) continue;
      for (const variant of product.variants) {
        rows.push({
          variantId: variant.id,
          label: `${product.name} — ${variant.name}`,
          priceKobo: variant.currentPriceKobo,
        });
      }
    }
    return rows;
  }, [ownProducts, query]);

  const matchingTemplates = useMemo(() => {
    if (!templates) return [];
    return templates.filter(
      (t) =>
        !ownProductNames.has(t.name.toLowerCase()) &&
        (!query || t.name.toLowerCase().includes(query)),
    );
  }, [templates, ownProductNames, query]);

  function pickExisting(row: { variantId: string; label: string }) {
    setTarget({ kind: 'existing', variantId: row.variantId, label: row.label });
    setVariantId(row.variantId);
    setCrateSize(getRememberedCrateSize(row.variantId));
    setPhase('receive');
  }

  function pickTemplate(t: CatalogueTemplate) {
    setTarget({ kind: 'template', template: t });
    const suggested = t.variants[0]?.suggestedPriceKobo ?? 0;
    setSellingPriceNaira(suggested > 0 ? String(suggested / 100) : '');
    setPhase('price');
  }

  function pickCustom() {
    setTarget({ kind: 'custom', name: search.trim() });
    setSellingPriceNaira('');
    setPhase('price');
  }

  async function confirmPrice() {
    if (!target || target.kind === 'existing' || !selectedBusinessId || !csrfToken) return;
    const priceKobo = Math.round(Number(sellingPriceNaira) * 100);
    setSubmitting(true);
    setError(null);
    try {
      let newVariantId: string;
      if (target.kind === 'template') {
        const [product] = await applyCatalogueTemplates(
          [target.template.id],
          selectedBusinessId,
          csrfToken,
        );
        const variant = product?.variants[0];
        if (!variant) throw new Error('template apply returned no variant');
        newVariantId = variant.id;
        // The template's suggested price is what apply used -- override it
        // only if the owner actually changed the number.
        if (priceKobo !== variant.currentPriceKobo) {
          await setVariantPrice(newVariantId, priceKobo, selectedBusinessId, csrfToken);
        }
      } else {
        const product = await createProduct(target.name, selectedBusinessId, csrfToken);
        const variant = await createVariant(
          product.id,
          DEFAULT_VARIANT_NAME,
          priceKobo,
          selectedBusinessId,
          csrfToken,
        );
        newVariantId = variant.id;
      }
      setVariantId(newVariantId);
      setCrateSize(getRememberedCrateSize(newVariantId));
      setPhase('receive');
      void loadCatalogue();
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : 'Could not save this product. Check your connection and try again.',
      );
    } finally {
      setSubmitting(false);
    }
  }

  const totalUnits =
    unitMode === 'crate'
      ? (Number(crateSize) || 0) * (Number(crateCount) || 0)
      : Number(bottleCount) || 0;
  const totalCostKobo = Math.round((Number(totalCostNaira) || 0) * 100);
  const perUnitPreview = totalUnits > 0 && totalCostNaira ? totalCostKobo / totalUnits / 100 : null;
  const canReceive =
    totalUnits > 0 && totalCostNaira.trim() !== '' && !Number.isNaN(Number(totalCostNaira));

  async function confirmReceive() {
    if (!variantId || !selectedBusinessId || !csrfToken || !canReceive) return;
    setSubmitting(true);
    setError(null);
    try {
      const receipt = await receiveStock(
        [{ variantId, quantity: totalUnits, totalCostKobo }],
        selectedBusinessId,
        csrfToken,
      );
      if (unitMode === 'crate') setRememberedCrateSize(variantId, crateSize);
      const line = receipt.lines[0];
      setLastResult({
        label:
          target?.kind === 'existing'
            ? target.label
            : target?.kind === 'template'
              ? target.template.name
              : (target?.name ?? ''),
        newBalance: line?.newBalance ?? totalUnits,
      });
      setPhase('done');
      void loadCatalogue();
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : 'Could not save this receipt. Check your connection and try again.',
      );
    } finally {
      setSubmitting(false);
    }
  }

  function reset() {
    setPhase('search');
    setSearch('');
    setTarget(null);
    setVariantId(null);
    setUnitMode('crate');
    setCrateCount('');
    setBottleCount('');
    setTotalCostNaira('');
    setError(null);
    setLastResult(null);
  }

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink pb-28">
      <div className="max-w-md mx-auto px-5 pt-6">
        <div className="flex items-center gap-2.5 mb-6">
          <Link
            to="/dashboard"
            aria-label="Back to dashboard"
            className="text-jb-ink/40 hover:text-jb-ink text-lg leading-none"
          >
            ←
          </Link>
          <Logo className="w-5 h-5 text-jb-ink/70" />
          <h1 className="text-lg font-medium">Stock</h1>
        </div>

        {loadError && (
          <p role="alert" className="text-[13px] text-red-700 mb-4">
            {loadError}
          </p>
        )}

        {phase === 'search' && (
          <div>
            <p className="text-[13px] text-jb-ink/45 mb-4">
              Search what you just bought. Pick it from your own products, add it from the
              catalogue, or add it as something new.
            </p>
            <input
              type="text"
              inputMode="search"
              placeholder="Search drinks…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3 text-[14px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none focus:border-jb-ink/40 transition-colors mb-4"
            />

            {matchingOwnVariants.length > 0 && (
              <div className="mb-4">
                <div className="text-[11px] font-medium text-jb-ink/40 uppercase tracking-wide mb-2">
                  Your products
                </div>
                <div className="space-y-2">
                  {matchingOwnVariants.map((row) => (
                    <button
                      key={row.variantId}
                      onClick={() => pickExisting(row)}
                      className="w-full flex items-center justify-between rounded-xl border border-jb-ink/10 bg-white px-4 py-3 text-left"
                    >
                      <span className="text-[14px] text-jb-ink/85">{row.label}</span>
                      <span className="text-[12.5px] text-jb-ink/40">
                        ₦{formatNaira(row.priceKobo)}
                      </span>
                    </button>
                  ))}
                </div>
              </div>
            )}

            {matchingTemplates.length > 0 && (
              <div className="mb-4">
                <div className="text-[11px] font-medium text-jb-ink/40 uppercase tracking-wide mb-2">
                  Add from catalogue
                </div>
                <div className="flex flex-wrap gap-2">
                  {matchingTemplates.slice(0, 30).map((t) => (
                    <button
                      key={t.id}
                      onClick={() => pickTemplate(t)}
                      className="px-4 py-2.5 rounded-full text-[14px] border border-jb-ink/15 text-jb-ink/60 bg-white"
                    >
                      {t.name}
                    </button>
                  ))}
                </div>
              </div>
            )}

            {query && (
              <button
                onClick={pickCustom}
                className="w-full rounded-xl border border-dashed border-jb-ink/25 text-jb-ink/60 px-4 py-3 text-[14px] text-left"
              >
                + Add &ldquo;{search.trim()}&rdquo; as a new product
              </button>
            )}
          </div>
        )}

        {phase === 'price' && target && target.kind !== 'existing' && (
          <div>
            <h2 className="text-lg font-medium mb-1">
              {target.kind === 'template' ? target.template.name : target.name}
            </h2>
            <p className="text-[13px] text-jb-ink/45 mb-4">
              What do you sell this for? You can change this anytime later.
            </p>
            <label className="block mb-4">
              <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">
                Selling price (₦)
              </span>
              <input
                type="number"
                inputMode="numeric"
                placeholder="0"
                value={sellingPriceNaira}
                onChange={(e) => setSellingPriceNaira(e.target.value)}
                className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3.5 text-[15px] text-jb-ink focus:outline-none focus:border-jb-ink/40 transition-colors"
              />
            </label>
            {error && (
              <p role="alert" className="text-[13px] text-red-700 mb-2">
                {error}
              </p>
            )}
            <PrimaryButton
              onClick={() => void confirmPrice()}
              disabled={submitting || !sellingPriceNaira.trim()}
            >
              {submitting ? 'Saving…' : 'Continue'}
            </PrimaryButton>
            <button
              onClick={() => setPhase('search')}
              className="w-full text-[13px] text-jb-ink/40 py-3"
            >
              Back
            </button>
          </div>
        )}

        {phase === 'receive' && target && (
          <div>
            <h2 className="text-lg font-medium mb-1">
              {target.kind === 'existing'
                ? target.label
                : target.kind === 'template'
                  ? target.template.name
                  : target.name}
            </h2>
            <p className="text-[13px] text-jb-ink/45 mb-4">How did you buy this?</p>

            <div className="flex gap-2 mb-4 p-1 rounded-xl bg-jb-ink/[0.05]">
              <button
                onClick={() => setUnitMode('crate')}
                className={`flex-1 rounded-lg py-2.5 text-[13px] font-medium transition-colors ${
                  unitMode === 'crate' ? 'bg-white text-jb-ink shadow-sm' : 'text-jb-ink/45'
                }`}
              >
                By the crate
              </button>
              <button
                onClick={() => setUnitMode('bottle')}
                className={`flex-1 rounded-lg py-2.5 text-[13px] font-medium transition-colors ${
                  unitMode === 'bottle' ? 'bg-white text-jb-ink shadow-sm' : 'text-jb-ink/45'
                }`}
              >
                By the bottle
              </button>
            </div>

            {unitMode === 'crate' ? (
              <div className="flex gap-3 mb-4">
                <label className="flex-1">
                  <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">
                    Per crate
                  </span>
                  <input
                    type="number"
                    inputMode="numeric"
                    placeholder="e.g. 12"
                    value={crateSize}
                    onChange={(e) => setCrateSize(e.target.value)}
                    className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3.5 text-[15px] text-jb-ink focus:outline-none focus:border-jb-ink/40 transition-colors"
                  />
                </label>
                <label className="flex-1">
                  <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">
                    Crates
                  </span>
                  <input
                    type="number"
                    inputMode="numeric"
                    placeholder="e.g. 4"
                    value={crateCount}
                    onChange={(e) => setCrateCount(e.target.value)}
                    className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3.5 text-[15px] text-jb-ink focus:outline-none focus:border-jb-ink/40 transition-colors"
                  />
                </label>
              </div>
            ) : (
              <label className="block mb-4">
                <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">Bottles</span>
                <input
                  type="number"
                  inputMode="numeric"
                  placeholder="e.g. 24"
                  value={bottleCount}
                  onChange={(e) => setBottleCount(e.target.value)}
                  className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3.5 text-[15px] text-jb-ink focus:outline-none focus:border-jb-ink/40 transition-colors"
                />
              </label>
            )}

            <label className="block mb-2">
              <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">
                Total cost (₦)
              </span>
              <input
                type="number"
                inputMode="numeric"
                placeholder="e.g. 43000"
                value={totalCostNaira}
                onChange={(e) => setTotalCostNaira(e.target.value)}
                className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3.5 text-[15px] text-jb-ink focus:outline-none focus:border-jb-ink/40 transition-colors"
              />
            </label>
            <p className="text-[12px] text-jb-ink/40 mb-4">
              {totalUnits > 0
                ? `${totalUnits} bottle${totalUnits === 1 ? '' : 's'}`
                : 'Enter a quantity'}
              {perUnitPreview !== null &&
                ` · ≈ ₦${perUnitPreview.toLocaleString(undefined, { maximumFractionDigits: 2 })}/bottle`}
            </p>

            {error && (
              <p role="alert" className="text-[13px] text-red-700 mb-2">
                {error}
              </p>
            )}
            <PrimaryButton
              onClick={() => void confirmReceive()}
              disabled={submitting || !canReceive}
            >
              {submitting ? 'Saving…' : 'Save'}
            </PrimaryButton>
            <button
              onClick={() => setPhase('search')}
              className="w-full text-[13px] text-jb-ink/40 py-3"
            >
              Back
            </button>
          </div>
        )}

        {phase === 'done' && lastResult && (
          <div className="text-center py-10">
            <div className="w-14 h-14 rounded-full bg-jb-ink text-jb-cream flex items-center justify-center text-xl mb-5 mx-auto">
              ✓
            </div>
            <h2 className="text-lg font-medium mb-1">{lastResult.label} restocked</h2>
            <p className="text-[13px] text-jb-ink/45 mb-8">Now {lastResult.newBalance} in stock.</p>
            <div className="flex flex-col gap-2.5">
              <PrimaryButton onClick={reset}>Add another</PrimaryButton>
              <Link to="/dashboard" className="text-[13px] text-jb-ink/40 py-2">
                Done for now
              </Link>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
