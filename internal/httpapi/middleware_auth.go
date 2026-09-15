package httpapi

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/auth"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/tenancy"
)

// requireAuth resolves the session cookie into a tenancy.Principal, or
// responds 401 and stops the chain. It must run before requireCSRF and
// requireBusinessContext, both of which depend on the principal/session it
// attaches to the request context.
func (api *API) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName(api.env))
		if err != nil || cookie.Value == "" {
			api.authenticationRequiredResponse(w, r)
			return
		}

		sess, err := api.identity.GetActiveSessionByToken(r.Context(), cookie.Value)
		if err != nil {
			if errors.Is(err, identity.ErrSessionNotFound) {
				api.authenticationRequiredResponse(w, r)
				return
			}
			api.internalErrorResponse(w, r, fmt.Errorf("look up session: %w", err))
			return
		}

		user, err := api.identity.GetUserByID(r.Context(), sess.UserID)
		if err != nil {
			api.internalErrorResponse(w, r, fmt.Errorf("load session user: %w", err))
			return
		}

		ctx := tenancy.WithPrincipal(r.Context(), tenancy.Principal{
			UserID:      user.ID,
			Email:       user.Email,
			DisplayName: user.DisplayName,
		})
		ctx = withSession(ctx, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireCSRF enforces docs/API_CONTRACT.md §4's CSRF contract for unsafe
// methods on cookie-authenticated routes: a same-origin Origin header plus a
// session-bound X-CSRF-Token. It must run after requireAuth.
func (api *API) requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		origin := r.Header.Get("Origin")
		originURL, err := url.Parse(origin)
		if origin == "" || err != nil || !strings.EqualFold(originURL.Host, r.Host) {
			api.permissionDeniedResponse(w, r, "This request did not come from a trusted origin.")
			return
		}

		sess, ok := sessionFromContext(r.Context())
		if !ok {
			api.internalErrorResponse(w, r, errors.New("requireCSRF ran without a session in context"))
			return
		}

		provided := r.Header.Get("X-CSRF-Token")
		if provided == "" || subtle.ConstantTimeCompare([]byte(auth.HashToken(provided)), []byte(sess.CSRFTokenHash)) != 1 {
			api.permissionDeniedResponse(w, r, "Invalid or missing CSRF token.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireBusinessContext resolves X-Business-ID plus the principal's
// membership in it into a tenancy.Business, or fails closed. A business
// that does not exist and one the principal simply isn't a member of are
// deliberately indistinguishable to the caller (NOT_FOUND either way) — see
// docs/API_CONTRACT.md's error table: "hidden across tenant boundary".
func (api *API) requireBusinessContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := tenancy.PrincipalFromContext(r.Context())
		if !ok {
			api.internalErrorResponse(w, r, errors.New("requireBusinessContext ran without requireAuth"))
			return
		}

		raw := r.Header.Get("X-Business-ID")
		if raw == "" {
			api.badRequestResponse(w, r, "The X-Business-ID header is required.")
			return
		}
		businessID, err := uuid.Parse(raw)
		if err != nil {
			api.badRequestResponse(w, r, "X-Business-ID must be a valid UUID.")
			return
		}

		membership, err := api.identity.GetMembership(r.Context(), principal.UserID, businessID)
		if err != nil {
			if errors.Is(err, identity.ErrMembershipNotFound) || errors.Is(err, identity.ErrMembershipNotActive) {
				api.notFoundResponse(w, r)
				return
			}
			api.internalErrorResponse(w, r, fmt.Errorf("look up membership: %w", err))
			return
		}

		business := tenancy.Business{
			BusinessID:   businessID,
			Role:         membership.Role,
			Capabilities: tenancy.ForRole(membership.Role),
		}
		next.ServeHTTP(w, r.WithContext(tenancy.WithBusiness(r.Context(), business)))
	})
}

// requireCapability reads the tenancy.Business a prior requireBusinessContext
// call attached to the request and confirms it grants capability, writing a
// 403 and returning ok=false if not. Handlers call this themselves (rather
// than a per-route middleware) because the required capability differs per
// handler even within the same route group.
func (api *API) requireCapability(w http.ResponseWriter, r *http.Request, capability tenancy.Capability) (tenancy.Business, bool) {
	business, ok := tenancy.BusinessFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("requireCapability ran without requireBusinessContext"))
		return tenancy.Business{}, false
	}
	if !business.Has(capability) {
		api.permissionDeniedResponse(w, r, "")
		return tenancy.Business{}, false
	}
	return business, true
}
