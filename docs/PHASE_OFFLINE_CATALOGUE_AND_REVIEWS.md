# Phase 4, revisited — offline catalogue caching and a sale-reviews UI

**Status:** Implemented and live-verified. Not literal Phase 4 — see §1 for why, and `docs/IMPLEMENTATION_PLAN.md`'s own Phase 4 section for what was actually specified there.
**Depends on:** `internal/sales` (already has `sale_reviews`, `ListOpenReviews`, `ResolveReview` since Phase 5), `web/src/lib/db.ts` (Dexie).
**Trigger:** the deferral recorded in project memory (`phase4-sync-foundation-deferred`) said to revisit Phase 4 once invitations bring a second device/staff member into a real business. Invitations shipped this session — this is that revisit.

---

## 1. Why this isn't literal Phase 4

`docs/IMPLEMENTATION_PLAN.md`'s Phase 4 is a generic event-sourcing protocol: client event envelopes with `device_seq`, a server idempotency inbox, an ordered change feed, `POST /sync/push`/`GET /sync/pull`, a generic "review cases" table. It was designed to be built *first*, before Phase 5 needed anything simpler to fall back on.

That's not the order this project actually took. Phases 5 and 6 shipped sales/bills/tabs as direct, synchronous REST endpoints, and — checked against the real code before writing this doc, not assumed — they already handle every concrete multi-device conflict scenario Phase 4's machinery exists for, with a much simpler mechanism that needs no changes to work correctly across more than one device:

