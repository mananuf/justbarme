# Platform admin: deep drill-down and corrective actions

**Status:** Built and verified live end-to-end against a running backend.
**Not** part of `docs/`'s frozen plan — like every other platform-admin
feature, designed and confirmed in-session.
**Depends on:** `internal/platformadmin` (the existing admin dashboard —
`docs/PHASE_ADMIN_DASHBOARD.md` — this widens it, it does not replace
it), and reuses `internal/sales`/`internal/expenses`/`internal/inventory`/
`internal/activity`/`internal/identity`'s existing Service methods
directly — no new Service methods were needed for reads, and only three
new ones for writes (`inventory.Service.AdminForceReverseStockReceipt`,
`AdminCorrectBalance`, `ListAllAdjustmentRequests`; see "As built" below
for exactly what changed from the original design).

## Why this exists

The existing admin dashboard (`GET /platform/businesses/{id}/activity`)
is deliberately aggregate-only: sales today/7d, outstanding tabs, an
alert count, stock health counts, owner/staff *counts*, last-activity
time, five recent-activity headlines. `CLAUDE.md`'s own words on it:
*"Widening this to line-item drill-down is a separate privacy decision —
do not add it casually."* This is that decision, made deliberately rather
than by accretion.

The concrete problem: when a pilot business reports something's wrong
(a sale that looks off, a stock balance that doesn't make sense, a bill
stuck open), the only way to investigate today is to open a `psql`
session against production, or ask the business to screenshot their own
dashboard. Neither scales, and `psql` access sidesteps the entire
capability/audit system this codebase otherwise takes seriously.

## Scope decisions (confirmed)

1. **Read depth: full line items, including staff identity. Customer PII
   stays redacted.** A platform admin can see an individual sale,
   expense, stock receipt, inventory adjustment, bill, and who recorded
   it — but a `customers` row's `name`/`phone`/`email`/`notes` (migration
   `000020`) are never returned verbatim; see "Customer PII redaction"
   below for the concrete shape.
2. **Write: corrective actions, including new superadmin-only mechanisms
   that don't exist for business owners.** Not full impersonation. Every
   corrective action is purpose-built, named, and auditable — never a
   generic "act as this user" escape hatch. See "Corrective actions"
   below for the concrete list.
3. **Transparency: silent on read, visible on write.** Viewing stays
   audit-logged-but-silent (matching the existing dashboard). Every
   corrective action shows up in the business's own `GET /activity` feed,
   attributed to a reserved "Platform Support" actor — the owner always
   sees that something was corrected, by platform support, with a reason,
   the same way they'd see any other correction.
4. **Role gate: read is both roles (Support + Superadmin); write is
   superadmin-only.** Matches the existing pattern exactly — Support
   never mutates anything, anywhere, by rule.

## New capabilities

In `internal/platformadmin/capabilities.go`, two additions to the
existing `ForRole` map (no change to its shape, just two more entries):

```go
// CapabilityBusinessesReadDetail reads full line-item detail inside one
// business -- individual sales/expenses/receipts/adjustments/bills/
// reviews, with staff identity resolved. Customer PII is never included
// regardless of this capability (see the detail doc's redaction rule).
// Both roles, same tier as CapabilityBusinessesReadActivity.
CapabilityBusinessesReadDetail Capability = "platform:businesses:read_detail"

// CapabilityBusinessesAdjust performs a corrective action against a
// business's tenant data on its behalf -- reversing a sale/expense/
// receipt, force-closing a bill, overriding a blocked reversal, directly
// correcting an inventory balance. Superadmin only, permanently, by the
// same "every platform write is superadmin-only by rule" convention
// CapabilityStaffManage already follows.
CapabilityBusinessesAdjust Capability = "platform:businesses:adjust"
```

`supportCapabilities` gains `CapabilityBusinessesReadDetail`.
`allCapabilities` gains both.

## Read endpoints

