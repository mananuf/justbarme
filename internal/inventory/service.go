package inventory

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/store"
	"github.com/mananuf/justbarme/internal/store/sqlc"
)

type Service struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func newID() (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("generate id: %w", err)
	}
	return id, nil
}

// ReceiveStock posts one stock receipt covering lines, all in a single
// transaction: the receipt header, one inventory_events row, and per line
// a stock_receipt_line, an immutable stock_lot, a signed
// inventory_movements row, and an atomic increment to inventory_balances.
// This is the same "action and its ledger entries either all land or none
// do" atomicity pattern internal/catalogue.Service.SetVariantPrice and
// internal/platformadmin.Service.SuspendBusiness already use.
//
// locationID is resolved by the caller (identity.Service.GetDefaultLocation)
// -- this package does not depend on internal/identity, matching
// docs/ARCHITECTURE.md's "each feature package stays self-contained".
func (s *Service) ReceiveStock(ctx context.Context, userID, businessID, locationID uuid.UUID, lines []ReceiptLine) (Receipt, error) {
	if len(lines) == 0 {
		return Receipt{}, ErrNoLines
	}

	receiptID, err := newID()
	if err != nil {
		return Receipt{}, err
	}
	eventID, err := newID()
	if err != nil {
		return Receipt{}, err
	}
	receivedAt := time.Now()

	var result Receipt
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		receipt, err := q.CreateStockReceipt(ctx, sqlc.CreateStockReceiptParams{
			ID: receiptID, BusinessID: businessID, LocationID: locationID,
			ReceivedBy: userID, ReceivedAt: pgTimestamptz(receivedAt),
		})
		if err != nil {
			return fmt.Errorf("create stock receipt: %w", err)
		}

		if _, err := q.CreateInventoryEvent(ctx, sqlc.CreateInventoryEventParams{
			ID: eventID, BusinessID: businessID, Type: "receipt", ActorID: userID, ReceiptID: pgUUID(receipt.ID),
		}); err != nil {
			return fmt.Errorf("create inventory event: %w", err)
		}

		result = Receipt{
			ID: receipt.ID, BusinessID: businessID, LocationID: locationID,
			ReceivedBy: userID, ReceivedAt: receivedAt,
		}

		for _, line := range lines {
			lineID, err := newID()
			if err != nil {
				return err
			}
			postedLine, err := q.CreateStockReceiptLine(ctx, sqlc.CreateStockReceiptLineParams{
				ID: lineID, BusinessID: businessID, ReceiptID: receipt.ID,
				VariantID: line.VariantID, Quantity: line.Quantity, TotalCostKobo: line.TotalCostKobo,
			})
			if err != nil {
				return fmt.Errorf("create stock receipt line: %w", err)
			}

			lotID, err := newID()
			if err != nil {
				return err
			}
			if _, err := q.CreateStockLot(ctx, sqlc.CreateStockLotParams{
				ID: lotID, BusinessID: businessID, ReceiptLineID: postedLine.ID,
				VariantID: line.VariantID, LocationID: locationID,
				ReceivedQuantity: line.Quantity, TotalCostKobo: line.TotalCostKobo,
				ReceivedAt: pgTimestamptz(receivedAt),
			}); err != nil {
				return fmt.Errorf("create stock lot: %w", err)
			}

			movementID, err := newID()
			if err != nil {
				return err
			}
			if _, err := q.CreateInventoryMovement(ctx, sqlc.CreateInventoryMovementParams{
				ID: movementID, BusinessID: businessID, EventID: eventID,
				VariantID: line.VariantID, LocationID: locationID, QuantityDelta: line.Quantity,
			}); err != nil {
				return fmt.Errorf("create inventory movement: %w", err)
			}

			balance, err := q.UpsertInventoryBalanceDelta(ctx, sqlc.UpsertInventoryBalanceDeltaParams{
				BusinessID: businessID, VariantID: line.VariantID, LocationID: locationID, Quantity: line.Quantity,
			})
			if err != nil {
				return fmt.Errorf("update inventory balance: %w", err)
			}

			result.Lines = append(result.Lines, toReceiptLineResult(postedLine, int64(balance.Quantity)))
		}
		return nil
	})
	if err != nil {
		if pgErrorCode(err) == pgForeignKeyViolation {
			return Receipt{}, ErrVariantNotFound
		}
		return Receipt{}, err
	}
	return result, nil
}

// GetBalances returns current stock quantity per variant, summed across
// locations (the pilot has exactly one location per business, but summing
// keeps this correct if that changes before a location-aware UI exists).
// A variant with no receipts yet is simply absent from the map -- callers
// treat a missing entry as zero, not an error.
func (s *Service) GetBalances(ctx context.Context, userID, businessID uuid.UUID) (map[uuid.UUID]int64, error) {
	balances := make(map[uuid.UUID]int64)
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		rows, err := q.ListInventoryBalances(ctx, businessID)
		if err != nil {
			return err
		}
		for _, row := range rows {
			balances[row.VariantID] += int64(row.Quantity)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list inventory balances: %w", err)
	}
	return balances, nil
}
