package inventory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// SubmitStockCount posts one physical count, idempotently (idempotencyKey
// is client-generated, safe to retry after a lost response or a stretch
// offline -- see docs/PHASE_INVENTORY_COUNTS_AND_ADJUSTMENTS.md §4). For
// each line, staleness is decided by comparing the caller's own submitted
// ExpectedQuantity against the live balance read inside this same
// transaction, never by comparing timestamps: if they match, nothing else
// touched this variant since the device formed its belief, and any
// nonzero variance becomes a pending AdjustmentRequest
// (ReasonCategory=count_correction); if they don't match, the line is
// marked stale and an open InventoryReview (stale_stock_count) is created
// instead of guessing which value is right.
func (s *Service) SubmitStockCount(
	ctx context.Context, userID, businessID, locationID, countedBy uuid.UUID,
	idempotencyKey uuid.UUID, startedAt time.Time, lines []StockCountLineInput,
) (StockCount, error) {
	if len(lines) == 0 {
		return StockCount{}, ErrNoCountLines
	}

	countID, err := newID()
	if err != nil {
		return StockCount{}, err
	}

	var result StockCount
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		if existing, err := q.GetStockCountByIdempotencyKey(ctx, sqlc.GetStockCountByIdempotencyKeyParams{
			BusinessID: businessID, IdempotencyKey: idempotencyKey,
		}); err == nil {
			replay, err := loadStockCountAggregate(ctx, q, businessID, existing)
			if err != nil {
				return err
			}
			result = replay
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check idempotency key: %w", err)
		}

		count, err := q.CreateStockCount(ctx, sqlc.CreateStockCountParams{
			ID: countID, BusinessID: businessID, LocationID: locationID, CountedBy: countedBy,
			IdempotencyKey: idempotencyKey, StartedAt: pgTimestamptz(startedAt),
		})
		if err != nil {
			return fmt.Errorf("create stock count: %w", err)
		}
		result = toStockCount(count)

		for _, line := range lines {
			liveBalance, err := q.GetInventoryBalance(ctx, sqlc.GetInventoryBalanceParams{
				BusinessID: businessID, VariantID: line.VariantID, LocationID: locationID,
			})
			if err != nil {
				return fmt.Errorf("read live balance: %w", err)
			}
			isStale := liveBalance != line.ExpectedQuantity
			variance := line.PhysicalQuantity - line.ExpectedQuantity

			lineID, err := newID()
			if err != nil {
				return err
			}
			postedLine, err := q.CreateStockCountLine(ctx, sqlc.CreateStockCountLineParams{
				ID: lineID, BusinessID: businessID, CountID: count.ID, VariantID: line.VariantID,
				ExpectedQuantity: line.ExpectedQuantity, PhysicalQuantity: line.PhysicalQuantity,
				Variance: variance, IsStale: isStale,
			})
			if err != nil {
				if pgErrorCode(err) == pgForeignKeyViolation {
					return ErrVariantNotFound
				}
				return fmt.Errorf("create stock count line: %w", err)
			}
			result.Lines = append(result.Lines, toStockCountLineResult(postedLine))

			if isStale {
				reviewID, err := newID()
				if err != nil {
					return err
				}
				if _, err := q.CreateInventoryReview(ctx, sqlc.CreateInventoryReviewParams{
					ID: reviewID, BusinessID: businessID, Type: ReviewTypeStaleStockCount,
					VariantID: line.VariantID, LocationID: locationID,
					RelatedCountLineID: pgUUID(postedLine.ID),
				}); err != nil {
					return fmt.Errorf("create stale-count review: %w", err)
				}
				continue
			}
			if variance == 0 {
				continue
			}
			requestID, err := newID()
			if err != nil {
				return err
			}
			if _, err := q.CreateInventoryAdjustmentRequest(ctx, sqlc.CreateInventoryAdjustmentRequestParams{
				ID: requestID, BusinessID: businessID, LocationID: locationID, VariantID: line.VariantID,
				RequestedBy: countedBy, IdempotencyKey: uuid.Must(uuid.NewV7()), QuantityDelta: variance,
				ReasonCategory: AdjustmentReasonCountCorrection, ReasonNote: "From physical count",
				SourceCountLineID: pgUUID(postedLine.ID),
			}); err != nil {
				return fmt.Errorf("create count-correction adjustment request: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return StockCount{}, err
	}
	return result, nil
}

func loadStockCountAggregate(ctx context.Context, q *sqlc.Queries, businessID uuid.UUID, count sqlc.StockCount) (StockCount, error) {
	result := toStockCount(count)
	lines, err := q.ListStockCountLines(ctx, sqlc.ListStockCountLinesParams{BusinessID: businessID, CountID: count.ID})
	if err != nil {
		return StockCount{}, fmt.Errorf("list stock count lines: %w", err)
	}
	for _, l := range lines {
		result.Lines = append(result.Lines, toStockCountLineResult(l))
	}
	return result, nil
}

// RequestAdjustment submits a standalone adjustment request (complimentary/
// broken/spoiled/staff_use/manual -- not sourced from a stock count, which
// creates its own count_correction requests directly inside
// SubmitStockCount). Idempotent, same reasoning as SubmitStockCount.
func (s *Service) RequestAdjustment(
	ctx context.Context, userID, businessID, locationID, variantID, requestedBy uuid.UUID,
	idempotencyKey uuid.UUID, quantityDelta int32, reasonCategory, reasonNote string,
) (AdjustmentRequest, error) {
	requestID, err := newID()
	if err != nil {
		return AdjustmentRequest{}, err
	}

	var result AdjustmentRequest
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		if existing, err := q.GetInventoryAdjustmentRequestByIdempotencyKey(ctx, sqlc.GetInventoryAdjustmentRequestByIdempotencyKeyParams{
			BusinessID: businessID, IdempotencyKey: idempotencyKey,
		}); err == nil {
			result = toAdjustmentRequest(existing)
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check idempotency key: %w", err)
		}

		created, err := q.CreateInventoryAdjustmentRequest(ctx, sqlc.CreateInventoryAdjustmentRequestParams{
			ID: requestID, BusinessID: businessID, LocationID: locationID, VariantID: variantID,
			RequestedBy: requestedBy, IdempotencyKey: idempotencyKey, QuantityDelta: quantityDelta,
			ReasonCategory: reasonCategory, ReasonNote: reasonNote,
		})
		if err != nil {
			if pgErrorCode(err) == pgForeignKeyViolation {
				return ErrVariantNotFound
			}
			return fmt.Errorf("create adjustment request: %w", err)
		}
		result = toAdjustmentRequest(created)
		return nil
	})
	if err != nil {
		return AdjustmentRequest{}, err
	}
	return result, nil
}

