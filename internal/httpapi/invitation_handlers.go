package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/invitations"
	"github.com/mananuf/justbarme/internal/tenancy"
	"github.com/mananuf/justbarme/internal/validator"
	"github.com/mananuf/justbarme/internal/verification"
)

func invitationsErrorResponse(api *API, w http.ResponseWriter, r *http.Request, err error, action string) {
	switch {
	case errors.Is(err, invitations.ErrInvitationNotFound):
		api.notFoundResponse(w, r)
	case errors.Is(err, invitations.ErrInvitationExpired), errors.Is(err, invitations.ErrInvitationNotOpen),
		errors.Is(err, invitations.ErrAlreadyPending), errors.Is(err, identity.ErrAlreadyMember):
		api.conflictResponse(w, r, err.Error())
	default:
		api.internalErrorResponse(w, r, fmt.Errorf("%s: %w", action, err))
	}
}

type invitationResponse struct {
	ID        string `json:"id"`
	Phone     string `json:"phone,omitempty"`
	Email     string `json:"email,omitempty"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
}

func toInvitationResponse(i invitations.Invitation) invitationResponse {
	return invitationResponse{
		ID: i.ID.String(), Phone: i.Phone, Email: i.Email, Role: i.Role, Status: i.Status,
		ExpiresAt: i.ExpiresAt.UTC().Format(time.RFC3339), CreatedAt: i.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// createInvitation implements POST /api/v1/invitations (members:manage) --
// sends via WhatsApp when phone is given, email otherwise (docs/PHASE_
// INVITATIONS_WHATSAPP.md: "invitations default to WhatsApp").
func (api *API) createInvitation(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilityMembersManage)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	var req struct {
		Phone string `json:"phone"`
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := readJSON(w, r, &req, 1<<12); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Phone = strings.TrimSpace(req.Phone)
	req.Email = strings.TrimSpace(req.Email)

	v := validator.New()
	v.Check(req.Phone != "" || req.Email != "", "phone", "either phone or email must be provided")
	v.Check(req.Phone == "" || validator.IsE164Phone(req.Phone), "phone", "must be in E.164 format, e.g. +2348031234567")
	v.Check(validator.PermittedValue(req.Role, invitations.RoleOwner, invitations.RoleStaff), "role", "must be owner or staff")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	if !api.invitationCreateBusinessLimiter.Allow(business.BusinessID.String()) || !api.invitationCreateIPLimiter.Allow(clientIP(r)) {
		api.rateLimitedResponse(w, r)
		return
	}

	inv, err := api.invitations.Create(r.Context(), business.BusinessID, principal.UserID, req.Phone, req.Email, req.Role)
	if err != nil {
		invitationsErrorResponse(api, w, r, err, "create invitation")
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toInvitationResponse(inv)}, nil); err != nil {
		api.logger.Error("write create invitation response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// listInvitations implements GET /api/v1/invitations (members:read).
func (api *API) listInvitations(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilityMembersRead)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	list, err := api.invitations.List(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list invitations: %w", err))
		return
	}
	out := make([]invitationResponse, 0, len(list))
	for _, inv := range list {
		out = append(out, toInvitationResponse(inv))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list invitations response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// revokeInvitation implements POST /api/v1/invitations/{invitation_id}/revoke
// (members:manage).
func (api *API) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilityMembersManage)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	invitationID, err := uuid.Parse(chi.URLParam(r, "invitation_id"))
	if err != nil {
		api.badRequestResponse(w, r, "invitation_id must be a valid UUID.")
		return
	}

	inv, err := api.invitations.Revoke(r.Context(), principal.UserID, business.BusinessID, invitationID)
	if err != nil {
		invitationsErrorResponse(api, w, r, err, "revoke invitation")
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toInvitationResponse(inv)}, nil); err != nil {
		api.logger.Error("write revoke invitation response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// getInvitationByToken implements GET /api/v1/invitations/{token}: public,
// unauthenticated -- the landing-page lookup an invitee hits before
// deciding "I'm new here" vs "I already have an account."
func (api *API) getInvitationByToken(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if !api.invitationLookupIPLimiter.Allow(clientIP(r)) {
		api.rateLimitedResponse(w, r)
		return
	}

	inv, err := api.invitations.GetByToken(r.Context(), token)
	if err != nil {
		invitationsErrorResponse(api, w, r, err, "get invitation")
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toInvitationResponse(inv)}, nil); err != nil {
		api.logger.Error("write get invitation response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// acceptInvitationAsNewUser implements POST
// /api/v1/invitations/{token}/accept: public, unauthenticated -- the
// "I'm new here" path (docs/PHASE_INVITATIONS_WHATSAPP.md flow D). No OTP
// challenge: receiving the invitation already proved channel ownership.
func (api *API) acceptInvitationAsNewUser(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	var req struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)

	v := validator.New()
	v.Check(req.Name != "", "name", "must be provided")
	v.Check(len(req.Password) >= 8, "password", "must be at least 8 characters")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	if !api.invitationAcceptIdentifierLimiter.Allow(token) || !api.invitationAcceptIPLimiter.Allow(clientIP(r)) {
		api.rateLimitedResponse(w, r)
		return
	}

	user, _, rawToken, rawCSRF, sess, err := api.invitations.AcceptAsNewUser(r.Context(), token, req.Name, req.Password, r.UserAgent(), api.sessionTTL)
	if err != nil {
		invitationsErrorResponse(api, w, r, err, "accept invitation")
		return
	}

	http.SetCookie(w, newSessionCookie(api.env, api.sessionCookieSecure, rawToken, sess.ExpiresAt))
	api.writeSessionPayload(w, r, http.StatusOK, user, rawCSRF)
}

// linkInvitationToExistingUser implements POST
// /api/v1/invitations/{token}/link: authenticated, not business-scoped --
// the "I already have an account" path (flow E). If the invitation's own
// identifier already matches something already on the caller's account,
// the membership is granted immediately; otherwise a fresh
// identity_verifications challenge is started and its ID returned, for
// the frontend to prompt a code and call
// POST /identity-verifications/{id}/confirm with the same invitation
// token to finish.
func (api *API) linkInvitationToExistingUser(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("linkInvitationToExistingUser ran without requireAuth"))
		return
	}
	token := chi.URLParam(r, "token")

	inv, err := api.invitations.GetByToken(r.Context(), token)
	if err != nil {
		invitationsErrorResponse(api, w, r, err, "get invitation")
		return
	}
	if inv.Status != invitations.StatusPending {
		api.conflictResponse(w, r, invitations.ErrInvitationNotOpen.Error())
		return
	}

	user, err := api.identity.GetUserByID(r.Context(), principal.UserID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("look up user: %w", err))
		return
	}

	var channel verification.Channel
	var identifier string
	if inv.Phone != "" {
		channel, identifier = verification.ChannelWhatsApp, inv.Phone
	} else {
		channel, identifier = verification.ChannelEmail, inv.Email
	}

	alreadyMatches := (channel == verification.ChannelWhatsApp && user.Phone == identifier) ||
		(channel == verification.ChannelEmail && user.Email == identifier)
	if alreadyMatches {
		accepted, err := api.invitations.AcceptAsExistingUser(r.Context(), token, principal.UserID)
		if err != nil {
			invitationsErrorResponse(api, w, r, err, "accept invitation")
			return
		}
		if err := writeJSON(w, http.StatusOK, envelope{"data": envelope{
			"verification_required": false,
			"invitation":            toInvitationResponse(accepted),
		}}, nil); err != nil {
			api.logger.Error("write link invitation response", "request_id", RequestID(r.Context()), "error", err)
		}
		return
	}

	v, err := api.verification.Start(r.Context(), principal.UserID, channel, identifier, verification.PurposeInvitationLink)
	if err != nil {
		verificationErrorResponse(api, w, r, err, "start invitation link verification")
		return
	}
	if err := writeJSON(w, http.StatusAccepted, envelope{"data": envelope{
		"verification_required": true,
		"verification_id":       v.ID.String(),
		"channel":               string(channel),
	}}, nil); err != nil {
		api.logger.Error("write link invitation response", "request_id", RequestID(r.Context()), "error", err)
	}
}
