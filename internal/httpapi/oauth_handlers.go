package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/mananuf/justbarme/internal/oauth"
	"github.com/mananuf/justbarme/internal/validator"
)

type googleSignInRequest struct {
	IDToken string `json:"id_token"`
}

// googleSignIn implements POST /api/v1/auth/google: public, unauthenticated,
// only registered when Google OAuth is configured (api.oauth != nil, see
// router.go). idToken comes from Google Identity Services running
// client-side -- this endpoint never sees a Google client secret or talks
// to Google's token endpoint itself, only verifies the JWT it's handed.
// Response shape matches POST /auth/login's exactly, so the frontend
// applies it through the same code path.
func (api *API) googleSignIn(w http.ResponseWriter, r *http.Request) {
	var req googleSignInRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}

	v := validator.New()
	v.Check(req.IDToken != "", "id_token", "must be provided")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	// A wasted-verification-work bound, not a brute-force defense --
	// cryptographic signature verification isn't guessable the way a
	// 6-digit OTP is.
	if !api.googleSignInIPLimiter.Allow(clientIP(r)) {
		api.rateLimitedResponse(w, r)
		return
	}

	user, rawToken, rawCSRF, sess, err := api.oauth.SignInWithGoogle(r.Context(), req.IDToken, r.UserAgent(), api.sessionTTL)
	if err != nil {
		if errors.Is(err, oauth.ErrGoogleSignInFailed) {
			api.googleSignInFailedResponse(w, r)
			return
		}
		api.internalErrorResponse(w, r, fmt.Errorf("google sign-in: %w", err))
		return
	}

	http.SetCookie(w, newSessionCookie(api.env, api.sessionCookieSecure, rawToken, sess.ExpiresAt))
	api.writeSessionPayload(w, r, http.StatusOK, user, rawCSRF)
}
