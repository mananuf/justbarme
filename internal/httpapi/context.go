package httpapi

import (
	"context"

	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/platformadmin"
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

// platformStaffContextKey carries the authenticated platformadmin.Staff for
// the current request, attached by requirePlatformAuth. Kept as its own
// context type (not tenancy.Principal) since a platform staff member is not
// a business member and must never be usable where a tenancy.Principal is
// expected.
type platformStaffContextKey struct{}

func withPlatformStaff(ctx context.Context, s platformadmin.Staff) context.Context {
	return context.WithValue(ctx, platformStaffContextKey{}, s)
}

func platformStaffFromContext(ctx context.Context) (platformadmin.Staff, bool) {
	s, ok := ctx.Value(platformStaffContextKey{}).(platformadmin.Staff)
	return s, ok
}

// platformSessionContextKey carries the validated platformadmin.Session,
// mirroring sessionContextKey's reasoning exactly (only the platform CSRF
// middleware needs CSRFTokenHash).
type platformSessionContextKey struct{}

func withPlatformSession(ctx context.Context, s platformadmin.Session) context.Context {
	return context.WithValue(ctx, platformSessionContextKey{}, s)
}

func platformSessionFromContext(ctx context.Context) (platformadmin.Session, bool) {
	s, ok := ctx.Value(platformSessionContextKey{}).(platformadmin.Session)
	return s, ok
}
