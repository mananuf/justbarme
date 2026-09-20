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

func pgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func toReceiptLineResult(l sqlc.StockReceiptLine, newBalance int64) ReceiptLineResult {
	return ReceiptLineResult{
		ID: l.ID, VariantID: l.VariantID, Quantity: l.Quantity, TotalCostKobo: l.TotalCostKobo,
		NewBalance: newBalance,
	}
}

func toUUID(id pgtype.UUID) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}
	return uuid.UUID(id.Bytes)
}

func toInt32(v pgtype.Int4) int32 {
	if !v.Valid {
		return 0
	}
	return v.Int32
}

func toText(v pgtype.Text) string {
	return v.String
}

func toStockCount(c sqlc.StockCount) StockCount {
	return StockCount{
		ID: c.ID, BusinessID: c.BusinessID, LocationID: c.LocationID,
		CountedBy: c.CountedBy, StartedAt: toTime(c.StartedAt),
	}
}

func toStockCountLineResult(l sqlc.StockCountLine) StockCountLineResult {
	return StockCountLineResult{
		ID: l.ID, VariantID: l.VariantID, ExpectedQuantity: l.ExpectedQuantity,
		PhysicalQuantity: l.PhysicalQuantity, Variance: l.Variance, IsStale: l.IsStale,
	}
}

func toAdjustmentRequest(r sqlc.InventoryAdjustmentRequest) AdjustmentRequest {
	return AdjustmentRequest{
		ID: r.ID, LocationID: r.LocationID, VariantID: r.VariantID, RequestedBy: r.RequestedBy,
		QuantityDelta: r.QuantityDelta, ReasonCategory: r.ReasonCategory, ReasonNote: r.ReasonNote,
		SourceCountLineID: toUUID(r.SourceCountLineID), Status: r.Status, CreatedAt: toTime(r.CreatedAt),
	}
}

func toAdjustmentRequestDetailed(r sqlc.ListPendingInventoryAdjustmentRequestsDetailedRow) AdjustmentRequest {
	return AdjustmentRequest{
		ID: r.ID, LocationID: r.LocationID, VariantID: r.VariantID, RequestedBy: r.RequestedBy,
		QuantityDelta: r.QuantityDelta, ReasonCategory: r.ReasonCategory, ReasonNote: r.ReasonNote,
		SourceCountLineID: toUUID(r.SourceCountLineID), Status: r.Status, CreatedAt: toTime(r.CreatedAt),
		VariantName: r.VariantName, ProductName: r.ProductName,
	}
}

func toInventoryReview(r sqlc.InventoryReview) InventoryReview {
	return InventoryReview{
		ID: r.ID, Type: r.Type, VariantID: r.VariantID, LocationID: r.LocationID,
		RelatedMovementID: toUUID(r.RelatedMovementID), RelatedCountLineID: toUUID(r.RelatedCountLineID),
		Status: r.Status, CreatedAt: toTime(r.CreatedAt),
	}
}

func toInventoryReviewDetailed(r sqlc.ListOpenInventoryReviewsDetailedRow) InventoryReview {
	return InventoryReview{
		ID: r.ID, Type: r.Type, VariantID: r.VariantID, LocationID: r.LocationID,
		RelatedMovementID: toUUID(r.RelatedMovementID), RelatedCountLineID: toUUID(r.RelatedCountLineID),
		Status: r.Status, CreatedAt: toTime(r.CreatedAt),
		VariantName: r.VariantName, ProductName: r.ProductName,
		CountExpectedQuantity: toInt32(r.CountExpectedQuantity), CountPhysicalQuantity: toInt32(r.CountPhysicalQuantity),
	}
}

func toMovementHistoryEntry(m sqlc.ListInventoryMovementsDetailedRow) MovementHistoryEntry {
	return MovementHistoryEntry{
		ID: m.ID, QuantityDelta: m.QuantityDelta, CreatedAt: toTime(m.CreatedAt),
		EventType: m.EventType, ActorID: m.ActorID,
		AdjustmentReasonCategory: toText(m.AdjustmentReasonCategory),
		AdjustmentReasonNote:     toText(m.AdjustmentReasonNote),
		AdjustmentDecidedBy:      toUUID(m.AdjustmentDecidedBy),
	}
}