New file `internal/httpapi/platform_admin_detail_handlers.go`, same shape
as `getPlatformBusinessActivity`: `requirePlatformCapability(w, r,
platformadmin.CapabilityBusinessesReadDetail)`, resolve `business_id`,
audit (see below), then call straight into the existing feature
packages' own Service methods with `uuid.Nil` as the acting user —
**no new RLS policy, no bypass role, identical mechanism to the existing
dashboard.** This is the one rule everything else in `internal/
platformadmin` already follows, and it does not change here:

| Endpoint | Backed by |
|---|---|
| `GET /platform/businesses/{id}/sales` | `sales.Service.ListSales` |
| `GET /platform/businesses/{id}/sales/{sale_id}` | `sales.Service.GetSale` |
| `GET /platform/businesses/{id}/expenses` | `expenses.Service.ListExpenses` |
| `GET /platform/businesses/{id}/expenses/{expense_id}` | `expenses.Service.GetExpense` |
| `GET /platform/businesses/{id}/bills` | `sales.Service.ListAllBills` |
| `GET /platform/businesses/{id}/bills/{bill_id}` | `sales.Service.GetBillDetail` |
| `GET /platform/businesses/{id}/stock-receipts/{receipt_id}` | `inventory.Service.GetStockReceipt` |
| `GET /platform/businesses/{id}/inventory-adjustments` | `inventory.Service.ListPendingAdjustmentRequests` (+ a new `ListAllAdjustmentRequests` — the existing method is pending-only, which is a real gap for drill-down of an already-decided one) |
| `GET /platform/businesses/{id}/stock/{variant_id}/history` | `inventory.Service.GetHistory` |
| `GET /platform/businesses/{id}/sale-reviews` / `inventory-reviews` | `sales.Service.ListOpenReviews` / `inventory.Service.ListOpenReviews` |
| `GET /platform/businesses/{id}/activity` (line-item, not the aggregate one) | `activity.Service.List` directly, unfiltered by actor (the aggregate endpoint's existing route name is already taken, so this needs a distinct path, e.g. `.../activity/feed`) |
| `GET /platform/businesses/{id}/members` | `identity.Service.ListMembersForBusiness` (already used by the aggregate dashboard for counts; this exposes the names) |

Staff identity is resolved the same way `resolveSellerNames` already does
for the business's own activity feed — platform admin's view is simply
never actor-scoped the way a Staff member's own view is, which is a
property of *who's asking*, not a new resolution mechanism.

### Customer PII redaction

Every response above that could surface a `customers` row (bill detail,
sale detail where a bill has a named customer) replaces
`name`/`phone`/`email`/`notes` with a stable, non-identifying reference —
`"Customer #<first 8 chars of the customer's own id>"` — so two entries
for the same customer are still recognizably the same customer (useful
for "this customer's tab keeps going negative") without exposing contact
details. `table_id`/table labels are not PII (an owner-assigned label
like "Table 3") and pass through unredacted.

## Corrective actions (superadmin-only)

Every action below is **modeled as the business's own existing
correction idiom wherever one exists**, called on the business's behalf
— never a raw `UPDATE`/`DELETE`. Where no idiom exists yet (bullet 3),
a new one is added to the owning feature package, not to
`internal/platformadmin` itself, keeping "each feature package stays
self-contained."

1. **Reverse a sale / expense / stock receipt**, same guardrails as the
   business's own reversal (`sales.Service.ReverseSale`,
   `expenses.Service.ReverseExpense`,
   `inventory.Service.ReverseStockReceipt`) — no new method needed, these
   already take an `actorID`; platform admin just calls them with the
   reserved system actor (below) instead of a real owner/staff id.
2. **Force-close or void a stuck bill**: `sales.Service.CloseBill`/
   `VoidBill` already exist and need no override — "stuck" here usually
   just means the owner doesn't know these exist or how to use them, not
   that a guardrail is blocking a legitimate case. Call directly.
