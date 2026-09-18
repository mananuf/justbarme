package identity

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/mananuf/justbarme/internal/store/sqlc"
)

func pgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func toText(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

func pgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func toTime(t pgtype.Timestamptz) time.Time {
	return t.Time
}

func toUser(u sqlc.User) User {
	return User{
		ID:          u.ID,
		Email:       toText(u.Email),
		Phone:       toText(u.Phone),
		DisplayName: u.DisplayName,
		Status:      u.Status,
		CreatedAt:   toTime(u.CreatedAt),
	}
}

func toBusiness(b sqlc.Business) Business {
	return Business{ID: b.ID, Name: b.Name, Timezone: b.Timezone, Currency: b.Currency, Status: b.Status}
}

func toMembership(m sqlc.BusinessMembership, businessName string) Membership {
	return Membership{
		BusinessID:   m.BusinessID,
		BusinessName: businessName,
		UserID:       m.UserID,
		Role:         m.Role,
		Status:       m.Status,
		JoinedAt:     toTime(m.JoinedAt),
	}
}

func toLocation(l sqlc.Location) Location {
	return Location{ID: l.ID, BusinessID: l.BusinessID, Name: l.Name, IsDefault: l.IsDefault}
}

func toDevice(d sqlc.Device) Device {
	return Device{
		ID:          d.ID,
		BusinessID:  d.BusinessID,
		UserID:      d.UserID,
		LocationID:  d.LocationID,
		DisplayName: d.DisplayName,
		Status:      d.Status,
		EnrolledAt:  toTime(d.EnrolledAt),
	}
}

func toSession(s sqlc.Session) Session {
	return Session{
		ID:            s.ID,
		UserID:        s.UserID,
		CSRFTokenHash: s.CsrfTokenHash,
		ExpiresAt:     toTime(s.ExpiresAt),
	}
}
