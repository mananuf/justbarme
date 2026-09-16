# justbarme Collaborative MVP Implementation Plan

**Status:** Phase 0 approved; Phase 1 backend ready to start  
**Date:** 2026-09-12  
**Backend owner:** Project owner/user  
**Frontend owner:** AI coding agent  
**Architecture source:** [`ARCHITECTURE.md`](ARCHITECTURE.md)

---

## 1. Working agreement

### Responsibilities

#### You own the backend

You will implement:

- Go application/server
- Chi routes and middleware
- PostgreSQL schema and migrations
- database queries and transactions
- authentication and authorization
- tenant isolation/RLS
- sync ingestion/change feed
- domain services
- API handlers
- backend tests
- backend deployment configuration

#### I own the frontend

I will implement:

- React/TypeScript/Vite application
- visual design system and responsive layout
- PWA manifest and service worker
- IndexedDB/Dexie schema
- local outbox and projection logic
- sync client
- authentication screens and local unlock UX
- catalogue, sell, bills, stock, expense, activity, reports, and settings UI
- frontend validation and API integration
- frontend unit/component/E2E tests
- accessibility and mobile performance work

Do not implement frontend code unless we explicitly reassign a task. Shared API contracts and domain terminology are reviewed together.

### Phase workflow

For every phase:

1. I give you the backend task checklist and acceptance criteria.
2. You implement only that phase’s backend scope.
3. You tell me the phase is ready and include the requested validation output or known failures.
4. I review your code against the architecture, security rules, and API contract.
5. I make focused backend corrections where appropriate, referencing the changed files and explaining why.
6. We agree/freeze the phase’s API contract.
7. I implement the corresponding frontend scope.
8. I run frontend and integration validation.
9. We close the phase before starting the next one.

I will preserve your code style where it is sound. I will not rewrite backend work merely to make it look like my preferred style. I will fix root causes, tenant/security gaps, correctness issues, API mismatches, and missing tests.

### Definition of phase-ready

When you finish a backend phase, send:

- a short summary of what you implemented
- migrations added
- routes added or changed
- tests/commands run and their results
- known limitations or decisions you made
- any API response examples that differ from the agreed contract

Do not commit unless you want to; commits are not required for review.

---

## 2. How `bluelight` informs this project

Reference project inspected read-only:

```text
/Users/bankat/Documents/golang/fullgodev/bluelight
```

### Patterns to retain

| Pattern | `bluelight` reference | justbarme use |
|---|---|---|
| Explicit application dependency container | `cmd/api/main.go` | Move composition to testable `internal/app`; keep `cmd/api/main.go` thin |
| Separate server lifecycle | `cmd/api/server.go` | Keep bounded timeouts and graceful shutdown in `internal/app`/`internal/httpapi` |
| Central route registration | `cmd/api/routes.go` | Use one clear Chi route composition point in `internal/httpapi/router.go` |
| Composable middleware | `cmd/api/middleware.go` | Keep HTTP middleware in `internal/httpapi`; domain authorization stays in services |
| Strict JSON decoder | `cmd/api/helpers.go` | Keep size limits, unknown-field rejection, and one-value enforcement |
| Consistent JSON writer/error helpers | `cmd/api/helpers.go`, `errors.go` | Upgrade to stable machine-readable error codes |
| Request context helpers | `cmd/api/context.go` | Use typed keys for principal/business context |
| Small validator package | `internal/validator/validator.go` | Retain for input/domain validation |
| SQL migrations | `migrations/` | Keep versioned up/down migrations |
| Structured startup and config | `internal/config/config.go` | Keep environment-based configuration |

### Patterns to improve rather than copy