3. **New: override-reverse a stock receipt that's already been partly
   consumed.** `inventory.Service.ReverseStockReceipt`'s existing guard
   (a lot's `remaining_quantity` must still equal what it received, *and*
   the live balance must not go negative) is correct for an owner — it
   must stay. A new `inventory.Service.AdminForceReverseStockReceipt(ctx,
   actorID, businessID, receiptID uuid.UUID, reason string)` skips only
   the *remaining-quantity* guard (never the negative-balance one — that
   one is a real correctness invariant, not a business-logic preference)
   and requires a non-empty `reason`, persisted on the reversal receipt's
   own audit trail via the existing `reversal_of_receipt_id` link plus a
   new `platform_audit_log` entry (see below). This is the one "new
   mechanism a business owner doesn't have" bullet 2's question actually
   motivated — named and narrow, not a generic bypass flag threaded
   through the normal method.
4. **New: resolve a sale/inventory review or adjustment request on the
   business's behalf.** `sales.Service.ResolveReview`/
   `inventory.Service.ResolveReview`/`ApproveAdjustmentRequest`/
   `RejectAdjustmentRequest` already exist and take a `resolvedBy`/
   `decidedBy` actor — call them with the reserved system actor, no new
   method needed.
5. **New: correct an inventory balance directly, with no receipt/
   adjustment trail to explain it** (e.g. a bug left a balance wrong with
   no normal event that caused it). This is the single most sensitive
   corrective action, because `inventory_balances` is documented as a
   *rebuildable projection* over `inventory_movements` — writing a raw
   balance number would break that invariant. `inventory.Service.
   AdminCorrectBalance(ctx, actorID, businessID, variantID uuid.UUID,
   newBalance int64, reason string)` must therefore compute `delta :=
   newBalance - currentBalance` and post it through the **existing**
   movement/event machinery (`inventory_events` type `'adjustment'`,
   `inventory_adjustment_requests.reason_category = 'platform_correction'`
   — a new allowed value, auto-approved since a superadmin is the actor)
   so the balance is still provably derived from its own ledger, same as
   every other balance change in this codebase.

### The reserved "Platform Support" actor

`seller_id`/`actor_id` columns across `sales`/`bills`/etc. are `NOT NULL
REFERENCES users (id)` — there is no existing nullable-actor path. Rather
than adding nullable columns everywhere (a much bigger migration surface,
and it would need every existing display path —
`resolveSellerNames`, the activity feed, bill rounds — to learn a new
null-actor rendering rule), the plan is a **single, fixed, seeded
system user** (one global row, well-known UUID, e.g. inserted by a new
migration with `name = 'Platform Support'`, an unguessable/unusable
password hash, no memberships anywhere). Every corrective action uses
this row's id as `actorID`/`seller_id`. Because every existing
name-resolution path already does a plain `identity.Service.GetUserByID`
lookup with no special-casing, "Platform Support" then shows up
correctly, automatically, in the business's own Recent Activity, bill
rounds, and `GET /activity` feed — **no changes needed to any existing
display code** for this to work. The one thing to verify before relying
on this: that no existing per-business "owner/staff count" logic
(`ListMembersForBusiness`) could ever pick this row up — it reads
`business_memberships`, and this user is deliberately a member of
nothing, so it shouldn't, but confirm this with a real query during
implementation, not just by inspection.

## Auditing

`platform_audit_log` (migration `000015`) currently has no column for
*which specific record* was touched — `target_business_id` plus a free
`reason` is enough for "suspended this business" but not for "viewed
this specific sale" or "reversed this specific receipt." A new migration
adds a nullable `target_resource TEXT` column (free-form, e.g.
`"sale:3f2a...-..."`, `"stock_receipt:9c1b...-..."`) — deliberately a
plain string, not a new foreign-key-per-resource-type column, matching
`platform_audit_log`'s own existing "nullable, free-text reason" style
rather than inventing a polymorphic-association schema for what is, in
practice, a handful of resource types.

- **Every read-detail endpoint logs before returning data**, same
  non-best-effort discipline as `RecordBusinessActivityViewed` — a new
  `ActionBusinessDetailViewed = "business.detail_viewed"`, with
  `target_resource` set to what was viewed (`"sale:<id>"`, etc.) and
  `target_business_id` set.
