package invitations

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
	return t.String
}

func toUUID(id pgtype.UUID) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}
	return uuid.UUID(id.Bytes)
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

func toTime(t pgtype.Timestamptz) time.Time {
	return t.Time
}

func pgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func toInvitation(i sqlc.Invitation) Invitation {
	return Invitation{
		ID: i.ID, BusinessID: i.BusinessID, InvitedBy: i.InvitedBy,
		Phone: toText(i.Phone), Email: toText(i.Email), Role: i.Role, Status: i.Status,
		ExpiresAt: toTime(i.ExpiresAt), AcceptedBy: toUUID(i.AcceptedBy), AcceptedAt: toTime(i.AcceptedAt),
		CreatedAt: toTime(i.CreatedAt),
	}
}
