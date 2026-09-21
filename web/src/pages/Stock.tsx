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
import {
  getInventoryHistory,
  receiveStock,
  type AdjustmentReasonCategory,
  type StockHistoryEntry,
} from '../api/inventory';
import { AppBottomNav } from '../components/AppBottomNav';
import { Logo } from '../components/Logo';
import { useConnectivity } from '../hooks/useConnectivity';
import { describeActionError } from '../lib/errors';
import {
  flushPendingAdjustmentRequests,
  flushPendingStockCounts,
  queuePendingAdjustmentRequest,
  queuePendingStockCount,
} from '../lib/inventorySync';
import { getRememberedCrateSize, setRememberedCrateSize } from '../lib/stockPreferences';
import { useSession } from '../lib/session';

// Standard bottle sizes for a brand-new custom product. 35cl/50cl/70cl/75cl
// are the sizes actually used across internal/catalogue.NigerianBarCatalogue's
// curated Nigerian-market research (beer/soft drinks=50cl, whiskey=70cl,
// gin/spirits/bitters/water=75cl, one imported beer=35cl) -- merged here
// with a broader set so anything genuinely different still has a one-tap
// option before falling back to free text.
const STANDARD_SIZES = [
  '33cl',
  '35cl',
  '45cl',
  '50cl',
  '60cl',
  '70cl',
  '75cl',
  '1L',
  '1.5L',
] as const;

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

interface SimpleVariant {
  variantId: string;
  label: string;
  currentStock: number;
}