- **Every corrective action logs in the same transaction as the action
  itself** — this is the existing `SuspendBusiness`-style atomicity
  pattern (action and its audit entry either both land or neither does),
  now applied to tenant-data mutations instead of just
  `businesses.status`. New actions: `ActionBusinessSaleReversed`,
  `ActionBusinessExpenseReversed`, `ActionBusinessReceiptReversed`,
  `ActionBusinessReceiptForceReversed`, `ActionBusinessReviewResolved`,
  `ActionBusinessBalanceCorrected`, etc. — one per corrective action type,
  not one generic `"business.adjusted"` bucket, so the audit log stays
  queryable by what actually happened.
- **`reason` is required (non-empty) on every corrective action**,
  enforced in the handler the same way `WriteOffBill`'s reason already
  is — a corrective action with no stated reason is not auditable in any
  meaningful sense.

## Business-facing transparency

Because every corrective action is posted through the existing
feature-package methods with the reserved "Platform Support" actor, it
**automatically appears in the business's own `GET /activity` feed and
Recent Activity** with no new code in `internal/activity` — the UNION ALL
already includes `sales`/`expenses`/`stock_receipts`/approved
`inventory_adjustment_requests`, and this just adds rows to those same
tables with a recognizable actor. The one piece of actual new frontend
work: `Activity.tsx`'s existing per-type summary strings (e.g. "Ada
recorded a sale") should read naturally when the actor is "Platform
Support" rather than a staff member's name — verify each summary string
reads sensibly either way rather than assuming it does.

## What this deliberately does not do

- **No impersonation.** A superadmin can never act *as* a specific
  owner/staff member, hold their session, or see anything through their
  exact permission lens. Every corrective action is its own named,
  reason-required, audited endpoint — the set of things a platform admin
  can do is enumerable by reading `platform_admin_detail_handlers.go`,
  not "anything an owner could theoretically do."
- **No customer PII exposure**, regardless of role or capability — this
  is a hard line, not a configurable one, per the scope decision above.
- **No raw balance overwrite.** Even the most direct corrective action
  (inventory balance correction) is expressed as a computed delta through
  the existing ledger, never a bare `UPDATE inventory_balances`.
- **Still no RLS bypass role**, anywhere. Every new read and write in
  this phase goes through `store.WithApp`/`WithTenant` exactly like every
  other platform-admin and tenant-facing call in this codebase.

## Compliance note (NDPA) — flagging, not a full assessment

[PRACTICE/flag, not a ruling] Reading a business's sales/expense/bill
line items as platform support is processing personal data about the
business's own staff (names attached to actions) and, via bill/customer
references, data that traces back to the business's customers even after
redaction (a stable pseudonymous reference is still indirectly
identifying if cross-referenced). Two things worth getting a real answer
on before this ships to production, not assumed:

1. **Lawful basis** for a platform operator processing a tenant
   business's own staff/customer-linked data for a support purpose — this
   likely sits on legitimate interest or contract-performance grounds
   (operating the platform the business contracted for), but NDPA s.25(2)
   narrows legitimate interest in ways GDPR intuition doesn't cover; this
   should be confirmed against the Act rather than assumed by analogy.
2. **Whether the business's own terms of service already disclose** that
   platform support can access this level of detail for troubleshooting —
   if not, the privacy-notice obligations this triggers (`docs/`'s own
   NDPA assessment pattern, see the earlier WhatsApp-messaging compliance
   note) should be closed before this feature is used on a real business,
   not after.

This is a flag for a real NDPA pass before shipping, not a blocker on
writing the code — the two are independent and can proceed in parallel.

## As built — what changed from the original design

- **`ListAllAdjustmentRequests` was built** (migration `000032` widens
  `inventory_adjustment_requests.reason_category`'s CHECK to add
  `'platform_correction'`, plus a new `ListAllInventoryAdjustmentRequestsDetailed`
  sqlc query) — the activity feed's approved-adjustments branch doesn't
  carry decision context (who decided it, their note), so a genuine new
  method was the right call, not a duplication. `AdjustmentRequest`
  gained `DecidedBy`/`DecidedAt`/`ResolutionNote` fields (zero-valued
  until decided), threaded through `GetAdjustmentRequest` and the new
  method; `toAdjustmentRequestResponse` gained a `decidedByName` parameter
  (every pre-existing call site passes `""`, unaffected).
