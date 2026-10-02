# CSRF token stability

**Status:** Built and verified (backend unit tests + full `make check`).
**Not** part of `docs/`'s frozen plan — a targeted bug fix made during a
session that validated a pasted external audit against this codebase's
actual code. Branch: `fix/csrf-and-offline-and-login`.
**Depends on:** `internal/identity` (sessions), `internal/httpapi`'s
`GET /api/v1/me` handler.

## The bug

`GET /api/v1/me` (`docs/API_CONTRACT.md` §8) rotates the session's CSRF
token on every call and returns the new raw value, specifically so a
reloaded frontend always has a usable token to send back as
`X-CSRF-Token` on its next write. Before this fix, "rotates" meant
`RotateCSRFToken` generated a **fresh random token** and overwrote the
session's stored hash on every single call.

That is a real race whenever more than one `GET /me` for the same
session lands close together:

1. Two browser tabs (or windows) sharing one session cookie both reload
   around the same time.
2. Tab A's `/me` call generates token X, stores `hash(X)`, returns X.
3. Tab B's `/me` call — already in flight, or fired moments later —
   generates token Y, stores `hash(Y)`, returns Y, **overwriting X**.
4. Tab A now holds X in memory, believing it's valid. The next write it
   attempts sends `X-CSRF-Token: X` against a session whose stored hash
   is `hash(Y)` — mismatch, `403 PERMISSION_DENIED`.

In local development this also shows up with a single tab, because
`main.tsx` wraps the app in `<StrictMode>`, which double-invokes effects
(including `SessionProvider`'s mount-time `/me` call) — two back-to-back
calls, same race, same symptom, easily mistaken for "CSRF is just broken"
rather than "two concurrent rotations stepped on each other." The
dev-only StrictMode trigger is not the real-world one; the real-world
trigger is the two-tabs case above.

## Confirmed design

Rejected approach: introduce a new server-side HMAC secret key (e.g.
`JBM_CSRF_SIGNING_KEY`) and derive the token from that plus the session
ID. This would work, but adds a new piece of operational infrastructure —
a new env var, new production provisioning, a new thing that can be
misconfigured — mirroring `internal/app.ensureOfflineSigningKeys`'s own
shape for a problem that doesn't need a new secret at all.

**Chosen approach: derive the CSRF token from the session's own existing
token hash, deterministically.**

- `deriveCSRFToken(sessionTokenHash string) string` (`internal/identity/
  service.go`) is `auth.HashToken(sessionTokenHash + ":csrf")` — a
  domain-separated hash of a secret the session already has
  (`sessionTokenHash`, `auth.GenerateToken()`'s own high-entropy output).
  This is a standard, safe way to derive more than one secret-equivalent
  value from one strong secret: unguessable to a third party (who would
  need the raw session token, not just its hash, to reproduce it), and
  deterministic — the same session always derives the same CSRF token, so
  there is no window where two concurrent derivations can disagree.
- `Session` (`internal/identity/model.go`) gained a `SessionTokenHash`
  field, populated by `toSession` (`convert.go`) from the stored row, so
  a handler that already has the session in context (via `requireAuth`)
  can derive its CSRF token without a second database round trip. Never
  the raw token — handlers must never serialize this field.
- `CreateSession` now computes `sessionTokenHash` once, derives
  `rawCSRFToken` from it via `deriveCSRFToken`, and stores
  `hash(rawCSRFToken)` — so the token login issues is already the same
  one `GET /me` will keep deriving for the rest of the session's life.
  No transition window between login and the first `/me` call.
- `RotateCSRFToken` (random generation, overwrite every call) is renamed
  `EnsureCSRFToken(ctx, sessionID, sessionTokenHash) (string, error)` —
  it re-derives the same deterministic value and persists it (a no-op
  write for any session created after this change; self-healing for an
  older session whose stored hash still reflects a pre-fix random token).
  `GET /me`'s handler (`internal/httpapi/auth_handlers.go`) was updated to
  call it with `sess.SessionTokenHash`.

## What this does and does not fix

- **Fixes**: the two-tabs-same-session race above, and its StrictMode
  dev-mode manifestation. A reload no longer has any chance of
  invalidating a token another concurrent caller just received.
- **Deliberately not touched**: `internal/platformadmin`'s parallel,
  structurally separate session/CSRF implementation (`platformMe`'s own
  rotation). Same bug shape exists there in principle, but it's a
  lower-traffic, single-operator tool where two tabs reloading the same
  platform session concurrently is not a realistic pilot-stage risk —
  scoped out deliberately, not an oversight. Revisit only if it's
  actually hit.

## Tests

`internal/identity/service_test.go`:
- `TestCSRFTokenIsStableAcrossConcurrentMeCalls` — two `EnsureCSRFToken`
  calls for the same session return the identical token, matching what
  `CreateSession` itself issued at login (no transition window).
- `TestCSRFTokenDiffersAcrossSessions` — two different sessions for the
  same user derive different tokens, and re-deriving session A's token
  still matches what login issued for it.
