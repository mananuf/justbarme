# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

justbarme is an offline-first PWA for small Nigerian bars ("The Place" is the pilot customer) to record sales, tabs, payments, inventory, and expenses without depending on a live connection. It is a Go modular monolith backed by PostgreSQL, with a separate React/TypeScript/Vite PWA frontend under `web/`.

Full design decisions live in `docs/`:
- `docs/ARCHITECTURE.md` — the frozen technical architecture (domain model, sync protocol, tenancy, costing, security). Read this before designing any new backend feature.
- `docs/API_CONTRACT.md` — the frozen HTTP/API conventions (envelopes, error codes, config keys, capability names).
- `docs/IMPLEMENTATION_PLAN.md` — the phase-by-phase build plan and the working agreement between the backend owner (this repo's primary author) and the frontend owner (an AI coding agent working in `web/`).

**Current state:** only Phase 1 (backend server lifecycle + health checks) is implemented. There is no auth, tenancy, database schema, or domain module yet — do not assume `users`/`business`/`sale` etc. exist until you check `internal/` and `migrations/` for them.

## Commands

### Backend (Go, run from repo root)

```
make run          # go run ./cmd/api
make test         # go test ./...
make test-race    # go test -race ./...
make vet          # go vet ./...
make fmt          # gofmt -w on all .go files
make fmt-check    # fails if any file needs gofmt
make benchmark    # go test -run '^$' -bench . -benchmem ./...
make check        # fmt-check + vet + test (run this before calling backend work done)
make tidy         # go mod tidy
```

Single test: `go test ./internal/httpapi/... -run TestName -v`

Requires `JBM_DATABASE_URL` (see `.env.example`) pointing at a real PostgreSQL instance — there is no SQLite fallback, by design (RLS, constraints, and numeric behavior are architectural and must be tested against real PostgreSQL).

### Frontend (`web/`, npm)

```
npm run dev            # vite dev server (proxies /api to localhost:8080)
npm run build           # tsc -b && vite build
npm run lint             # eslint . --max-warnings 0
npm run format:check     # prettier --check .
npm run test             # vitest run
npm run test:watch
npm run test:coverage
npm run test:e2e         # playwright test (builds+serves via preview, needs backend running for real API calls)
npm run check            # format:check + lint + test + build — run before calling frontend work done
```

## Backend architecture

Composition flows `cmd/api/main.go` (thin entry point, signal handling) → `internal/app.Run` (loads config, opens the PostgreSQL pool, builds the HTTP handler, runs the server with graceful shutdown) → `internal/httpapi.NewHandler` (wires middleware and routes) → `internal/httpapi.NewServer` (the `net/http.Server` with explicit timeouts).

Key conventions, enforced project-wide:

- **`internal/app` is the composition root.** `cmd/api/main.go` must stay minimal — no business logic, no dependency wiring there.
- **Config is environment-only**, prefixed `JBM_`, parsed and validated in `internal/config`. Never hardcode secrets or read config from anywhere else. `config.Load()` fails closed on invalid/missing values — extend `load()` and its `problems` accumulation pattern rather than validating ad hoc elsewhere.
- **HTTP layer uses Chi** (`internal/httpapi/router.go` is the single place routes are registered). Handlers must not contain SQL — they parse/validate transport data and call service-layer code (once feature packages exist under `internal/`).
- **JSON envelope is fixed** (`internal/httpapi/json.go`, `errors.go`): success responses are `{"data": ...}` (optionally `"meta"`), errors are `{"error": {"code", "message", "request_id", "details"}}`. HTTP status is authoritative — never add a redundant `status` field to the JSON body. Use `writeJSON`/`readJSON`/`api.errorResponse` rather than encoding JSON by hand; `readJSON` already enforces `Content-Type: application/json`, unknown-field rejection, single-JSON-value, and body size limits.
- **Every response includes `X-Request-ID`**, sourced from `RequestID(ctx)`; get/propagate it via the existing middleware rather than reinventing request tracing.
- **Liveness never touches the database; readiness always does** (`internal/httpapi/health.go`). Preserve that split when adding new health-adjacent behavior.
- **Money is integer minor units (kobo), never floats.** Quantities are `numeric`/decimal strings, never JSON floats. This applies to both Go and TypeScript.
- **IDs are UUIDv7** via `github.com/google/uuid` (`uuid.NewV7()`), not v4, and never carry presentation prefixes like `usr_`.
- **Timestamps are UTC** (`timestamptz` in PostgreSQL, RFC 3339 in JSON).
- When the domain layer exists: posted financial/inventory records (sales, payments, receipts, expenses) are immutable — corrections are new reversal/replacement events, never `UPDATE`/`DELETE` of history. Tenant-owned tables always carry `business_id` and are accessed through a transaction with `app.business_id` set for RLS; never trust a `business_id` from a request body.
- `internal/validator` is the shared hand-rolled validation helper (`Validator.Check`/`AddError`/`PermittedValue`) — prefer extending it over pulling in a validation library, per `docs/ARCHITECTURE.md`.
- Use `sqlc` for static queries once schema work starts; use direct `pgx` only for genuinely dynamic query shapes (e.g., optional report filters), never by concatenating untrusted strings into SQL.

## Frontend architecture

The PWA (`web/src`) is offline-first: IndexedDB (via Dexie, once introduced) is meant to be the operational source of truth for business data, not an in-memory store or query cache. Current scaffolding is Phase 1 only — routing shell, connectivity/health hooks, error boundary; no IndexedDB/outbox/sync code exists yet.

- `src/api/client.ts` — `apiRequest<T>` wraps `fetch`, enforces `credentials: 'same-origin'`, and unwraps the `{data}`/`{error}` envelope into a typed result or a thrown `ApiError`. Route new API calls through this rather than calling `fetch` directly.
- `src/hooks/useConnectivity.ts` — thin `navigator.onLine` wrapper via `useSyncExternalStore`. This is a hint only, per the architecture doc; do not treat it as proof of real connectivity for sync decisions.
- `src/hooks/useServiceHealth.ts` — polls `/api/v1/health/ready` to distinguish `checking`/`available`/`unavailable`/`offline`.
- Vite dev server proxies `/api` to `http://localhost:8080` — run the Go backend alongside `npm run dev` for real API integration.
- PWA config (`vite-plugin-pwa` in `vite.config.ts`) explicitly denylists `/api/` from the navigation fallback and starts with no runtime caching — do not cache authenticated API responses in the service worker; persist controlled data in IndexedDB instead, per the architecture doc.
- ESLint uses flat config with `typescript-eslint` recommendedTypeChecked plus `react-hooks`/`react-refresh` — `npm run lint` requires zero warnings (`--max-warnings 0`).

## Working agreement (from `docs/IMPLEMENTATION_PLAN.md`)

Backend and frontend are treated as separately owned scopes with a phase-gated workflow: each phase has a frozen contract, backend implements it, then frontend implements against the frozen contract. When implementing a phase, stay inside that phase's listed scope — the plan explicitly calls out non-goals per phase (e.g., Phase 1 must not create auth/tenancy/domain packages or `sqlc` output). Check `docs/IMPLEMENTATION_PLAN.md` for the current phase's task list and acceptance criteria before adding new backend packages.
