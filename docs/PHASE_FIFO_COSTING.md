# Phase 10 — FIFO costing and gross margin (draft)

**Status: drafted, not built.** This is `docs/IMPLEMENTATION_PLAN.md`'s actual Phase 10 ("Post-pilot FIFO costing") and `docs/ARCHITECTURE.md` §8.5's already-frozen costing decision — this doc doesn't invent the approach, it makes it concrete enough to implement. See §8 for the one open sequencing question before any code gets written.

## 1. Why this exists, and what's already true

`internal/inventory.ReceiveStock` has stored every receipt's exact cost since Phase 7 (`stock_lots`: quantity + total cost, immutable, one row per receipt line) specifically so this phase wouldn't need a schema redesign — see migration `000018`'s own comment. But nothing has ever read a lot back. Concretely, today:

- Receiving 10 bottles for ₦10,000 then 5 more for ₦5,700 creates two honest, separately-costed lots.
- `inventory_balances` immediately collapses that to a single number — 15 — with no cost attached.
- Selling Coke records the *selling* price on `sale_items`; nothing looks at `stock_lots` at all.
- There is no cost-of-goods-sold, no gross margin, no inventory *value* anywhere in the system — not approximated, genuinely absent.

`docs/ARCHITECTURE.md` §8.5 already decided the target design (FIFO, not moving-weighted-average) and named the eventual table (`sale_item_lot_allocations`, immutable). This doc is that decision made concrete: exact schema, exact allocation algorithm, and the two hard edge cases (oversell, kobo rounding) the frozen doc calls out but doesn't resolve.

## 2. Schema (new migration)

```sql
ALTER TABLE stock_lots ADD COLUMN remaining_quantity INT NOT NULL DEFAULT 0;
-- Backfilled to received_quantity for existing rows; ReceiveStock sets it
-- equal to received_quantity going forward. Rebuildable in principle from
-- sale_item_lot_allocations (same "projection, not source of truth"
-- status as inventory_balances), but stored directly since it's read on
-- every sale, not just for display.

ALTER TABLE stock_lots ADD COLUMN source TEXT NOT NULL DEFAULT 'receipt'
    CHECK (source IN ('receipt', 'review_resolution'));
-- See §5's resolution design -- a review-resolved cost creates a
-- synthetic lot distinct from a real receipt, so a report can always
-- explain where a cost figure actually came from.

CREATE TABLE sale_item_lot_allocations (
    id             UUID PRIMARY KEY,
    business_id    UUID NOT NULL REFERENCES businesses (id),
    sale_item_id   UUID NOT NULL,
    -- NULL means "sold, cost not yet known" -- the oversell case, never a
    -- fabricated cost. See §4.
    stock_lot_id   UUID,
    -- Signed: positive for an ordinary sale's consumption, negative for a
    -- reversal's give-back (mirrors the exact lots the original sale
    -- drew from -- see §6) or a resolution's pending-quantity close-out
    -- (see §5). Never updated once written -- "immutable sale-item lot
    -- allocations" per docs/IMPLEMENTATION_PLAN.md Phase 10 task 2.
    quantity       INT NOT NULL,
    -- The TOTAL cost of this allocation's quantity -- never a per-unit
    -- figure. Same "store the total, never a rounded per-unit cost"
    -- principle docs/PHASE_STOCK_RECEIVING.md already established for
    -- stock_receipt_lines, for the same reason: multiplying a rounded
    -- per-unit price back out drifts. See §7.
    allocated_cost_kobo BIGINT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (business_id, sale_item_id) REFERENCES sale_items (business_id, id),
    FOREIGN KEY (business_id, stock_lot_id) REFERENCES stock_lots (business_id, id),
    CHECK ((stock_lot_id IS NULL) = (allocated_cost_kobo IS NULL))
);
```

Same tenant-isolation pattern as every other table in this codebase (`FORCE ROW LEVEL SECURITY`, `NULLIF(current_setting(...), '')::uuid` policies) — omitted above for brevity, not skipped in the real migration.

## 3. The allocator: when and how it runs

Lives in `internal/sales`, called from inside `postSaleRound` (so `CreateSale`/`AddSaleRound`/`RemoveBillItem` all get it for free, same as the existing inventory-movement logic) — **not** a new `internal/inventory` dependency, matching this codebase's "each feature package writes to another domain's tables directly via the shared sqlc layer rather than importing that package" convention (already the case for `inventory_movements`/`inventory_balances`/`inventory_reviews` in this exact function).