1. **Router:** use Chi instead of `httprouter`. Chi remains compatible with `net/http` and supports grouped tenant/auth middleware cleanly.
2. **Database:** use `pgx/v5` and `pgxpool`, not `lib/pq`. Pass request context into every query; do not create `context.Background()` inside models.
3. **Architecture:** avoid one large generic `data.Models` package as the application grows. Keep domain services grouped by feature.
4. **Tenant isolation:** every tenant query executes in a transaction with transaction-local `app.business_id`, backed by RLS and composite foreign keys.
5. **Database errors:** inspect `pgconn.PgError` codes/constraint names; do not match arbitrary error strings.
6. **Authentication:** permissions are membership/business-scoped, not global user permissions.
7. **Errors:** return `{error: {code, message, request_id, details}}`, not only human-readable strings.
8. **Logging:** never log response bodies, auth tokens, customer details, or sync event payloads by default.
9. **Secrets:** no SMTP credentials, signing keys, or passwords in source/default flags. Existing secrets found in a reference project should not be copied and should be rotated if they are real.
10. **Rate limiting:** apply targeted limits to authentication/public endpoints. A process-local IP map is acceptable only for a single-instance pilot and must not be mistaken for distributed protection.
11. **Timeouts:** use realistic idle/read-header/read/write/shutdown settings. A very short global idle timeout is unnecessary.
12. **Background work:** do not use fire-and-forget goroutines for financial/sync correctness. Critical changes commit synchronously in PostgreSQL.

### Selected backend layout

This takes the useful explicitness from `bluelight` but keeps the executable thin and separates HTTP, database infrastructure, and business features:

```text
cmd/api/
  main.go
internal/
  app/
    app.go
    run.go
  config/
    config.go
  httpapi/
    server.go
    router.go
    middleware.go
    json.go
    errors.go
    health.go
  store/
    postgres.go
    tx.go
    queries/
    sqlc/
  auth/
  tenancy/
  identity/
  business/
  catalogue/
  syncer/
  sale/
  bill/
  payment/
  inventory/
  expense/
  activity/
  report/
  review/
  sharing/
migrations/
web/
```

`cmd/api/main.go` should contain little more than process-context setup and a call into `internal/app`. Do not create empty feature packages preemptively. Add each package when its phase starts.

---

## 3. Shared engineering conventions

These should be agreed before implementation diverges.

### API

- Prefix: `/api/v1`
- JSON uses `snake_case`.
- IDs are UUIDv7 strings.
- Timestamps are RFC 3339 UTC strings.
- Money is integer kobo in JSON fields ending `_kobo`.
- Quantities are decimal strings, never JSON floats.
- Success responses use `{ "data": ... }` with optional `meta`; HTTP status is not duplicated in a JSON `status` field.
- Errors use `{ "error": { "code", "message", "request_id", "details" } }` with stable codes.
- Collection endpoints use cursor pagination where feeds/history can grow indefinitely; page/limit is acceptable for small administrative lists.
- Posted business events are corrected through new endpoints/events, not `PATCH` or `DELETE`.

### Database

- PostgreSQL only in integration tests.
- `business_id` is first in tenant-scoped indexes.
- Every tenant child uses a composite tenant-aware foreign key.
- Posted financial/inventory tables deny application-role update/delete where practical.
- Every migration includes a down migration unless reversal would be unsafe; unsafe reversal must be documented.
- Store all instants as `timestamptz`.

### Go

- Handlers parse/validate transport data and call services; they do not contain SQL.
- Services own use cases and transactions.
- Repositories/query functions accept `context.Context` and a transaction-capable interface.
- Wrap errors with operation context while preserving `errors.Is`/`errors.As` behavior.
- Never use floating point for money or quantities.
- Avoid package-global mutable state.
- Use table-driven tests where appropriate.

### Contract changes

Once a phase contract is frozen, a breaking response/request change requires:

1. updating the contract/example
2. updating backend tests
3. updating frontend schemas/client
4. running the phase integration scenario

---

# 4. Phase-by-phase plan

## Phase 0 — Contracts and project decisions

**Status:** Approved on 2026-09-12.  
**Goal:** freeze conventions and the first vertical slice before production code.

### Frozen decisions

- Module: `github.com/mananuf/justbarme`
- Minimum Go version: 1.22
- Router: Chi over `net/http`
- UUIDs: `github.com/google/uuid` with UUIDv7
- Migrations: `golang-migrate`
- Database: `sqlc` + `pgx`; direct `pgx` for genuinely dynamic queries
- Pilot login: email/password
- Online session: hashed opaque server-side session in a secure same-origin cookie
- Offline authority: signed seven-day device lease
- API success envelope: `data` with optional `meta`
- API error envelope: stable code/message/request ID/details
- Tenant selector: authenticated `X-Business-ID` header on tenant-scoped routes
- Authorization: fixed Owner/Staff capability mapping in Go for MVP

The accepted detailed contract is in [`API_CONTRACT.md`](API_CONTRACT.md).

