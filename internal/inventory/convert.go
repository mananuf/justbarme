package inventory

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mananuf/justbarme/internal/store/sqlc"
)

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

func toReceiptLineResult(l sqlc.StockReceiptLine, newBalance int64) ReceiptLineResult {
	return ReceiptLineResult{
		ID: l.ID, VariantID: l.VariantID, Quantity: l.Quantity, TotalCostKobo: l.TotalCostKobo,
		NewBalance: newBalance,
	}
}
