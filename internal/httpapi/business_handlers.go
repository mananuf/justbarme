package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mananuf/justbarme/internal/tenancy"
	"github.com/mananuf/justbarme/internal/validator"
)

type createBusinessRequest struct {
	Name string `json:"name"`
}

type businessResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
	Currency string `json:"currency"`
}

type membershipDetailResponse struct {
	BusinessID string `json:"business_id"`
	Role       string `json:"role"`
	Status     string `json:"status"`
}

// createBusiness implements POST /api/v1/businesses: it creates the
// business, owner membership, and default location atomically. The
// authenticated caller always becomes Owner — see docs/API_CONTRACT.md §8.
func (api *API) createBusiness(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("createBusiness ran without requireAuth"))
		return
	}

	var req createBusinessRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)

	v := validator.New()
	v.Check(req.Name != "", "name", "must be provided")
	v.Check(len(req.Name) <= 200, "name", "must be at most 200 characters")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	business, membership, _, err := api.identity.CreateBusinessWithOwner(r.Context(), principal.UserID, req.Name)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("create business: %w", err))
		return
	}

	payload := envelope{"data": envelope{
		"business": businessResponse{
			ID: business.ID.String(), Name: business.Name,
			Timezone: business.Timezone, Currency: business.Currency,
		},
		"membership": membershipDetailResponse{
			BusinessID: membership.BusinessID.String(), Role: membership.Role, Status: membership.Status,
		},
	}}
	if err := writeJSON(w, http.StatusCreated, payload, nil); err != nil {
		api.logger.Error("write create business response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// getBusiness implements GET /api/v1/business: the tenant-scoped business
// resolved by requireBusinessContext. Any active member may read it —
// editing settings (PATCH) is deferred to the Phase 3 catalogue/branding
// work in docs/IMPLEMENTATION_PLAN.md.
func (api *API) getBusiness(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("getBusiness ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityBusinessRead)
	if !ok {
		return
	}

	found, err := api.identity.GetBusiness(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("get business: %w", err))
		return
	}

	payload := envelope{"data": businessResponse{
		ID: found.ID.String(), Name: found.Name, Timezone: found.Timezone, Currency: found.Currency,
	}}
	if err := writeJSON(w, http.StatusOK, payload, nil); err != nil {
		api.logger.Error("write business response", "request_id", RequestID(r.Context()), "error", err)
	}
}