### Your backend tasks

Draft, but do not yet implement:

1. Proposed Go module path and minimum Go version.
2. Exact top-level backend directory structure.
3. Configuration keys for environment, HTTP server, database, auth signing, and CORS/same-origin behavior.
4. Standard success/error JSON shapes.
5. UUIDv7 library choice or generation approach.
6. Migration tool choice: `goose` or `golang-migrate`.
7. SQL approach choice:
   - explicit `pgx` queries, or
   - `sqlc` over explicit SQL.
8. Initial authentication choice:
   - email/password for pilot, or
   - phone/password if delivery/reset support is ready.
9. A draft capability list for Owner and Staff.
10. Draft endpoint contracts for:
    - health/readiness
    - login/logout/me and opaque session lifecycle
    - business creation/current business
    - device enrollment

### My frontend tasks after review

- Draft route map and application navigation.
- Define the frontend API envelope types.
- Draft mobile-first wireframes for login, dashboard shell, and sync status.
- Define browser support and PWA install criteria.
- Define IndexedDB naming/versioning conventions.

### Review gate

No scaffolding starts until we agree on:

- module path
- Go version
- migration/query tools
- auth/session approach
- error envelope
- identifiers, money, quantity, and timestamps
- initial capability names

### What to send me

Send your Phase 0 choices as a short document or message. I will review them against `ARCHITECTURE.md` and turn the accepted decisions into the project documentation before either of us writes application code.

---

## Phase 1 — Backend and frontend foundations

**Goal:** start both applications with production-safe lifecycle and test scaffolding.

### Your backend tasks

1. Initialize the Go module.
2. Add Chi and `pgx/v5`.
3. Build configuration parsing with secrets supplied only through environment variables.
4. Create the application composition root.
5. Configure `http.Server` with:
   - read-header timeout
   - read timeout
   - write timeout
   - idle timeout
   - graceful shutdown timeout
6. Add:
   - request ID middleware
   - panic recovery
   - structured request logging
   - security headers
7. Implement:
   - `GET /api/v1/health/live`
   - `GET /api/v1/health/ready`
8. Open and verify a PostgreSQL pool during startup.
9. Add strict JSON read/write helpers and stable error envelope.
10. Add unit tests for JSON/error helpers and handler smoke tests.
11. Add Makefile or task commands for test, run, migration, and formatting.

### Backend acceptance criteria

- Service starts and stops cleanly on SIGINT/SIGTERM.
- Readiness fails if PostgreSQL is unavailable; liveness does not depend on a query.
- Unknown JSON fields and oversized bodies fail predictably.
- Logs contain request ID but no request/response body.
- `go test ./...`, `go vet ./...`, and formatting checks pass.

### My frontend tasks

- Initialize React + TypeScript + Vite under `web/`.
- Add routing, app shell, error boundary, and responsive layout primitives.
- Add design tokens and initial justbarme visual direction.
- Configure linting, formatting, tests, and Playwright.
- Add PWA manifest and application-shell service worker.
- Build health-aware API client foundation and stable error parsing.
- Add offline/connectivity and global sync-status placeholders.

### Integration gate

The frontend can load from the intended origin, call readiness in development, and display a controlled unavailable state.

### What to send me

Tell me Phase 1 is ready with the commands and outputs. I will review all backend foundation files, adjust issues, then implement the frontend foundation.

---

## Phase 2 — Businesses, memberships, tenancy, and authentication

**Goal:** establish the security boundary before tenant data modules exist.

### Your backend tasks

1. Create migrations for:
   - users
   - businesses
   - business memberships
   - locations
   - opaque sessions and CSRF bindings
   - devices
2. Add globally unique IDs and timestamp conventions.
3. Create separate migration-owner and application database roles/documentation.
4. Enable/force RLS on tenant-owned tables.
5. Implement transaction-local `app.business_id` setup.
6. Add composite `(business_id, id)` keys and tenant-aware foreign keys.
7. Implement password hashing with Argon2id.
8. Implement invited-account/session flow:
   - login/session issuance
   - session lookup and security-event rotation
   - logout/revocation
   - current principal/business
9. Implement owner/staff capability checks.
10. Implement device enrollment and revocation basis.
11. Implement the seven-day signed offline authorization lease.
12. Add tenant-isolation integration tests using two businesses.
13. Add authentication rate limits.

