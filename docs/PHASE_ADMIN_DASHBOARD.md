# Phase: Admin dashboard

Widens `internal/platformadmin` (see CLAUDE.md's Platform admin section) from
"list businesses, suspend/reactivate, read the audit log" into an oversight
tool that stays useful once there is more than one pilot bar. The design was
worked out and confirmed in conversation before any code; the confirmed
decisions are recorded here.

## Confirmed decisions

1. **Per-business activity/usage is in scope**, and platform staff can read it
   across bars — but **aggregate only** (chosen over full line-item drill-down):
   sales totals, outstanding tabs, alert count, stock-health counts, owner/staff
   counts, last activity, five recent headlines.
2. **Only superadmin ever writes.** Support gains read capabilities only. Every
   future platform write capability defaults to superadmin-only; it is a rule,
   not a per-feature decision.
3. **No RLS bypass.** The activity summary runs each read under the normal
   tenant-isolation policy scoped to one business (`store.WithTenant`), acting
   as `uuid.Nil`. Tenant policies key on `app.business_id` only, so no policy
   change was needed.
4. **Every activity view is audited first.** If the audit row can't be written
   the request is refused.

## What shipped

- Capabilities: `platform:businesses:read_activity` and `platform:staff:read`
  (both roles); `platform:staff:manage` (superadmin).
- Endpoints: `GET /platform/businesses/{id}/activity`, `GET /platform/stats`,
  `GET/POST /platform/staff`, `POST /platform/staff/{id}/revoke`.
- Migration `000027`: nullable `platform_audit_log.target_staff_id`.
- `identity.Service.ListMembersForBusiness` (there was no per-business member
  list before; `Team.tsx` only ever listed invitations).
- Frontend: stats strip, name search, Team tab (superadmin-only create/revoke),
  expandable read-only activity row per business.

## Explicitly not built

- Line-item drill-down into a business's sales/tabs/stock.
- Any write into a business's data from the platform side.
- Subscription/plan management and ML/AI insights (later phases).

## Verified

Backend tests (`internal/platformadmin`), frontend tests
(`PlatformDashboard.test.tsx`), and a live pass against a local API: two
businesses, sales in only one — the summary for each showed only its own data;
support got `403` on staff creation and `200` on reads; a revoked staff
member's existing session and login both returned `401` immediately; every
view and staff action appeared in the audit log.
