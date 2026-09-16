package sales

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

func pgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func toSaleItem(i sqlc.SaleItem) SaleItem {
	return SaleItem{
		ID: i.ID, VariantID: i.VariantID, Description: i.Description,
		Quantity: i.Quantity, UnitPriceKobo: i.UnitPriceKobo, LineTotalKobo: i.LineTotalKobo,
	}
}

func toPayment(p sqlc.Payment) Payment {
	return Payment{ID: p.ID, AmountKobo: p.AmountKobo, Method: p.Method}
}

func toReview(r sqlc.SaleReview) Review {
	return Review{ID: r.ID, SaleItemID: toUUID(r.SaleItemID), Reason: r.Reason, Status: r.Status}
}

func toSale(s sqlc.Sale) Sale {
	return Sale{
		ID: s.ID, BusinessID: s.BusinessID, BillID: s.BillID, SellerID: s.SellerID,
		OccurredAt: toTime(s.OccurredAt), ReceivedAt: toTime(s.ReceivedAt),
		TotalKobo: s.TotalKobo, ReversalOf: toUUID(s.ReversalOfSaleID),
	}
}
