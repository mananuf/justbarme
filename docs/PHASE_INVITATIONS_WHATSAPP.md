# Phase — Invitations and WhatsApp messaging (via Zavu)

**Status:** Implemented and live-verified end-to-end (backend and frontend), with one deliberate gap — see "Implementation notes" at the end. Not part of the frozen `docs/IMPLEMENTATION_PLAN.md` phase sequence — like platform admin, signup, and Google OAuth before it, this is a deviation designed in-session because the original plan never anticipated it. Building this is also the trigger the project set for revisiting Phase 4 (multi-device sync) — see §6.
**Depends on:** `internal/identity` (extended for dual phone/email identity), `internal/signup` (generalized for a WhatsApp channel), `internal/email`'s `Provider` pattern (mirrored, not reused directly).
**Supersedes:** `docs/PHASE_WHATSAPP_MESSAGING.md`'s earlier draft — narrows and finalizes it after review. Its NDPA compliance assessment carries forward unchanged (§7).
**Vendor:** Zavu — a WhatsApp Business account is already connected on Zavu's side, and its abstraction handles Meta's template-approval requirement internally, so no template-management concept belongs in justbarme's own code or config.

---

## 1. What this actually builds

Two things, tied together but separable in the code:

1. **Dual identity** — a user account can have a phone number, an email address, or both; either can be used to sign up, log in, or be the target of an invitation. `users.phone` already exists (added in migration `000002`, unused by any flow until now) — this phase is what finally uses it.
2. **Invitations** — an owner can invite a staff member by phone (default channel: WhatsApp) or email (default channel: email). `POST /invitations` has never existed — Phase 2 explicitly deferred it.

Both defaults are deliberately asymmetric, not a blanket "WhatsApp everywhere" policy:

| Flow | Default channel | Backup | Why |
|---|---|---|---|
| Self-service signup | Email | WhatsApp (explicit choice) | Higher volume, ongoing, cost-sensitive — email is free; WhatsApp business-initiated messaging isn't. |
| Staff invitation | WhatsApp | Email (explicit choice, or when no phone is given) | Lower volume (bounded by headcount), and the audience (bar staff) skews even less email-native than owners — see `docs/PHASE_WHATSAPP_MESSAGING.md` §1's adoption research. |

Neither is a hard restriction — a signup can go via WhatsApp if someone prefers it; an invitation can go via email if there's no phone. It's which option is presented/selected first, not which channel is technically possible.

## 2. `internal/whatsapp` — the pluggable provider

Mirrors `internal/email`'s exact shape (`Provider` interface → `ZavuProvider`/`ConsoleProvider` → `WithRetry`), one method, freeform text — Zavu's own abstraction removes the template-management complexity the earlier draft assumed existed on justbarme's side:

```go
package whatsapp

type Message struct {
    To   string // E.164 phone number
    Text string
}

// Provider sends one WhatsApp message. Implementations must not log Text
// (may carry an OTP code or invite link) -- only that a send was
// attempted and its outcome.
type Provider interface {
    Send(ctx context.Context, msg Message) (messageID string, err error)
}
```

