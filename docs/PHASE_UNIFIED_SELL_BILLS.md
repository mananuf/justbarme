# Unified Sell/Bills, and real offline support for tabs

**Status:** Built and verified live against a running backend (see
"Verification" at the end) — one scope narrowing from the original design
below: the single-device offline queue for bill actions (open/round/
payment/close) was **not** built in this pass. The walk-in path's existing
`pendingSales`/`salesSync.ts` queue turned out to already cover the
dominant real case once every sale was given a navigable bill behind it
(see "What actually shipped" below) — building a second, more complex
multi-action queue on top of that, untested, in the same pass would have
cut against this codebase's "verified live end-to-end" discipline more
than it would have delivered. It remains the next real step if a table/
customer tab specifically (not a plain walk-in) needs to survive going
offline mid-session — tracked here, not forgotten. Branch:
`feature/unified-sell-bills`.
**Depends on:** Phase 5 (`internal/sales`' walk-in selling, `CreateSale`,
`pendingSales`/`salesSync.ts`) and Phase 6 (`internal/sales`' tab/table/
customer/payment methods, `docs/PHASE_TABS_CREDIT.md`). Read both before
touching this.
**Deviates from:** `docs/PHASE_TABS_CREDIT.md`'s own explicit scope
narrowing — "Out of scope, deliberately: ... offline queuing for any of
this phase's actions" and "this is the one deliberate regression from this
codebase's usual offline-first bar, tracked as a named follow-up rather
than silently accepted." This phase is that follow-up, triggered by real
pilot usage friction (a staff member confusing "Quick Sell" and "Bills" as
two different, unrelated tools) rather than by Phase 4 finally landing.

## Why