### Confirmed role behavior

Staff can:

- sell
- grant credit to a named customer
- accept cash/transfer/card payment
- record ordinary expenses
- submit stock discrepancy/adjustment requests

Owner additionally:

- manages business/members/catalogue/prices
- approves/posts stock adjustments
- resolves review cases
- writes off debt
- sees full reports

### Backend acceptance criteria

- Missing business context fails closed.
- Business A cannot read/write/reference Business B.
- Revoked or expired session tokens cannot authenticate, and only token hashes are stored.
- Offline lease is bound to user, business, device, location, capabilities, and seven-day expiry.
- Revoked membership/device cannot sync once online.

### My frontend tasks

- Login and session renewal UX.
- Invitation/account setup UX once invitation delivery exists.
- Business context shell.
- Local identity cache and seven-day lease verification.
- Local PIN/unlock UX if retained after design review.
- Device and session error states.
- Offline lease expiry warnings.

### Integration gate

An owner and staff member can authenticate into the same business; a second business remains inaccessible; the installed PWA can reopen offline using a valid lease.

---

## Phase 3 — Catalogue and business setup

**Goal:** configure what the bar sells before sales are accepted.

### Your backend tasks

1. Add migrations/services for:
   - platform catalogue templates
   - business categories
   - products
   - product variants
   - product price history
   - business branding/settings fields
2. Seed the initial Nigerian product catalogue through a repeatable seed mechanism.
3. Implement template selection that copies editable data into a business catalogue.
4. Implement product/category/variant CRUD as conventional versioned state.
5. Implement price changes by closing the current price row and inserting a new row atomically.
6. Prevent hard deletion of referenced products/variants; support deactivation.
7. Implement endpoints and tenant/role checks.
8. Add tests for price history, deactivation, duplicate constraints, and cross-tenant access.

### Backend acceptance criteria

- Selecting a template creates business-owned editable records.
- Price change never alters an old price row.
- Exactly one current price exists per variant.
- Staff can read catalogue but cannot change catalogue/prices.

### My frontend tasks

- Business onboarding flow.
- Prefilled product picker.
- Product/category/variant management.
- Price management.
- Branding/settings forms.
- Offline replicated catalogue storage.
- Staff read-only catalogue experience.

### Integration gate

An owner can configure The Place from templates and custom products; staff receive the catalogue locally and can reopen it offline.

---

## Phase 4 — Sync foundation

**Goal:** establish durable local-to-server event delivery before operational modules multiply.

### Your backend tasks

1. Add migrations for:
   - client events/inbox
   - event outcomes
   - batch receipts
   - change feed
   - review cases
2. Implement event envelope parsing and schema-version dispatch.
3. Implement uniqueness for event ID and `(device_id, device_seq)` within a business.
4. Implement canonical payload hashing.
5. Implement `POST /api/v1/sync/push` with request-level and event-level outcomes.
6. Implement duplicate retry behavior and collision detection.
7. Implement explicit event dependencies.
8. Implement ordered change-feed writes in the same transaction as accepted changes.
9. Implement checkpointed `GET /api/v1/sync/pull`.
10. Implement bootstrap for new/expired checkpoints.
11. Add fault/idempotency integration tests.
12. Initially support a harmless test event or catalogue acknowledgement before financial event handlers are added.

### Backend acceptance criteria

- Lost response followed by retry creates one server event.
- Same event ID with different payload is rejected as a collision.
- Pulling/applying a page twice is safe.
- Business change feeds are isolated.
- Every event gets an explicit durable outcome.

### My frontend tasks

- Dexie database and migration framework.
- Device sequence allocation.
- Atomic local event/outbox transaction.
- Foreground sync coordinator and IndexedDB sync-owner lease.
- Push retry/backoff with jitter.
- Pull checkpoint application in one IndexedDB transaction.
- Bootstrap that preserves pending events.
- Per-event and global sync status UI.
- Fault-oriented sync tests.

### Integration gate

A locally queued test event survives reload, retries safely, receives one server outcome, and advances the local checkpoint.

---

## Phase 5 — Walk-in selling vertical slice

**Goal:** complete the most important operational path online and offline.

### Your backend tasks

1. Add migrations/services for:
   - bills
   - sales
   - sale items
   - activity events
   - initial inventory movement linkage placeholder
