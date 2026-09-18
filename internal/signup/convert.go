package signup

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func pgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func pgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func toText(t pgtype.Text) string {
	return t.String
}
