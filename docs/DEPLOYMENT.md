# Deployment runbook

Confirmed architecture: `docs/PHASE_PILOT_RELEASE.md` §2. Read that first —
this doc is the concrete "how," not a second design discussion. Everything
here applies identically to staging and production; the only difference
between them is which VPS you're on and whether a deploy is automatic (CI,
staging, on every push to the `staging` branch) or a deliberate
manual/tagged promotion (production, `main` is reserved for
production-ready code and never auto-deploys anything itself).

## 1. One-time VPS setup

Each environment is its own Netcup VPS 500 G12, provisioned and reached
the same way:

1. Install Docker Engine + the Docker Compose plugin (Netcup's default
   Ubuntu image doesn't ship these).
2. Install the `migrate` CLI (github.com/golang-migrate/migrate) on the
   VPS host itself — not in a container. It's what `make migrate-up` shells
   out to (see CLAUDE.md's Commands section: "not a Go dependency").
3. Clone this repo onto the VPS (or `rsync` a release tarball — either
   way, the VPS needs `docker-compose.yml`, `docker-compose.prod.yml`,
   `migrations/`, and the `Makefile`). **On the staging VPS specifically,
   check out the `staging` branch** (`git checkout staging`) — CI's
   `deploy-staging` job just runs `git pull` against whatever branch is
   already checked out, it never switches branches for you. Production's
   checkout stays on `main`.
4. Create the VPS's own root `.env` (never committed — see
   `.env.example`), with two things worth calling out because they're
   easy to get backwards:
   - `JBM_DATABASE_URL` in this file must point at
     `127.0.0.1:5432` (the loopback port `docker-compose.yml` publishes),
     because this file is read by *host*-side tooling (`make migrate-up`,
     `scripts/backup.sh` indirectly via `docker compose exec`) — never by
     a container. The `api` service's own `JBM_DATABASE_URL` is set
     separately, to `postgres:5432`, directly in `docker-compose.yml`'s
     `environment:` block, because that one's read *inside* the compose
     network. Same variable name, two different values, two different
     places — mixing them up produces `dial tcp: lookup postgres` from
     the host or a refused connection from the container.
   - `JBM_ENV=production` and `JBM_SESSION_COOKIE_SECURE=true` for a real
     deploy — `config.Load()` already refuses to start in production
     without every other required secret (SMTP, Zavu, object storage,
     offline signing keys) set correctly; let it fail closed rather than
     loosening any of those checks.
5. Point the domain's DNS at the VPS before doing anything TLS-related —
   the certificate bootstrap below needs the domain to actually resolve
   there for Let's Encrypt's HTTP-01 challenge to succeed.

## 2. First-ever certificate

`web/Dockerfile`'s `ensure-self-signed-cert.sh` means `docker compose up`
succeeds even before a real certificate exists (nginx gets a throwaway
self-signed one instead of crash-looping) — but browsers will show a
certificate warning until you run this real, one-time issuance step:

```sh
docker compose up -d --build           # brings up postgres/api/web/certbot,
                                        # web serves the self-signed placeholder

# ensure-self-signed-cert.sh (web/Dockerfile) writes its throwaway cert at
# the exact path -- /etc/letsencrypt/live/<domain>/ -- certbot itself uses
# for a real one. certbot refuses to issue into a "live" directory that
# already exists (it's not one of its own lineages), so that placeholder
# has to come out first -- a real, one-time step, not paranoia:
docker compose exec -T web rm -rf \
  /etc/letsencrypt/live/your-domain.example \
  /etc/letsencrypt/archive/your-domain.example \
  /etc/letsencrypt/renewal/your-domain.example.conf

# --entrypoint certbot is required here: the certbot service's own
# entrypoint (below) is a fixed shell script that only ever runs
# `certbot renew` in a loop and never looks at extra arguments -- without
# this override, `docker compose run certbot certonly ...` silently runs
# the renew-loop instead (reporting "No renewals were attempted") and
# never actually requests anything.
docker compose run --rm --entrypoint certbot certbot certonly \
  --webroot -w /var/www/certbot \
  -d your-domain.example \
  --email you@example.com --agree-tos --non-interactive

docker compose restart web             # nginx picks up the real certificate
```

The `certbot` service's own long-running loop (`certbot renew`, checked
every 12h) only ever **renews** an existing certificate — it does not
perform this initial issuance, which is what makes this a genuinely
one-time step, not something the compose stack does for you automatically
on first boot. Once the real certificate is in place, that background loop
picks up renewing it with no further action needed -- the two commands
above are ONLY for the very first issuance on a given domain.

## 3. Migration release procedure

`docs/PHASE_PILOT_RELEASE.md` §6's own words: "a documented, explicit
step in the deploy pipeline — not implicit, not run automatically by the
API container on startup." `scripts/deploy-release.sh` is that step:
back up, `make migrate-up`, pull + restart the containers, verify
`/health/ready`. Same script whether CI runs it against staging on every
merge to `main`, or a human runs it by hand for a production promotion:

