package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mananuf/justbarme/internal/business"
	"github.com/mananuf/justbarme/internal/tenancy"
	"github.com/mananuf/justbarme/internal/validator"
)

// maxLogoUploadBodyBytes bounds the raw HTTP request body for a logo
// upload -- kept slightly above business.maxLogoUploadBytes (2MB) so a
// multipart-adjacent request that's just over the image limit gets a
// clean, service-level ErrImageTooLarge rather than being truncated mid
// -read by readJSON's own limiter.
const maxLogoUploadBodyBytes = 3 << 20

type businessSettingsResponse struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Timezone            string `json:"timezone"`
	Currency            string `json:"currency"`
	Phone               string `json:"phone"`
	Address             string `json:"address"`
	ReceiptWording      string `json:"receipt_wording"`
	ReceiptFooter       string `json:"receipt_footer"`
	PaymentInstructions string `json:"payment_instructions"`
	LogoURL             string `json:"logo_url"`
}

func toBusinessSettingsResponse(b business.Business) businessSettingsResponse {
	return businessSettingsResponse{
		ID: b.ID.String(), Name: b.Name, Timezone: b.Timezone, Currency: b.Currency,
		Phone: b.Phone, Address: b.Address, ReceiptWording: b.ReceiptWording,
		ReceiptFooter: b.ReceiptFooter, PaymentInstructions: b.PaymentInstructions,
		LogoURL: b.LogoURL,
	}
}

func businessErrorResponse(api *API, w http.ResponseWriter, r *http.Request, err error, action string) {
	switch {
	case errors.Is(err, business.ErrBusinessNotFound):
		api.notFoundResponse(w, r)
	case errors.Is(err, business.ErrImageTooLarge), errors.Is(err, business.ErrInvalidImage), errors.Is(err, business.ErrUnsupportedImageFormat):
		api.badRequestResponse(w, r, err.Error())
	default:
		api.internalErrorResponse(w, r, fmt.Errorf("%s: %w", action, err))
	}
}

type updateBusinessSettingsRequest struct {
	Phone               string `json:"phone"`
	Address             string `json:"address"`
	ReceiptWording      string `json:"receipt_wording"`
	ReceiptFooter       string `json:"receipt_footer"`
	PaymentInstructions string `json:"payment_instructions"`
}

// updateBusinessSettings implements PATCH /api/v1/business: owner-only
// branding fields (docs/PHASE_PILOT_RELEASE.md §5). Catalogue editing on
// the same Settings screen reuses the existing internal/catalogue
// endpoints directly -- this handler only ever touches the businesses
// table's own branding columns.
func (api *API) updateBusinessSettings(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("updateBusinessSettings ran without requireAuth"))
		return
	}
	businessCtx, ok := api.requireCapability(w, r, tenancy.CapabilityBusinessUpdate)
	if !ok {
		return
	}

	var req updateBusinessSettingsRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Phone = strings.TrimSpace(req.Phone)
	req.Address = strings.TrimSpace(req.Address)
	req.ReceiptWording = strings.TrimSpace(req.ReceiptWording)
	req.ReceiptFooter = strings.TrimSpace(req.ReceiptFooter)
	req.PaymentInstructions = strings.TrimSpace(req.PaymentInstructions)

	v := validator.New()
	v.Check(len(req.Phone) <= 32, "phone", "must be at most 32 characters")
	v.Check(len(req.Address) <= 500, "address", "must be at most 500 characters")
	v.Check(len(req.ReceiptWording) <= 200, "receipt_wording", "must be at most 200 characters")
	v.Check(len(req.ReceiptFooter) <= 500, "receipt_footer", "must be at most 500 characters")
	v.Check(len(req.PaymentInstructions) <= 500, "payment_instructions", "must be at most 500 characters")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	updated, err := api.business.UpdateBranding(r.Context(), principal.UserID, businessCtx.BusinessID, business.UpdateBrandingParams{
		Phone: req.Phone, Address: req.Address, ReceiptWording: req.ReceiptWording,
		ReceiptFooter: req.ReceiptFooter, PaymentInstructions: req.PaymentInstructions,
	})
	if err != nil {
		businessErrorResponse(api, w, r, err, "update business settings")
		return
	}

	payload := envelope{"data": toBusinessSettingsResponse(updated)}
	if err := writeJSON(w, http.StatusOK, payload, nil); err != nil {
		api.logger.Error("write update business settings response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// uploadBusinessLogo implements POST /api/v1/business/logo: owner-only.
// The request body is the raw image bytes -- no multipart wrapper, since
// this is always a single whole-file upload from the Settings page, never
// part of a larger form submission.
func (api *API) uploadBusinessLogo(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("uploadBusinessLogo ran without requireAuth"))
		return
	}
	businessCtx, ok := api.requireCapability(w, r, tenancy.CapabilityBusinessUpdate)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxLogoUploadBodyBytes)
	data, err := readRawBody(r)
	if err != nil {
		api.badRequestResponse(w, r, "The uploaded file is too large or could not be read.")
		return
	}
	if len(data) == 0 {
		api.badRequestResponse(w, r, "No file was uploaded.")
		return
	}

	updated, err := api.business.UpdateLogo(r.Context(), principal.UserID, businessCtx.BusinessID, data)
	if err != nil {
		businessErrorResponse(api, w, r, err, "upload business logo")
		return
	}

	payload := envelope{"data": toBusinessSettingsResponse(updated)}
	if err := writeJSON(w, http.StatusOK, payload, nil); err != nil {
		api.logger.Error("write upload business logo response", "request_id", RequestID(r.Context()), "error", err)
	}
}