- **`AdminCorrectBalance` needed no new Service-level ledger-posting
  logic duplication risk the plan worried about** — it's a single new
  method that opens one `WithTenant` transaction, reads the live balance,
  computes the delta, and inlines the same
  create-request→approve→event→movement→balance-upsert→negative-review
  sequence `ApproveAdjustmentRequest` already uses (necessarily inlined,
  not called as two separate public methods, since each of those opens
  its own transaction — composing them as two calls would mean a
  created-but-unapproved request could survive a failure between the
  two). `ErrBalanceAlreadyCorrect` added for the zero-delta case (the
  `quantity_delta <> 0` CHECK would otherwise surface as a confusing raw
  constraint violation).
- **`AdminForceReverseStockReceipt` turned out to need a real design
  decision the plan only gestured at**: giving back "whatever's left"
  means computing `giveBack := min(originalQuantity, lot.RemainingQuantity)`
  **per line**, in a first pass with no writes, so a receipt with nothing
  left anywhere (`ErrReceiptFullyConsumed`) can be rejected before any row
  exists — and a partially-given-back line's reversed cost is computed
  proportionally (`lineTotalCost * giveBack / originalQuantity`), the one
  place in this codebase a per-unit cost is deliberately derived rather
  than stored, accepted here as a bounded exception for this specific
  admin-override edge case. Three dedicated unit tests
  (`TestAdminForceReverseStockReceiptGivesBackOnlyWhatsLeftInTheLot`,
  `...RejectsFullyConsumedReceipt`, `...StillBlocksOnNegativeBalance`)
  pin this, using a `consumeLot` test helper that decrements both
  `stock_lots.remaining_quantity` and `inventory_balances` together (a
  real sale always moves both; an earlier draft of the helper only moved
  the lot and produced a silently wrong balance expectation, caught by
  the first test run before it ever reached a commit).
- **The "same transaction" claim for `RecordCorrectiveAction` was
  corrected during implementation.** The original draft said each
  corrective action's audit entry is written "in the same transaction as
  the action itself," copying `SuspendBusiness`'s phrasing. That's not
  achievable here: the mutation runs through a different package's
  `store.WithTenant` transaction (tenant-owned tables), while the audit
  write is `internal/platformadmin`'s own `store.WithApp` transaction
  (non-tenant `platform_audit_log`) — two different transactions, no way
  to span both. The real contract, implemented and documented on
  `RecordCorrectiveAction` itself: mutate first, audit immediately after,
  and surface a loud `500` (never swallow) if the audit write fails after
  the mutation already succeeded — the same self-healing-not-atomic
  honesty `internal/oauth`'s account-link write already accepts for an
  analogous constraint.
- **A real bug was caught by live curl verification, not by unit
  tests**: `listPlatformAuditLog`'s HTTP response type
  (`platformAuditEntryResponse`) was never updated to include the new
  `target_resource` field, even though the domain model, the sqlc query,
  and the Service-level `AuditEntry.TargetResource` field were all
  correct and covered by a passing Go test
  (`TestRecordDetailViewedAndCorrectiveActionCarryTargetResource` asserts
  against the `platformadmin.Service` layer directly, which was already
  right). The gap only showed up hitting the real HTTP endpoint with
  curl — a reminder that a unit test one layer below the HTTP response
  struct doesn't catch a forgotten field in that struct. Fixed in
  `internal/httpapi/platform_handlers.go`.
- **Frontend: built as a dedicated page**, `PlatformBusinessDetail.tsx`
  (`/platform/businesses/:businessId`, linked from `PlatformDashboard.tsx`'s
  business row via a new "Deep drill" link), with the seven resource
  types as tabs (Sales, Expenses, Bills, Stock, Reviews, Members,
  Activity) rather than expanding `BusinessActivityPanel.tsx` — the
  amount of content made a dedicated page the clear right call once
  actually building it. `src/api/platformDetail.ts` is the typed client.
  Every corrective-action button opens a shared inline `ActionPrompt`
  (one reason field, matching `PlatformDashboard.tsx`'s existing
  suspend/reactivate prompt shape) except balance correction, which needed
  its own two-field `CorrectBalancePrompt` (target quantity + reason) —
  an early draft tried overloading the single-reason-field `ActionPrompt`
  with a pipe-delimited "quantity | reason" string, which worked but was
  bad UX, replaced before this was considered done.

