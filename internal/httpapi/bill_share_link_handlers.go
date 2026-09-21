package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/tenancy"
	"github.com/mananuf/justbarme/internal/validator"
)

type shareLinkRequest struct {
	// ExpiresInHours is optional -- omitted or zero means the link never
	// expires until explicitly revoked or rotated (docs/PHASE_PILOT_
	// RELEASE.md §4's "configurable expiry").
	ExpiresInHours int `json:"expires_in_hours"`
}

type shareLinkResponse struct {
	// Token is the raw, unhashed token -- returned exactly once, here, at
	// creation, the same one-time-reveal convention GET /me's CSRF
	// rotation already uses. It is never returned by any other endpoint.
	Token string `json:"token"`
	URL   string `json:"url"`
}

// createOrRotateBillShareLink implements POST
// /api/v1/bills/{bill_id}/share-link (bills:share -- both roles, per the
// existing capability map's own long-standing anticipation of this
// feature). Calling it again on a bill that already has a link rotates it
// -- the old token stops working immediately.
func (api *API) createOrRotateBillShareLink(w http.ResponseWriter, r *http.Request) {
	businessCtx, ok := api.requireCapability(w, r, tenancy.CapabilityBillsShare)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	billID, err := uuid.Parse(chi.URLParam(r, "bill_id"))
	if err != nil {
		api.badRequestResponse(w, r, "bill_id must be a valid UUID.")
		return
	}

	var req shareLinkRequest
	if r.ContentLength != 0 {
		if err := readJSON(w, r, &req, 1<<12); err != nil {
			api.badRequestResponse(w, r, err.Error())
			return
		}
	}
	v := validator.New()
	v.Check(req.ExpiresInHours >= 0, "expires_in_hours", "must not be negative")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	var expiresAt *time.Time
	if req.ExpiresInHours > 0 {
		t := time.Now().Add(time.Duration(req.ExpiresInHours) * time.Hour)
		expiresAt = &t
	}

	rawToken, url, _, err := api.sales.CreateOrRotateShareLink(r.Context(), principal.UserID, businessCtx.BusinessID, billID, expiresAt)
	if err != nil {
		tabsErrorResponse(api, w, r, err, "create bill share link")
		return
	}

	payload := envelope{"data": shareLinkResponse{Token: rawToken, URL: url}}
	if err := writeJSON(w, http.StatusCreated, payload, nil); err != nil {
		api.logger.Error("write create bill share link response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// revokeBillShareLink implements POST
// /api/v1/bills/{bill_id}/share-link/revoke (bills:share). Idempotent --
// revoking a bill with no active link is not an error, matching
// sales.Service.RevokeShareLink's own documented behavior.
func (api *API) revokeBillShareLink(w http.ResponseWriter, r *http.Request) {
	businessCtx, ok := api.requireCapability(w, r, tenancy.CapabilityBillsShare)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	billID, err := uuid.Parse(chi.URLParam(r, "bill_id"))
	if err != nil {
		api.badRequestResponse(w, r, "bill_id must be a valid UUID.")
		return
	}

	if err := api.sales.RevokeShareLink(r.Context(), principal.UserID, businessCtx.BusinessID, billID); err != nil {
		tabsErrorResponse(api, w, r, err, "revoke bill share link")
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": envelope{"revoked": true}}, nil); err != nil {
		api.logger.Error("write revoke bill share link response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type publicBillItemResponse struct {
	Description   string `json:"description"`
	Quantity      int32  `json:"quantity"`
	UnitPriceKobo int64  `json:"unit_price_kobo"`
	LineTotalKobo int64  `json:"line_total_kobo"`
}

type publicBillResponse struct {
	BusinessName        string                   `json:"business_name"`
	LogoURL             string                   `json:"logo_url"`
	Phone               string                   `json:"phone"`
	Address             string                   `json:"address"`
	ReceiptWording      string                   `json:"receipt_wording"`
	ReceiptFooter       string                   `json:"receipt_footer"`
	PaymentInstructions string                   `json:"payment_instructions"`
	Status              string                   `json:"status"`
	OpenedAt            string                   `json:"opened_at"`
	Items               []publicBillItemResponse `json:"items"`
	TotalKobo           int64                    `json:"total_kobo"`
	BalanceKobo         int64                    `json:"balance_kobo"`
}

// getPublicBill implements GET /api/v1/public/bills/{token}: fully public,
// unauthenticated, rate-limited by IP (docs/PHASE_PILOT_RELEASE.md §4's
// "needs its own rate limiter... a genuinely unauthenticated surface next
// to a guessable-adjacent token" -- same shape as
// invitationLookupIPLimiter). Deliberately excludes customer names,
// seller/staff names, and table labels -- only items, total, and business
// branding, per §15's "avoid exposing internal customer IDs or activity."
// Composed here at the HTTP layer from two self-contained packages
// (internal/sales for the bill, internal/business for branding), the same
// way GET /products already composes catalogue + inventory.
func (api *API) getPublicBill(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if !api.publicBillLookupIPLimiter.Allow(clientIP(r)) {
		api.rateLimitedResponse(w, r)
		return
	}

	businessID, billID, err := api.sales.ResolveShareLink(r.Context(), token)
	if err != nil {
		tabsErrorResponse(api, w, r, err, "resolve bill share link")
		return
	}

	detail, err := api.sales.GetBillDetail(r.Context(), uuid.Nil, businessID, billID)
	if err != nil {
		tabsErrorResponse(api, w, r, err, "get shared bill")
		return
	}
	biz, err := api.business.Get(r.Context(), uuid.Nil, businessID)
	if err != nil {
		businessErrorResponse(api, w, r, err, "get business for shared bill")
		return
	}

	var items []publicBillItemResponse
	var totalKobo int64
	for _, sale := range detail.Sales {
		totalKobo += sale.TotalKobo
		for _, item := range sale.Items {
			items = append(items, publicBillItemResponse{
				Description: item.Description, Quantity: item.Quantity,
				UnitPriceKobo: item.UnitPriceKobo, LineTotalKobo: item.LineTotalKobo,
			})
		}
	}

	payload := envelope{"data": publicBillResponse{
		BusinessName: biz.Name, LogoURL: biz.LogoURL, Phone: biz.Phone, Address: biz.Address,
		ReceiptWording: biz.ReceiptWording, ReceiptFooter: biz.ReceiptFooter, PaymentInstructions: biz.PaymentInstructions,
		Status: detail.Status, OpenedAt: detail.OpenedAt.UTC().Format(time.RFC3339),
		Items: items, TotalKobo: totalKobo, BalanceKobo: detail.BalanceKobo,
	}}
	if err := writeJSON(w, http.StatusOK, payload, nil); err != nil {
		api.logger.Error("write public bill response", "request_id", RequestID(r.Context()), "error", err)
	}
}