2. Implement `sale.recorded` for a walk-in multi-item sale.
3. Recompute line/header totals server-side.
4. Preserve description and actual unit-price snapshots.
5. Accept stale/deactivated-product offline sales as policy-defined `accepted_needs_review` rather than silently changing the sale.
6. Create bill, sale, items, activity, projections, and change-feed rows atomically.
7. Implement sale reversal as a new event; do not update/delete the original.
8. Implement sales/activity query endpoints needed by the UI.
9. Add tests for totals, historical prices, idempotency, reversal, permissions, and tenancy.

### Backend acceptance criteria

- Multi-item sale posts atomically.
- Retry does not duplicate it.
- Current price changes never alter the sale snapshot.
- Offline stale-price sale preserves the submitted actual charge and is visibly reviewed.
- Staff can sell; unauthorized users cannot.

### My frontend tasks

- Category/product quick-sell screen.
- Favorites/recent ordering.
- Cart and quantity controls.
- Cash/transfer/card selection where an immediate payment is recorded.
- Atomic IndexedDB sale save.
- Optimistic local sale/activity/dashboard projection.
- Confirmation and per-sale sync status.
- Offline reload and retry E2E tests.

### Integration gate

A staff member records a multi-item walk-in sale in a few taps while offline; it survives restart and appears once on the server after reconnecting.

---

## Phase 6 — Flexible tabs, named credit, and payments

**Goal:** support each bar’s chosen table/customer/walk-in workflow without restaurant complexity.

### Confirmed bill modes

- Walk-in: no table and no named customer required.
- Table: optional configured table, customer optional while no debt is carried.
- Customer credit: customer name mandatory whenever an outstanding balance remains.
- A bill may have multiple immutable sale rounds and multiple payments.
- Staff may grant credit.
- Payment methods: cash, transfer, card.
- No mandatory daily close.

### Your backend tasks

1. Add migrations/services for:
   - tables
   - customers
   - payments
   - bill write-offs
2. Implement bill status transitions:
   - open
   - closed_unpaid
   - settled
   - void
3. Implement adding immutable sale rounds to an open bill.
4. Enforce customer name when closing/retaining an outstanding balance.
5. Implement partial payments and authoritative outstanding balance.
6. Reject overpayment in MVP.
7. Implement payment reversal as a new event.
8. Implement owner-only debt write-off with required reason.
9. Implement outstanding bill/customer queries.
10. Add tests for concurrent payment/bill locking, status transitions, credit permission, and customer-name requirement.

### Backend acceptance criteria

- Staff can create named credit.
- Anonymous walk-in sales remain valid.
- Tables are optional, not required by the domain.
- Closing a bill does not imply payment.
- Partial payment reduces but does not erase outstanding debt.
- Only owners can write off debt.

### My frontend tasks

- Flexible mode selection optimized per business preference.
- Table grid and open tabs.
- Customer lookup/quick-create.
- Named-credit validation.
- Add-round flow.
- Partial payment and payment history.
- Outstanding money view.
- Close/settle/write-off role-aware UI.
- Offline bill/payment projection and conflict states.

### Integration gate

The same application supports table-based, named-credit, and walk-in workflows without forcing any one mode on every business.

---

## Phase 7 — Inventory, counts, approvals, and reviews

**Goal:** establish auditable expected stock while preserving offline sales.

### Your backend tasks

1. Add migrations/services for:
   - stock receipts and lines
   - stock lots
   - inventory events and movements
   - inventory balance projection
   - stock counts and lines
   - adjustment requests/approvals
   - inventory review cases
2. Post receipt, costed lot, quantity movement, activity, and feed changes atomically.
3. Add sale quantity movements without rejecting sales that make stock negative.
4. Create/update `NEGATIVE_INVENTORY` reviews.
5. Implement physical count observations with basis checkpoint/expected quantity.
6. Do not require a stock freeze.
7. Create `STALE_STOCK_COUNT` review if relevant activity occurred after the basis.
8. Allow Staff to submit adjustment requests.
9. Require Owner approval before final adjustment movement posts.
10. Require configured reason for complimentary/broken/spoiled/staff-use/manual adjustments.
11. Represent complimentary stock as a zero-charge sale/consumption event, not a hidden stock edit.
12. Preserve costed receipt lots but defer authoritative FIFO COGS allocation.
13. Add ledger/projection reconciliation tests.

