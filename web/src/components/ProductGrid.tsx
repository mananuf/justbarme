import { useState } from 'react';
import type { ReactNode } from 'react';

export interface PickableVariant {
  variantId: string;
  description: string;
  priceKobo: number;
  // True when there is nothing left to add right now -- computed by the
  // caller (Sell.tsx/Tabs.tsx), since "how much is already spoken for"
  // means something different for a not-yet-posted draft cart vs. a tab
  // whose rounds post immediately. Always false for a non-stocked
  // service item. A tile for this stays visible but disabled rather than
  // silently letting a tap create a negative inventory balance.
  outOfStock?: boolean;
}

function formatNaira(kobo: number): string {
  return (kobo / 100).toLocaleString();
}

interface ProductGridProps {
  variants: PickableVariant[];
  isLoading: boolean;
  loadError?: string | null;
  /** Shown instead of the grid when there are no variants at all (not a search-filter miss). */
  emptyMessage?: ReactNode;
  /** variantId -> quantity already picked, for highlighting a card and showing its count. */
  cartQuantities: Record<string, number>;
  /** Label under the count badge on a picked card, e.g. "in sale" or "in round". */
  inCartLabel: string;
  onAdd: (variant: PickableVariant) => void;
  /** Tailwind max-height class (e.g. "max-h-[42vh]") applied to the results
   * grid only -- the search input stays fixed above it. Tabs.tsx sets this
   * so a long catalogue scrolls in place instead of pushing "current
   * order"/payment/rounds far down the page; the catalogue is searchable,
   * so nothing is lost by not showing every tile at once. Sell.tsx leaves
   * this unset since its own outer layout already scrolls the whole
   * product-picker half independently. */
  scrollMaxHeightClassName?: string;
}

// ProductGrid is the shared search + tap-to-add drink picker used by both
// Sell.tsx (a walk-in sale) and Tabs.tsx (adding a round to an open tab) --
// extracted so "seamless adding" can never drift between the two contexts,
// since they render the exact same component rather than two hand-kept-
// in-sync copies.
export function ProductGrid({
  variants,
  isLoading,
  loadError,
  emptyMessage,
  cartQuantities,
  inCartLabel,
  onAdd,
  scrollMaxHeightClassName,
}: ProductGridProps) {
  const [search, setSearch] = useState('');
  const query = search.trim().toLowerCase();
  const filtered = query
    ? variants.filter((v) => v.description.toLowerCase().includes(query))
    : variants;

  const grid = (
    <div className="grid grid-cols-2 gap-2.5">
      {filtered.map((v) => {
        const qty = cartQuantities[v.variantId] ?? 0;
        const outOfStock = !!v.outOfStock;
        return (
          <button
            key={v.variantId}
            onClick={() => onAdd(v)}
            disabled={outOfStock}
            className={`rounded-xl border p-4 text-left transition-colors ${
              outOfStock
                ? 'border-jb-ink/10 bg-jb-ink/5 text-jb-ink/40 cursor-not-allowed'
                : qty > 0
                  ? 'border-jb-ink bg-jb-ink text-jb-cream active:scale-[0.97]'
                  : 'border-jb-ink/10 bg-white text-jb-ink active:scale-[0.97]'
            }`}
          >
            <div className="text-[14px] font-medium">{v.description}</div>
            <div
              className={`text-[11px] mt-0.5 ${
                outOfStock ? 'text-jb-ink/35' : qty > 0 ? 'text-jb-cream/60' : 'text-jb-ink/45'
              }`}
            >
              ₦{formatNaira(v.priceKobo)}
            </div>
            {outOfStock ? (
              <div className="mt-2 text-[11px] font-medium text-red-600">
                {qty > 0
                  ? `${qty} ${inCartLabel} — none left to add`
                  : 'Out of stock — add stock first'}
              </div>
            ) : (
              qty > 0 && (
                <div className="mt-2 text-[12px] font-medium">
                  {qty} {inCartLabel}
                </div>
              )
            )}
          </button>
        );
      })}
    </div>
  );

  return (
    <div>
      <input
        type="text"
        inputMode="search"
        placeholder="Search drinks…"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3 text-[14px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none focus:border-jb-ink/40 transition-colors mb-3"
      />
      {loadError && (
        <p role="alert" className="text-[13px] text-red-700 mb-2">
          {loadError}
        </p>
      )}
      {isLoading && !loadError && <p className="text-[13px] text-jb-ink/45">Loading products…</p>}
      {!isLoading && variants.length === 0 && emptyMessage}
      {!isLoading && variants.length > 0 && filtered.length === 0 && (
        <p className="text-[13px] text-jb-ink/40">No drinks match &ldquo;{search.trim()}&rdquo;.</p>
      )}
      {scrollMaxHeightClassName ? (
        <div className={`${scrollMaxHeightClassName} overflow-y-auto -mr-1 pr-1`}>{grid}</div>
      ) : (
        grid
      )}
    </div>
  );
}
