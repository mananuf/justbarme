# Unified Sell/Bills, and real offline support for tabs

**Status:** Design confirmed in conversation, not yet built. This doc is the
record of that conversation before any code changes, per the project's
working agreement (design confirmed, then implemented) and the newly
established rule that every new feature/fix branches off `staging` and is
run there before merging. Branch: `feature/unified-sell-bills`.
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

## Verification plan (before calling this done)

Same discipline every other phase in this file was held to:

- Live, single-device: start a sale with defaults (walk-in path), confirm
  it still posts via the existing one-shot call; start a sale against a
  real table, add two rounds, go offline mid-session (airplane mode),
  keep adding rounds and record a partial payment, reconnect, confirm the
  bill on the server reflects every queued action in the right order and
  the balance is correct.
- Pick an already-open table a second time (same device, same session)
  and confirm it resumes the existing bill rather than opening a second
  one.
- Force a real conflict: open the same table's bill on two sessions, go
  offline on one, remove an item on the other (changing what's available),
  reconnect the offline one with a queued removal that no longer fits —
  confirm it surfaces as "needs attention" locally rather than corrupting
  the bill or silently vanishing.
- Bills list filters correctly by status; the full-page detail view opens
  correctly via direct link/back button both for an owner and for staff
  scoped to their own view (reuse `mayOpenActivityEntry`-style scoping if
  Staff shouldn't see every bill's detail — confirm this against
  `bills:read`'s actual current scope before assuming either way).