### Backend acceptance criteria

- Receipt cost history is immutable.
- Two offline overselling devices preserve both sales and create a review.
- Staff cannot approve their own/final stock adjustment unless they are also an owner under policy.
- Count observations are never silently rewritten.
- Inventory balances can be rebuilt from movements.
- Profit/COGS is not falsely reported for unresolved negative quantities.

### My frontend tasks

- Current stock and provisional-state UI.
- Receive-stock flow with quantity and cost.
- Physical count flow.
- Staff adjustment-request flow.
- Owner approval/rejection flow.
- Stock history.
- Review-required screens.
- Complimentary consumption reason flow integrated with selling.
- Offline inventory warnings without blocking legitimate sale capture.

### Integration gate

Inventory expected state, physical observations, receipt costs, owner approvals, and negative-stock reviews remain consistent across multiple offline devices.

---

## Phase 8 — Expenses, dashboard, activity, and reports

**Goal:** complete the digital notebook and basic operational understanding.

### Your backend tasks

1. Add expense categories and immutable expense/reversal records.
2. Permit Staff to record ordinary expenses.
3. Define whether Staff can reverse only their recent unsynced/local mistake or whether server reversals are owner-only; default server-side reversal is owner-only.
4. Implement activity filters by actor/type/date.
5. Implement dashboard projection/query:
   - today’s sales
   - items sold
   - expenses
   - outstanding
   - alerts/reviews
   - recent activity
6. Implement reports:
   - sales by date range
   - products/quantities
   - staff sales
   - expenses by category
   - stock/discrepancies
   - outstanding bills
7. Apply business timezone boundaries.
8. Add pagination and indexes verified by query plans for history endpoints.
9. Add role/tenant/report tests.

### Backend acceptance criteria

- Staff can record an expense but cannot rewrite history.
- Owner sees complete business activity.
- Staff visibility follows the agreed capability matrix.
- “Today” uses business timezone, not server-local time.
- Reports indicate/provide enough data for provisional sync status.

### My frontend tasks

- Quick expense form and history.
- Simple dashboard with quick actions.
- Activity timeline and filters.
- Sales, staff-sales, expense, stock, and outstanding reports.
- Provisional/offline report indicators.
- Mobile and desktop owner layouts.

### Integration gate

The owner can understand what happened today and who did it, while offline data is clearly distinguished from consolidated server totals.

---

## Phase 9 — Branded bills, sharing, hardening, and pilot release

**Goal:** prepare a safe real-world pilot rather than only a development demo.

### Your backend tasks

1. Implement business branding persistence and secure logo upload/object storage.
2. Implement revocable, unguessable public bill links.
3. Implement branded bill/receipt HTML data endpoint/view.
4. Add transactional email only if required for pilot; keep provider behind an interface.
5. Add production configuration validation.
6. Add migration release procedure.
7. Add structured operational metrics/logging.
8. Add backup and restore runbook.
9. Add health/readiness and deployment checks.
10. Complete security review:
    - tenant isolation
    - auth/session/device revocation
    - public links
    - upload validation
    - rate limits
    - secret handling
11. Run sync soak/fault tests.
12. Document known MVP limitations.

### Backend acceptance criteria

- Public link exposes only intended bill/branding data.
- Logo upload rejects unsafe/oversized data.
- Production cannot start with missing critical secrets.
- Backup restore has been exercised.
- Tenant-isolation suite passes before adding another business.

### My frontend tasks

- Business branding settings.
- Receipt/bill presentation.
- Web Share API flow with WhatsApp/email/copy fallback.
- Install guidance and update UX.
- Storage-pressure and old-pending-event warnings.
- Accessibility audit.
- Low-end Android performance work.
- Full Playwright offline/multi-device pilot scenarios.
- Pilot feedback/support affordances.

### Integration gate

The Place can complete a controlled real shift with tested offline behavior, recovery guidance, monitoring, backups, and a documented support path.

---

## Phase 10 — Post-pilot FIFO costing

**Goal:** add reliable profitability only after the stock workflow produces trustworthy inputs.

### Your backend tasks

