package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/platformadmin"
	"github.com/mananuf/justbarme/internal/validator"
)

type platformLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type platformStaffResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type platformSessionResponse struct {
	Staff     platformStaffResponse `json:"staff"`
	CSRFToken string                `json:"csrf_token"`
}

// platformLogin implements POST /api/v1/platform/auth/login. Mirrors
// login's shape exactly, against a wholly separate identity/session store.
func (api *API) platformLogin(w http.ResponseWriter, r *http.Request) {
	var req platformLoginRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Email = strings.TrimSpace(req.Email)

	v := validator.New()
	v.Check(req.Email != "", "email", "must be provided")
	v.Check(req.Password != "", "password", "must be provided")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	if !api.platformLoginLimiter.Allow(strings.ToLower(req.Email)) {
		api.rateLimitedResponse(w, r)
		return
	}

	staff, err := api.platformAdmin.Authenticate(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, platformadmin.ErrInvalidCredentials) {
			api.invalidCredentialsResponse(w, r)
			return
		}
		api.internalErrorResponse(w, r, fmt.Errorf("authenticate platform staff: %w", err))
		return
	}

	rawToken, rawCSRF, sess, err := api.platformAdmin.CreateSession(r.Context(), staff.ID, r.UserAgent(), api.platformSessionTTL)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("create platform session: %w", err))
		return
	}

	// Best-effort: a failed audit write must never block a successful
	// login, but is worth knowing about.
	if err := api.platformAdmin.RecordLogin(r.Context(), staff.ID, RequestID(r.Context())); err != nil {
		api.logger.Error("record platform login audit entry", "request_id", RequestID(r.Context()), "error", err)
	}

	http.SetCookie(w, newPlatformSessionCookie(api.env, api.sessionCookieSecure, rawToken, sess.ExpiresAt))
	api.writePlatformSessionPayload(w, r, http.StatusOK, staff, rawCSRF)
}

// platformLogout implements POST /api/v1/platform/auth/logout.
func (api *API) platformLogout(w http.ResponseWriter, r *http.Request) {
	staff, ok := platformStaffFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("platformLogout ran without requirePlatformAuth"))
		return
	}
	sess, ok := platformSessionFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("platformLogout ran without a platform session in context"))
		return
	}

	if err := api.platformAdmin.RevokeSession(r.Context(), staff.ID, sess.ID, "logout"); err != nil && !errors.Is(err, platformadmin.ErrSessionNotFound) {
		api.internalErrorResponse(w, r, fmt.Errorf("revoke platform session: %w", err))
		return
	}

	http.SetCookie(w, expiredPlatformSessionCookie(api.env, api.sessionCookieSecure))
	w.WriteHeader(http.StatusNoContent)
}

// platformMe implements GET /api/v1/platform/me: current staff and a
// freshly rotated CSRF token, mirroring GET /me exactly -- without
// rotation, a reloaded platform admin page would have no way to obtain a
// usable CSRF token (only its hash is ever stored) and every mutating
// request would fail until the staff member logged in again.
func (api *API) platformMe(w http.ResponseWriter, r *http.Request) {
	staff, ok := platformStaffFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("platformMe ran without requirePlatformAuth"))
		return
	}
	sess, ok := platformSessionFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("platformMe ran without a platform session in context"))
		return
	}

	csrfToken, err := api.platformAdmin.RotateCSRFToken(r.Context(), sess.ID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("rotate platform csrf token: %w", err))
		return
	}
	api.writePlatformSessionPayload(w, r, http.StatusOK, staff, csrfToken)
}

