package httpapi

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/mananuf/justbarme/internal/auth"
	"github.com/mananuf/justbarme/internal/platformadmin"
)

// requirePlatformAuth resolves the platform session cookie into a
// platformadmin.Staff, or responds 401 and stops the chain. Deliberately
// separate from requireAuth: it never touches tenancy.Principal, and a
// business session cookie can never satisfy it (different cookie name).
func (api *API) requirePlatformAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(platformSessionCookieName(api.env))
		if err != nil || cookie.Value == "" {
			api.authenticationRequiredResponse(w, r)
			return
		}

		sess, err := api.platformAdmin.GetActiveSessionByToken(r.Context(), cookie.Value)
		if err != nil {
			if errors.Is(err, platformadmin.ErrSessionNotFound) {
				api.authenticationRequiredResponse(w, r)
				return
			}
			api.internalErrorResponse(w, r, fmt.Errorf("look up platform session: %w", err))
			return
		}

		staff, err := api.platformAdmin.GetStaffByID(r.Context(), sess.StaffID)
		if err != nil {
			api.internalErrorResponse(w, r, fmt.Errorf("load platform staff: %w", err))
			return
		}

		ctx := withPlatformStaff(r.Context(), staff)
		ctx = withPlatformSession(ctx, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requirePlatformCSRF mirrors requireCSRF exactly, against the platform
// session's own CSRF hash. Must run after requirePlatformAuth.
func (api *API) requirePlatformCSRF(next http.Handler) http.Handler {
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

		sess, ok := platformSessionFromContext(r.Context())
		if !ok {
			api.internalErrorResponse(w, r, errors.New("requirePlatformCSRF ran without a platform session in context"))
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

// requirePlatformCapability reads the platformadmin.Staff a prior
// requirePlatformAuth call attached to the request and confirms their role
// grants capability, writing a 403 and returning ok=false if not.
func (api *API) requirePlatformCapability(w http.ResponseWriter, r *http.Request, capability platformadmin.Capability) (platformadmin.Staff, bool) {
	staff, ok := platformStaffFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("requirePlatformCapability ran without requirePlatformAuth"))
		return platformadmin.Staff{}, false
	}
	if !platformadmin.ForRole(staff.Role).Has(capability) {
		api.permissionDeniedResponse(w, r, "")
		return platformadmin.Staff{}, false
	}
	return staff, true
}
