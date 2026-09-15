# justbarme API and Backend Foundation Contract

**Status:** Phase 0 frozen contract  
**Date:** 2026-09-12  
**Applies to:** Backend Phases 1–2; later phases extend this contract

---

## 1. Foundation decisions

| Concern | Decision |
|---|---|
| Go module | `github.com/mananuf/justbarme` |
| Minimum Go | 1.22 |
| HTTP | `net/http` with `github.com/go-chi/chi/v5` |
| Database | PostgreSQL through `github.com/jackc/pgx/v5/pgxpool` |
| Static SQL | `sqlc` |
| Dynamic SQL | Direct `pgx` only when query shape is genuinely dynamic |
| Migrations | `golang-migrate` |
| IDs | UUIDv7 through `github.com/google/uuid` |
| Pilot identity | Email/password |
| Online session | Opaque server-side session cookie |
| Offline authorization | Signed seven-day device lease |
| Tenant selector | `X-Business-ID` on tenant-scoped routes |
| JSON naming | `snake_case` |
| Money | Integer kobo |
| Quantities | Decimal strings |
| Time | RFC 3339 UTC over API; `timestamptz` in PostgreSQL |

`github.com/google/uuid` IDs must be actual UUIDv7 strings, for example:

```text
01993f62-8b9d-7a21-9d2c-6b8e2dc4d632
```

Do not use presentation prefixes such as `usr_` in persisted IDs. The entity type is already known from the field/resource. UI may display shortened references separately.

---

## 2. Response envelope

HTTP status is authoritative. Do not duplicate it with a JSON `status: "success"` or `status: "error"` field, because the values can drift.

### Single-resource success

```json
{
  "data": {
    "id": "01993f62-8b9d-7a21-9d2c-6b8e2dc4d632",
    "name": "Jane Doe",
    "email": "jane@example.com"
  }
}
```

### Collection success

```json
{
  "data": [
    {
      "id": "01993f62-8b9d-7a21-9d2c-6b8e2dc4d632",
      "name": "Jane Doe"
    }
  ],
  "meta": {
    "next_cursor": "opaque-token",
    "has_more": true
  }
}
```

Page/limit metadata is acceptable for small settings/admin lists:

```json
{
  "data": [],
  "meta": {
    "page": 1,
    "limit": 20,
    "total_records": 142
  }
}
```

Do not run an expensive `COUNT(*)` merely to provide `total_records` on large history, activity, sales, or sync feeds. Those use cursor/checkpoint pagination.

### No-content success

Use HTTP `204 No Content` with no JSON body where there is genuinely nothing to return.

