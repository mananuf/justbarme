# Multi-buy / bundle pricing

**Status:** Built and verified live against a running backend. **Not** part
of `docs/`'s frozen plan — designed and confirmed in conversation after a
concrete need (a snooker table's "2 games for ₦500" pricing) made the
existing plain-linear-price-only model insufficient. Branch:
`feature/multibuy-pricing`.
**Depends on:** `internal/catalogue` (variants, prices) and `internal/sales`
(`postSaleRound`, the shared helper behind `CreateSale`/`AddSaleRound`/
`RemoveBillItem`). Read both before touching this.

## The concrete need

> 1 game ₦300. 2 games ₦500 (discounted). Continuously: 3 games = 500+300 =
> 800, 4 games = 500+500 = 1,000, and so on.

This is a classic multi-buy / pack pricing scheme — "N for a flat total,"
repeating indefinitely in combination with the plain per-unit price and
any other packs a variant might have. It needed to generalize: a different
service could have its own, differently-shaped discount (a different
quantity, a different combination of pack sizes), without new code per
service.

## Confirmed design

- **A variant's own current price doubles as the implicit "pack of 1."**
  A new table, `product_variant_price_packs` (migration `000031`, same
  append-oriented, effective-dated shape as `product_prices`), only ever
  holds packs of size > 1. A variant with no current packs behaves exactly
  as it always has — this is purely additive; nothing about an ordinary
  drink's pricing changes.
- **The price for any quantity is a genuine dynamic-programming minimum,
  not a "biggest/best-rate pack first" shortcut.** `internal/sales`'
  `priceForQuantity` (`pricing.go`) builds `cost[0..n]` bottom-up, always
  considering "one more at the plain price" alongside every defined pack
  at every step. A greedy shortcut is not guaranteed optimal once pack
  shapes stop being as simple as a single 2-for-X deal (the classic
  coin-change counterexample) — `TestPriceForQuantityPicksTrueOptimumNotGreedyShortcut`
  pins a real case where greedy would overcharge.
- **No change to how a sale is recorded — only to how one unit's price is
  decided.** Every tap in the unified Sell/Bills flow already adds exactly
  one unit, live. For a pack-priced variant, `postSaleRound` computes the
  *marginal* price of that tap: the cost of moving this bill's net
  quantity of the variant from its current count to current+quantity
  (reusing `SumBillItemQuantityByVariant`, which already existed for
  `RemoveBillItem`'s own cap check). This is sign-agnostic: a removal
  (negative quantity) produces a negative delta — the correct refund —
  computed the exact same way, order-independent regardless of how adds
  and removes interleave.
- **The server is authoritative on price for a pack-priced variant — a
  deliberate, narrow exception to this codebase's usual "trust the
  client's submitted unit price, only flag a stale one for review"
  contract.** A client cannot be trusted to correctly reconstruct bundle
  math; getting it wrong here is real money, not a display nicety. Every
  other variant keeps today's exact behavior (client-submitted price,
  staleness flagged via `sale_reviews`, never corrected) — this exception
  applies only when the variant has current price packs at all. Verified
  live: a client sending a flat, wrong `unit_price_kobo` on every single
  tap still gets charged the correct 300/200/300/200 sequence regardless.
- **`RemoveBillItem`'s "already fully paid" guard was restructured to
  check the real computed refund, not a client-estimated one.** It used
  to check `quantity × unitPriceKobo > bill.BalanceKobo` *before* posting
  the compensating round; for a pack-priced variant that estimate would
  use the wrong basis entirely. It now posts the round first (via
  `postSaleRound`, same transaction, nothing committed yet) and checks
  `bill.BalanceKobo + sale.TotalKobo < 0` against the *real* amount,
  rolling back if it would go negative. Exactly reproduces the old
  behavior for a non-pack-priced item (same numbers, just computed after
  instead of before) — confirmed by the full existing test suite passing
  unchanged, including `TestRemoveBillItemRejectsOnceFullyPaid`.
- **`unitPriceKobo` stored on a pack-priced `sale_item` is
  `lineTotalKobo ÷ quantity`** — informational display only (exact, no
  rounding loss, for the overwhelmingly common quantity=1 case every live
  tap uses; an approximation only for a hypothetical larger batch).
  `lineTotalKobo` is what the bill's balance is actually built from.
- **`BillWorkspace`'s "current order" netting was fixed to show a true
  average, not an arbitrary single round's price.** It used to take
  whichever round happened to be processed last as the displayed
  "₦X each" for a netted quantity — harmless when every round shares one
  flat price, but misleading once rounds can carry genuinely different
  marginal prices. It now sums each variant's real `lineTotalKobo` across
  rounds and derives "each" as total ÷ quantity, which is identical to the
  old number for any non-pack-priced variant (every round already shared
  the same price) and the honest average for one that isn't.
- **Receipt/rounds history is left as individual, varying-per-unit-price
  rounds, not grouped into one summarized line.** A deliberate first-cut
  choice: this keeps the simplest, most honest option (an immutable trail
  of what actually happened, matching how every other correction in this
  app is already shown) rather than building display-layer grouping for a
  need not yet confirmed to exist. Revisit only if real usage shows the
  raw per-round breakdown reads as confusing on an actual bill.
- **No live marginal-price preview in the product grid before tapping.**
  The server is authoritative regardless of what the UI optimistically
  shows beforehand, so this is UX polish, not a correctness requirement —
  deferred. The correct price is visible immediately after tapping, via
  the bill's own live-updating total.

## Backend

- **Migration `000031`**: `product_variant_price_packs` — composite FK to
  `product_variants(business_id, id)`, `FORCE ROW LEVEL SECURITY`, the
  same `NULLIF(current_setting(...), '')::uuid` tenant-isolation policy as
  every other tenant table, and a partial unique index
  (`business_id, variant_id, pack_quantity) WHERE valid_to IS NULL`) for
  "at most one current pack per quantity," the same reasoning
  `product_prices_one_current_per_variant` already established.
- **`catalogue.Service.SetVariantPricePacks`** replaces a variant's
  *complete* set of packs in one transaction — full-replacement, matching
  `UpdateVariant`'s own PATCH convention ("the caller resends the whole
  mutable set, not a partial merge"). An empty slice removes multi-buy
  pricing entirely. Validates quantity > 1, price > 0, and no duplicate
  quantity within one submission (`ErrInvalidPackQuantity`/
  `ErrInvalidPackPrice`/`ErrDuplicatePackQuantity`, mapped to `400`).
  `ListVariantPricePacks` reads the current set back;
  `VariantWithPrice.PricePacks` is populated by `ListCatalogue` for every
  variant (empty for a plain one), surfaced in `GET /products` as
  `price_packs` (omitted when empty).
- **`internal/sales` reads `product_variant_price_packs` directly through
  the shared sqlc layer**, not through `catalogue.Service` — the same
  "each feature package stays self-contained, duplicate a small pure
  check rather than call into another package's Service" convention
  Phase 7's negative-inventory detection already established. The actual
  pricing math (`priceForQuantity`) lives in `internal/sales/pricing.go`
  since that is the one place it is actually needed for charging.
- **Endpoints**: `GET`/`POST /api/v1/variants/{variant_id}/price-packs`,
  reusing the existing `catalogue:read`/`catalogue:manage` capabilities —
  no new capability needed, same owner-manages/both-roles-read split every
  other catalogue route already has.
- **New tests**: 4 pure unit tests for `priceForQuantity` (the snooker
  example exactly, a no-packs-is-plain-linear regression guard, a
  negative-quantity guard, and the greedy-vs-optimal counterexample), 3
  `internal/catalogue` integration tests (full-replacement semantics,
  validation errors, `ListCatalogue` surfacing current packs), and 3
  `internal/sales` integration tests against real Postgres (the exact
  four-tap marginal sequence and resulting balance, a removal's correct
  marginal refund, and the restructured fully-paid check using the real
  pack-aware amount).

## Frontend

- **`web/src/api/catalogue.ts`**: `PricePack` type, `pricePacks` on
  `CatalogueVariant` (always present, empty for a plain variant),
  `listVariantPricePacks`/`setVariantPricePacks` clients.
- **`Stock.tsx` gained a fourth mode, "Pricing"** (owner-only, matching
  `catalogue:manage`), alongside Restock/Count/Report issue — reuses the
  existing `VariantPicker` search component, but over a new
  `allPricableVariants` list that (unlike `allVariants`) does **not**
  exclude non-stocked service items, since a service like a snooker game
  is exactly the case this exists for. Picking a variant shows its
  current packs as editable quantity/price rows (add/remove freely, save
  replaces the whole set) — the smallest reasonable scoped UI for this,
  not a general catalogue-editing screen (which still doesn't exist, per
  the Phase 3 section's own named gap).

## Verified live end-to-end

Against a running backend: created a non-stocked "Snooker — Game" variant
at a plain ₦300, set a "2 for ₦500" pack via the new endpoint, confirmed
`GET /products` surfaces it. Posted four rounds on a fresh bill, each one
deliberately sending the *same wrong* flat `unit_price_kobo` (30000) the
client would have shown before any discount — the server charged the
correct 300 / 200 / 300 / 200 marginal sequence regardless, landing on the
correct ₦1,000 cumulative balance. Removed one unit and confirmed the
refund was the true marginal (₦200, not a flat ₦300), leaving the bill at
the correct three-game price (₦800). Test data cleaned up via the
RLS-aware `SET LOCAL ROLE jbm_app` transaction pattern. Backend: full
`go test ./cmd/... ./internal/...` passes (including every pre-existing
test, unchanged). Frontend: full `npm run check` (format, lint, all 45
tests, production build) passes.

**Not done**: an actual browser/Playwright pass over the new `Stock.tsx`
"Pricing" mode and the live product grid while a multi-buy variant is
being sold — Claude in Chrome's extension was not connected in this
environment, same named gap this codebase already carries elsewhere.
