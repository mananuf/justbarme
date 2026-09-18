package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/tenancy"
	"github.com/mananuf/justbarme/internal/validator"
	"github.com/mananuf/justbarme/internal/verification"
)

func verificationErrorResponse(api *API, w http.ResponseWriter, r *http.Request, err error, action string) {
	switch {
	case errors.Is(err, verification.ErrVerificationNotFound):
		api.notFoundResponse(w, r)
	case errors.Is(err, verification.ErrIdentifierAlreadyInUse), errors.Is(err, verification.ErrInvalidOrExpiredCode):
		api.conflictResponse(w, r, err.Error())
	default:
		api.internalErrorResponse(w, r, fmt.Errorf("%s: %w", action, err))
	}
}

// startAddIdentifier implements POST /api/v1/me/identifiers: authenticated
// account-settings entry point -- "add a phone/email to my account"
// (docs/PHASE_INVITATIONS_WHATSAPP.md flow C). Starts an
// identity_verifications challenge; the identifier is only attached once
// POST /identity-verifications/{id}/confirm succeeds.
func (api *API) startAddIdentifier(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("startAddIdentifier ran without requireAuth"))
		return
	}

	var req struct {
		Channel string `json:"channel"`
		Phone   string `json:"phone"`
		Email   string `json:"email"`
	}
	if err := readJSON(w, r, &req, 1<<12); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Channel = strings.TrimSpace(req.Channel)
	req.Phone = strings.TrimSpace(req.Phone)
	req.Email = strings.TrimSpace(req.Email)

	v := validator.New()
	v.Check(validator.PermittedValue(req.Channel, string(verification.ChannelEmail), string(verification.ChannelWhatsApp)), "channel", "must be email or whatsapp")
	var identifier string
	if verification.Channel(req.Channel) == verification.ChannelWhatsApp {
		v.Check(req.Phone != "", "phone", "must be provided")
		v.Check(req.Phone == "" || validator.IsE164Phone(req.Phone), "phone", "must be in E.164 format, e.g. +2348031234567")
		identifier = req.Phone
	} else {
		v.Check(req.Email != "", "email", "must be provided")
		identifier = req.Email
	}
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	result, err := api.verification.Start(r.Context(), principal.UserID, verification.Channel(req.Channel), identifier, verification.PurposeAddIdentifier)
	if err != nil {
		verificationErrorResponse(api, w, r, err, "start identity verification")
		return
	}
	if err := writeJSON(w, http.StatusAccepted, envelope{"data": envelope{
		"verification_id": result.ID.String(),
		"channel":         string(result.Channel),
	}}, nil); err != nil {
		api.logger.Error("write start identity verification response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// confirmIdentityVerification implements POST
// /api/v1/identity-verifications/{verification_id}/confirm: authenticated.
// invitation_token is optional -- when present (the caller arrived via
// POST /invitations/{token}/link), a successful confirm also finalizes
// that invitation's membership grant, orchestrated here rather than
// inside internal/verification or internal/invitations (docs/PHASE_
// INVITATIONS_WHATSAPP.md's deliberate package-boundary choice).
func (api *API) confirmIdentityVerification(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("confirmIdentityVerification ran without requireAuth"))
		return
	}

	verificationID, err := uuid.Parse(chi.URLParam(r, "verification_id"))
	if err != nil {
		api.badRequestResponse(w, r, "verification_id must be a valid UUID.")
		return
	}

	var req struct {
		Code            string `json:"code"`
		InvitationToken string `json:"invitation_token"`
	}
	if err := readJSON(w, r, &req, 1<<12); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Code = strings.TrimSpace(req.Code)

	v := validator.New()
	v.Check(req.Code != "", "code", "must be provided")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	user, err := api.verification.Confirm(r.Context(), principal.UserID, verificationID, req.Code)
	if err != nil {
		verificationErrorResponse(api, w, r, err, "confirm identity verification")
		return
	}

	if req.InvitationToken != "" {
		if _, err := api.invitations.AcceptAsExistingUser(r.Context(), req.InvitationToken, principal.UserID); err != nil {
			invitationsErrorResponse(api, w, r, err, "accept invitation after verification")
			return
		}
	}

	// No new CSRF token here: the caller is already inside an existing,
	// still-valid session (requireCSRF just checked it to allow this very
	// request) -- unlike login/signup, there is no fresh session being
	// established that would need one handed back.
	out := userResponse{ID: user.ID.String(), Name: user.DisplayName, Email: user.Email, Phone: user.Phone}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write confirm identity verification response", "request_id", RequestID(r.Context()), "error", err)
	}
}
