package httpapi

import (
	"net/http"
	"time"

	"github.com/mananuf/justbarme/internal/config"
)

// sessionCookieName follows docs/API_CONTRACT.md §4: a `__Host-` prefixed
// name in production (which itself forces Secure, Path=/, and no Domain
// attribute at the browser level), a plain name otherwise so local HTTPS-less
// development still works.
func sessionCookieName(env string) string {
	if env == config.Production {
		return "__Host-jbm_session"
	}
	return "jbm_session"
}

func newSessionCookie(env string, secure bool, token string, expiresAt time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName(env),
		Value:    token,
		Path:     "/",
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	}
}

func expiredSessionCookie(env string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName(env),
		Value:    "",
		Path:     "/",
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
}
