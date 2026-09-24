package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/activity"
	"github.com/mananuf/justbarme/internal/platformadmin"
	"github.com/mananuf/justbarme/internal/validator"
)

type platformStatsResponse struct {
	TotalBusinesses     int   `json:"total_businesses"`
	ActiveBusinesses    int   `json:"active_businesses"`
	SuspendedBusinesses int   `json:"suspended_businesses"`
	NewBusinessesLast7d int   `json:"new_businesses_last_7d"`
	TotalUsers          int64 `json:"total_users"`
}

// getPlatformStats implements GET /api/v1/platform/stats: platform-wide
// counts from non-tenant tables only (businesses, users).
func (api *API) getPlatformStats(w http.ResponseWriter, r *http.Request) {
	if _, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesRead); !ok {
		return
	}
	stats, err := api.platformAdmin.Stats(r.Context(), time.Now())
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("platform stats: %w", err))
		return
	}
	payload := envelope{"data": platformStatsResponse{
		TotalBusinesses: stats.TotalBusinesses, ActiveBusinesses: stats.ActiveBusinesses,
		SuspendedBusinesses: stats.SuspendedBusinesses, NewBusinessesLast7d: stats.NewBusinessesLast7d,
		TotalUsers: stats.TotalUsers,
	}}
	if err := writeJSON(w, http.StatusOK, payload, nil); err != nil {
		api.logger.Error("write platform stats response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type platformActivityHeadline struct {
	Type       string `json:"type"`
	OccurredAt string `json:"occurred_at"`
	Summary    string `json:"summary"`
	AmountKobo int64  `json:"amount_kobo"`
}

type platformBusinessActivityResponse struct {
	Business platformBusinessResponse `json:"business"`

	SalesTodayKobo   int64 `json:"sales_today_kobo"`
	SalesTodayCount  int64 `json:"sales_today_count"`
	ItemsSoldToday   int64 `json:"items_sold_today"`
	Sales7dKobo      int64 `json:"sales_7d_kobo"`
	Sales7dCount     int64 `json:"sales_7d_count"`
	OutstandingKobo  int64 `json:"outstanding_kobo"`
	OutstandingBills int   `json:"outstanding_bills"`
	AlertsCount      int64 `json:"alerts_count"`

	TrackedVariants int `json:"tracked_variants"`
	OutOfStock      int `json:"out_of_stock"`
	NegativeStock   int `json:"negative_stock"`

	Owners int `json:"owners"`
	Staff  int `json:"staff"`

	LastActivityAt string                     `json:"last_activity_at,omitempty"`
	RecentActivity []platformActivityHeadline `json:"recent_activity"`
}

// getPlatformBusinessActivity implements
// GET /api/v1/platform/businesses/{business_id}/activity: an aggregate-only,
// read-only operational summary of one business (no line items, no customer
// or staff detail beyond role counts). Composed from the same self-contained
// services the owner's own dashboard uses, each running under the ordinary
// tenant-isolation RLS policy scoped to exactly this business -- there is no
// bypass role. uuid.Nil is the acting user, matching WithApp's documented
// convention for "no business user is acting"; authorization is the platform
// capability check, not membership.
//
// The audit entry is written BEFORE any tenant data is read, and a failure
// to record it refuses the request: an unauditable read must not happen.
func (api *API) getPlatformBusinessActivity(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesReadActivity)
	if !ok {
		return
	}
	businessID, err := uuid.Parse(chi.URLParam(r, "business_id"))
	if err != nil {
		api.badRequestResponse(w, r, "business_id must be a valid UUID.")
		return
	}

	biz, err := api.platformAdmin.GetBusiness(r.Context(), businessID)
	if err != nil {
		if errors.Is(err, platformadmin.ErrBusinessNotFound) {
			api.notFoundResponse(w, r)
			return
		}
		api.internalErrorResponse(w, r, fmt.Errorf("get business for activity: %w", err))
		return
	}
	if err := api.platformAdmin.RecordBusinessActivityViewed(r.Context(), staff.ID, businessID, RequestID(r.Context())); err != nil {
		api.internalErrorResponse(w, r, err)
		return
	}

	ctx := r.Context()
	loc, lerr := time.LoadLocation(biz.Timezone)
	if lerr != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	weekStart := startOfDay.AddDate(0, 0, -6)

	out := platformBusinessActivityResponse{
		Business:       toPlatformBusinessResponse(biz),
		RecentActivity: []platformActivityHeadline{},
	}
	nobody := uuid.Nil

	if out.SalesTodayKobo, out.SalesTodayCount, err = api.sales.SumSalesTotalSince(ctx, nobody, businessID, startOfDay); err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("sum sales today: %w", err))
		return
	}
	if out.ItemsSoldToday, err = api.sales.SumItemsSoldSince(ctx, nobody, businessID, startOfDay); err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("sum items sold: %w", err))
		return
	}
	if out.Sales7dKobo, out.Sales7dCount, err = api.sales.SumSalesTotalSince(ctx, nobody, businessID, weekStart); err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("sum sales 7d: %w", err))
		return
	}

	bills, err := api.sales.ListOutstandingBills(ctx, nobody, businessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list outstanding bills: %w", err))
		return
	}
	out.OutstandingBills = len(bills)
	for _, b := range bills {
		out.OutstandingKobo += b.BalanceKobo
	}

	if out.AlertsCount, err = api.countOpenAlerts(ctx, nobody, businessID); err != nil {
		api.internalErrorResponse(w, r, err)
		return
	}

	balances, err := api.inventory.GetBalances(ctx, nobody, businessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("get stock balances: %w", err))
		return
	}
	out.TrackedVariants = len(balances)
	for _, qty := range balances {
		if qty < 0 {
			out.NegativeStock++
		}
		if qty <= 0 {
			out.OutOfStock++
		}
	}

	members, err := api.identity.ListMembersForBusiness(ctx, nobody, businessID)
	if err != nil {
		api.internalErrorResponse(w, r, err)
		return
	}
	for _, m := range members {
		if m.Role == "owner" {
			out.Owners++
		} else {
			out.Staff++
		}
	}

	entries, err := api.activity.List(ctx, nobody, businessID, activity.Filter{}, 5)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list recent activity: %w", err))
		return
	}
	for i, e := range entries {
		if i == 0 {
			out.LastActivityAt = e.OccurredAt.UTC().Format(time.RFC3339)
		}
		out.RecentActivity = append(out.RecentActivity, platformActivityHeadline{
			Type: e.Type, OccurredAt: e.OccurredAt.UTC().Format(time.RFC3339),
			Summary: e.Summary, AmountKobo: e.AmountKobo,
		})
	}

	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write platform business activity response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type platformStaffListItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

