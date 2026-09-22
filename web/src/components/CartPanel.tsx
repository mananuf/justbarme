import type { ReactNode } from 'react';

export interface CartLine {
  variantId: string;
  description: string;
  unitPriceKobo: number;
  quantity: number;
  // True when there's nothing left to add beyond what's already on this
  // line -- see PickableVariant.outOfStock's doc comment for why this is
  // computed by the caller rather than here. Disables the "+" button
  // instead of silently letting a tap create a negative inventory
  // balance.
  atStockLimit?: boolean;
}

function formatNaira(kobo: number): string {
  return (kobo / 100).toLocaleString();
}

interface CartPanelProps {
  /** e.g. "THIS SALE" or "THIS ROUND". */
  label: string;
  lines: CartLine[];
  emptyMessage: string;
  onChangeQty: (variantId: string, delta: number) => void;
  /** Rendered below the line list only once there's at least one line -- a
   * payment method + Complete Sale button for a walk-in, or a plain "Add
   * round" button for a tab. */
  footer?: ReactNode;
}

// CartPanel is the shared "what's been picked so far" bottom sheet, paired
// with ProductGrid above it. Kept as its own component (rather than folded
// into ProductGrid) because the two callers need different footers, not
// because the picking/reviewing halves ever need to vary independently.
export function CartPanel({ label, lines, emptyMessage, onChangeQty, footer }: CartPanelProps) {
  const itemCount = lines.reduce((sum, l) => sum + l.quantity, 0);

  return (
    <div className="shrink-0 border-t border-jb-ink/10 bg-white/70">
      <div className="max-w-md mx-auto w-full px-5 pt-3 pb-2 flex flex-col max-h-[42vh]">
        <div className="shrink-0 flex items-center justify-between mb-2">
          <span className="text-[11px] text-jb-ink/45 tracking-wide">{label}</span>
          {itemCount > 0 && (
            <span className="text-[11px] text-jb-ink/45">
              {itemCount} item{itemCount === 1 ? '' : 's'}
            </span>
          )}
        </div>

        <div className="flex-1 min-h-0 overflow-y-auto space-y-2">
          {lines.length === 0 && (
            <p className="text-[12.5px] text-jb-ink/40 text-center py-4">{emptyMessage}</p>
          )}
          {lines.map((line) => (
            <div
              key={line.variantId}
              className="flex items-center justify-between rounded-xl border border-jb-ink/10 bg-white px-4 py-3"
            >
              <div>
                <div className="text-[13.5px] text-jb-ink/80">{line.description}</div>
                <div className="text-[11px] text-jb-ink/40">
                  ₦{formatNaira(line.unitPriceKobo)} each
                </div>
                {line.atStockLimit && (
                  <div className="text-[11px] font-medium text-red-600 mt-0.5">
                    No more in stock — add stock first
                  </div>
                )}
              </div>
              <div className="flex items-center gap-3">
                <button
                  onClick={() => onChangeQty(line.variantId, -1)}
                  className="w-7 h-7 rounded-full border border-jb-ink/20 flex items-center justify-center text-jb-ink"
                >
                  −
                </button>
                <span className="text-[14px] font-medium w-4 text-center">{line.quantity}</span>
                <button
                  onClick={() => onChangeQty(line.variantId, 1)}
                  disabled={line.atStockLimit}
                  className="w-7 h-7 rounded-full border border-jb-ink/20 flex items-center justify-center text-jb-ink disabled:opacity-30 disabled:cursor-not-allowed"
                >
                  +
                </button>
              </div>
            </div>
          ))}
        </div>

        {lines.length > 0 && footer && <div className="shrink-0 pt-3">{footer}</div>}
      </div>
    </div>
  );
}