```sh
COMPOSE_PROJECT_DIR=/opt/justbarme IMAGE_TAG=v1.4.0 ./scripts/deploy-release.sh
```

It fails loudly (non-zero exit) if readiness never comes back healthy —
that's the signal to stop and look, not to assume the deploy worked.

## 4. Backups and the restore drill

`scripts/backup.sh` does the nightly `pg_dump`-and-push-to-R2 (or B2, or
any S3-compatible target) `docs/PHASE_PILOT_RELEASE.md` §2 calls for.
Cron entry on each VPS (adjust the env values to that environment's own):

```
0 2 * * * COMPOSE_PROJECT_DIR=/opt/justbarme POSTGRES_DB=justbarme POSTGRES_USER=justbarme BACKUP_S3_ENDPOINT=https://<account>.r2.cloudflarestorage.com BACKUP_S3_BUCKET=justbarme-backups-production /opt/justbarme/scripts/backup.sh >> /var/log/justbarme-backup.log 2>&1
```

Use a **separate** bucket (or at minimum a separate prefix) per
environment, and never point staging's backup at production's bucket —
same "don't cross the streams" reasoning as `docs/ARCHITECTURE.md` §16.4's
staging/production data separation.

**The restore drill is a real, required task, not just a line in this
file** — `docs/PHASE_PILOT_RELEASE.md` §2 names it as something that "needs
to actually be run once before Phase 9 is called done." Quarterly, and the
first time before calling this phase complete:

1. Download the most recent backup: `aws s3 cp s3://<bucket>/<latest>.sql.gz . --endpoint-url <endpoint>`.
2. Spin up a throwaway Postgres (a second, temporary `docker compose`
   project, or a local container) — never restore onto a live
   environment's own database as the "test."
3. `gunzip -c <file>.sql.gz | psql <throwaway-connection-string>`.
4. Confirm the schema and a handful of known rows look right, then
   discard the throwaway database.
5. Record the date and outcome somewhere durable (this file's own git
   history is fine) — the point of a drill is having evidence it was
   actually done, not just that it theoretically would work.

## 5. Required CI secrets

`.github/workflows/ci.yml`'s deploy job needs these set as GitHub Actions
repository secrets before it can do anything beyond the check/build
steps — until they exist, the checks still run and pass on every push,
but the deploy job fails at the SSH step, which is expected, not a bug to
chase:

| Secret | Purpose |
|---|---|
| `STAGING_SSH_HOST` | Staging VPS address |
| `STAGING_SSH_USER` | SSH user with Docker access on staging |
| `STAGING_SSH_KEY` | Private key for that user (public key installed on the VPS) |
| `GHCR_TOKEN` | Token with `write:packages` to push images to GHCR (or omit and use `GITHUB_TOKEN`, which already has this scope for the repo's own packages) |

Production promotion (`workflow_dispatch`, manual) needs the equivalent
`PRODUCTION_SSH_*` set — deliberately separate from staging's, so a
compromised or misconfigured staging credential can't reach production.

## 6. Security review (docs/ARCHITECTURE.md §17, applied concretely)

- **Tenant isolation**: enforced by RLS, not application code, and
  exercised by an ongoing test discipline
  (`internal/store/tenant_isolation_test.go` and the tenant-isolation
  tests each feature package carries) — nothing new needed for this
  phase, but every new tenant-owned table this phase added
  (`bill_share_links` is the one deliberate exception — see its own
  migration comment for why) followed the same `FORCE ROW LEVEL
  SECURITY` pattern.
- **Auth/session/device revocation**: the backend (`GET /devices`,
  `DELETE /devices/{id}`) has been correct since Phase 2; this phase's own
  gap was the missing frontend UI, which is a `docs/PHASE_PILOT_RELEASE.md`
  §7 frontend task, not a backend hardening item.
- **Public links and upload validation**: new this phase.
  `GET /public/bills/{token}` never exposes customer/seller/table data
  (verified live — see CLAUDE.md's Public bill links section);
  `internal/business.Service.UpdateLogo` decodes and re-encodes every
  upload rather than trusting a declared content type, caps it at 2MB.
- **Rate limits**: extended to both new public surfaces —
  `publicBillLookupIPLimiter` on `GET /public/bills/{token}`, matching
  `invitationLookupIPLimiter`'s existing shape and numbers.
- **Secret handling**: unchanged — environment-only
  (`docs/API_CONTRACT.md` §5), `config.Load()` fails closed in production
  on anything missing. The one new item, `JBM_METRICS_TOKEN`, is
  deliberately optional rather than production-required — see
  `internal/config/config.go`'s own comment on why a leaked scrape token
  is lower-stakes than the other secrets.
- **Dependency scanning**: `govulncheck` (Go) and `npm audit` (frontend)
  run in CI on every push — see `.github/workflows/ci.yml` — substituting
  for §17's "container scanning," since this phase deliberately has no
  base-image registry scanning set up (out of scope, matching
  `docs/PHASE_PILOT_RELEASE.md` §8's general "keep the pilot's
  operational surface small" posture).
