// Package identity implements users, memberships, locations, devices, and
// sessions — the identity and tenancy primitives every later feature
// package builds on. Domain types here are deliberately distinct from the
// generated sqlc structs: sqlc output is infrastructure, not the shape any
// caller (especially an HTTP handler) should depend on.
package identity

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID          uuid.UUID
	Email       string
	Phone       string
	DisplayName string
	Status      string
	CreatedAt   time.Time
}

type Business struct {
	ID       uuid.UUID
	Name     string
	Timezone string
	Currency string
	Status   string
}

type Membership struct {
	BusinessID   uuid.UUID
	BusinessName string
	UserID       uuid.UUID
	Role         string
	Status       string
	JoinedAt     time.Time
}

type Location struct {
	ID         uuid.UUID
	BusinessID uuid.UUID
	Name       string
	IsDefault  bool
}

type Device struct {
	ID          uuid.UUID
	BusinessID  uuid.UUID
	UserID      uuid.UUID
	LocationID  uuid.UUID
	DisplayName string
	Status      string
	EnrolledAt  time.Time
}

// Session is the identity package's view of an online session row. The raw
// bearer token is never stored and never returned from a lookup — only
// Service.CreateSession, at issuance time, ever sees it.
type Session struct {
	ID     uuid.UUID
	UserID uuid.UUID
	// CSRFTokenHash is for internal comparison against an incoming
	// X-CSRF-Token header's hash only. Handlers must never serialize it.
	CSRFTokenHash string
	ExpiresAt     time.Time
}
