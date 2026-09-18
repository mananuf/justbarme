package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/tenancy"
	"github.com/mananuf/justbarme/internal/validator"
)

type loginRequest struct {
	// Identifier is either an email address or a phone number --
	// identity.Service.Authenticate detects which by format (docs/PHASE_
	// INVITATIONS_WHATSAPP.md's dual-identity design: either logs in).
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type userResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

type membershipResponse struct {
	BusinessID   string `json:"business_id"`
	BusinessName string `json:"business_name"`
	Role         string `json:"role"`
}

type sessionResponse struct {
	User        userResponse         `json:"user"`
	Memberships []membershipResponse `json:"memberships"`
	CSRFToken   string               `json:"csrf_token"`
}

// login implements POST /api/v1/auth/login. See docs/API_CONTRACT.md §8.
func (api *API) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Identifier = strings.TrimSpace(req.Identifier)

	v := validator.New()
	v.Check(req.Identifier != "", "identifier", "must be provided")
	v.Check(req.Password != "", "password", "must be provided")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	// Rate-limited per identifier so a single account cannot be
	// brute-forced, independent of how many source addresses an attacker
	// rotates through.
	if !api.loginLimiter.Allow(strings.ToLower(req.Identifier)) {
		api.rateLimitedResponse(w, r)
		return
	}

	user, err := api.identity.Authenticate(r.Context(), req.Identifier, req.Password)
	if err != nil {
		if errors.Is(err, identity.ErrInvalidCredentials) {
			api.invalidCredentialsResponse(w, r)
			return
		}
		api.internalErrorResponse(w, r, fmt.Errorf("authenticate: %w", err))
		return
	}

	rawToken, rawCSRF, sess, err := api.identity.CreateSession(r.Context(), user.ID, r.UserAgent(), api.sessionTTL)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("create session: %w", err))
		return
	}

	http.SetCookie(w, newSessionCookie(api.env, api.sessionCookieSecure, rawToken, sess.ExpiresAt))
	api.writeSessionPayload(w, r, http.StatusOK, user, rawCSRF)
}

// logout implements POST /api/v1/auth/logout. Requires an authenticated,
// CSRF-checked request — see the router's route group wiring.
func (api *API) logout(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("logout ran without requireAuth"))
		return
	}
	sess, ok := sessionFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("logout ran without a session in context"))
		return
	}

	if err := api.identity.RevokeSession(r.Context(), principal.UserID, sess.ID, "logout"); err != nil && !errors.Is(err, identity.ErrSessionNotFound) {
		api.internalErrorResponse(w, r, fmt.Errorf("revoke session: %w", err))
		return
	}

	http.SetCookie(w, expiredSessionCookie(api.env, api.sessionCookieSecure))
	w.WriteHeader(http.StatusNoContent)
}

// me implements GET /api/v1/me: current user, active memberships, and a
// freshly rotated CSRF token so a reloaded frontend can resume safely (see
// docs/API_CONTRACT.md §8). It is global — it does not require
// X-Business-ID.
func (api *API) me(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("me ran without requireAuth"))
		return
	}
	sess, ok := sessionFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("me ran without a session in context"))
		return
	}

	user, err := api.identity.GetUserByID(r.Context(), principal.UserID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("load user: %w", err))
		return
	}

	csrfToken, err := api.identity.RotateCSRFToken(r.Context(), sess.ID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("rotate csrf token: %w", err))
		return
	}

	api.writeSessionPayload(w, r, http.StatusOK, user, csrfToken)
}

func (api *API) writeSessionPayload(w http.ResponseWriter, r *http.Request, status int, user identity.User, csrfToken string) {
	memberships, err := api.identity.ListMembershipsForUser(r.Context(), user.ID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list memberships: %w", err))
		return
	}

	active := make([]membershipResponse, 0, len(memberships))
	for _, m := range memberships {
		if m.Status != "active" {
			continue
		}
		active = append(active, membershipResponse{
			BusinessID:   m.BusinessID.String(),
			BusinessName: m.BusinessName,
			Role:         m.Role,
		})
	}

	payload := envelope{"data": sessionResponse{
		User:        userResponse{ID: user.ID.String(), Name: user.DisplayName, Email: user.Email, Phone: user.Phone},
		Memberships: active,
		CSRFToken:   csrfToken,
	}}
	if err := writeJSON(w, status, payload, nil); err != nil {
		api.logger.Error("write session response", "request_id", RequestID(r.Context()), "error", err)
	}
}