- **`ZavuProvider`** talks to Zavu's REST API directly over `net/http` (`POST https://api.zavu.dev/v1/messages`, `Authorization: Bearer <key>`, `Zavu-Sender: <sender_id>`) rather than the `github.com/zavudev/zavu-go` module the original sketch assumed — that module does not actually resolve (`go list -m ... @latest` returns "Repository not found"); the real contract was confirmed against Zavu's published docs instead. Swap this for an official SDK later if one appears.
- **`ConsoleProvider`** logs instead of sending — same dev-fallback reasoning as `email.ConsoleProvider` (Zavu's own sandbox restricts real sends to team-registered numbers, worse for local dev than logging).
- **`WithRetry`** — same exponential-backoff wrapper pattern as `email.WithRetry`, applied only to `ZavuProvider` (`ConsoleProvider` can't fail).
- **Config is enforced like SMTP, not like OAuth** (confirmed explicitly): `config.Load()` refuses to start in **production** without real Zavu credentials. Outside production, missing credentials fall back to `ConsoleProvider` automatically — dev convenience, not a silent gap.

### `.env`

```
JBM_ZAVU_API_KEY                # zv_live_... in prod, zv_test_... in dev
JBM_ZAVU_SENDER_ID               # the Zavu-Sender header value for the connected WhatsApp Business account
JBM_ZAVU_WEBHOOK_SECRET           # HMAC-SHA256 verification for delivery-status callbacks (if built this pass -- see §8)
JBM_ZAVU_MAX_SEND_ATTEMPTS        # optional, default 3
JBM_ZAVU_RETRY_BASE_DELAY         # optional, default 500ms
```

## 3. Schema

- **`users.email` becomes nullable** (`ALTER COLUMN email DROP NOT NULL`) — same precedent as `password_hash` going nullable for Google-only accounts (migration `000017`). The existing `CREATE UNIQUE INDEX users_email_key ON users (lower(email))` needs no change — Postgres already treats multiple `NULL`s as non-conflicting for uniqueness. Add `CHECK (email IS NOT NULL OR phone IS NOT NULL)` so no account ends up unreachable by both.
- **`signup_verifications` gains `phone` and `channel` columns** (mirroring `docs/PHASE_WHATSAPP_MESSAGING.md`'s original N.2 sketch) — one `Start`/`Verify` code path handles either channel.
- **New `invitations` table**: `business_id`, `invited_by`, `phone` (nullable), `email` (nullable, `CHECK` at least one present), `role`, a hashed token (never store the raw value, same pattern as sessions/`signup_verifications`), `status` (`pending`/`accepted`/`revoked`/`expired`), `expires_at`, `accepted_by` (nullable, filled once known). Partial unique constraints: one pending invite per `(business_id, phone)` and per `(business_id, email)`.
- **New `identity_verifications` table** — the "verify before link" mechanism, distinct from both `signup_verifications` (a brand-new account) and `invitations` (a brand-new membership): `user_id` (an *already-existing* account), `channel` (phone/email), `identifier` value, hashed OTP, `attempts`, `expires_at`, `purpose` (`add_identifier` from account settings, or `invitation_link` from the auto-link flow below). Used by both:
  - Account settings — "add a phone/email to my account."
  - Invitation auto-link — an invitee who already has an account, verifying the invite's phone/email before it's attached.

## 4. Flows

**A. Self-service signup (email default).** Unchanged from today's `internal/signup`, just one path of a channel-aware `Start`/`Verify`.

**B. Self-service signup via WhatsApp (explicit choice).** Same OTP mechanics, `channel='whatsapp'`, delivered via `internal/whatsapp` instead of `internal/email`.

**C. Adding a second identifier to an existing account (settings).** Logged-in user submits a phone or email not yet on their account → `identity_verifications` row (`purpose='add_identifier'`) → OTP sent to the *new* identifier → confirmed → attached to `users`.

**D. Owner invites someone with no existing account.** `POST /invitations` (phone → WhatsApp default, email → email default) → invitee taps the link → landing page offers "I'm new here" → sets display name + password only, **no OTP** (receiving the invite already proved channel ownership) → account created, membership attached immediately.

**E. Owner invites someone who already has an account under a different identifier.** Same invite link, invitee picks "I already have an account" → logs in with whatever they originally registered with → system finds the invite's phone/email isn't yet on their account → `identity_verifications` (`purpose='invitation_link'`) → OTP sent to the invite's phone/email → confirmed → attached to their *existing* account, membership granted there (no duplicate account created). Symmetric regardless of direction (email-registered user verifying a new phone, or the reverse).

## 5. Endpoints

Reuses existing capabilities (`members:manage`/`members:read`) — no new permission design needed:

- `POST /api/v1/invitations` (owner-only) — create + send.
- `GET /api/v1/invitations` (owner-only) — list pending/accepted/revoked.
- `POST /api/v1/invitations/{id}/revoke` (owner-only).
- `GET /api/v1/invitations/{token}` (public) — landing-page lookup (business name, role, both "new"/"existing" options).
- `POST /api/v1/invitations/{token}/accept` (public, new-account path) — creates the account + membership together.
- `POST /api/v1/invitations/{token}/link` (authenticated, existing-account path) — starts an `identity_verifications` challenge.
- `POST /api/v1/identity-verifications/{id}/confirm` (authenticated) — shared confirm endpoint for both settings-driven and invitation-driven verification.
- `POST /api/v1/me/identifiers` (authenticated) — account-settings "add a phone/email" entry point.

## 6. The Phase 4 trigger

Recorded in project memory (`phase4-sync-foundation-deferred`): Phase 4 (multi-device sync) was deferred specifically "until invitations bring a second device into a real business." This phase is that trigger. Not proposing to build Phase 4 as part of this — flagging that once a real invitation is accepted on a second device, that's the point to revisit it, not something to solve preemptively here.

## 7. NDPA 2023 compliance — carried forward, now sharper

Full assessment already run via the `ndpc` skill (2026-09-16), preserved in project memory (`ndpa_whatsapp_messaging`) and `docs/PHASE_WHATSAPP_MESSAGING.md` §5. What's new now that phone becomes a real *identity*, not just a delivery channel:

- Phone number is now authentication-security data in the same sense email already is — the DPIA that was "recommended anyway" leans firmer toward "should do," not just good practice.
- The indirect-collection privacy-notice requirement (s.27(2) — the invitee's phone/email is collected via the inviting owner, not the invitee themselves) applies to **every** invitation now, not just a WhatsApp-delivery detail.
- Still not done, still needed before real phone numbers flow: an actual read of Zavu's and Meta's DPA/Terms against the s.29(1)(a)-(e) processor-obligations checklist, and the staff-invitation lawful-basis question is still flagged for counsel, not resolved by this document.

**This is advisory output from an in-session skill, not a substitute for Nigerian counsel sign-off before this ships to real users.**

## 8. Confirmed decisions (for the record)

1. Zavu config is production-required, dev-fallback via `ConsoleProvider` — same pattern as SMTP.
2. Zavu's abstraction handles Meta template approval — no template-ID concept in justbarme's code or config.
3. Signup defaults to email; invitations default to WhatsApp — both explicitly overridable, never an automatic dual-send (cost-driven: business-initiated WhatsApp messaging isn't free, email is).
4. Dual identity: phone and email both nullable, either logs in, either can be added later via account settings.
5. Auto-link on an invitation match is **verify-then-link**, never blind — a fresh OTP to the new identifier is mandatory before any account merge, in both directions.
6. Invitee signup (brand-new account) skips OTP entirely — receiving the invite already proves channel ownership.
7. **`identity_verifications` OTP**: 3-attempt lockout (tighter than `internal/signup`'s existing 5 — this guards an account-merge, a higher-stakes action than a brand-new signup), 10-minute TTL — matching `internal/signup`'s existing `JBM_SIGNUP_OTP_TTL` default exactly, no new constant introduced. `internal/signup` itself is untouched; this is `identity_verifications`' own constant, deliberately not shared, since the two flows may need to diverge later without one accidentally changing the other.
8. **Rate limiting**, mirroring the exact shape `internal/httpapi/router.go` already uses for signup (a per-identifier limiter as the primary layer, an IP limiter as defense-in-depth — the attempt-lockout above is the real brute-force defense, same reasoning already documented for signup's verify limiters):
   - `POST /invitations` (owner creating an invite): 10/hour per business, 30/hour per IP — looser than signup-start's 3/hour-per-email since one owner may legitimately invite several staff in a burst (opening a new location, a shift changeover), but still bounded.
   - `GET /invitations/{token}` (public lookup): 30/15min per IP — same shape as `signupVerifyIPLimiter`, since an invite token is a bearer credential once issued even though it's a high-entropy hashed value, not a 6-digit code.
   - `POST /invitations/{token}/accept` and `.../link`, and `POST /identity-verifications/{id}/confirm`: 10/15min per identifier (phone/email), 30/15min per IP — identical numbers to `signupVerifyEmailLimiter`/`signupVerifyIPLimiter`.
9. **Delivery-status webhook is in scope for this pass**, not deferred — `POST /api/v1/webhooks/zavu`, unauthenticated but signature-verified (`X-Zavu-Signature`, `t=<ts>,v2=<hash>` HMAC-SHA256 over `"{t}.{body}"`, per Zavu's documented scheme). Standard practice for any real messaging integration: without it there's no visibility into whether an OTP or invite actually delivered, which matters more here than for email (Zavu's own smart-routing/fail-over behavior on delivery failure is itself something worth being able to observe, not just trust silently).

## 9. Implementation notes

- **Backend**: `internal/whatsapp` (Zavu/Console providers, retry, webhook signature verification), `internal/verification` (the `identity_verifications` "verify before link" service, shared by settings and invitation flows), `internal/invitations`, and the dual-identity changes to `internal/identity`/`internal/signup` are all built, unit-tested, and `make check`-clean. Migration `000021` adds the schema from §3. Live-verified end-to-end against a real running server: dual-identity signup and login (email and phone), invitation create → accept-as-new-user, invitation create → link-as-existing-user through the full verify-then-link OTP challenge (flow E, both directions), duplicate-invitation rejection, revoke, and webhook fail-closed behavior on a bad signature.
- **A real bug was found and fixed during that live verification**: `AcceptAsExistingUser` used to propagate `identity.ErrAlreadyMember` as a hard failure (surfaced as a raw 500) when an invitee who picks "I already have an account" turns out to already be a member of the target business (a real case once a business re-invites someone). Fixed to treat it as a fulfilled invitation instead — see `internal/invitations/service.go`'s `AcceptAsExistingUser` and the regression test `TestAcceptAsExistingUserTreatsAlreadyMemberAsFulfilled`.
- **Known, accepted scope limitation**: `users.phone` is a single column, not a set. Verifying a *new* phone through an invitation's verify-then-link flow overwrites whatever phone the account already had, rather than tracking multiple verified phones per account. Flagged during live verification, not fixed — the schema doesn't support more than one today and nothing in this phase asked for it.
- **Frontend**: `web/src/lib/session.tsx` (`login`/`startSignup`/`verifySignup` generalized to channel+identifier, new `acceptInvitation`), `web/src/api/invitations.ts` (typed client), `web/src/pages/SignUp.tsx` (email/WhatsApp channel toggle, email default), `web/src/pages/Login.tsx` (generic identifier field), `web/src/pages/InviteAccept.tsx` (public `/invite/:token` landing page implementing flows D and E in full, including the verify-then-link OTP step), and `web/src/pages/Team.tsx` (owner-facing invite/list/revoke screen at `/dashboard/team`, linked from Dashboard's "More" section) are all built. `npm run check`-equivalent (tsc, eslint --max-warnings 0, prettier, vitest) passes, and the same live server used for the backend verification above was also driven end-to-end via curl using the frontend's exact request/response shapes (not just a browser-driven pass) to confirm the two sides agree byte-for-byte.
- **Flow C ("add a phone/email to an existing account" from account settings) has no frontend UI.** The backend endpoints (`POST /me/identifiers`, `POST /identity-verifications/{id}/confirm`) exist and work — `internal/verification` doesn't distinguish where a challenge came from — but no settings screen calls them yet. Deliberately deferred, not dropped: flows D and E (invitations) were the priority since they're what actually gets a second person into a business; a self-service "add my other identifier" screen is a smaller, lower-urgency addition that can reuse `internal/verification` and `confirmIdentityVerification` from `web/src/api/invitations.ts` as-is whenever it's picked up.
- **A second real bug, found live once a real invitation was actually opened**: Zavu rejects any phone that isn't strict E.164 (no leading local `0`, no spaces) — nothing validated or normalized phone input before this, so a locally-formatted number sailed through every layer and only failed once it hit Zavu's API, surfacing as an unhandled `500` instead of a normal validation error. Fixed with `validator.IsE164Phone` (`internal/validator/validator.go`), checked in `createInvitation`, `signupStart`'s WhatsApp channel, and `startAddIdentifier` — a malformed phone now fails closed as `422 VALIDATION_FAILED`. On the frontend, `web/src/components/PhoneInput.tsx` (a styled country-code-plus-local-number picker, flags computed from ISO2, Nigeria default, backed by `web/src/lib/countryCodes.ts`) replaces the old bare `<input type="tel">` in `SignUp.tsx`'s WhatsApp channel and `Team.tsx`'s invite form, so a malformed number can no longer be typed through the UI at all.
- **A third real bug, found from the same live invitation open**: `JBM_PUBLIC_BASE_URL` was set to the API's own address (`http://localhost:4000`), so every invitation link built as `"{JBM_PUBLIC_BASE_URL}/invite/{token}"` pointed at the Go backend instead of the Vite frontend that actually serves the `/invite/:token` route — opening a real invitation link 404'd. Fixed in `.env` (now the frontend's own origin, `http://localhost:5173` locally) and `.env.example`'s comment reworded to say explicitly that this must be the frontend's origin, not this API's — see the note in CLAUDE.md's Invitations and WhatsApp messaging section.
- **Every transactional email was plain text with no styling** — a real gap noticed once an actual invitation email was opened in Gmail: one bare sentence with a raw link, easy to mistake for spam (and Gmail's spam filter agreed, in the reporting owner's own inbox). `internal/email` gained a real HTML capability: `Message.HTML` (optional), `NewHTMLTemplate`/`MustNewHTMLTemplate` (alongside the original text-only `NewTemplate`, kept for backward compatibility), and `Layout`/`MustLayout` (`internal/email/layout.go`) — a shared, on-brand HTML shell (dark header band with the wordmark, cream content card, muted footer; colors kept in sync by hand with `web/src/styles/global.css`'s `--color-jb-*` tokens, since an email client can't read a CSS file) that every feature package's own content renders inside. `SMTPProvider` now sends a real `multipart/alternative` message (plain-text part first, HTML last, per RFC 2046) whenever `Message.HTML` is set, falling back to the original plain-text-only format otherwise. All three existing templates (`internal/signup`'s and `internal/verification`'s OTP emails, `internal/invitations`' invite email) were redesigned with real content: a large letter-spaced code block for OTP, a proper pill-shaped CTA button plus a plain-text link fallback for the invitation. Verified with two real sends through the pilot's actual Gmail relay (an OTP email and an invitation email, both to Gmail-plus-tagged addresses landing in the same real inbox) — not just a local HTML render.
- **Follow-up polish after seeing the real email design and the invitation UI live**: the email shell (`internal/email/layout.go`) now carries the actual brand mark (`web/src/components/Logo.tsx`'s SVG path, copied in by hand since email has no build step to share a component through) inlined at header and footer size, not just the text wordmark; an OTP's `{{.ExpiresIn}}` is now a `humanDuration` helper's "10 minutes" rather than Go's raw `time.Duration.String()` ("10m0s"), duplicated in `internal/signup` and `internal/verification` per this doc's own duplication convention. On the frontend, `web/src/components/ChannelIcon.tsx` (`WhatsAppIcon`/`EmailIcon`) puts a small monochrome icon before the label on every WhatsApp/Email toggle (`SignUp.tsx`, `Team.tsx`), and `Team.tsx`'s email-channel view carries a one-line reminder for the inviter that invite emails can land in spam — worth telling their invitee, since that's literally what happened during this phase's own live testing.