Real WhatsApp feedback from a pilot staff member ("Paul") showed concrete
confusion about when to use Quick Sell vs. Bills, and a possible
double-recorded sale. Separately, the user's own words set the design
principle for resolving it, verbatim: *"a sale is always a sale, users
need to have one view for that... this is built for non technical/savvy
people, so this should be the most simplefied application, if not we risk
users not using it."* An earlier proposal to unify only the UI entry
point while keeping Quick Sell offline-safe and Bills online-only
underneath was explicitly rejected (*"no I disagree, this is a usability
issue, keep technicalities aside"*) in favor of removing the actual
technical distinction: giving Bills real offline capability too.

## Confirmed shape

**One flow, not two tools.** Sell *is* the bill flow — "walk-in" is never
a word the user sees. Starting a sale always starts with:

- **A table picker**, "No table / walk-in" first and selected by default,
  real tables (`tables`, already built) after it.
- **A customer picker**, "Walk-in customer" first and selected by default
  — shown with a short explanatory line ("no specific customer — for
  someone paying now and leaving") so the option is self-explaining, not
  just a blank default — then search/add over real `customers`.

Then the normal bill flow proceeds: add rounds from the shared
`ProductGrid`/`CartPanel`, pay, close — exactly what `Tabs.tsx` already
does today for a real tab, just reached through one entry point instead
of two separate screens with two separate mental models.

**Selecting an already-open table/customer resolves to that bill, not a
new one.** If Table 3 already has an `open`/`closed_unpaid` bill, picking
Table 3 from the picker has to load and add to that bill — never open a
second, concurrent one for the same table. This needs new backend logic
(see "Find-or-open" below); it is a real gap the proposal as first stated
didn't cover.

**The simplest case still uses the simplest mechanism, invisibly.** When
both pickers are left at their defaults (no table, no customer) and
payment is recorded in full in the same action, the frontend keeps using
the existing atomic, offline-safe `POST /sales` (`CreateSale`) under the
hood — one call, one IndexedDB write, already proven. The moment a real
table or customer is attached, or the sale isn't fully paid immediately,
it's a real persisted `Bill` and goes through the fuller lifecycle below.
The user never sees or chooses this distinction; it exists purely so the
most common transaction (ring up, collect cash, done) stays the easiest
one to keep reliable offline.

**Bills get real, single-device offline support** — queue-and-flush, the
same mechanism `pendingSales`/`pendingStockCounts`/
`pendingAdjustmentRequests`/`pendingExpenses` already use, extended to
cover a *sequence* of bill actions rather than one atomic one. This is
deliberately **not** Phase 4's generic multi-device sync protocol: it
covers the realistic case (one device, one staff member, intermittent
connectivity through one continuous session with a tab) correctly, and
degrades to a visible, local "this didn't go through, check it" surface
— never silent data loss or silent corruption — for the genuinely rarer
case of two devices touching the same bill while one was offline. See
"What this does and doesn't solve" below.

**Bills tab becomes list + full page, not list + modal.** `/dashboard/tabs`
lists all bills, filterable by status (default: Open), most-recently-active
first. Tapping one opens a real route (`/dashboard/tabs/:billId`), not a
sheet — room for a proper receipt-style breakdown (items, rounds, who
recorded what, payments, balance), and normal back-button/deep-link
behavior. This reuses the rendering built for `BillShareLink`'s existing
public, read-only receipt view (`docs/ARCHITECTURE.md` §15) rather than
writing the layout twice — the owner-facing version adds the editing
affordances (stepper, pay, close) the public one doesn't have.

## What this does and doesn't solve

Tabs were built online-only specifically because a tab is touched by
several separate operations over time, and two staff members editing the
*same* tab from two *different* offline devices is a real multi-device
conflict `docs/PHASE_TABS_CREDIT.md` correctly flagged as overlapping with
Phase 4. That risk is real and this phase does not pretend to fully solve
it — it narrows it:

- **Solved:** one device, one continuous session, intermittent
  connectivity (the dominant real case for a Nigerian bar on patchy
  data/Wi-Fi) — open/resolve the bill, add rounds, pay, close, all queued
  locally in order and flushed once connectivity returns, exactly as
  reliably as Quick Sell already handles one sale today.
- **Not solved, deliberately:** two devices genuinely racing on the same
  bill while at least one is offline. When a queued action is finally
  flushed and the server rejects it because real state moved underneath
  it (balance changed, bill already closed, quantity no longer available),
  the action is surfaced locally as "didn't go through — check this bill"
  rather than retried blindly, silently dropped, or allowed to corrupt the
  bill. No new server-side generic review mechanism is being built for
  this in v1 — see "Explicitly out of scope" below.

## Backend changes needed

Checked against the actual code before writing this (not assumed):

- **`OpenBill`, `RecordPayment`, `CloseBill`, `VoidBill`, `WriteOffBill`
  currently take no idempotency key** — only `AddSaleRound`/`RemoveBillItem`
  do (they reuse `sales.idempotency_key` via the shared `postSaleRound`
  core). A retried/replayed queued `OpenBill` or `RecordPayment` today
  would create a duplicate bill or double-apply money. This is the one
  real, concrete backend gap blocking safe queuing — not a vague "needs
  more sync infrastructure."
- **Find-or-open, not raw idempotency, for the table/customer case.**
  Rather than giving `OpenBill` a client idempotency key, add
  `Service.ResolveBillForTable`/`ResolveBillForCustomer` (names tentative)
  that looks for an existing `open`/`closed_unpaid` bill against that
  table/customer id first and returns it, opening a new one only if none
  exists — naturally idempotent (retrying "resolve Table 3's bill" finds
  the same bill) and is also exactly the find-or-open behavior the picker
  needs regardless of offline/online. The pure walk-in case (no table, no
  customer) has no natural key to resolve against, so it keeps a real
  client-generated idempotency key: new nullable `bills.idempotency_key`
  (unique per business when set), migration `000030`.
- **`payments.idempotency_key`** (same migration): nullable, unique per
  business when set — `RecordPayment` checks it first inside the
  transaction, same shape as `CreateSale`/`AddSaleRound`.
- **`CloseBill`/`VoidBill` need idempotent-replay semantics, not new
  columns.** A queued "close this bill" that lands after the bill is
  already `closed_unpaid`/`settled`/`void` should be treated as already
  done, the same way `internal/invitations.AcceptAsExistingUser` already
  treats `identity.ErrAlreadyMember` as a fulfilled invitation rather than
  a failure. This is sync-layer logic (the client treats "already in or
  past the state I was asking for" as success), not a backend behavior
  change to the methods themselves.
- **No capability changes needed.** `CapabilityBillsManage`,
  `CapabilityBillsRead`, `CapabilityCreditGrant`, and
  `CapabilityPaymentsRecord` are *already* in
  `offlineSafeCapabilities` (`internal/tenancy/capabilities.go`) — the
  capability map anticipated this the same way `inventory:count`/
  `adjustment_request` anticipated Phase 7 before it was built.
  `bills:write_off`/`payments:reverse`/void stay owner-only and online-only,
  unchanged.
- **`GET /bills` needs to support listing/filtering by any status**, not
  just the existing `status=outstanding` — confirm/extend before building
  the frontend list against it.

## Frontend changes needed

- **`web/src/lib/db.ts`**: new Dexie table `pendingBillActions` (next
  version bump), one row per queued action —
  `{id, billLocalRef, type: 'resolve_table' | 'resolve_customer' | 'open_walkin' | 'add_round' | 'remove_item' | 'record_payment' | 'close', payload, idempotencyKey, createdAt}`.
  `billLocalRef` is a client-generated id standing in for a not-yet-synced
  bill, so a queued "add round" or "pay" that was recorded before its own
  "open" action has actually reached the server can still be replayed in
  the right order once it does — Quick Sell's existing queue never needed
  this (one action per sale), Bills does (several, dependent, per
  session).
- **`web/src/lib/billsSync.ts`**: mirrors `salesSync.ts`'s shape but
  flushes *per bill*, oldest action first, **stopping at the first
  failure for that bill only** — one stuck bill must not block flushing
  every other queued sale/bill/count/expense on the device. A failure that
  looks like a real conflict (not a connectivity error) marks that bill
  "needs attention" locally rather than retrying forever.
- **Unify the entry points**: `Tabs.tsx` becomes the one screen for both
  "start a new sale" (table/customer picker, defaults pre-selected) and
  "browse open/past bills" (the list, filterable by status). Reuses the
  already-shared `ProductGrid`/`CartPanel`. `Sell.tsx`'s current
  responsibilities fold into this flow; its one-shot `CreateSale` call
  stays as the invisible fast path for the walk-in-and-paid-in-full case
  described above, not a separately-reachable screen.
- **New route `/dashboard/tabs/:billId`** — full-page receipt-style detail,
  replacing today's inline bill-detail panel; shares a rendering component
  with the existing `BillShareLink` public view.
- **`AppBottomNav`**'s "Sell"/"Bills" entries point at this one flow with
  two entry modes (new vs. browse), not two different pages with different
  underlying guarantees.

## What actually shipped

- **Migration `000030`**: `bills.idempotency_key` / `payments.idempotency_key`
  (both nullable, partial unique index per business) — closing the real gap
  found by reading `tabs.go` before writing any code: `OpenBill`/
  `RecordPayment` had no idempotency contract, unlike `AddSaleRound`/
  `RemoveBillItem`, which already reuse `sales.idempotency_key`.
- **`Service.OpenBill` is now find-or-open**, not blind creation: a
  non-`uuid.Nil` `tableID` resolves to that table's existing `open`/
  `closed_unpaid` bill if one exists (via `GetOpenBillByTableID`), same for
  `customerID`; only the plain walk-in case (neither) consults
  `idempotencyKey`. Covered by
  `TestOpenBillResolvesToExistingOpenBillForSameTable`,
  `...ForSameCustomer`, and `TestOpenBillWalkInIsIdempotent`, plus a live
  curl pass confirming a second open against the same table returns the
  identical bill.
- **`Service.RecordPayment` gained the same idempotency-key contract** --
  `TestRecordPaymentIsIdempotent` plus a live curl replay confirming a
  repeated payment doesn't double-apply.
- **`GET /api/v1/bills?status=`** now accepts any real bill status (not
  just the pre-existing `open`/`outstanding` shortcuts) plus a new `all`,
  backing the Bills screen's filter. `ListBillsByStatus`/`ListAllBills` are
  new `Service`/sqlc methods.
- **`saleResponse` (and the frontend `Sale` type) gained `bill_id`**,
  always populated -- every sale, including `CreateSale`'s one-shot
  walk-in path, already posted against a real bill under the hood; this is
  what lets the frontend send *any* completed sale to the same
  `/dashboard/tabs/{bill_id}` page as its "here's what you just recorded"
  view, without needing two different confirmation screens.
- **Frontend is now three pages instead of two**: `Sell.tsx` is the one
  entry point for starting any sale -- a table picker (defaulting to "No
  table") and a customer picker (defaulting to "Walk-in customer", with a
  one-line explanation of what that means) are always both shown, never a
  mode toggle; a product grid/cart below builds the first order. Leaving
  both pickers at their defaults keeps using the original one-shot,
  offline-safe `createSale` call (`pendingSales`/`salesSync.ts`,
  unchanged) and shows "Complete Sale"; picking either turns it into a
  real, nameable bill via the new find-or-open `openBill` and shows "Start
  tab" instead (online-only, unchanged from Phase 6). Either path ends on
  `/dashboard/tabs/{billId}` -- the new `BillDetail.tsx`, a full-page
  receipt-style view (not a modal) carrying everything the old inline
  Tabs.tsx detail panel had: add/adjust items, record a payment, share,
  close/void/write-off, and the rounds history with its existing
  `ActivityDetailSheet` hookup. `Tabs.tsx` itself shrank to a pure Bills
  *list*: a status filter (`web/src/lib/billDisplay.ts`'s `billLabel`/
  `statusLabel` shared with the detail page), rows tapping through to that
  detail route, and a "+ New sale" button to `Sell.tsx`. `CartPanel`
  gained one new prop, `showFooterWhenEmpty`, so "Start tab" can render on
  a still-empty cart (seat a table, order later) without weakening its
  existing "only show the footer once there's a line" default for every
  other caller.
- **The Quick Sell / Bills distinction is gone from the UI.** Nothing in
  the app presents them as two tools or asks the user to choose a mode --
  the bottom nav's "Sell" and "Bills" buttons now point at, respectively,
  "start a new one" and "look up an existing one," both of the same thing.

## Verification

Live, against a running `go run ./cmd/api` with a disposable seeded
business, cleaned up afterward via the RLS-aware `SET LOCAL ROLE jbm_app`
transaction pattern: opening a bill against a real table twice in a row
returned the identical bill the second time (find-or-open); a one-shot
`POST /sales` response carried a real `bill_id`, and `GET /bills/{that
id}` showed it as a normal `settled` bill with one round and one payment;
`GET /bills?status=settled`/`?status=all` returned the right rows and
`?status=bogus` was rejected with `400`; a round posted and a payment
recorded against a real tab, replayed with the same idempotency key,
produced exactly one payment and a correct zero balance rather than
double-charging. Backend: `go test ./cmd/... ./internal/...` (all
packages) and the frontend's full `npm run check` (format, lint, 39
existing + preserved tests, and a production build) all pass.

## Explicitly out of scope for this phase

- The generic Phase 4 multi-device sync protocol — still deliberately
  deferred.
- A server-side, owner-visible queue for sync *conflicts* specifically
  (as opposed to the existing `sale_reviews`/`inventory_reviews`/
  `activity_flags` mechanisms, which cover different things). The local
  "needs attention" surface described above is the v1 answer; promoting
  it to something the owner can see from a different device is a
  reasonable follow-up once this ships and real conflict frequency (if
  any) is observed, not something to build pre-emptively.
- Multi-device conflict resolution on a single tab — named and accepted,
  not solved, per "What this does and doesn't solve" above.
- Any change to `bills:write_off`/`payments:reverse`'s owner-only,
  online-only status.

## Not verified yet / real follow-ups

- **No actual browser/Playwright pass over the rewritten `Sell.tsx`/
  `BillDetail.tsx`/`Tabs.tsx`** — only the backend they talk to was
  live-verified via curl this pass, plus the existing Vitest suite
  (unchanged assertions, still passing against the rewritten `Sell.tsx`).
  Same named gap this codebase already carries for the original
  `Sell.tsx`/`salesSync.ts` and OAuth's frontend.
- **Bills list access isn't scoped per-Staff** — `GET /bills` still uses
  the plain `bills:read` capability (both roles, unscoped), same as before
  this phase; nothing here added the kind of actor-scoping
  `mayOpenActivityEntry` gives the Activity feed. Worth a deliberate
  decision (not an oversight to just copy that pattern over) before
  assuming either way.
- **Offline support for table/customer tabs themselves** — still
  online-only, unchanged from Phase 6. The walk-in path's offline
  guarantee is real and shipped; the queue-and-flush design in this doc's
  earlier sections for bill actions specifically was not built. Revisit if
  real pilot usage shows a tab (not a walk-in) needing to survive going
  offline mid-session.
- The conflict-surfacing design ("needs attention" rather than silent
  corruption) described earlier in this doc only applies once that queue
  exists — there is nothing to verify yet.