func (api *API) writePlatformSessionPayload(w http.ResponseWriter, r *http.Request, status int, staff platformadmin.Staff, csrfToken string) {
	payload := envelope{"data": platformSessionResponse{
		Staff: platformStaffResponse{
			ID: staff.ID.String(), Name: staff.DisplayName, Email: staff.Email, Role: staff.Role,
		},
		CSRFToken: csrfToken,
	}}
	if err := writeJSON(w, status, payload, nil); err != nil {
		api.logger.Error("write platform session response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type platformBusinessResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Timezone  string `json:"timezone"`
	Currency  string `json:"currency"`
	CreatedAt string `json:"created_at"`
}

func toPlatformBusinessResponse(b platformadmin.Business) platformBusinessResponse {
	return platformBusinessResponse{
		ID: b.ID.String(), Name: b.Name, Status: b.Status,
		Timezone: b.Timezone, Currency: b.Currency, CreatedAt: b.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// listPlatformBusinesses implements GET /api/v1/platform/businesses:
// oversight only, non-sensitive summary fields -- no sales, members, or
// catalogue data. Requires platform:businesses:read (Support has this).
func (api *API) listPlatformBusinesses(w http.ResponseWriter, r *http.Request) {
	if _, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesRead); !ok {
		return
	}

	businesses, err := api.platformAdmin.ListBusinesses(r.Context())
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list platform businesses: %w", err))
		return
	}
	out := make([]platformBusinessResponse, 0, len(businesses))
	for _, b := range businesses {
		out = append(out, toPlatformBusinessResponse(b))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list platform businesses response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type platformBusinessActionRequest struct {
	Reason string `json:"reason"`
}

// suspendPlatformBusiness implements
// POST /api/v1/platform/businesses/{business_id}/suspend. Requires
// platform:businesses:suspend (Superadmin only) -- Support may see, never
// act.
func (api *API) suspendPlatformBusiness(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesSuspend)
	if !ok {
		return
	}
	api.setPlatformBusinessStatus(w, r, staff, true)
}

// reactivatePlatformBusiness implements
// POST /api/v1/platform/businesses/{business_id}/reactivate. Same
// capability as suspend -- reversing the action is not a lesser privilege.
func (api *API) reactivatePlatformBusiness(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesSuspend)
	if !ok {
		return
	}
	api.setPlatformBusinessStatus(w, r, staff, false)
}

func (api *API) setPlatformBusinessStatus(w http.ResponseWriter, r *http.Request, staff platformadmin.Staff, suspend bool) {
	businessID, err := uuid.Parse(chi.URLParam(r, "business_id"))
	if err != nil {
		api.badRequestResponse(w, r, "business_id must be a valid UUID.")
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

	var (
		business platformadmin.Business
		opErr    error
	)
	if suspend {
		business, opErr = api.platformAdmin.SuspendBusiness(r.Context(), staff.ID, businessID, req.Reason, RequestID(r.Context()))
	} else {
		business, opErr = api.platformAdmin.ReactivateBusiness(r.Context(), staff.ID, businessID, req.Reason, RequestID(r.Context()))
	}
	if opErr != nil {
		if errors.Is(opErr, platformadmin.ErrBusinessNotFound) {
			api.notFoundResponse(w, r)
			return
		}
		api.internalErrorResponse(w, r, fmt.Errorf("set business status: %w", opErr))
		return
	}

	if err := writeJSON(w, http.StatusOK, envelope{"data": toPlatformBusinessResponse(business)}, nil); err != nil {
		api.logger.Error("write set business status response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type platformAuditEntryResponse struct {
	ID               string `json:"id"`
	StaffID          string `json:"staff_id"`
	Action           string `json:"action"`
	TargetBusinessID string `json:"target_business_id,omitempty"`
	Reason           string `json:"reason,omitempty"`
	CreatedAt        string `json:"created_at"`
}

// listPlatformAuditLog implements GET /api/v1/platform/audit-log. Requires
// platform:audit:read (Support has this) -- oversight without mutation
// rights still needs to be able to see what other staff have done.
func (api *API) listPlatformAuditLog(w http.ResponseWriter, r *http.Request) {
	if _, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityAuditRead); !ok {
		return
	}

	entries, err := api.platformAdmin.ListAuditLog(r.Context(), 200)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list platform audit log: %w", err))
		return
	}
	out := make([]platformAuditEntryResponse, 0, len(entries))
	for _, e := range entries {
		row := platformAuditEntryResponse{
			ID: e.ID.String(), StaffID: e.StaffID.String(), Action: e.Action,
			Reason: e.Reason, CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339),
		}
		if e.TargetBusinessID != uuid.Nil {
			row.TargetBusinessID = e.TargetBusinessID.String()
		}
		out = append(out, row)
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list platform audit log response", "request_id", RequestID(r.Context()), "error", err)
	}
}