// Shared search-and-pick list for the Count and Report-issue flows -- a
// smaller version of the search/pick step the Restock flow already has,
// since neither of these needs the catalogue-template or custom-product
// paths (both only ever act on a product that already exists).
function VariantPicker({
  variants,
  onPick,
  onHistory,
}: {
  variants: SimpleVariant[];
  onPick: (v: SimpleVariant) => void;
  onHistory?: (v: SimpleVariant) => void;
}) {
  const [search, setSearch] = useState('');
  const query = search.trim().toLowerCase();
  const matches = query ? variants.filter((v) => v.label.toLowerCase().includes(query)) : variants;

  return (
    <div>
      <input
        type="text"
        inputMode="search"
        placeholder="Search your products…"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3 text-[14px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none focus:border-jb-ink/40 transition-colors mb-3"
      />
      <div className="space-y-2">
        {matches.length === 0 && (
          <p className="text-[13px] text-jb-ink/40 px-1">No products found.</p>
        )}
        {matches.map((v) => (
          <div
            key={v.variantId}
            className="w-full flex items-center justify-between rounded-xl border border-jb-ink/10 bg-white px-4 py-3"
          >
            <button onClick={() => onPick(v)} className="flex-1 text-left min-w-0">
              <span className="text-[14px] text-jb-ink/85">{v.label}</span>
            </button>
            <div className="flex items-center gap-3 shrink-0">
              <span className="text-[12.5px] text-jb-ink/40">{v.currentStock} in stock</span>
              {onHistory && (
                <button
                  onClick={() => onHistory(v)}
                  aria-label={`View history for ${v.label}`}
                  className="text-[12.5px] text-jb-ink/40 hover:text-jb-ink transition-colors underline"
                >
                  History
                </button>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

const ADJUSTMENT_REASONS: {
  value: Exclude<AdjustmentReasonCategory, 'count_correction'>;
  label: string;
}[] = [
  { value: 'complimentary', label: 'Complimentary' },
  { value: 'broken', label: 'Broken' },
  { value: 'spoiled', label: 'Spoiled' },
  { value: 'staff_use', label: 'Staff use' },
  { value: 'manual', label: 'Other correction' },
];

function formatWhen(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  });
}

function historyEventLabel(entry: StockHistoryEntry): string {
  switch (entry.eventType) {
    case 'receipt':
      return 'Stock received';
    case 'sale':
      return 'Sold';
    case 'sale_reversal':
      return 'Sale reversed';
    case 'adjustment': {
      const reasonLabels: Record<string, string> = {
        complimentary: 'Complimentary',
        broken: 'Broken',
        spoiled: 'Spoiled',
        staff_use: 'Staff use',
        manual: 'Manual correction',
        count_correction: 'Stock count correction',
      };
      return reasonLabels[entry.adjustmentReasonCategory] ?? 'Adjustment';
    }
  }
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
type Mode = 'restock' | 'count' | 'adjust';

export function Stock() {
  const { csrfToken, selectedBusinessId, memberships } = useSession();
  const isOwner = memberships.find((m) => m.businessId === selectedBusinessId)?.role === 'owner';
  const isOnline = useConnectivity();

  const [mode, setMode] = useState<Mode>('restock');

  const [phase, setPhase] = useState<Phase>('search');
  const [search, setSearch] = useState('');
  const [ownProducts, setOwnProducts] = useState<CatalogueProduct[] | null>(null);
  const [templates, setTemplates] = useState<CatalogueTemplate[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  // Count flow -- expectedQuantity is read from the just-loaded catalogue
  // (this device's last-known balance) at pick time, never re-derived
  // later, so an offline count still carries a meaningful comparison point
  // even if it sits queued for hours before the server sees it.
  const [countVariant, setCountVariant] = useState<SimpleVariant | null>(null);
  const [countPhysical, setCountPhysical] = useState('');
  const [countSubmitting, setCountSubmitting] = useState(false);
  const [countError, setCountError] = useState<string | null>(null);
  const [countDone, setCountDone] = useState<{
    label: string;
    variance: number;
    queuedOffline: boolean;
  } | null>(null);

  // Report-issue (adjustment request) flow.
  const [adjustVariant, setAdjustVariant] = useState<SimpleVariant | null>(null);
  const [adjustReason, setAdjustReason] =
    useState<Exclude<AdjustmentReasonCategory, 'count_correction'>>('complimentary');
  const [adjustDirection, setAdjustDirection] = useState<'remove' | 'add'>('remove');
  const [adjustQuantity, setAdjustQuantity] = useState('');
  const [adjustNote, setAdjustNote] = useState('');
  const [adjustSubmitting, setAdjustSubmitting] = useState(false);
  const [adjustError, setAdjustError] = useState<string | null>(null);
  const [adjustDone, setAdjustDone] = useState<{ label: string; queuedOffline: boolean } | null>(
    null,
  );

  // Stock history viewer, reachable from any product row.
  const [historyVariant, setHistoryVariant] = useState<SimpleVariant | null>(null);
  const [history, setHistory] = useState<StockHistoryEntry[] | null>(null);
  const [historyError, setHistoryError] = useState<string | null>(null);

  // Quick price edit on the "Your products" restock list -- owner only
  // (catalogue:manage). editingPriceVariantId identifies which row (if
  // any) currently shows an editable price instead of the plain badge.
  const [editingPriceVariantId, setEditingPriceVariantId] = useState<string | null>(null);
  const [editingPriceValue, setEditingPriceValue] = useState('');
  const [savingPriceVariantId, setSavingPriceVariantId] = useState<string | null>(null);
  const [priceEditError, setPriceEditError] = useState<string | null>(null);

  const [target, setTarget] = useState<Target | null>(null);
  const [variantId, setVariantId] = useState<string | null>(null);
  const [sellingPriceNaira, setSellingPriceNaira] = useState('');

  // New-custom-product sizing (target.kind === 'custom' only -- a
  // template's variant name already comes from the platform catalogue).
  // Standard sizes are the ones actually seeded in
  // internal/catalogue.NigerianBarCatalogue (35/50/70/75cl) merged with
  // a broader "just in case" set, plus a Can container option and a
  // free-text escape hatch for anything genuinely nonstandard.
  const [selectedSize, setSelectedSize] = useState<(typeof STANDARD_SIZES)[number]>('50cl');
  const [containerType, setContainerType] = useState<'Bottle' | 'Can'>('Bottle');
  const [useCustomSize, setUseCustomSize] = useState(false);
  const [customSizeName, setCustomSizeName] = useState('');
  // A service item (a snooker/pool game, table time) has no size at all --
  // see internal/catalogue.Variant's tracks_inventory doc comment.
  const [isServiceItem, setIsServiceItem] = useState(false);
  const [serviceItemName, setServiceItemName] = useState('');

  const [unitMode, setUnitMode] = useState<'crate' | 'bottle'>('crate');
  const [crateSize, setCrateSize] = useState('');
  const [crateCount, setCrateCount] = useState('');
  const [bottleCount, setBottleCount] = useState('');
  const [totalCostNaira, setTotalCostNaira] = useState('');

  const [lastResult, setLastResult] = useState<{
    label: string;
    newBalance: number;
    isServiceItem?: boolean;
  } | null>(null);
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
    } catch (err) {
      setLoadError(describeActionError(err, 'Could not load your products.'));
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
        // A service item (tracks_inventory=false) has nothing to
        // restock, ever -- see internal/catalogue.Variant's own doc
        // comment -- so it's deliberately absent from this list. Its
        // price is still editable from Settings.
        if (!variant.tracksInventory) continue;
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

  const allVariants: SimpleVariant[] = useMemo(() => {
    if (!ownProducts) return [];
    const rows: SimpleVariant[] = [];
    for (const product of ownProducts) {
      for (const variant of product.variants) {
        // Neither counting nor reporting a stock issue makes sense for a
        // non-stocked service item -- same reasoning as
        // matchingOwnVariants above.
        if (!variant.active || !variant.tracksInventory) continue;
        rows.push({
          variantId: variant.id,
          label: `${product.name} — ${variant.name}`,
          currentStock: variant.currentStock,
        });
      }
    }
    return rows;
  }, [ownProducts]);

  useEffect(() => {
    if (!isOnline || !selectedBusinessId || !csrfToken) return;
    // Catches up on anything queued from an earlier offline session, same
    // reasoning as Sell.tsx's flushPendingSales effect.
    void flushPendingStockCounts(selectedBusinessId, csrfToken);
    void flushPendingAdjustmentRequests(selectedBusinessId, csrfToken);
  }, [isOnline, selectedBusinessId, csrfToken]);

  function pickCountVariant(v: SimpleVariant) {
    setCountVariant(v);
    setCountPhysical('');
    setCountError(null);
  }

  const countPhysicalNum = Number(countPhysical);
  const canSubmitCount =
    countVariant !== null && countPhysical.trim() !== '' && !Number.isNaN(countPhysicalNum);

  async function submitCount() {
    if (!countVariant || !selectedBusinessId || !canSubmitCount) return;
    setCountSubmitting(true);
    setCountError(null);
    try {
      const idempotencyKey = crypto.randomUUID();
      const startedAt = new Date().toISOString();
      const lines = [
        {
          variantId: countVariant.variantId,
          expectedQuantity: countVariant.currentStock,
          physicalQuantity: countPhysicalNum,
        },
      ];
      // Written to IndexedDB first, before any network call -- same
      // "queue recorded" discipline as Sell.tsx's completeSale.
      await queuePendingStockCount({
        idempotencyKey,
        businessId: selectedBusinessId,
        startedAt,
        lines,
        status: 'pending',
        createdAt: startedAt,
      });
      setCountDone({
        label: countVariant.label,
        variance: countPhysicalNum - countVariant.currentStock,
        queuedOffline: !isOnline || !csrfToken,
      });
      if (isOnline && csrfToken) {
        void flushPendingStockCounts(selectedBusinessId, csrfToken).then(
          () => void loadCatalogue(),
        );
      }
    } finally {
      setCountSubmitting(false);
    }
  }

  function resetCount() {
    setCountVariant(null);
    setCountPhysical('');
    setCountError(null);
    setCountDone(null);
  }

  function pickAdjustVariant(v: SimpleVariant) {
    setAdjustVariant(v);
    setAdjustReason('complimentary');
    setAdjustDirection('remove');
    setAdjustQuantity('');
    setAdjustNote('');
    setAdjustError(null);
  }

  const adjustQuantityNum = Number(adjustQuantity);
  const canSubmitAdjustment =
    adjustVariant !== null &&
    adjustQuantity.trim() !== '' &&
    !Number.isNaN(adjustQuantityNum) &&
    adjustQuantityNum > 0 &&
    adjustNote.trim() !== '';

  async function submitAdjustment() {
    if (!adjustVariant || !selectedBusinessId || !canSubmitAdjustment) return;
    setAdjustSubmitting(true);
    setAdjustError(null);
    try {
      const idempotencyKey = crypto.randomUUID();
      const createdAt = new Date().toISOString();
      const magnitude = Math.round(adjustQuantityNum);
      // Complimentary/broken/spoiled/staff_use are always a loss (negative);
      // only 'manual' offers a direction, since it's the general-purpose
      // catch-all correction (docs/PHASE_INVENTORY_COUNTS_AND_ADJUSTMENTS.md
      // §7).
      const quantityDelta =
        adjustReason === 'manual' && adjustDirection === 'add' ? magnitude : -magnitude;
      await queuePendingAdjustmentRequest({
        idempotencyKey,
        businessId: selectedBusinessId,
        variantId: adjustVariant.variantId,
        quantityDelta,
        reasonCategory: adjustReason,
        reasonNote: adjustNote.trim(),
        status: 'pending',
        createdAt,
      });
      setAdjustDone({ label: adjustVariant.label, queuedOffline: !isOnline || !csrfToken });
      if (isOnline && csrfToken) {
        void flushPendingAdjustmentRequests(selectedBusinessId, csrfToken);
      }
    } finally {
      setAdjustSubmitting(false);
    }
  }

  function resetAdjustment() {
    setAdjustVariant(null);
    setAdjustQuantity('');
    setAdjustNote('');
    setAdjustError(null);
    setAdjustDone(null);
  }

  function openHistory(v: SimpleVariant) {
    setHistoryVariant(v);
    setHistory(null);
    setHistoryError(null);
    if (!selectedBusinessId) return;
    getInventoryHistory(v.variantId, selectedBusinessId)
      .then(setHistory)
      .catch((err: unknown) =>
        setHistoryError(describeActionError(err, 'Could not load history.')),
      );
  }

  function pickExisting(row: { variantId: string; label: string }) {
    setTarget({ kind: 'existing', variantId: row.variantId, label: row.label });
    setVariantId(row.variantId);
    setCrateSize(getRememberedCrateSize(row.variantId));
    setPhase('receive');
  }

  function startEditingPrice(row: { variantId: string; priceKobo: number }) {
    setPriceEditError(null);
    setEditingPriceVariantId(row.variantId);
    setEditingPriceValue((row.priceKobo / 100).toString());
  }

  function cancelEditingPrice() {
    setEditingPriceVariantId(null);
    setPriceEditError(null);
  }

  async function saveEditingPrice(variantIdToSave: string) {
    if (!selectedBusinessId || !csrfToken) return;
    const priceKobo = Math.round(Number(editingPriceValue) * 100);
    if (!priceKobo || priceKobo <= 0) {
      setPriceEditError('Enter a price greater than zero.');
      return;
    }
    setSavingPriceVariantId(variantIdToSave);
    setPriceEditError(null);
    try {
      await setVariantPrice(variantIdToSave, priceKobo, selectedBusinessId, csrfToken);
      setEditingPriceVariantId(null);
      void loadCatalogue();
    } catch (err) {
      setPriceEditError(describeActionError(err, 'Could not update this price.'));
    } finally {
      setSavingPriceVariantId(null);
    }
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
    setSelectedSize('50cl');
    setContainerType('Bottle');
    setUseCustomSize(false);
    setCustomSizeName('');
    setIsServiceItem(false);
    setServiceItemName('');
    setPhase('price');
  }

  function customVariantName(): string {
    if (isServiceItem) return serviceItemName.trim();
    if (useCustomSize) return customSizeName.trim();
    return `${selectedSize} ${containerType}`;
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
          customVariantName(),
          priceKobo,
          selectedBusinessId,
          csrfToken,
          !isServiceItem,
        );
        newVariantId = variant.id;
      }
      setVariantId(newVariantId);
      setCrateSize(getRememberedCrateSize(newVariantId));
      // A service item (tracks_inventory=false) can never be "received" --
      // there's nothing to restock -- so skip straight to done instead of
      // the crate/bottle counting phase.
      if (target.kind === 'custom' && isServiceItem) {
        setLastResult({
          label: customVariantName() || target.name,
          newBalance: 0,
          isServiceItem: true,
        });
        setPhase('done');
      } else {
        setPhase('receive');
      }
      void loadCatalogue();
    } catch (err) {
      setError(describeActionError(err, 'Could not save this product.'));
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
      setError(describeActionError(err, 'Could not save this receipt.'));
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
    setSelectedSize('50cl');
    setContainerType('Bottle');
    setUseCustomSize(false);
    setCustomSizeName('');
    setIsServiceItem(false);
    setServiceItemName('');
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

        <div className="flex gap-2 mb-5 p-1 rounded-xl bg-jb-ink/[0.05]">
          <button
            onClick={() => setMode('restock')}
            className={`flex-1 rounded-lg py-2.5 text-[13px] font-medium transition-colors ${
              mode === 'restock' ? 'bg-white text-jb-ink shadow-sm' : 'text-jb-ink/45'
            }`}
          >
            Restock
          </button>
          <button
            onClick={() => setMode('count')}
            className={`flex-1 rounded-lg py-2.5 text-[13px] font-medium transition-colors ${
              mode === 'count' ? 'bg-white text-jb-ink shadow-sm' : 'text-jb-ink/45'
            }`}
          >
            Count
          </button>
          <button
            onClick={() => setMode('adjust')}
            className={`flex-1 rounded-lg py-2.5 text-[13px] font-medium transition-colors ${
              mode === 'adjust' ? 'bg-white text-jb-ink shadow-sm' : 'text-jb-ink/45'
            }`}
          >
            Report issue
          </button>
        </div>

        {mode === 'count' && (
          <div>
            {countDone ? (
              <div className="text-center py-10">
                <div className="w-14 h-14 rounded-full bg-jb-ink text-jb-cream flex items-center justify-center text-xl mb-5 mx-auto">
                  ✓
                </div>
                <h2 className="text-lg font-medium mb-1">{countDone.label} counted</h2>
                <p className="text-[13px] text-jb-ink/45 mb-1">
                  {countDone.variance === 0
                    ? 'Matched what was expected.'
                    : `Variance: ${countDone.variance > 0 ? '+' : ''}${countDone.variance} bottle${
                        Math.abs(countDone.variance) === 1 ? '' : 's'
                      }.`}
                </p>
                {countDone.variance !== 0 && (
                  <p className="text-[12.5px] text-jb-ink/40 mb-8">
                    A correction request has been sent to the owner for approval.
                  </p>
                )}
                {countDone.queuedOffline && (
                  <p className="text-[12.5px] text-jb-ink/40 mb-8">
                    Saved on this device — will send once you're back online.
                  </p>
                )}
                <div className="flex flex-col gap-2.5">
                  <PrimaryButton onClick={resetCount}>Count another</PrimaryButton>
                  <Link to="/dashboard" className="text-[13px] text-jb-ink/40 py-2">
                    Done for now
                  </Link>
                </div>
              </div>
            ) : countVariant ? (
              <div>
                <h2 className="text-lg font-medium mb-1">{countVariant.label}</h2>
                <p className="text-[13px] text-jb-ink/45 mb-4">
                  Expected {countVariant.currentStock} in stock. How many did you actually count?
                </p>
                <label className="block mb-2">
                  <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">
                    Physical count
                  </span>
                  <input
                    type="number"
                    inputMode="numeric"
                    placeholder="e.g. 46"
                    value={countPhysical}
                    onChange={(e) => setCountPhysical(e.target.value)}
                    className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3.5 text-[15px] text-jb-ink focus:outline-none focus:border-jb-ink/40 transition-colors"
                  />
                </label>
                {countPhysical.trim() !== '' && !Number.isNaN(countPhysicalNum) && (
                  <p className="text-[12px] text-jb-ink/40 mb-4">
                    Variance: {countPhysicalNum - countVariant.currentStock > 0 ? '+' : ''}
                    {countPhysicalNum - countVariant.currentStock}
                  </p>
                )}
                {countError && (
                  <p role="alert" className="text-[13px] text-red-700 mb-2">
                    {countError}
                  </p>
                )}
                <PrimaryButton
                  onClick={() => void submitCount()}
                  disabled={countSubmitting || !canSubmitCount}
                >
                  {countSubmitting ? 'Saving…' : 'Save count'}
                </PrimaryButton>
                <button
                  onClick={() => setCountVariant(null)}
                  className="w-full text-[13px] text-jb-ink/40 py-3"
                >
                  Back
                </button>
              </div>
            ) : (
              <div>
                <p className="text-[13px] text-jb-ink/45 mb-4">
                  Pick a product to count its physical stock.
                </p>
                <VariantPicker
                  variants={allVariants}
                  onPick={pickCountVariant}
                  onHistory={openHistory}
                />
              </div>
            )}
          </div>
        )}

        {mode === 'adjust' && (
          <div>
            {adjustDone ? (
              <div className="text-center py-10">
                <div className="w-14 h-14 rounded-full bg-jb-ink text-jb-cream flex items-center justify-center text-xl mb-5 mx-auto">
                  ✓
                </div>
                <h2 className="text-lg font-medium mb-1">Reported</h2>
                <p className="text-[13px] text-jb-ink/45 mb-1">
                  Sent to the owner for approval — {adjustDone.label}.
                </p>
                {adjustDone.queuedOffline && (
                  <p className="text-[12.5px] text-jb-ink/40 mb-8">
                    Saved on this device — will send once you're back online.
                  </p>
                )}
                <div className="flex flex-col gap-2.5 mt-8">
                  <PrimaryButton onClick={resetAdjustment}>Report another</PrimaryButton>
                  <Link to="/dashboard" className="text-[13px] text-jb-ink/40 py-2">
                    Done for now
                  </Link>
                </div>
              </div>
            ) : adjustVariant ? (
              <div>
                <h2 className="text-lg font-medium mb-1">{adjustVariant.label}</h2>
                <p className="text-[13px] text-jb-ink/45 mb-4">What happened to this stock?</p>

                <div className="flex flex-wrap gap-2 mb-4">
                  {ADJUSTMENT_REASONS.map((r) => (
                    <button
                      key={r.value}
                      onClick={() => setAdjustReason(r.value)}
                      className={`px-3.5 py-2 rounded-full text-[13px] border transition-colors ${
                        adjustReason === r.value
                          ? 'bg-jb-ink text-jb-cream border-jb-ink'
                          : 'border-jb-ink/15 text-jb-ink/60 bg-white'
                      }`}
                    >
                      {r.label}
                    </button>
                  ))}
                </div>

                {adjustReason === 'manual' && (
                  <div className="flex gap-2 mb-4 p-1 rounded-xl bg-jb-ink/[0.05]">
                    <button
                      onClick={() => setAdjustDirection('remove')}
                      className={`flex-1 rounded-lg py-2 text-[13px] font-medium transition-colors ${
                        adjustDirection === 'remove'
                          ? 'bg-white text-jb-ink shadow-sm'
                          : 'text-jb-ink/45'
                      }`}
                    >
                      Remove stock
                    </button>
                    <button
                      onClick={() => setAdjustDirection('add')}
                      className={`flex-1 rounded-lg py-2 text-[13px] font-medium transition-colors ${
                        adjustDirection === 'add'
                          ? 'bg-white text-jb-ink shadow-sm'
                          : 'text-jb-ink/45'
                      }`}
                    >
                      Add stock
                    </button>
                  </div>
                )}

                <label className="block mb-4">
                  <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">
                    How many bottles?
                  </span>
                  <input
                    type="number"
                    inputMode="numeric"
                    placeholder="e.g. 1"
                    value={adjustQuantity}
                    onChange={(e) => setAdjustQuantity(e.target.value)}
                    className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3.5 text-[15px] text-jb-ink focus:outline-none focus:border-jb-ink/40 transition-colors"
                  />
                </label>

                <label className="block mb-2">
                  <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">Note</span>
                  <textarea
                    value={adjustNote}
                    onChange={(e) => setAdjustNote(e.target.value)}
                    placeholder="What happened? e.g. bottle dropped at the bar."
                    rows={2}
                    className="w-full rounded-lg border border-jb-ink/15 bg-white px-3 py-2 text-[13px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none focus:border-jb-ink/40 transition-colors"
                  />
                </label>

                {adjustError && (
                  <p role="alert" className="text-[13px] text-red-700 mb-2 mt-2">
                    {adjustError}
                  </p>
                )}
                <div className="mt-4">
                  <PrimaryButton
                    onClick={() => void submitAdjustment()}
                    disabled={adjustSubmitting || !canSubmitAdjustment}
                  >
                    {adjustSubmitting ? 'Sending…' : 'Send for approval'}
                  </PrimaryButton>
                  <button
                    onClick={() => setAdjustVariant(null)}
                    className="w-full text-[13px] text-jb-ink/40 py-3"
                  >
                    Back
                  </button>
                </div>
              </div>
            ) : (
              <div>
                <p className="text-[13px] text-jb-ink/45 mb-4">
                  Pick a product to report a loss (complimentary, broken, spoiled, staff use, or
                  another correction).
                </p>
                <VariantPicker
                  variants={allVariants}
                  onPick={pickAdjustVariant}
                  onHistory={openHistory}
                />
              </div>
            )}
          </div>
        )}

        {historyVariant && (
          <div className="fixed inset-0 bg-jb-ink/40 flex items-end sm:items-center justify-center z-50 px-0 sm:px-5">
            <div className="w-full sm:max-w-md max-h-[80vh] overflow-y-auto bg-jb-cream rounded-t-2xl sm:rounded-2xl p-5">
              <div className="flex items-center justify-between mb-4">
                <h2 className="text-lg font-medium">{historyVariant.label}</h2>
                <button
                  onClick={() => setHistoryVariant(null)}
                  aria-label="Close"
                  className="text-jb-ink/40 hover:text-jb-ink text-lg leading-none"
                >
                  ✕
                </button>
              </div>
              {historyError && <p className="text-[13px] text-red-700 mb-3">{historyError}</p>}
              {history === null && !historyError && (
                <p className="text-[13px] text-jb-ink/40">Loading…</p>
              )}
              {history !== null && history.length === 0 && (
                <p className="text-[13px] text-jb-ink/40">No movements yet.</p>
              )}
              <div className="divide-y divide-jb-ink/[0.06]">
                {(history ?? []).map((entry) => (
                  <div key={entry.id} className="py-3">
                    <div className="flex items-center justify-between gap-3">
                      <span className="text-[13px] text-jb-ink/80">{historyEventLabel(entry)}</span>
                      <span
                        className={`text-[13px] font-medium ${
                          entry.quantityDelta < 0 ? 'text-red-700/80' : 'text-jb-green-dark'
                        }`}
                      >
                        {entry.quantityDelta > 0 ? '+' : ''}
                        {entry.quantityDelta}
                      </span>
                    </div>
                    <div className="text-[11px] text-jb-ink/40 mt-0.5">
                      {entry.actorName && (
                        <span className="font-semibold text-jb-ink/55">{entry.actorName}</span>
                      )}{' '}
                      {formatWhen(entry.createdAt)}
                    </div>
                    {entry.adjustmentReasonNote && (
                      <div className="text-[11.5px] text-jb-ink/50 mt-1">
                        {entry.adjustmentReasonNote}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}

        {mode === 'restock' && phase === 'search' && (
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
                    <div
                      key={row.variantId}
                      className="w-full flex items-center justify-between gap-2 rounded-xl border border-jb-ink/10 bg-white px-4 py-3"
                    >
                      <button
                        onClick={() => pickExisting(row)}
                        className="flex-1 min-w-0 text-left text-[14px] text-jb-ink/85 truncate"
                      >
                        {row.label}
                      </button>

                      {editingPriceVariantId === row.variantId ? (
                        <div
                          className="flex items-center gap-1.5 shrink-0"
                          onClick={(e) => e.stopPropagation()}
                        >
                          <span className="text-[12.5px] text-jb-ink/40">₦</span>
                          <input
                            type="number"
                            inputMode="decimal"
                            autoFocus
                            value={editingPriceValue}
                            onChange={(e) => setEditingPriceValue(e.target.value)}
                            className="w-16 rounded-lg border border-jb-ink/15 px-2 py-1 text-[12.5px] focus:outline-none focus:border-jb-ink/40"
                          />
                          <button
                            onClick={() => void saveEditingPrice(row.variantId)}
                            disabled={savingPriceVariantId === row.variantId}
                            className="text-[12px] font-medium text-jb-green disabled:opacity-40"
                          >
                            {savingPriceVariantId === row.variantId ? '…' : 'Save'}
                          </button>
                          <button
                            onClick={cancelEditingPrice}
                            className="text-[12px] text-jb-ink/40"
                          >
                            ✕
                          </button>
                        </div>
                      ) : isOwner ? (
                        <button
                          onClick={() => startEditingPrice(row)}
                          className="shrink-0 text-[12.5px] text-jb-ink/40 hover:text-jb-ink/70 transition-colors underline decoration-dotted underline-offset-2"
                        >
                          ₦{formatNaira(row.priceKobo)}
                        </button>
                      ) : (
                        <span className="shrink-0 text-[12.5px] text-jb-ink/40">
                          ₦{formatNaira(row.priceKobo)}
                        </span>
                      )}
                    </div>
                  ))}
                </div>
                {priceEditError && (
                  <p className="text-[12px] text-red-700 mt-2">{priceEditError}</p>
                )}
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

        {mode === 'restock' && phase === 'price' && target && target.kind !== 'existing' && (
          <div>
            <h2 className="text-lg font-medium mb-1">
              {target.kind === 'template' ? target.template.name : target.name}
            </h2>
            <p className="text-[13px] text-jb-ink/45 mb-4">
              What do you sell this for? You can change this anytime later.
            </p>

            {target.kind === 'custom' && (
              <div className="mb-4">
                <button
                  onClick={() => setIsServiceItem((v) => !v)}
                  className={`w-full text-left rounded-xl border px-4 py-3 mb-3 transition-colors ${
                    isServiceItem
                      ? 'border-jb-ink/30 bg-jb-ink/[0.04]'
                      : 'border-jb-ink/15 bg-white'
                  }`}
                >
                  <span className="text-[13.5px] font-medium text-jb-ink/80">
                    {isServiceItem ? '☑' : '☐'} This is a service, not stock
                  </span>
                  <span className="block text-[11.5px] text-jb-ink/40 mt-0.5">
                    e.g. a snooker/pool game, table time — nothing to restock, ever
                  </span>
                </button>

                {isServiceItem ? (
                  <label className="block">
                    <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">
                      What do you call it?
                    </span>
                    <input
                      type="text"
                      placeholder="e.g. Game, Hour"
                      value={serviceItemName}
                      onChange={(e) => setServiceItemName(e.target.value)}
                      className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3 text-[14px] focus:outline-none focus:border-jb-ink/40"
                    />
                  </label>
                ) : (
                  <div>
                    <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">
                      Size
                    </span>
                    {!useCustomSize ? (
                      <>
                        <div className="flex flex-wrap gap-2 mb-2">
                          {STANDARD_SIZES.map((size) => (
                            <button
                              key={size}
                              onClick={() => setSelectedSize(size)}
                              className={`px-3.5 py-2 rounded-full text-[13px] border transition-colors ${
                                selectedSize === size
                                  ? 'border-jb-ink bg-jb-ink text-jb-cream'
                                  : 'border-jb-ink/15 text-jb-ink/60 bg-white'
                              }`}
                            >
                              {size}
                            </button>
                          ))}
                        </div>
                        <div className="flex gap-2 p-1 rounded-xl bg-jb-ink/[0.05] mb-2">
                          {(['Bottle', 'Can'] as const).map((c) => (
                            <button
                              key={c}
                              onClick={() => setContainerType(c)}
                              className={`flex-1 rounded-lg py-2 text-[12.5px] font-medium transition-colors ${
                                containerType === c
                                  ? 'bg-white text-jb-ink shadow-sm'
                                  : 'text-jb-ink/45'
                              }`}
                            >
                              {c}
                            </button>
                          ))}
                        </div>
                        <button
                          onClick={() => setUseCustomSize(true)}
                          className="text-[12px] text-jb-ink/40 underline underline-offset-2"
                        >
                          Not a standard size? Enter your own
                        </button>
                      </>
                    ) : (
                      <>
                        <input
                          type="text"
                          placeholder="e.g. Half Bottle, Shot"
                          value={customSizeName}
                          onChange={(e) => setCustomSizeName(e.target.value)}
                          className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3 text-[14px] focus:outline-none focus:border-jb-ink/40 mb-2"
                        />
                        <button
                          onClick={() => setUseCustomSize(false)}
                          className="text-[12px] text-jb-ink/40 underline underline-offset-2"
                        >
                          Use a standard size instead
                        </button>
                      </>
                    )}
                  </div>
                )}
              </div>
            )}

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
              disabled={
                submitting ||
                !sellingPriceNaira.trim() ||
                (target.kind === 'custom' &&
                  ((isServiceItem && !serviceItemName.trim()) ||
                    (!isServiceItem && useCustomSize && !customSizeName.trim())))
              }
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

        {mode === 'restock' && phase === 'receive' && target && (
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

        {mode === 'restock' && phase === 'done' && lastResult && (
          <div className="text-center py-10">
            <div className="w-14 h-14 rounded-full bg-jb-ink text-jb-cream flex items-center justify-center text-xl mb-5 mx-auto">
              ✓
            </div>
            <h2 className="text-lg font-medium mb-1">
              {lastResult.isServiceItem
                ? `${lastResult.label} added`
                : `${lastResult.label} restocked`}
            </h2>
            <p className="text-[13px] text-jb-ink/45 mb-8">
              {lastResult.isServiceItem
                ? 'Ready to sell — no stock to track for this one.'
                : `Now ${lastResult.newBalance} in stock.`}
            </p>
            <div className="flex flex-col gap-2.5">
              <PrimaryButton onClick={reset}>Add another</PrimaryButton>
              <Link to="/dashboard" className="text-[13px] text-jb-ink/40 py-2">
                Done for now
              </Link>
            </div>
          </div>
        )}
      </div>
      <AppBottomNav />
    </div>
  );
}