1. Validate FIFO with pilot observations/accounting needs.
2. Add immutable sale-item lot allocations.
3. Define deterministic server allocation order.
4. Resolve pending-cost/negative quantities through reviewed events.
5. Handle exact kobo rounding across partial lot depletion.
6. Add COGS/profit projections only when allocation completeness permits them.
7. Add rebuild/reconciliation tooling and tests.

### My frontend tasks

- Inventory valuation review UX.
- Pending-cost indicators.
- Gross-profit reporting with data-quality explanations.
- Reconciliation workflows.

### Gate

No profit figure is shown as authoritative unless its underlying inventory consumption is fully and audibly costed.

---

### In progress, out of frozen order

- **Stock receiving** — a thin, deliberately partial slice of Phase 7 (receipts, lots, movements, balances; no counts/approvals/reviews) pulled forward because it doesn't depend on sync, selling, or tabs existing first, and "get the bar running with one user" needs it now. See [`PHASE_STOCK_RECEIVING.md`](PHASE_STOCK_RECEIVING.md).
- **Walk-in selling** — Phase 5, built as a single-device slice ahead of Phase 4 (sync foundation), which is deferred until invitations bring a second device into a real business. Real sales, real stock deduction, real offline persistence; no tabs/credit (Phase 6) and no multi-device sync yet. See [`PHASE_WALKIN_SELLING.md`](PHASE_WALKIN_SELLING.md).

### Proposed future phases (not part of this frozen sequence)

- **WhatsApp messaging via Zavu** — OTP delivery, an email-resend backup, bar operational updates, and WhatsApp-based staff invitations. Draft only, undecided, with an NDPA 2023 compliance assessment already run: see [`PHASE_WHATSAPP_MESSAGING.md`](PHASE_WHATSAPP_MESSAGING.md).

---

## 5. Current action — Phase 1 backend foundation

Implement **Phase 1 only**. Do not create tenant/auth/domain packages, `sqlc` output, query directories, tenant transaction helpers, or migrations yet. Establish only the server lifecycle and PostgreSQL pool.

### Required files for this phase

```text
go.mod
go.sum
Makefile
.env.example
cmd/api/main.go
internal/app/app.go
internal/app/run.go
internal/config/config.go
internal/httpapi/server.go
internal/httpapi/router.go
internal/httpapi/middleware.go
internal/httpapi/errors.go
internal/httpapi/json.go
internal/httpapi/health.go
internal/store/postgres.go
```

Tests may sit beside their target files. Do not create empty future domain packages.

### Required dependencies/tools

- Chi
- `pgx/v5` and `pgxpool`
- `github.com/google/uuid`
- `golang-migrate` CLI/process may be documented through Make targets, but the first migration starts in Phase 2
- `sqlc` configuration and generated/query directories start in Phase 2 with the first real schema query

### Required behavior

1. Parse and validate configuration from environment variables.
2. Refuse startup when production-critical configuration is missing or invalid.
3. Open a PostgreSQL pool and ping it with a bounded startup context.
4. Compose the application in `internal/app`; keep `cmd/api/main.go` as a thin entry point.
5. Configure `http.Server` with explicit read-header, read, write, idle, and shutdown timeouts.
6. Handle SIGINT/SIGTERM graceful shutdown.
7. Add middleware for request ID, panic recovery, structured request logging, and baseline security headers.
8. Implement strict JSON reading and consistent JSON/error writing from [`API_CONTRACT.md`](API_CONTRACT.md).
9. Implement:
   - `GET /api/v1/health/live`
   - `GET /api/v1/health/ready`
10. Liveness must not query PostgreSQL; readiness must verify PostgreSQL with a short timeout.
11. Avoid logging request/response bodies.
12. Pass request contexts downward; do not create background contexts inside request-driven database helpers.

### Phase 1 validation to run

```text
gofmt check
go vet ./...
go test ./...
```

Also manually demonstrate:

- liveness returns success with the database healthy
- readiness returns success with the database healthy
- readiness returns `503` when PostgreSQL is unavailable
- an unknown route returns the standard `NOT_FOUND` envelope
- a handler test proves request IDs appear in errors

### When finished

Tell me **“Phase 1 backend ready for review”** and include:

- file summary
- dependency/tool choices and versions
- environment keys implemented
- commands run and output summary
- any deliberate deviation from `API_CONTRACT.md`

I will then review and adjust the backend, reference every affected file, freeze the Phase 1 behavior, and implement the complete Phase 1 frontend foundation.
