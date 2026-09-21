package inventory

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mananuf/justbarme/internal/store/sqlc"
)

func pgInt64(v int64) pgtype.Int8 {
	return pgtype.Int8{Int64: v, Valid: true}
}

// resolvePendingAllocation closes out a sale item's pending (uncosted)
// FIFO allocation by an owner's explicit, one-time supplied per-unit cost
// -- docs/PHASE_FIFO_COSTING.md §5. Never mutates the original pending
// allocation row (immutable ledger) -- instead creates a synthetic
// stock_lots row (source='review_resolution', so a report can always
// explain a resolved cost didn't come from a real receipt), a negative
// allocation that nets the old pending quantity to zero, and a positive
// allocation pointing at the new synthetic lot. A no-op if there's
// nothing net-pending left for this sale item (already resolved, or this
// review never had a pending allocation to begin with).
func resolvePendingAllocation(ctx context.Context, q *sqlc.Queries, businessID, saleItemID, variantID, locationID uuid.UUID, resolvedUnitCostKobo int64) error {
	netPending, err := q.SumNetPendingQuantityForSaleItem(ctx, sqlc.SumNetPendingQuantityForSaleItemParams{
		BusinessID: businessID, SaleItemID: saleItemID,
	})
	if err != nil {
		return fmt.Errorf("sum net pending quantity: %w", err)
	}
	if netPending <= 0 {
		return nil
	}
	quantity := int32(netPending)
	totalCostKobo := netPending * resolvedUnitCostKobo

	lotID, err := newID()
	if err != nil {
		return err
	}
	lot, err := q.CreateStockLot(ctx, sqlc.CreateStockLotParams{
		ID: lotID, BusinessID: businessID, ReceiptLineID: pgUUID(uuid.Nil),
		VariantID: variantID, LocationID: locationID,
		ReceivedQuantity: quantity, TotalCostKobo: totalCostKobo,
		ReceivedAt: pgTimestamptz(time.Now()), Source: "review_resolution",
	})
	if err != nil {
		return fmt.Errorf("create review-resolution lot: %w", err)
	}

	closeID, err := newID()
	if err != nil {
		return err
	}
	if _, err := q.CreateSaleItemLotAllocation(ctx, sqlc.CreateSaleItemLotAllocationParams{
		ID: closeID, BusinessID: businessID, SaleItemID: saleItemID,
		StockLotID: pgtype.UUID{}, Quantity: -quantity, AllocatedCostKobo: pgtype.Int8{},
	}); err != nil {
		return fmt.Errorf("close out pending allocation: %w", err)
	}

	resolvedID, err := newID()
	if err != nil {
		return err
	}
	if _, err := q.CreateSaleItemLotAllocation(ctx, sqlc.CreateSaleItemLotAllocationParams{
		ID: resolvedID, BusinessID: businessID, SaleItemID: saleItemID,
		StockLotID: pgUUID(lot.ID), Quantity: quantity, AllocatedCostKobo: pgInt64(totalCostKobo),
	}); err != nil {
		return fmt.Errorf("create resolved allocation: %w", err)
	}

	if _, err := q.DecrementLotRemainingQuantity(ctx, sqlc.DecrementLotRemainingQuantityParams{
		BusinessID: businessID, ID: lot.ID, RemainingQuantity: quantity,
	}); err != nil {
		return fmt.Errorf("deplete review-resolution lot: %w", err)
	}
	return nil
}