// ListPendingAdjustmentRequests returns every not-yet-decided adjustment
// request, newest first, with enough product/variant context to act on.
func (s *Service) ListPendingAdjustmentRequests(ctx context.Context, userID, businessID uuid.UUID) ([]AdjustmentRequest, error) {
	var out []AdjustmentRequest
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		rows, err := q.ListPendingInventoryAdjustmentRequestsDetailed(ctx, businessID)
		if err != nil {
			return err
		}
		out = make([]AdjustmentRequest, 0, len(rows))
		for _, r := range rows {
			out = append(out, toAdjustmentRequestDetailed(r))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list pending adjustment requests: %w", err)
	}
	return out, nil
}

// ApproveAdjustmentRequest is the only path that actually posts an
// inventory movement for an adjustment -- rejecting never does. Posts the
// inventory_events/movement/balance update in the same transaction as
// marking the request approved. If the resulting balance is negative, this
// opens the same negative_inventory review a sale-driven oversell would --
// an owner-approved write-off that turns out to push stock below zero
// deserves the same visibility as a concurrent offline oversell.
func (s *Service) ApproveAdjustmentRequest(ctx context.Context, userID, businessID, requestID, decidedBy uuid.UUID, note string) (AdjustmentRequest, error) {
	eventID, err := newID()
	if err != nil {
		return AdjustmentRequest{}, err
	}
	movementID, err := newID()
	if err != nil {
		return AdjustmentRequest{}, err
	}

	var result AdjustmentRequest
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		updated, err := q.ApproveInventoryAdjustmentRequest(ctx, sqlc.ApproveInventoryAdjustmentRequestParams{
			BusinessID: businessID, ID: requestID, DecidedBy: pgUUID(decidedBy), ResolutionNote: pgText(note),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrAdjustmentRequestNotFound
			}
			return fmt.Errorf("approve adjustment request: %w", err)
		}

		if _, err := q.CreateInventoryEventForAdjustment(ctx, sqlc.CreateInventoryEventForAdjustmentParams{
			ID: eventID, BusinessID: businessID, ActorID: decidedBy, AdjustmentRequestID: pgUUID(updated.ID),
		}); err != nil {
			return fmt.Errorf("create inventory event: %w", err)
		}
		if _, err := q.CreateInventoryMovement(ctx, sqlc.CreateInventoryMovementParams{
			ID: movementID, BusinessID: businessID, EventID: eventID,
			VariantID: updated.VariantID, LocationID: updated.LocationID, QuantityDelta: updated.QuantityDelta,
		}); err != nil {
			return fmt.Errorf("create inventory movement: %w", err)
		}
		balance, err := q.UpsertInventoryBalanceDelta(ctx, sqlc.UpsertInventoryBalanceDeltaParams{
			BusinessID: businessID, VariantID: updated.VariantID, LocationID: updated.LocationID,
			Quantity: updated.QuantityDelta,
		})
		if err != nil {
			return fmt.Errorf("update inventory balance: %w", err)
		}
		if balance.Quantity < 0 {
			reviewID, err := newID()
			if err != nil {
				return err
			}
			if _, err := q.CreateInventoryReview(ctx, sqlc.CreateInventoryReviewParams{
				ID: reviewID, BusinessID: businessID, Type: ReviewTypeNegativeInventory,
				VariantID: updated.VariantID, LocationID: updated.LocationID,
				RelatedMovementID: pgUUID(movementID),
			}); err != nil {
				return fmt.Errorf("create negative-inventory review: %w", err)
			}
		}

		result = toAdjustmentRequest(updated)
		return nil
	})
	if err != nil {
		return AdjustmentRequest{}, err
	}
	return result, nil
}