## Verified live end-to-end

Against a running backend (`go run ./cmd/api`, a real local Postgres),
using curl with the frontend's own exact request/response shapes, after
seeding a superadmin, a support staff account, and a real business with a
product/variant/stock receipt/sale:

- Support: `200` reading sales, members, bill detail, stock receipts,
  the activity feed; `403 PERMISSION_DENIED` attempting to reverse a sale
  — confirmed again after restarting the server (ruling out a stale
  compiled binary masking the real result, which is exactly what
  happened the first time through, see below).
- Superadmin: reversing a sale without a `reason` correctly `422`s
  (`VALIDATION_FAILED`); with a reason, returns `201` and the reversal's
  `seller_id` is the reserved Platform Support UUID.
- The reversal's audit log entry carries the right `action`,
  `target_resource` (`"sale:<id>"`), and `reason` — confirmed only after
  fixing the `listPlatformAuditLog` response gap above (a real `kill
  <pid>` + restart was needed mid-verification: an earlier `pkill -f "go
  run ./cmd/api"` missed the actual compiled child process, so a stale
  pre-fix binary kept serving requests and made the bug look unfixed for
  one extra round).
- The reversal shows up in the **business's own** `GET /activity` feed
  (the ordinary, non-platform endpoint) as "Sale reversed" with
  `actor_name: "Platform Support"` — zero new code in `internal/activity`
  or its frontend rendering made this work, exactly as designed.
- A balance correction (`target_quantity: 50` on a variant sitting at
  48) returned a pre-approved `platform_correction` adjustment, and
  `GET .../stock/{variant}/history` showed it as a real `+2` ledger
  movement attributed to Platform Support, alongside the receipt and
  sale/reversal movements that came before it.
- `ListAllAdjustmentRequests` showed the balance-correction request with
  both `requested_by_name` and `decided_by_name` as "Platform Support"
  and the auto-generated resolution note.
- Customer PII: confirmed by inspection that no response anywhere in this
  feature's endpoints includes a `customers` row's `name`/`phone`/`email`/
  `notes` — `billResponse`/`billDetailResponse` only ever expose
  `customer_id` (an opaque UUID, already the existing shape business
  owners themselves see), and no `ListCustomers`/`GetCustomer`-equivalent
  endpoint was added to the platform routes.
- All test data (businesses, users, platform staff, audit log entries)
  cleaned up afterward via the RLS-aware `SET LOCAL ROLE jbm_app`
  transaction pattern, in FK-respecting order (events before the
  adjustment requests/receipts they reference, movements before events)
  — the first cleanup attempt got this order wrong and was rolled back
  cleanly before retrying, no partial state left behind.

Backend: full `go test ./cmd/... ./internal/...` passes, including 10 new
tests (7 in `internal/inventory`, 2 in `internal/platformadmin`, 1 in
`internal/identity`) alongside every pre-existing test unchanged.
Frontend: full `npm run check`-equivalent (format, lint, 59 tests
including 2 new ones for `PlatformBusinessDetail.tsx`, production build)
passes.

**Not done**: an actual browser/Playwright click-through of the new
frontend page — Claude in Chrome's extension was not connected in this
environment, the same named gap several other phases in this codebase
already carry (OAuth, Unified Sell/Bills' rewritten pages, Phase 7's
offline-queueing UI). The backend every one of this page's calls hits was
independently, thoroughly live-verified via curl above; only the
frontend's own rendering and interaction code was not driven through a
real browser. Worth a manual pass before trusting this in production.

## Open questions genuinely still open

- Pagination/filtering shape for the line-item list endpoints beyond a
  simple `limit` — no date-range filter was added to
  `platformListSales`/`platformListExpenses`, unlike the business-facing
  reports endpoints. Add if a real support case needs it; not built
  speculatively.
- The NDPA lawful-basis question flagged above is still open — this
  shipped the code with that flag, not a legal sign-off.