- Idempotency keys are per-device, client-generated random UUIDs, unique per business — two devices can never collide on one, no coordination needed.
- A stale price, a deactivated variant, or an oversell are never rejected. The sale posts exactly as submitted and opens a `sale_reviews` row (`internal/sales/service.go`'s `postSaleRound`). There is no non-negative `CHECK` constraint on `inventory_balances` — stock going negative is accepted, not blocked. This is `docs/ARCHITECTURE.md` §10.6's conflict policy table, already implemented, already tested, already live-verified during Phase 5.
- `CLAUDE.md`'s own Walk-in selling section already documents that `sale_reviews` was a deliberate narrowing away from Phase 4's generic "review cases" concept.

Building the generic protocol now would mean either a risky rewrite of already-shipped, live-verified endpoints, or a second, parallel system nothing uses — exactly the "speculative complexity with no real user to validate it against" the original deferral note warned against, just now aimed at problems that turn out to already be solved.

## 2. What actually needs attention

Two real, narrow gaps, both about a *second device actually existing* now rather than about multi-device *conflict resolution*:

1. **No offline catalogue cache.** `Sell.tsx` fetches products fresh on every mount with nothing persisted locally. A device that's offline the first time it opens Quick Sell, or reloads while offline, sees a load error and can't sell at all — not a multi-device problem, a single-device offline-first gap that was always supposed to be covered (`phase4-sync-foundation-deferred`'s own note: "single-device offline support... is still required per `docs/ARCHITECTURE.md`'s offline-first principle... that is not the same as Phase 4's multi-device sync foundation, and should not be skipped"), just not built yet because nothing had forced the question until a second device became real.
2. **No reviews UI exists anywhere.** `sale_reviews` has been accumulating silently since Phase 5 — the backend (`GET /sale-reviews`, `POST /sale-reviews/{id}/resolve`) works, owner-only, but nothing in `web/` calls it. With more staff on more devices, price-mismatch and deactivated-variant reviews will start actually happening instead of being a theoretical policy.

Explicitly **not** in scope for this pass:

- `Tabs.tsx` stays online-only, as already documented in `docs/PHASE_TABS_CREDIT.md` — raised as a question, not reopened without a concrete reason to.
- `Stock.tsx`'s catalogue view is not cached offline — receiving stock is lower-frequency and needs a fresh view to add genuinely new products sensibly; not urgent the way Quick Sell is.
- The generic event-envelope sync protocol itself. If a real conflict shows up that accept-and-review can't handle, that's the trigger to reopen it — not before.

## 3. Offline catalogue cache (`Sell.tsx`)

- New Dexie table `catalogueCache` (`web/src/lib/db.ts`, version 3): `{ businessId, products: CatalogueProduct[], cachedAt }`, keyed by `businessId` (a device could hold cache for more than one business it has membership in, same as `authMeta`'s multi-membership handling).
- `Sell.tsx`'s product-load effect: on a successful `listProducts`, write the result to `catalogueCache` before setting state (cache always refreshed when reachable — never a reason to prefer stale data over fresh). On failure, fall back to whatever's cached for that business instead of showing a load error, and show a small "showing saved products — may be out of date" note so the cashier knows this isn't guaranteed current (matching `docs/ARCHITECTURE.md` §10.7's "one of a fixed set of honest sync states" philosophy, not a silent guess).
- Only `Sell.tsx` for this pass, not `Stock.tsx`/`Tabs.tsx` — see §2.

## 4. Sale-reviews UI

The backend already has everything except query context — `ListSaleReviews` is a bare `SELECT * FROM sale_reviews`, giving only `{id, sale_item_id, reason, status}`. That's not enough for an owner to actually act on: which sale, which product, what was charged, who rang it up, when.

- New sqlc query `ListSaleReviewsDetailed` (`internal/store/queries/sales.sql`) joins `sale_reviews` → `sales` (for `occurred_at`, `seller_id`) → `sale_items` (for `description`, `quantity`, `unit_price_kobo`, `line_total_kobo`) via a `LEFT JOIN` on `sale_item_id` (nullable in schema, always populated in practice today, but the query stays correct either way).
- `sales.Review` (`internal/sales/model.go`) gains the joined fields. `Service.ListOpenReviews` uses the new query; `Service.ResolveReview`'s `RETURNING *` path is unchanged (still the bare row — the resolve response doesn't need the full context a second time).
- `internal/httpapi`'s `reviewResponse` gains the new fields plus `seller_name`, resolved via the same `resolveSellerNames` helper `getBillDetail`/`listSales` already use — one seller-name-resolution mechanism for the whole codebase, not a third copy.
- Frontend: new `web/src/pages/Reviews.tsx` (`/dashboard/reviews`, linked from Dashboard's "More" menu, shown only when the caller's own role is `owner` — matching the backend's actual owner-only gate on both endpoints, `activity:read`/`sales:reverse` reused rather than the dedicated-but-unwired `reviews:read`/`reviews:resolve` constants, an existing choice from Phase 5, not something this pass changes). Each open review shows the item, the flagged reason in plain language, who rang it up and when, and a required note field before resolving — mirroring the write-off pattern already established in `Tabs.tsx`.

## 5. Confirmed decisions

1. Not literal Phase 4 — see §1.
2. Scope: offline catalogue cache for Quick Sell, plus a reviews UI. Nothing else.
3. Reviews stay owner-only, matching the existing (already-shipped) backend capability gates.
4. `Tabs.tsx` remains online-only; not reopened this pass.

## 6. Implementation notes

- **Backend**: `ListSaleReviewsDetailed` (`internal/store/queries/sales.sql`) replaces the old bare `ListSaleReviews` (removed, nothing else called it) — a `JOIN` on `sales` plus a `LEFT JOIN` on `sale_items` (nullable in schema even though every review today always has one). `sales.Review` gained `SaleID`/`SaleOccurredAt`/`SellerID`/`Item*` fields, populated only by `ListOpenReviews` — `ResolveReview`'s `RETURNING *` path leaves them zero-valued on purpose, matching `saleResponse.SellerName`'s existing "only where actually needed" pattern from the Tabs and credit phase. `resolveSellerNames` (`internal/httpapi/tabs_handlers.go`) was generalized from taking `[]sales.Sale` to taking a plain `[]uuid.UUID`, so it could be reused for reviews too without a third near-duplicate implementation — one seller-name-resolution mechanism for bill rounds, the sales list, and reviews.
- **A real bug was caught live, not in a unit test**: `toSaleResponse`'s embedded `Reviews` array (the reviews returned inline on `POST /sales`/`POST /bills/{id}/rounds` when a sale posts with a stale price) was still building `reviewResponse` without `SaleID` — a leftover from before `SaleID` existed on the response type at all. Live-verified: `POST /sales` with a deliberately mismatched price came back with `"sale_id":""` in the embedded review. Fixed by adding it to that third construction site (there were three: `listSaleReviews`, `resolveSaleReview`, and this embedded one — easy to fix two and miss the third, which is exactly what happened first).
- **Frontend**: `web/src/lib/db.ts` gained `catalogueCache` (Dexie version 3, keyed by `businessId`). `Sell.tsx`'s product-load effect now writes the cache on every successful fetch and reads it back only when the fetch fails, showing a `usingCachedCatalogue` banner ("Showing saved products — may be out of date") rather than silently presenting stale data as current. `web/src/api/reviews.ts` (typed client) and `web/src/pages/Reviews.tsx` (`/dashboard/reviews`, linked from Dashboard's "More" menu only when the caller's own membership role is `owner`) round out the reviews UI — each open review shows the item, the reason in plain language, who rang it up and when, and requires a note before resolving, mirroring `Tabs.tsx`'s write-off pattern.
- **Verified live** against an isolated backend instance (a separate port, never the developer's own running `:4000`): recorded a sale with a price that didn't match the variant's current price, confirmed `GET /sale-reviews` returned full context (item description/quantity/price, `occurred_at`, `seller_name`) rather than a bare reason string, resolved it with a note, and confirmed it dropped out of the open list. Also confirmed `GET /sales` still returns `seller_name` correctly (a regression check on `resolveSellerNames`'s signature change). Test data cleaned up afterward.
- **Verified with real Dexie/IndexedDB, not a mock** — `web/src/pages/Sell.test.tsx` (using the existing `fake-indexeddb` polyfill already wired into `src/test/setup.ts` for Dexie) exercises the actual cache: a successful load writes to `db.catalogueCache` and shows no cache-banner; a failed fetch with a warm cache falls back to the cached products and shows the banner; a failed fetch with nothing cached shows a real, honest load error (via `OfflineError`/`describeActionError`) instead of a blank screen. `src/test/setup.ts`'s per-test IndexedDB reset now also clears `catalogueCache`, matching the existing pattern for `authMeta`/`device`/`pendingSales`.
