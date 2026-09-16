# Phase — Stock receiving (thin slice of Phase 7)

**Status:** Approved and built — backend (`internal/inventory`, migration `000018`) and the `/dashboard/stock` frontend are live and verified against a running backend. Not built: `GET /stock-receipts` history (see §4) and the standalone catalogue-management Settings page (see CLAUDE.md's Phase 3 section).
**Depends on:** `internal/catalogue` (products/variants/prices), `internal/identity` (default location).
**Deviates from:** `docs/IMPLEMENTATION_PLAN.md`'s frozen phase order. Full inventory is Phase 7, scheduled after sync (4), walk-in selling (5), and tabs/payments (6). This pulls forward only the receiving half of Phase 7 — enough for one owner to record what they bought and see current stock — because it doesn't depend on any of 4–6 existing first, and "get the bar running with one user" needs it now. Counts, staff adjustment requests/approvals, negative-inventory reviews, and complimentary-consumption handling are real Phase 7 work this deliberately does not build yet — they need staff and sales to exist to mean anything.

## Why this shape

Two conversations converged on this: (1) onboarding shouldn't force product/price entry up front — that's real friction for a "takes a minute" signup, and (2) buying stock by the crate (e.g. "12 bottles for ₦8,750") shouldn't require the owner to do division themselves to get a per-bottle cost. Both point the same direction: catalogue definition and stock receiving become one continuous action, triggered the first time an owner actually has something to log, not a mandatory onboarding step.

`docs/ARCHITECTURE.md` §8.4 already specifies the receiving model in detail — this phase implements exactly that, not a new design:

> Store exact line total in kobo, not only a rounded unit cost.

A receipt line stores **quantity + total cost**; per-unit cost is a display-time computation (`total ÷ quantity`), never the stored value. This is why "4 crates × 12 bottles at ₦43,000 total" and "1 crate of 12 at ₦8,750" are the same shape: one receipt line each, quantity = crates × bottles-per-crate, cost = the one total figure entered.

## Schema (migration `000018`)

Mirrors `docs/ARCHITECTURE.md` §8.4's model precisely, using the same tenant-isolation pattern (`business_id` + composite FKs + `FORCE ROW LEVEL SECURITY`) every other tenant-owned table in this codebase already uses:

- **`stock_receipts`** — header: business, location, who received it, when.
- **`stock_receipt_lines`** — one row per variant in a receipt: quantity, total cost. A receipt can cover several variants delivered together, though the v1 UI only ever submits one.
- **`stock_lots`** — one immutable costed lot per receipt line. Duplicates receipt-line data today (nothing consumes a lot yet — no sales exist), kept as its own table specifically so a later FIFO allocator (Phase 10) can consume lots without a schema redesign, per the architecture doc's own reasoning: "existing receipt, sale, and movement records do not need redesign."
- **`inventory_events`** — operation headers. `type` is `CHECK`-constrained to `'receipt'` only for now; adding `'sale'`, `'adjustment'`, etc. later is a migration, not a silent widening.
- **`inventory_movements`** — append-only signed quantity deltas, linked to an event. Receipts are always positive; nothing else writes here yet.
- **`inventory_balances`** — the rebuildable projection (`business_id, variant_id, location_id` → `quantity`), maintained as an atomic increment alongside each movement, never treated as the source of truth (`inventory_movements` is).

No `stock_counts`, `adjustment_requests`, or review-case tables — those are real Phase 7 scope, not this slice.

## Backend

- `internal/inventory` (new package, mirrors `internal/catalogue`'s shape): `Service.ReceiveStock(ctx, userID, businessID, locationID, lines)` — one transaction creates the receipt, its lines, their lots, one `inventory_events` row, one `inventory_movements` row per line, and increments `inventory_balances` — atomicity here is the same non-negotiable pattern as `SetVariantPrice`/`CreateVariant` elsewhere in this codebase. `Service.GetBalances(ctx, userID, businessID)` returns a `variant_id → quantity` map for merging into a catalogue read.
- **Capability names already existed** in `internal/tenancy/capabilities.go` before this phase started — `inventory:receive` (Owner-only for now, matching the existing comment: "whether Staff also receive inventory:receive remains a pilot observation") and `inventory:read` (both Owner and Staff). No new capability needed.
- **`GET /api/v1/products`'s response gains a `current_stock` field per variant** (merged in the handler from `inventory.Service.GetBalances`, not by having `internal/catalogue` know about inventory) rather than a parallel "list stock" endpoint that would just re-fetch the same product/variant data.
- **New route**: `POST /api/v1/stock-receipts`, tenant-scoped, `inventory:receive`. Location is resolved server-side from the business's default location (`identity.Service.GetDefaultLocation`) — the client never sends one, since multi-location isn't a pilot concern.
- **Not built this pass**: `GET /stock-receipts` (receipt history/audit trail). Cheap to add later; left out to keep this slice to exactly what the UI needs first.

## Frontend

- **Onboarding shrinks to two steps**: business name, then done. The product-picker and pricing steps built last session are removed — their job is now this phase's "Add Stock" screen, triggered by an actual purchase rather than a forced signup step.
- **New `/dashboard/stock` screen**, replacing the dead `#stock` anchor already reserved in `AppBottomNav`. One flow, branching on what's selected:
  1. Search across your own catalogue and platform templates together. Picking your own product skips straight to the receiving form (selling price already known). Picking a template, or typing something not found in either, asks for a selling price once before continuing.
  2. **Crate / Bottle toggle.** Crate mode: bottles-per-crate (remembered per product) × number of crates. Bottle mode: a plain count. Either way, one **total cost** field — a live "≈ ₦X/bottle" preview underneath, never a separately-entered or stored per-unit figure.
  3. Save.
- Tap budget: 2–3 taps to reach and confirm the action (pick product, confirm unit mode, save); the quantity/cost fields themselves are unavoidable data entry, not something a tap count can compress further.

## Explicitly out of scope

- Sale-side stock deduction — no sales exist yet (Phase 5).
- Physical counts, `STALE_STOCK_COUNT` reviews.
- Staff adjustment requests and owner approval.
- `NEGATIVE_INVENTORY` reviews (nothing can go negative when only receipts exist).
- Complimentary/spoiled/staff-use consumption events.
- FIFO cost allocation against sales (Phase 10) — lots are preserved now specifically so that allocator has correct data to work from later, but nothing allocates against them yet.
- Multi-location UI — the schema supports it (every table carries `location_id`), the UI doesn't expose it.
