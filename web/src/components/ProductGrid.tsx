import { useState } from 'react';
import type { ReactNode } from 'react';

export interface PickableVariant {
  variantId: string;
  description: string;
  priceKobo: number;
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
}: ProductGridProps) {
  const [search, setSearch] = useState('');
  const query = search.trim().toLowerCase();
  const filtered = query
    ? variants.filter((v) => v.description.toLowerCase().includes(query))
    : variants;

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
      <div className="grid grid-cols-2 gap-2.5">
        {filtered.map((v) => {
          const qty = cartQuantities[v.variantId] ?? 0;
          return (
            <button
              key={v.variantId}
              onClick={() => onAdd(v)}
              className={`rounded-xl border p-4 text-left transition-colors active:scale-[0.97] ${
                qty > 0
                  ? 'border-jb-ink bg-jb-ink text-jb-cream'
                  : 'border-jb-ink/10 bg-white text-jb-ink'
              }`}
            >
              <div className="text-[14px] font-medium">{v.description}</div>
              <div
                className={`text-[11px] mt-0.5 ${qty > 0 ? 'text-jb-cream/60' : 'text-jb-ink/45'}`}
              >
                ₦{formatNaira(v.priceKobo)}
              </div>
              {qty > 0 && (
                <div className="mt-2 text-[12px] font-medium">
                  {qty} {inCartLabel}
                </div>
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
}
