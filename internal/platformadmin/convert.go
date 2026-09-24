package platformadmin

import (
	"time"

	"github.com/google/uuid"
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

func pgUUID(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

func toUUID(id pgtype.UUID) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}
	return uuid.UUID(id.Bytes)
}

func pgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func toTime(t pgtype.Timestamptz) time.Time {
	return t.Time
}

func toStaff(s sqlc.PlatformStaff) Staff {
	return Staff{
		ID:          s.ID,
		Email:       s.Email,
		DisplayName: s.DisplayName,
		Role:        s.Role,
		Status:      s.Status,
		CreatedAt:   toTime(s.CreatedAt),
	}
}

func toSession(s sqlc.PlatformSession) Session {
	return Session{
		ID:            s.ID,
		StaffID:       s.StaffID,
		CSRFTokenHash: s.CsrfTokenHash,
		ExpiresAt:     toTime(s.ExpiresAt),
	}
}

func toBusiness(b sqlc.Business) Business {
	return Business{
		ID: b.ID, Name: b.Name, Status: b.Status,
		Timezone: b.Timezone, Currency: b.Currency, CreatedAt: toTime(b.CreatedAt),
	}
}

func toAuditEntry(a sqlc.PlatformAuditLog) AuditEntry {
	return AuditEntry{
		ID:               a.ID,
		StaffID:          a.PlatformStaffID,
		Action:           a.Action,
		TargetBusinessID: toUUID(a.TargetBusinessID),
		TargetStaffID:    toUUID(a.TargetStaffID),
		Reason:           toText(a.Reason),
		RequestID:        toText(a.RequestID),
		CreatedAt:        toTime(a.CreatedAt),
	}
}
