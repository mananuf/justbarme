package httpapi

import (
	"context"

	"github.com/mananuf/justbarme/internal/identity"
)

// sessionContextKey carries the validated identity.Session for the current
// request. It is deliberately httpapi-private (unlike tenancy.Principal,
// which handlers read directly): only the CSRF middleware in this package
// needs the session's CSRFTokenHash, and that field must never leak into a
// JSON response or a lower-level package's public context API.
type sessionContextKey struct{}

func withSession(ctx context.Context, s identity.Session) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, s)
}

func sessionFromContext(ctx context.Context) (identity.Session, bool) {
	s, ok := ctx.Value(sessionContextKey{}).(identity.Session)
	return s, ok
}
