# Phase 9 (draft) — branded bills, sharing, hardening, and pilot release

**Status:** Confirmed, ready to implement. Matches `docs/IMPLEMENTATION_PLAN.md`'s actual Phase 9 ("Branded bills, sharing, hardening, and pilot release"), narrowed and made concrete the same way Phases 7 and 8 were before implementation began. See §9 for the confirmed decisions.
**Depends on:** everything shipped so far. This phase is the first one whose backend tasks are mostly genuinely new capability (object storage, public sharing, deployment) rather than composing existing domain packages.
**Builds on:** `docs/ARCHITECTURE.md` §15 (receipts/branding/sharing), §16 (deployment/infrastructure), §17 (security), §18 (observability) — all frozen guidance that's never been implemented or even revisited since the doc was written.

---

## 1. What's already built vs. what this phase adds

| Piece | State |
|---|---|
| Business branding columns (`phone`, `address`, `receipt_wording`, `receipt_footer`, `payment_instructions`, `logo_object_key`) | ✅ Exist on `businesses` since migration `000003` (Phase 2) — but **nothing writes them**. `GET /business` reads them; there is no `PATCH /business`, no `internal/business` package. |
| Object storage (logo upload) | ❌ **Nothing at all.** No S3 client anywhere in `internal/` or `web/src/`. `logo_object_key` is a bare column with no reader/writer beyond the raw passthrough. |
| Public bill links / sharing | ❌ **Nothing at all.** Every bill route is inside the authenticated, business-scoped group. No token table, no public route. |
| Structured logging | ✅ Mostly built — `internal/httpapi/middleware.go`'s `accessLogMiddleware` already logs `request_id`, `method`, `path`, `status`, `response_bytes`, `duration` via `slog`. Gap: logs the raw path, not the chi route pattern (a real problem once paths carry IDs — `/bills/{bill_id}` vs. `/bills/01a0...` — for grouping metrics by route). |
| Health/readiness | ✅ Fully built since Phase 1 (`internal/httpapi/health.go`) — liveness never touches the database, readiness does, matches `docs/ARCHITECTURE.md` §18 exactly. |
| Production config validation | ✅ Substantial — `config.Load()` already refuses to start in production without offline signing keys, SMTP creds, Zavu creds, a `https://` public base URL, and requires `JBM_SESSION_COOKIE_SECURE=true` in staging/production. Gaps: nothing for object-storage credentials (feature doesn't exist yet), no Argon2 cost-parameter floor. |
| Rate limiting | ✅ Extensive existing coverage — login, platform login, signup (email+IP), invitations (create/lookup/accept), Google sign-in all have `auth.NewLimiter` instances in `router.go`. Gap: nothing for public bill links or exports, because neither exists yet. |
| CI | ❌ **Nothing at all.** No `.github/workflows/`, no automated `make check`/`npm run check`, no dependency scanning. Repo confirmed on GitHub (`origin` → `github.com/mananuf/justbarme`), so GitHub Actions is directly usable. |
| Session/device revocation | ✅ Backend fully built since Phase 2 (`GET /devices`, `DELETE /devices/{id}`, owner-only `devices:manage`) — ❌ **no frontend UI at all**. `Install.tsx` only ever calls `POST /devices/enroll` for the current device; nothing lists or revokes others. |
| Deployment | ❌ **Nothing built yet, but fully decided.** `docs/ARCHITECTURE.md` §16.2 recommends "a managed application platform and managed PostgreSQL" (Render/Fly.io/Railway) — the actual pilot plan is Netcup VPS hosting instead (§2). This is a real, deliberate deviation from the frozen doc, named and reasoned through in §2, the same way Platform admin/Signup/OAuth/Invitations were each a named deviation before being built. |
| Backups | ❌ Not built. §16.5's posture (daily backups, periodic export to separate storage, a tested restore) has no implementation or runbook yet. |

## 2. Deployment architecture (the deviation from `docs/ARCHITECTURE.md` §16)

**Confirmed:** Netcup VPS hosting for both staging and production — **VPS 500 G12** (2 vCores, 4GB DDR5 RAM, 128GB NVMe, ~€5.91/mo flat, no promotional-then-renewal price jump) — for each environment, in place of the managed-platform-plus-managed-Postgres topology §16.1/§16.2 sketches. Chosen over Hostinger's equivalent KVM 1 tier after a real price/spec comparison: Netcup's flat rate undercuts what Hostinger would actually charge *after* its promotional first term ends, for roughly the same RAM and more disk. Latency from Nigeria specifically wasn't independently measured before this decision — §16.2's own "measure from The Place's actual networks" standard is worth a real check once the pilot is live, even though the provider choice is made. That recommendation was written for a topology this pilot isn't using; the reasoning below is what replaces it, not a rejection of the doc's actual concerns (statelessness, backups, security) — those all still apply, just realized differently.

- **Everything on one VPS per environment, via Docker Compose**: nginx (TLS via Let's Encrypt/certbot, serves the Vite build's static output directly, reverse-proxies `/api/*` to the API container), the Go API, and PostgreSQL — three services in one `docker-compose.yml`, identical on staging and production. **Confirmed: Docker, not bare-metal/systemd** — the earlier draft hedged this on RAM, but 4GB is comfortable headroom for this modest stack, and a container-based deploy is what actually makes the confirmed CI auto-deploy-to-staging (§6) simple and reliable: build an image, push it, pull and restart on the server — rather than compiling and `scp`-ing a bare binary by hand. This also means §16.3's "multi-stage Docker build" is followed as written, not deviated from. The frontend needs no separate host anywhere — the production build is fully static assets, served by the same nginx container. This directly resolves the open question CLAUDE.md's OAuth section has flagged since it was written ("whatever eventually serves `index.html`" is now: this nginx).
- **PostgreSQL runs as its own container** in the same compose file, with `shared_buffers`/`work_mem`/`max_connections` set modestly below stock defaults — 4GB shared three ways (Postgres, API, nginx) still isn't a dedicated database host, so some tuning remains sensible even though it's no longer the tight squeeze a 1GB box would have demanded. `make migrate-up` (or a thin wrapper around it) is the controlled release step §16.3 calls for, run as an explicit deploy-pipeline step against the running Postgres container — never automatically on every app start.
- **Statelessness still holds**: sessions live in Postgres (`sessions` table), not in-process memory, so the API container itself remains stateless. One instance per environment for the pilot; nothing about this design prevents adding a second instance later if traffic ever justifies it.
- **Environments: confirmed, three tiers** — local development, staging, and production, per §16.4. **Staging is a second Netcup VPS 500 G12**, same provider and plan as production — simpler to operate (one runbook, one set of assumptions, one provider relationship) than mixing tiers for a cost difference that's negligible at this scale. Separate databases and a separate object-storage bucket/prefix (§3) for staging, per §16.4's "never reuse production customer data in staging without explicit anonymization."
- **Release shape: confirmed.** CI (§6) builds and pushes images, deploying every merge to `main` to the staging VPS automatically; production is a deliberate, separate promotion step (a manual trigger or a tagged release), never the same push driving both.
- **Backups**: nightly `pg_dump` from the Postgres container via a host cron job, pushed off-box to the same external object-storage account used for logos (§3) — never left only on the VPS's own disk, per §16.5's "periodic logical export to separate object storage/account." A quarterly restore drill (§16.5's own cadence for MVP) is a real task, not just a line in a runbook — it needs to actually be run once before Phase 9 is called done, matching the plan's own acceptance criterion ("Backup restore has been exercised").

## 3. Object storage (logo upload)

Self-hosting object storage (e.g. MinIO) on the VPS is ruled out even at 4GB — it's an always-on service competing for RAM to serve, realistically, one logo image for one pilot business, for no benefit over a managed provider that costs nothing at this scale.

**Confirmed: Cloudflare R2.** R2 and Backblaze B2 are both free at this scale (a handful of business logos) and both genuinely S3-compatible, so the code is identical either way — the deciding factor is egress, not storage cost. R2 has zero egress fees with no cap; B2's free egress is capped at roughly 3x whatever's currently stored, then bills per GB past that. Public bill links (§4) mean logo images get fetched by strangers opening shared receipts — an unpredictable, potentially bursty read pattern that R2's uncapped-free-egress model handles without a second thought, where B2 would need watching.

- **New `internal/storage` package**, matching this codebase's established provider-behind-interface pattern (`internal/email`, `internal/whatsapp`): an `ObjectStore` interface (`Put`, `Get`/public-URL, `Delete`), an `S3Provider` (AWS SDK v2's S3 client pointed at whichever endpoint is configured — R2 and B2 both accept the standard S3 client with a custom endpoint), and a dev-only `LocalDiskProvider` mirroring `ConsoleProvider`'s "don't require real credentials to run locally" convenience.
- **Upload flow**: a new `POST /business/logo` (owner-only, likely reusing `business:update`) validates MIME type (PNG/JPEG/WebP only), **decodes and re-encodes the image server-side** rather than trusting the uploaded bytes directly (per §15/§17's explicit "validate and re-encode" requirement — this also strips EXIF and any embedded payload), enforces a max size (e.g. 2MB), generates an opaque UUID-based object key (never derived from user input), uploads it, and updates `businesses.logo_object_key` in the same transaction pattern every other mutation in this codebase uses.
- **Serving**: a bar's logo isn't sensitive data (it's shown on public bill links anyway, §4), so recommend a public-read bucket with a direct CDN URL rather than proxying image bytes through the VPS on every request — that keeps the VPS's resources for the API and database, not image serving.

## 4. Public bill links (branded sharing)

Implements `docs/ARCHITECTURE.md` §15's requirements exactly: unguessable signed token, revocable, exposes only bill + branding (never internal customer IDs or activity), configurable expiry.

- **New `bill_share_links` table**: `business_id`, `bill_id`, `token_hash` (never the raw token — same `internal/auth.HashToken` pattern sessions and CSRF tokens already use), `created_by`, `created_at`, `expires_at`, `revoked_at`.
- **New endpoints**: `POST /bills/{id}/share-link` (owner-only, creates or rotates a link, returns the raw token exactly once — same one-time-reveal convention `GET /me`'s CSRF rotation already uses), `POST /bills/{id}/share-link/revoke`, and a fully public, unauthenticated `GET /public/bills/{token}` returning the branded bill view (items, total, business branding/payment instructions) — explicitly excluding customer names, staff/seller names, or table labels, matching §15's "avoid exposing internal customer IDs or activity."
- **Needs its own rate limiter** (§17 names "public bill links" explicitly) — an IP-based limiter on the public lookup route, same shape as `invitationLookupIPLimiter`'s existing precedent, since this is a genuinely unauthenticated surface next to a guessable-adjacent token.
- **Frontend**: a new public route (e.g. `/bill/:token`, no `RequireAuth`) rendering the branded receipt; `Tabs.tsx`'s bill detail gets a "Share" action that creates/fetches the link and invokes the Web Share API with WhatsApp/email/copy-link fallbacks, per the plan's own frontend task list.

## 5. Settings (branding + catalogue)

- **New `PATCH /business`** (a new, small `internal/business` package — Phase 3's own deferred note already anticipated this exact gap) for `phone`/`address`/`receipt_wording`/`receipt_footer`/`payment_instructions`, separate from the logo-upload endpoint in §3.
- **Confirmed: also closes Phase 3's long-standing catalogue-editing gap.** CLAUDE.md's Phase 3 section has flagged since it was written that "no standalone Settings page for managing the catalogue exists yet" (renaming a product, deactivating something, editing a price outside the stock-receiving flow). Rather than build a narrow branding-only page now and a second catalogue-editing page later, this phase's new Settings page (`/dashboard/settings` or similar) covers both — branding fields + logo, and product/category/variant editing using the already-existing `catalogue:manage` capability and `internal/catalogue` service methods (`UpdateProduct`, `UpdateCategory`, `UpdateVariant`, `SetVariantPrice` are all already built; this is a new frontend surface for them, not new backend work).

## 6. Production hardening

- **Logging**: add the chi route pattern (not the raw path) to `accessLogMiddleware`'s fields, so metrics group correctly by route instead of fragmenting per distinct ID in the URL.
- **Metrics: confirmed, included.** §18 wants request/error latency by route, DB pool usage, login failures, open review counts, pending-event age. A full Prometheus/Grafana *stack* (a separate always-on TSDB plus dashboard server) stays out of scope (§8) — too much operational overhead for what a pilot needs — but a lightweight `/metrics` endpoint on the API itself, in Prometheus text format, costs almost nothing to expose and gives a real, scrapable signal. Recommend a free external scraper/dashboard (Grafana Cloud's free tier accepts remote-write from a simple exporter, or even just periodic polling into a free hosted Grafana) rather than running any collector on the VPS itself.
- **CI: confirmed.** A GitHub Actions workflow running `make check` and `npm run check` on every push/PR, then deploying to staging automatically on every push to the `staging` branch (per §2's confirmed release shape). `main` is reserved for production-ready code — pushes there run the same checks but never build or deploy anything; promoting to production is always the separate, manual/tagged `deploy-production.yml` step, applied to an image tag that already ran on staging. Costs nothing at this repo's size and catches regressions before they reach either VPS. Dependency scanning via `govulncheck`/`npm audit` (or GitHub's own Dependabot) substitutes for §17's "container scanning," since Phase 9 deliberately has no container to scan.
- **Security review** (§17's checklist, applied concretely): tenant isolation — already an ongoing test discipline (`internal/store/tenant_isolation_test.go` and friends); auth/session/device revocation — backend already correct, frontend UI is this phase's own task (§7); public links and upload validation — new, built in §3/§4; rate limits — extend to the two new public surfaces; secret handling — already environment-only, unchanged.
- **Migration release procedure**: a documented, explicit step in the deploy pipeline (not implicit, not run automatically by the API container on startup) — back up, `make migrate-up` against the target environment's Postgres container, then deploy/restart the API, then verify readiness. The same procedure whether CI runs it against staging automatically or a human runs it by hand for a production promotion.

## 7. Frontend tasks

- Branding settings page (§5) and the public bill/receipt view (§4) — both new.
- Web Share API flow with WhatsApp/email/copy fallback (§4) — new.
- **Device/session revocation UI** — the backend (`GET /devices`, `DELETE /devices/{id}`) has existed since Phase 2 with no frontend ever calling it; this phase finally builds that screen.
- Install guidance/update UX — `Install.tsx` already exists for enrollment; `vite.config.ts`'s `registerType: 'prompt'` means the app is expected to prompt the user when a new service-worker version is waiting, which needs verifying (or building) as part of this phase, not assumed already wired up.
- Storage-pressure and old-pending-event warnings — new: warn when IndexedDB has sales/expenses/counts/adjustments that have sat unsynced too long, matching §16.5's "the UI should warn when transactions remain pending too long" (offline records are explicitly outside server backup coverage until they sync).
- Accessibility audit, low-end Android performance work, full Playwright offline/multi-device pilot scenarios, pilot feedback/support affordances — all new, all as scoped in the frozen plan.

## 8. Explicitly out of scope

- Multi-instance/horizontal scaling — one VPS, one instance, per environment, for the pilot.
- A full Prometheus/Grafana observability stack (a separate always-on TSDB plus dashboard server) — a lightweight `/metrics` endpoint on the API is in scope (§6), running a collector is not.
- Multi-region or HA database — a single Postgres container per environment.
- Server-side PDF generation — deferred per §15 itself, if the browser print/share view proves reliable during the pilot.
- A full third-party security penetration test — §17's "security review" is a checklist pass by the people building this, not an external pentest engagement.
- A real Nigeria-latency measurement before choosing Netcup — the provider decision was made on price/spec grounds (§2); measuring actual latency is worth doing once the pilot is live, not a blocker to starting.

## 9. Confirmed decisions (for the record)

1. Netcup VPS 500 G12 (4GB RAM, 2 vCores, 128GB NVMe) for both staging and production, Docker Compose (nginx + API + Postgres) on each — §2.
2. Cloudflare R2 for object storage, used by both environments with separate buckets/prefixes — §3.
3. Staging + production are both real deployments; no environment is skipped — §2.
4. Metrics are in scope this phase: a lightweight `/metrics` endpoint, no separate collector stack — §6.
5. The new Settings page covers both branding and catalogue editing — §5.
6. CI auto-deploys every merge to `main` to staging; production is a separate manual/tagged promotion — §2, §6.
