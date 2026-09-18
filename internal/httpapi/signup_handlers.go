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
	Channel  string `json:"channel"` // "email" (default) or "whatsapp"
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

// signupStart implements POST /api/v1/auth/signup/start: public,
// unauthenticated. Hashes the password and delivers a one-time code over
// email or WhatsApp; nothing in `users` is created yet (see
// internal/signup). Calling this again for the same identifier before it
// verifies is how "resend the code" works -- there is no separate resend
// endpoint. Channel defaults to email if omitted, matching
// docs/PHASE_INVITATIONS_WHATSAPP.md's "signup defaults to email" decision.
func (api *API) signupStart(w http.ResponseWriter, r *http.Request) {
	var req signupStartRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Channel = strings.TrimSpace(req.Channel)
	if req.Channel == "" {
		req.Channel = string(signup.ChannelEmail)
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Phone = strings.TrimSpace(req.Phone)
	req.Name = strings.TrimSpace(req.Name)

	v := validator.New()
	v.Check(validator.PermittedValue(req.Channel, string(signup.ChannelEmail), string(signup.ChannelWhatsApp)), "channel", "must be email or whatsapp")
	v.Check(req.Name != "", "name", "must be provided")
	v.Check(len(req.Name) <= 200, "name", "must be at most 200 characters")
	v.Check(len(req.Password) >= 8, "password", "must be at least 8 characters")

	var identifier string
	switch signup.Channel(req.Channel) {
	case signup.ChannelWhatsApp:
		v.Check(req.Phone != "", "phone", "must be provided")
		v.Check(req.Phone == "" || validator.IsE164Phone(req.Phone), "phone", "must be in E.164 format, e.g. +2348031234567")
		v.Check(req.Email == "", "email", "must not be provided for the whatsapp channel")
		identifier = req.Phone
	default:
		_, emailErr := mail.ParseAddress(req.Email)
		v.Check(req.Email != "" && emailErr == nil, "email", "must be a valid email address")
		v.Check(req.Phone == "", "phone", "must not be provided for the email channel")
		identifier = req.Email
	}
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	// Rate-limited by both identifier (a spammed inbox/phone is the direct
	// harm) and IP (a public endpoint that creates database rows and sends
	// a real message is a more attractive target than login ever was).
	identifierKey := strings.ToLower(identifier)
	if !api.signupStartEmailLimiter.Allow(identifierKey) || !api.signupStartIPLimiter.Allow(clientIP(r)) {
		api.rateLimitedResponse(w, r)
		return
	}

	if err := api.signup.Start(r.Context(), signup.Channel(req.Channel), identifier, req.Name, req.Password); err != nil {
		if errors.Is(err, signup.ErrAlreadyRegistered) {
			if signup.Channel(req.Channel) == signup.ChannelWhatsApp {
				api.phoneAlreadyRegisteredResponse(w, r)
			} else {
				api.emailAlreadyRegisteredResponse(w, r)
			}
			return
		}
		api.internalErrorResponse(w, r, fmt.Errorf("start signup: %w", err))
		return
	}

	if err := writeJSON(w, http.StatusOK, envelope{"data": envelope{"channel": req.Channel, "identifier": identifier}}, nil); err != nil {
		api.logger.Error("write signup start response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type signupVerifyRequest struct {
	Channel string `json:"channel"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	Code    string `json:"code"`
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
	req.Channel = strings.TrimSpace(req.Channel)
	if req.Channel == "" {
		req.Channel = string(signup.ChannelEmail)
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Phone = strings.TrimSpace(req.Phone)
	req.Code = strings.TrimSpace(req.Code)

	v := validator.New()
	v.Check(validator.PermittedValue(req.Channel, string(signup.ChannelEmail), string(signup.ChannelWhatsApp)), "channel", "must be email or whatsapp")
	v.Check(req.Code != "", "code", "must be provided")

	var identifier string
	if signup.Channel(req.Channel) == signup.ChannelWhatsApp {
		v.Check(req.Phone != "", "phone", "must be provided")
		identifier = req.Phone
	} else {
		v.Check(req.Email != "", "email", "must be provided")
		identifier = req.Email
	}
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	identifierKey := strings.ToLower(identifier)
	if !api.signupVerifyEmailLimiter.Allow(identifierKey) || !api.signupVerifyIPLimiter.Allow(clientIP(r)) {
		api.rateLimitedResponse(w, r)
		return
	}

	user, rawToken, rawCSRF, sess, err := api.signup.Verify(r.Context(), signup.Channel(req.Channel), identifier, req.Code, r.UserAgent(), api.sessionTTL)
	if err != nil {
		switch {
		case errors.Is(err, signup.ErrAlreadyRegistered):
			if signup.Channel(req.Channel) == signup.ChannelWhatsApp {
				api.phoneAlreadyRegisteredResponse(w, r)
			} else {
				api.emailAlreadyRegisteredResponse(w, r)
			}
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
