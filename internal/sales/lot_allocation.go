package sales

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mananuf/justbarme/internal/store/sqlc"
)

func pgInt64(v int64) pgtype.Int8 {
	return pgtype.Int8{Int64: v, Valid: true}
}

// allocateSaleItemLots consumes stock_lots FIFO for one sale item, in the
// same transaction as the inventory movement that already deducted
// inventory_balances -- see docs/PHASE_FIFO_COSTING.md §3. It never
// affects whether the sale itself posts; a shortfall just becomes one
// pending allocation row (stock_lot_id NULL, allocated_cost_kobo NULL),
// never a fabricated cost (docs/ARCHITECTURE.md §8.5: "Do not invent
// zero-cost stock to make profit reports look complete").
//
// Skipped entirely by the caller for a tracks_inventory=false variant --
// a service item has no lots to allocate from.
func allocateSaleItemLots(ctx context.Context, q *sqlc.Queries, businessID, saleItemID, variantID, locationID uuid.UUID, quantity int32) error {
	lots, err := q.ListLotsForAllocation(ctx, sqlc.ListLotsForAllocationParams{
		BusinessID: businessID, VariantID: variantID, LocationID: locationID,
	})
	if err != nil {
		return fmt.Errorf("list lots for allocation: %w", err)
	}

	stillNeeded := quantity
	for _, lot := range lots {
		if stillNeeded <= 0 {
			break
		}
		consumed := lot.RemainingQuantity
		if consumed > stillNeeded {
			consumed = stillNeeded
		}

		// Kobo-exact rounding across partial lot depletion
		// (docs/PHASE_FIFO_COSTING.md §7): allocated_cost_kobo is the
		// TOTAL cost of just this allocation's quantity, computed as the
		// difference between two cumulative-floor points, so the sum of
		// every allocation ever drawn from this lot exactly equals its
		// total_cost_kobo -- never a per-unit figure multiplied back out,
		// which would drift.
		alreadyConsumed := int64(lot.ReceivedQuantity - lot.RemainingQuantity)
		receivedQty := int64(lot.ReceivedQuantity)
		allocatedSoFarKobo := lot.TotalCostKobo * alreadyConsumed / receivedQty
		throughThisKobo := lot.TotalCostKobo * (alreadyConsumed + int64(consumed)) / receivedQty
		allocationKobo := throughThisKobo - allocatedSoFarKobo

		allocID, err := newID()
		if err != nil {
			return err
		}
		if _, err := q.CreateSaleItemLotAllocation(ctx, sqlc.CreateSaleItemLotAllocationParams{
			ID: allocID, BusinessID: businessID, SaleItemID: saleItemID,
			StockLotID: pgUUID(lot.ID), Quantity: consumed, AllocatedCostKobo: pgInt64(allocationKobo),
		}); err != nil {
			return fmt.Errorf("create sale item lot allocation: %w", err)
		}
		if _, err := q.DecrementLotRemainingQuantity(ctx, sqlc.DecrementLotRemainingQuantityParams{
			BusinessID: businessID, ID: lot.ID, RemainingQuantity: consumed,
		}); err != nil {
			return fmt.Errorf("decrement lot remaining quantity: %w", err)
		}
		stillNeeded -= consumed
	}

	if stillNeeded > 0 {
		allocID, err := newID()
		if err != nil {
			return err
		}
		if _, err := q.CreateSaleItemLotAllocation(ctx, sqlc.CreateSaleItemLotAllocationParams{
			ID: allocID, BusinessID: businessID, SaleItemID: saleItemID,
			StockLotID: pgtype.UUID{}, Quantity: stillNeeded, AllocatedCostKobo: pgtype.Int8{},
		}); err != nil {
			return fmt.Errorf("create pending lot allocation: %w", err)
		}
	}
	return nil
}

// reverseSaleItemLots mirrors every allocation the original sale item drew
// (docs/PHASE_FIFO_COSTING.md §6) onto the new reversal sale item, with
// negated quantity, giving quantity back to the exact lots it came from --
// never re-running the allocator, which could draw from a different lot
// than the one actually depleted.
func reverseSaleItemLots(ctx context.Context, q *sqlc.Queries, businessID, originalSaleItemID, reversalSaleItemID uuid.UUID) error {
	allocations, err := q.ListAllocationsBySaleItem(ctx, sqlc.ListAllocationsBySaleItemParams{
		BusinessID: businessID, SaleItemID: originalSaleItemID,
	})
	if err != nil {
		return fmt.Errorf("list allocations for reversal: %w", err)
	}

	for _, a := range allocations {
		allocID, err := newID()
		if err != nil {
			return err
		}
		if _, err := q.CreateSaleItemLotAllocation(ctx, sqlc.CreateSaleItemLotAllocationParams{
			ID: allocID, BusinessID: businessID, SaleItemID: reversalSaleItemID,
			StockLotID: a.StockLotID, Quantity: -a.Quantity, AllocatedCostKobo: negateInt8(a.AllocatedCostKobo),
		}); err != nil {
			return fmt.Errorf("create reversal lot allocation: %w", err)
		}
		if a.StockLotID.Valid {
			if _, err := q.IncrementLotRemainingQuantity(ctx, sqlc.IncrementLotRemainingQuantityParams{
				BusinessID: businessID, ID: uuid.UUID(a.StockLotID.Bytes), RemainingQuantity: a.Quantity,
			}); err != nil {
				return fmt.Errorf("restore lot remaining quantity: %w", err)
			}
		}
	}
	return nil
}

func negateInt8(v pgtype.Int8) pgtype.Int8 {
	if !v.Valid {
		return v
	}
	return pgtype.Int8{Int64: -v.Int64, Valid: true}
}
