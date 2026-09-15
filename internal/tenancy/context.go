package tenancy

import (
	"context"

	"github.com/google/uuid"
)

// Principal identifies the authenticated user making a request, resolved
// from the session cookie. It exists independently of any business context:
// GET /api/v1/me works from a Principal alone.
type Principal struct {
	UserID      uuid.UUID
	Email       string
	DisplayName string
}

// Business is the tenant context resolved from the X-Business-ID header
// plus an active membership row for the current Principal. Its presence in
// a request context is what tenant-scoped handlers require before touching
// any tenant-owned table.
type Business struct {
	BusinessID   uuid.UUID
	Role         string
	Capabilities Set
}

// Has reports whether the resolved business context grants capability.
func (b Business) Has(capability Capability) bool {
	return b.Capabilities.Has(capability)
}

type principalContextKey struct{}
type businessContextKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, p)
}

// PrincipalFromContext returns the request's authenticated principal. ok is
// false if no principal was attached — callers must treat that as
// unauthenticated, never as an anonymous-but-valid principal.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalContextKey{}).(Principal)
	return p, ok
}

func WithBusiness(ctx context.Context, b Business) context.Context {
	return context.WithValue(ctx, businessContextKey{}, b)
}

// BusinessFromContext returns the request's resolved tenant context. ok is
// false if no business context was attached — callers must fail closed, not
// assume any default business.
func BusinessFromContext(ctx context.Context) (Business, bool) {
	b, ok := ctx.Value(businessContextKey{}).(Business)
	return b, ok
}