### Error

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "Invalid input provided.",
    "request_id": "01993f71-47ac-7dc3-a1df-71c998a0a6d8",
    "details": {
      "email": "must be a valid email address",
      "password": "must be at least 8 characters long"
    }
  }
}
```

`details` is an object and may be empty. Validation keys use request-field paths such as `items.0.quantity`.

### Initial error codes

| HTTP | Code | Use |
|---:|---|---|
| 400 | `BAD_REQUEST` | Malformed JSON, invalid query syntax |
| 401 | `AUTHENTICATION_REQUIRED` | No valid online session |
| 403 | `PERMISSION_DENIED` | Authenticated but capability missing |
| 404 | `NOT_FOUND` | Resource/route absent or hidden across tenant boundary |
| 405 | `METHOD_NOT_ALLOWED` | Known route, unsupported method |
| 409 | `CONFLICT` | Version/state conflict |
| 413 | `REQUEST_TOO_LARGE` | Body exceeds route limit |
| 422 | `VALIDATION_FAILED` | Structurally valid request violates field/domain validation |
| 429 | `RATE_LIMITED` | Request limit exceeded |
| 500 | `INTERNAL_ERROR` | Unexpected server failure |
| 503 | `DEPENDENCY_UNAVAILABLE` | Readiness dependency unavailable |

Never expose SQL, stack traces, session tokens, or internal panic text in a response.

---

## 3. Request conventions

### Headers

```text
Content-Type: application/json
Accept: application/json
X-Business-ID: <UUIDv7>       tenant routes only
X-CSRF-Token: <session token> unsafe cookie-authenticated routes
X-Request-ID: <optional client request ID>
```

The server accepts a syntactically valid incoming request ID or creates one. It returns:

```text
X-Request-ID: <request ID>
```

The error body repeats that ID.

### Tenant context

`X-Business-ID` is required on tenant-owned routes.

The authentication/business middleware must:

1. parse the ID
2. load an active membership for the authenticated user
3. attach a typed principal/business context to the request
4. start or provide a transaction that sets transaction-local `app.business_id`
5. fail closed if any step is missing

Never trust a `business_id`, `user_id`, or actor field in a request body when it can be derived from the authenticated context.

### JSON parser

- Apply a per-route body-size limit.
- Reject unknown fields.
- Reject multiple JSON values.
- Distinguish malformed JSON from validation failure.
- Require `Content-Type: application/json` on JSON-body routes.

---

## 4. Online session contract

### Token

Generate at least 32 cryptographically random bytes. Encode with unpadded base64url. Store only SHA-256 token hash in PostgreSQL.

Session record includes:

- ID
- token hash
- user ID
- CSRF token hash/value binding
- created time
- last-used time
- absolute expiry
- revoked time/reason
- optional device/user-agent metadata for account display

### Cookie

Production cookie:

```text
Name: __Host-jbm_session
Secure: true
HttpOnly: true
SameSite: Lax
Path: /
Domain: omitted
Max-Age: 30 days
```

Development may use `jbm_session` with `Secure=false` only when HTTPS localhost is unavailable. Production configuration must reject insecure cookies.

### CSRF

Every unsafe cookie-authenticated request (`POST`, `PUT`, `PATCH`, `DELETE`) must pass:

- same-origin `Origin` validation, and
- a session-bound `X-CSRF-Token` value.

Login returns the CSRF token in the response. `GET /api/v1/me` may return a fresh/current CSRF token so a reloaded frontend can resume safely. This token is not an authentication credential by itself.

### Expiry and renewal

- Online session absolute lifetime: 30 days.
- Revoke on logout, password reset, account suspension, or security event.
- Rotate after password/identity changes and other authentication-sensitive events.
- Do not create a JWT refresh/access pair for the MVP.
- The seven-day signed offline authorization lease is separate from the cookie session.

---

## 5. Configuration contract

Environment variables use the `JBM_` prefix.

### Phase 1 required configuration

| Key | Example/default | Notes |
|---|---|---|
| `JBM_ENV` | `development` | `development`, `test`, `staging`, `production` |
| `JBM_HTTP_ADDR` | `:8080` | Listen address |
| `JBM_HTTP_READ_HEADER_TIMEOUT` | `5s` | Must be positive |
| `JBM_HTTP_READ_TIMEOUT` | `10s` | Must be positive |
| `JBM_HTTP_WRITE_TIMEOUT` | `15s` | Sync/export routes may later need route-aware policy |
| `JBM_HTTP_IDLE_TIMEOUT` | `60s` | Must be positive |
| `JBM_HTTP_SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown budget |
| `JBM_DATABASE_URL` | none | Required; never committed |
| `JBM_DB_MAX_CONNS` | `10` | Tune against provider limits |
| `JBM_DB_MIN_CONNS` | `1` | May be `0` in test |
| `JBM_DB_MAX_CONN_LIFETIME` | `30m` | Pool setting |
| `JBM_DB_MAX_CONN_IDLE_TIME` | `5m` | Pool setting |
| `JBM_DB_HEALTH_TIMEOUT` | `2s` | Startup/readiness ping |
| `JBM_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

Application version should normally be injected at build time and exposed in health responses. It does not need to be a secret environment variable.

### Phase 2 configuration

| Key | Notes |
|---|---|
| `JBM_SESSION_TTL` | Default `720h` (30 days) |
| `JBM_SESSION_COOKIE_SECURE` | Must be true in staging/production |
| `JBM_ARGON2_MEMORY_KIB` | Versioned password parameters |
| `JBM_ARGON2_ITERATIONS` | Versioned password parameters |
| `JBM_ARGON2_PARALLELISM` | Versioned password parameters |
| `JBM_OFFLINE_LEASE_TTL` | Fixed/default `168h` (seven days) |
| `JBM_OFFLINE_SIGNING_PRIVATE_KEY` | Base64-encoded Ed25519 private key; secret |
| `JBM_OFFLINE_SIGNING_PUBLIC_KEY` | Base64-encoded Ed25519 public key |
| `JBM_PUBLIC_BASE_URL` | Canonical HTTPS application URL |

Do not put live SMTP, database, cookie, or signing credentials in source, Makefiles, committed `.env` files, or command defaults.

`.env.example` contains names and safe placeholders only.

---

## 6. Authorization capabilities

Capabilities are stable string constants mapped from the two MVP roles in Go. Do not add a configurable permission-builder UI or generic permissions schema yet.

### Capability constants

```text
business:read
business:update
members:read
members:manage
devices:manage
catalogue:read
catalogue:manage
sales:record
sales:reverse
bills:read
bills:manage
bills:share
credit:grant
payments:record
payments:reverse
expenses:record
expenses:reverse
inventory:read
inventory:receive
inventory:count
inventory:adjustment_request
inventory:adjustment_approve
activity:read
reviews:read
reviews:resolve
reports:read
bills:write_off
```

### Staff mapping

```text
business:read
members:read
catalogue:read
sales:record
bills:read
bills:manage
bills:share
credit:grant
payments:record
expenses:record
inventory:read
inventory:count
inventory:adjustment_request
```

Whether Staff receive `inventory:receive` remains a pilot observation. Default to Owner-only until confirmed.

Staff activity visibility can be filtered by service policy even if an activity read capability is later granted. Do not confuse “has endpoint access” with “may see every business event.”

### Owner mapping

Owners receive every listed MVP capability explicitly. Avoid a wildcard in signed offline leases; only include the offline-safe subset.

### Offline-safe subset

Potential lease capabilities:

```text
catalogue:read
sales:record
bills:read
bills:manage
bills:share
credit:grant
payments:record
expenses:record
inventory:read
inventory:count
inventory:adjustment_request
```

Owner-only approval, reversal, write-off, member, device, catalogue mutation, and review resolution require online server authorization.

---

## 7. Phase 1 endpoint contract

### `GET /api/v1/health/live`

No authentication or database query.

HTTP `200`:

```json
{
  "data": {
    "status": "available",
    "version": "dev"
  }
}
```

### `GET /api/v1/health/ready`

Checks PostgreSQL with the configured health timeout.

HTTP `200`:

```json
{
  "data": {
    "status": "ready",
    "version": "dev"
  }
}
```

HTTP `503`:

```json
{
  "error": {
    "code": "DEPENDENCY_UNAVAILABLE",
    "message": "The service is not ready.",
    "request_id": "01993f71-47ac-7dc3-a1df-71c998a0a6d8",
    "details": {}
  }
}
```

Do not include database host, error, or credentials in the response. Log the internal cause with request ID.

---

## 8. Phase 2 endpoint list

These contracts may gain fields during Phase 2 review, but route semantics are fixed.

### Global/authentication routes

```text
POST /api/v1/auth/login
POST /api/v1/auth/logout
GET  /api/v1/me
POST /api/v1/businesses
POST /api/v1/invitations/{token}/accept
```

### Tenant routes requiring `X-Business-ID`

```text
GET   /api/v1/business
PATCH /api/v1/business
GET   /api/v1/members
POST  /api/v1/invitations
PATCH /api/v1/members/{user_id}
GET   /api/v1/devices
POST  /api/v1/devices/enroll
DELETE /api/v1/devices/{device_id}
```

### Login request

```json
{
  "email": "jane@example.com",
  "password": "correct horse battery staple"
}
```

### Login response

Sets the session cookie.

```json
{
  "data": {
    "user": {
      "id": "01993f62-8b9d-7a21-9d2c-6b8e2dc4d632",
      "name": "Jane Doe",
      "email": "jane@example.com"
    },
    "memberships": [
      {
        "business_id": "01993f8b-f814-73c8-aa41-1fa86026743f",
        "business_name": "The Place",
        "role": "staff"
      }
    ],
    "csrf_token": "opaque-session-bound-value"
  }
}
```

Use one generic invalid-credentials response. Do not reveal whether an email exists.

### `GET /api/v1/me`

Returns current user, active memberships, and current CSRF token. It is global and does not require `X-Business-ID`.

### Business creation

`POST /api/v1/businesses` creates the business, owner membership, and default location atomically. It returns the created business and membership. The authenticated creator becomes Owner.

### Device enrollment

Requires an online session, tenant header, active membership, and device public key. Returns the enrolled device and signed seven-day offline lease. The exact lease serialization is frozen during Phase 2 implementation review.

---

## 9. SQL and `sqlc` boundary

Use `sqlc` for:

- fixed inserts/updates/selects
- row locks with known shape
- membership/session lookups
- ordinary lists with fixed filters

Use direct `pgx` deliberately for:

- reports whose selected filters construct optional predicates
- carefully whitelisted dynamic sorting
- later aggregate queries whose shape cannot be expressed cleanly as fixed named SQL

Do not build SQL by concatenating untrusted strings. Dynamic values remain parameters; dynamic identifiers/order clauses use explicit allowlists.

Generated `sqlc` code is infrastructure, not the domain model. Domain services should not expose generated database structs directly in API responses.

Every tenant-owned query must run through a tenant-scoped transaction/helper so RLS context cannot be forgotten.

---

## 10. Phase 1 non-goals

Do not implement these in Phase 1:

- users or business tables
- authentication/session logic
- RLS policies
- capability middleware
- device enrollment
- sync event tables
- product/sale/inventory modules
- frontend code
- speculative empty domain packages

Phase 1 establishes a safe executable foundation. Phase 2 establishes identity and tenancy before business modules begin.