Skipped entirely for a `tracks_inventory = false` variant (a service item never has lots to allocate from — see the tracks-inventory work already shipped).

For each tracked sale item needing `quantity` units allocated:

1. `SELECT ... FOR UPDATE` every `stock_lots` row for this variant+location with `remaining_quantity > 0`, ordered `received_at ASC, id ASC` (UUIDv7 IDs are themselves time-ordered, so this is a stable tiebreaker for two lots received in the same instant). The row lock is what makes "server posting/allocation sequence" (§8.5's ordering rule) real: two concurrent sales racing for the same lot serialize on this lock exactly like `TestConcurrentPaymentsNeverExceedBalance` already does for bill balances — whichever transaction commits first genuinely does get first claim on the oldest stock, and the second sale sees the already-decremented `remaining_quantity`. No offline device clock is ever consulted for this ordering, per §8.5's explicit instruction not to reorder posted COGS by an unreliable client timestamp.
2. Walk the locked lots oldest-first. For each, consume `min(remaining_quantity, still_needed)`:
   - Insert one `sale_item_lot_allocations` row (`stock_lot_id`, `quantity = consumed`, `allocated_cost_kobo` — the total cost of just this allocation, see §7 for the exact rounding-safe formula).
   - Decrement that lot's `remaining_quantity` by `consumed`.
   - Reduce `still_needed` by `consumed`.
3. If `still_needed > 0` after every lot is exhausted (an oversell — the same condition that already opens a `negative_inventory` review today), insert exactly one more allocation row: `stock_lot_id = NULL`, `quantity = still_needed`, `allocated_cost_kobo = NULL`. This is the literal "mark unmatched sale quantity as pending cost allocation" instruction from §8.5 — never a fabricated cost, never zero, just an honest gap sitting next to the review that already exists for the same event.

The sale itself is **never** affected by any of this — it posts exactly as submitted regardless of lot coverage, same as today. This is pure bookkeeping alongside the existing inventory movement, not a new rejection path.

## 4. Oversell = pending cost, on purpose

`docs/ARCHITECTURE.md` §8.5, verbatim: *"Do not invent zero-cost stock to make profit reports look complete."* A pending allocation (`stock_lot_id IS NULL`) is a real, permanent ledger entry until someone deliberately resolves it — it does not silently disappear, does not default to ₦0, and does not get backfilled automatically the next time stock arrives (backfilling from a *later* receipt would mean quietly rewriting an already-posted sale's COGS after the fact, which is exactly what §8.5 rules out).

## 5. Resolving a pending allocation

Resolution is a **deliberate, human, reviewed action** (Phase 10 task 4's own wording: "through reviewed events") — never automatic. It piggybacks on the `negative_inventory` review that already exists for this exact situation, rather than inventing a second review mechanism:

- `POST /inventory-reviews/{id}/resolve` gains an optional `resolved_unit_cost_kobo` field (a *per-unit* price, since that's what an owner actually knows and types in — converted to a total the moment it's stored, never carried around as a per-unit figure), meaningful only when the review's type is `negative_inventory`.
- When supplied, the resolution:
  1. Creates a **synthetic `stock_lots` row** (`source = 'review_resolution'`, `received_quantity` = the resolved quantity, `total_cost_kobo` = quantity × the given per-unit cost, `remaining_quantity = 0` since it's immediately fully consumed).
  2. Inserts a **negative** pending-closing allocation (`stock_lot_id = NULL`, `quantity = -<resolved quantity>`) to net the old pending amount to zero.
  3. Inserts a **positive** allocation pointing at the new synthetic lot for the same quantity.
- Net effect: total allocated quantity for the original sale item is unchanged, the ledger is append-only throughout (nothing already written is ever edited), and the audit trail honestly shows *"12 units sold, cost unknown at the time, resolved on 3 Oct by the owner at ₦1,140/unit."*
- If the owner resolves the review without supplying a cost (today's existing flow, unchanged), the allocation just stays pending. Reports keep excluding/flagging it, indefinitely if need be — that's an accepted, named end state, not a bug to chase.

## 6. Reversal symmetry

`ReverseSale` must give quantity back to the **exact** lots the original sale drew from — re-running the allocator on the reversal would let it draw from whatever's oldest *now*, which could easily be the wrong lot and would silently misstate which stock is actually left. Concretely: for each original sale item being reversed, look up its `sale_item_lot_allocations` rows (including a pending one, if any) and, for each, insert a mirrored allocation on the new reversal sale item with the **same** `stock_lot_id` and **negated** `quantity` — incrementing `remaining_quantity` back up on any real lot involved. No new allocation decision is ever made on a reversal.

## 7. Kobo-exact rounding across partial lot depletion

The same "never store a rounded per-unit cost" principle `docs/PHASE_STOCK_RECEIVING.md` already established for receiving applies here, in the harder direction: a lot's `total_cost_kobo` doesn't always divide evenly by its `received_quantity` (₦10,000 ÷ 3 is not a whole kobo amount), and naively rounding a per-unit figure then multiplying it back out across several partial consumptions of the same lot will drift the sum away from the lot's real total.

Fix: track *cumulative kobo already allocated from this lot* as a running value, and compute each new allocation's cost as the difference between two cumulative-floor points:

```
allocated_so_far_kobo = floor(lot.total_cost_kobo * units_consumed_so_far / lot.received_quantity)
allocated_cost_kobo   = floor(lot.total_cost_kobo * (units_consumed_so_far + consumed) / lot.received_quantity) - allocated_so_far_kobo
```

This guarantees the sum of every allocation ever drawn from one lot exactly equals that lot's `total_cost_kobo`, with any rounding remainder landing on whichever allocation happens to cross a rounding boundary — never a growing drift, and never a lot's last unit silently absorbing an unexplained kobo or two more than the arithmetic actually requires.

## 8. Reporting: gross margin, honestly labeled

A new `internal/reports.GrossMargin` (or an added field on the existing sales report — TBD at implementation time) computing, per product/variant/period:

- **Revenue**: unchanged, from `sale_items.line_total_kobo` (already correct today).
- **Resolved COGS**: `SUM(allocated_cost_kobo)` over allocations where `stock_lot_id IS NOT NULL` (already a total per row, never multiplied by quantity again).
- **Unresolved units**: `SUM(quantity)` where `stock_lot_id IS NULL` and still net-positive (a pending allocation not yet closed out by a resolution).
- **Gross margin**: revenue − resolved COGS, always shown *alongside* the unresolved-unit count, never presented as complete when that count is nonzero — the direct implementation of §8.5's "omit or clearly label COGS/profit as unavailable while unresolved quantities exist."

## 9. Explicitly out of scope

- **Recipe/cocktail costing** (a drink built from several ingredient lots) — `docs/ARCHITECTURE.md`'s own non-goals list excludes this; every allocation here is one variant, one lot, one quantity.
- **Automatic historical COGS replay** — if the allocator's logic changes later, existing allocations are not recomputed; a schema/logic change only affects sales posted after it.
- **Retroactive correction of a wrong receipt cost** — if a receipt's total was mistyped and already partially or fully allocated, fixing it is a future append-only mechanism of its own, not designed here.
- **Multi-location lot sharing** — a sale only ever allocates from lots at its own location; this is already how `stock_lots`/`inventory_balances` are scoped today, just called out explicitly since FIFO makes the constraint load-bearing instead of incidental.
- **`RemoveBillItem`'s compensating negative round** (an in-progress tab correction, before the bill ever settles — see CLAUDE.md's Tabs and credit section) is excluded from lot allocation. It has no single original sale item to give quantity back to, and approximating one (e.g. reversing whichever lots were most recently touched) would be a guess dressed up as a real allocation. `inventory_balances`/`inventory_movements` still correctly reflect the quantity given back either way (unaffected by this); only the cost ledger has no entry for it, which can very slightly overstate COGS in the rare case a removed item's cost isn't given back. Accepted, named gap — a real reversal (`ReverseSale`, after a bill has settled) is fully handled by §6.

## 10. Confirmed vs. open

**Already confirmed** (frozen in `docs/ARCHITECTURE.md` §8.5, not re-litigated here): FIFO over moving-weighted-average; allocation ordered by server transaction sequence, then receipt time, then lot ID; never reorder by an offline device clock; never fabricate zero-cost stock.

**Open, needs your call before implementation starts:** `docs/IMPLEMENTATION_PLAN.md` names this Phase 10 **"Post-pilot"** — its own first task is literally "validate FIFO with pilot observations/accounting needs" before building the rest. That's a real, deliberate sequencing choice in the frozen plan, not an oversight. Building it now would be the same kind of conscious reordering this project has already done a few times this session (stock receiving/selling/tabs pulled forward, Phase 9 built ahead of a literal Phase-numbered order) — which is fine *if* it's a deliberate choice, not a default. Do you want this built now, ahead of the pilot, or kept as a ready design to implement once the pilot has actually run and you know whether FIFO-as-designed matches how The Place really operates?