func toPlatformStaffListItem(s platformadmin.Staff) platformStaffListItem {
	return platformStaffListItem{
		ID: s.ID.String(), Name: s.DisplayName, Email: s.Email, Role: s.Role, Status: s.Status,
		CreatedAt: s.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// listPlatformStaff implements GET /api/v1/platform/staff
// (platform:staff:read, both roles).
func (api *API) listPlatformStaff(w http.ResponseWriter, r *http.Request) {
	if _, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityStaffRead); !ok {
		return
	}
	staff, err := api.platformAdmin.ListStaff(r.Context())
	if err != nil {
		api.internalErrorResponse(w, r, err)
		return
	}
	out := make([]platformStaffListItem, 0, len(staff))
	for _, s := range staff {
		out = append(out, toPlatformStaffListItem(s))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list platform staff response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// createPlatformStaff implements POST /api/v1/platform/staff
// (platform:staff:manage, superadmin only).
func (api *API) createPlatformStaff(w http.ResponseWriter, r *http.Request) {
	actor, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityStaffManage)
	if !ok {
		return
	}
	var req struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Name = strings.TrimSpace(req.Name)

	v := validator.New()
	v.Check(req.Email != "" && strings.Contains(req.Email, "@"), "email", "must be a valid email address")
	v.Check(req.Name != "", "name", "must be provided")
	v.Check(len(req.Password) >= 8, "password", "must be at least 8 characters")
	v.Check(validator.PermittedValue(req.Role, platformadmin.RoleSupport, platformadmin.RoleSuperadmin), "role", "must be support or superadmin")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	created, err := api.platformAdmin.CreateStaffAudited(r.Context(), actor.ID, req.Email, req.Name, req.Password, req.Role, RequestID(r.Context()))
	if err != nil {
		if errors.Is(err, platformadmin.ErrEmailTaken) {
			api.conflictResponse(w, r, "A platform staff account with this email already exists.")
			return
		}
		api.internalErrorResponse(w, r, err)
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toPlatformStaffListItem(created)}, nil); err != nil {
		api.logger.Error("write create platform staff response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// revokePlatformStaff implements POST /api/v1/platform/staff/{staff_id}/revoke
// (platform:staff:manage, superadmin only).
func (api *API) revokePlatformStaff(w http.ResponseWriter, r *http.Request) {
	actor, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityStaffManage)
	if !ok {
		return
	}
	targetID, err := uuid.Parse(chi.URLParam(r, "staff_id"))
	if err != nil {
		api.badRequestResponse(w, r, "staff_id must be a valid UUID.")
		return
	}
	var req platformBusinessActionRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	v := validator.New()
	v.Check(req.Reason != "", "reason", "must be provided")
	v.Check(len(req.Reason) <= 500, "reason", "must be at most 500 characters")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	revoked, err := api.platformAdmin.RevokeStaff(r.Context(), actor.ID, targetID, req.Reason, RequestID(r.Context()))
	if err != nil {
		switch {
		case errors.Is(err, platformadmin.ErrCannotRevokeSelf):
			api.conflictResponse(w, r, err.Error())
		case errors.Is(err, platformadmin.ErrStaffNotFound):
			api.notFoundResponse(w, r)
		default:
			api.internalErrorResponse(w, r, err)
		}
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toPlatformStaffListItem(revoked)}, nil); err != nil {
		api.logger.Error("write revoke platform staff response", "request_id", RequestID(r.Context()), "error", err)
	}
}
