# Phase 5 — Walk-in selling (single-device slice)

**Status:** Approved and built — backend (`internal/sales`, migration `000019`) and the real `/dashboard/sell` frontend are live and verified against a running backend. The frontend's own IndexedDB offline-queueing path has not been driven through a real browser as part of this pass (only the backend it talks to was live-verified) — worth a manual or Playwright pass before trusting it in production.
**Depends on:** `internal/catalogue` (products/variants/prices), `internal/inventory` (stock deduction), `internal/identity` (default location).
**Deviates from:** `docs/IMPLEMENTATION_PLAN.md`'s frozen phase order, the same way `docs/PHASE_STOCK_RECEIVING.md` does — Phase 5 is specified as depending on Phase 4 ("sync foundation": multi-device event envelope, push/pull protocol, conflict resolution). Phase 4 is deferred until invitations bring a second device into a real business (see this session's project memory note). With one owner on one device, there is nothing to reconcile across devices, so this slice builds single-device offline persistence (a real requirement, per `docs/ARCHITECTURE.md`'s offline-first principle) without the multi-device machinery.

## What "single-device slice" means concretely

- **In scope:** recording a walk-in sale (multi-item, one immediate payment), atomic stock deduction, a client-generated idempotency key so a retried POST is safe, a narrow review mechanism for stale/deactivated-variant sales, and sale reversal.
- **Out of scope, deliberately:** tabs/credit/partial payments (`docs/ARCHITECTURE.md` §8.3's `tables`/`customers`/open-bill concepts — Phase 6), the generic multi-purpose `review_cases` system Phase 4 describes (stock counts and adjustments don't exist yet to need it), background-sync-while-closed or any multi-device conflict resolution (Phase 4).

## Schema (migration `000019`)

- **`bills`** — only `open`/`settled` statuses exist; a walk-in sale is always paid in full immediately.
- **`sales`** — immutable posted round. `idempotency_key` (client-generated UUID) is unique per business. `total_kobo` is `SUM(sale_items.line_total_kobo)`, sign included — positive for an ordinary sale, negative for a reversal (see below) — never a separately-asserted magnitude.
- **`sale_items`** — variant, a name snapshot (`description`), quantity (negative only for a reversal's compensating lines), the actual unit price charged (preserved exactly, never overwritten by a server-recomputed price), and a recomputed line total.
- **`payments`** — one row per sale (or per reversal, as a refund). Cash/transfer/card only; no partial payments.
- **`sale_reviews`** — scoped specifically to `deactivated_variant`/`price_mismatch`, not the generic Phase 4 review-case system. Never changes the sale it's attached to.
- **`inventory_events.type`** widens from `('receipt')` to `('receipt', 'sale', 'sale_reversal')`.

## The two production rules this schema encodes

1. **Store the total's sign, never compute it downstream.** An ordinary sale's line items are all positive quantity; a reversal's are all negative. `SumSalesTotalSince` is a plain `SUM(total_kobo)` — no `CASE WHEN` for reversals anywhere, because the sign already carries the meaning.
2. **A sale is never rejected for staleness.** A deactivated variant or a price that doesn't match anything ever actually in effect at the claimed sale time still posts, exactly as submitted — a `sale_reviews` row opens instead, for the owner to look at later. Only a genuinely nonexistent `variant_id` is rejected (`ErrVariantNotFound`), since that's a malformed request, not staleness.

## Backend

- **`internal/sales.Service.CreateSale`** resolves everything (variant lookups, staleness checks, the running total) *before* creating the `sales` row, then creates every child row (items, inventory event, movements, reviews, payment) after — every child has a foreign key back to `sales`, so that row must exist first, which means its total must be known first. Getting this ordering backwards was an actual mistake made and caught during this feature's own test-writing, not a hypothetical — read the method before changing the write order.
- **Idempotency check happens first, inside the transaction**, before any other write — a retried POST (lost response, offline retry queued and later flushed) returns the sale already posted the first time, doing zero additional writes, rather than creating a second sale or erroring.
- **`Service.ReverseSale`** creates new, equal-and-opposite rows — never edits or deletes the original: a second `sales` row (`reversal_of_sale_id` back-reference), negative-quantity `sale_items`, positive inventory movements restocking the exact original location, and a negative-amount refund payment. At most one reversal per sale (`ErrAlreadyReversed`).
- **`POST /api/v1/sales`** — `sales:record` (Owner and Staff). **`POST /api/v1/sales/{id}/reverse`** and resolving a review — `sales:reverse` (Owner-only). Both capability names already existed in `internal/tenancy/capabilities.go` before this phase. `GET /sales`, `GET /sale-reviews`, and `GET /sales/summary` use `activity:read`/`reports:read` (Owner-only) — whether Staff should see their own sales history is an open pilot question, same shape as `inventory:receive`'s already-flagged Staff-scoping ambiguity.
- **`GET /sales/summary`** computes "today" in the business's own timezone, not the server's or the browser's.

## Frontend

- **`Sell.tsx`** reads real, active products (`GET /products`) instead of a hardcoded list, and writes every completed sale to a new Dexie table (`pendingSales`, `src/lib/db.ts` version 2) *before* any network call — that write is what "Sale recorded" now actually means. `src/lib/salesSync.ts`'s `flushPendingSales` posts pending sales to the server oldest-first, stopping at the first failure, run on mount / regaining connectivity / right after queuing a new one.
- **Dashboard's "Today's Sales" and "Recent Activity"** are wired to `GET /sales/summary` / `GET /sales`. The other three mock stats that used to sit beside "Today's Sales" (items sold, expenses, tabs) were removed rather than left half-real, since none of those modules exist. "Stock alerts" and "Outstanding" remain mock (no low-stock threshold or tabs concept yet).

## Explicitly out of scope

- Tabs, named-customer credit, partial payments (Phase 6).
- The generic `review_cases` system (Phase 4) — this phase's `sale_reviews` is narrow and sale-specific by design.
- Multi-device sync, background-sync-while-closed, conflict resolution (Phase 4, deferred until invitations).
- FIFO cost allocation against sales (Phase 10) — `internal/inventory`'s lots are preserved for this already; nothing allocates against them yet.
- A UI for browsing/resolving `sale_reviews` — the backend supports it (`GET /sale-reviews`, resolve), no screen consumes it yet.
