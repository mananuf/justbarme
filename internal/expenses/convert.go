package expenses

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

func toUUID(id pgtype.UUID) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}
	return uuid.UUID(id.Bytes)
}

func toTime(t pgtype.Timestamptz) time.Time {
	return t.Time
}

func pgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func toCategory(c sqlc.ExpenseCategory) Category {
	return Category{ID: c.ID, Name: c.Name, Active: c.Active}
}

func toExpense(e sqlc.Expense) Expense {
	return Expense{
		ID: e.ID, LocationID: e.LocationID, CategoryID: e.CategoryID,
		Description: e.Description, AmountKobo: e.AmountKobo, PaymentMethod: e.PaymentMethod,
		RecordedBy: e.RecordedBy, ReversalOf: toUUID(e.ReversalOfExpenseID),
		OccurredAt: toTime(e.OccurredAt), CreatedAt: toTime(e.CreatedAt),
	}
}

func toExpenseDetailed(e sqlc.ListExpensesDetailedRow) Expense {
	return Expense{
		ID: e.ID, LocationID: e.LocationID, CategoryID: e.CategoryID, CategoryName: e.CategoryName,
		Description: e.Description, AmountKobo: e.AmountKobo, PaymentMethod: e.PaymentMethod,
		RecordedBy: e.RecordedBy, ReversalOf: toUUID(e.ReversalOfExpenseID),
		OccurredAt: toTime(e.OccurredAt), CreatedAt: toTime(e.CreatedAt),
	}
}