// RejectAdjustmentRequest marks a pending request rejected. No movement
// ever posts -- stock is completely unaffected.
func (s *Service) RejectAdjustmentRequest(ctx context.Context, userID, businessID, requestID, decidedBy uuid.UUID, note string) (AdjustmentRequest, error) {
	var result AdjustmentRequest
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		updated, err := q.RejectInventoryAdjustmentRequest(ctx, sqlc.RejectInventoryAdjustmentRequestParams{
			BusinessID: businessID, ID: requestID, DecidedBy: pgUUID(decidedBy), ResolutionNote: pgText(note),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrAdjustmentRequestNotFound
			}
			return fmt.Errorf("reject adjustment request: %w", err)
		}
		result = toAdjustmentRequest(updated)
		return nil
	})
	if err != nil {
		return AdjustmentRequest{}, err
	}
	return result, nil
}

// ListOpenReviews returns every open inventory review (negative-inventory
// or stale-count), newest first, with enough product/variant/count context
// to act on.
func (s *Service) ListOpenReviews(ctx context.Context, userID, businessID uuid.UUID) ([]InventoryReview, error) {
	var out []InventoryReview
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		rows, err := q.ListOpenInventoryReviewsDetailed(ctx, businessID)
		if err != nil {
			return err
		}
		out = make([]InventoryReview, 0, len(rows))
		for _, r := range rows {
			out = append(out, toInventoryReviewDetailed(r))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list open inventory reviews: %w", err)
	}
	return out, nil
}

// ResolveReview marks a review acknowledged, with a required note -- like
// internal/sales's sale reviews, this never changes the movement or count
// it's attached to, it's purely informational.
func (s *Service) ResolveReview(ctx context.Context, userID, businessID, reviewID, resolvedBy uuid.UUID, note string) (InventoryReview, error) {
	var result InventoryReview
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		updated, err := q.ResolveInventoryReview(ctx, sqlc.ResolveInventoryReviewParams{
			BusinessID: businessID, ID: reviewID, ResolvedBy: pgUUID(resolvedBy), ResolvedNote: pgText(note),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInventoryReviewNotFound
			}
			return fmt.Errorf("resolve inventory review: %w", err)
		}
		result = toInventoryReview(updated)
		return nil
	})
	if err != nil {
		return InventoryReview{}, err
	}
	return result, nil
}

// GetHistory returns one variant's stock history, most recent first --
// receipts, sale-driven movements, and adjustments (with their reason)
// in one chronological feed.
func (s *Service) GetHistory(ctx context.Context, userID, businessID, variantID uuid.UUID, limit int32) ([]MovementHistoryEntry, error) {
	var out []MovementHistoryEntry
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		rows, err := q.ListInventoryMovementsDetailed(ctx, sqlc.ListInventoryMovementsDetailedParams{
			BusinessID: businessID, VariantID: variantID, Limit: limit,
		})
		if err != nil {
			return err
		}
		out = make([]MovementHistoryEntry, 0, len(rows))
		for _, m := range rows {
			out = append(out, toMovementHistoryEntry(m))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list inventory history: %w", err)
	}
	return out, nil
}
