package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"

	"github.com/mananuf/justbarme/internal/signup"
	"github.com/mananuf/justbarme/internal/validator"
)

type signupStartRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

// signupStart implements POST /api/v1/auth/signup/start: public,
// unauthenticated. Hashes the password and emails a one-time code; nothing
// in `users` is created yet (see internal/signup). Calling this again for
// the same address before it verifies is how "resend the code" works --
// there is no separate resend endpoint.
func (api *API) signupStart(w http.ResponseWriter, r *http.Request) {
	var req signupStartRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Name = strings.TrimSpace(req.Name)

	v := validator.New()
	_, emailErr := mail.ParseAddress(req.Email)
	v.Check(req.Email != "" && emailErr == nil, "email", "must be a valid email address")
	v.Check(req.Name != "", "name", "must be provided")
	v.Check(len(req.Name) <= 200, "name", "must be at most 200 characters")
	v.Check(len(req.Password) >= 8, "password", "must be at least 8 characters")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	// Rate-limited by both email (a spammed inbox is the direct harm) and
	// IP (a public endpoint that creates database rows and sends real
	// email is a more attractive target than login ever was).
	emailKey := strings.ToLower(req.Email)
	if !api.signupStartEmailLimiter.Allow(emailKey) || !api.signupStartIPLimiter.Allow(clientIP(r)) {
		api.rateLimitedResponse(w, r)
		return
	}

	if err := api.signup.Start(r.Context(), req.Email, req.Name, req.Password); err != nil {
		if errors.Is(err, signup.ErrEmailAlreadyRegistered) {
			api.emailAlreadyRegisteredResponse(w, r)
			return
		}
		api.internalErrorResponse(w, r, fmt.Errorf("start signup: %w", err))
		return
	}

	if err := writeJSON(w, http.StatusOK, envelope{"data": envelope{"email": req.Email}}, nil); err != nil {
		api.logger.Error("write signup start response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type signupVerifyRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

// signupVerify implements POST /api/v1/auth/signup/verify: public,
// unauthenticated. On a correct code this is where the real user and
// session are created -- the response is identical in shape to
// POST /auth/login's, so the frontend can reuse the same session-apply
// logic for both.
func (api *API) signupVerify(w http.ResponseWriter, r *http.Request) {
	var req signupVerifyRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Code = strings.TrimSpace(req.Code)

	v := validator.New()
	v.Check(req.Email != "", "email", "must be provided")
	v.Check(req.Code != "", "code", "must be provided")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	emailKey := strings.ToLower(req.Email)
	if !api.signupVerifyEmailLimiter.Allow(emailKey) || !api.signupVerifyIPLimiter.Allow(clientIP(r)) {
		api.rateLimitedResponse(w, r)
		return
	}

	user, rawToken, rawCSRF, sess, err := api.signup.Verify(r.Context(), req.Email, req.Code, r.UserAgent(), api.sessionTTL)
	if err != nil {
		switch {
		case errors.Is(err, signup.ErrEmailAlreadyRegistered):
			api.emailAlreadyRegisteredResponse(w, r)
		case errors.Is(err, signup.ErrNoPendingSignup), errors.Is(err, signup.ErrInvalidOrExpiredCode):
			api.invalidSignupCodeResponse(w, r)
		default:
			api.internalErrorResponse(w, r, fmt.Errorf("verify signup: %w", err))
		}
		return
	}

	http.SetCookie(w, newSessionCookie(api.env, api.sessionCookieSecure, rawToken, sess.ExpiresAt))
	api.writeSessionPayload(w, r, http.StatusOK, user, rawCSRF)
}
