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
			variant, err := q.GetVariantByID(ctx, sqlc.GetVariantByIDParams{BusinessID: businessID, ID: line.VariantID})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrVariantNotFound
				}
				return fmt.Errorf("look up variant: %w", err)
			}
			if !variant.TracksInventory {
				return ErrVariantNotTracked
			}

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
				ID: lotID, BusinessID: businessID, ReceiptLineID: pgUUID(postedLine.ID),
				VariantID: line.VariantID, LocationID: locationID,
				ReceivedQuantity: line.Quantity, TotalCostKobo: line.TotalCostKobo,
				ReceivedAt: pgTimestamptz(receivedAt), Source: "receipt",
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

// ListAllAdjustmentRequests returns every adjustment request regardless of
// status (pending, approved, or rejected), newest first -- unlike
// ListPendingAdjustmentRequests, which only ever returns the still-open
// approval queue. For platform admin's deep-drill view
// (docs/PHASE_PLATFORM_ADMIN_DEEP_DRILL.md), which needs to see what was
// already decided, not just what's still pending.
func (s *Service) ListAllAdjustmentRequests(ctx context.Context, userID, businessID uuid.UUID) ([]AdjustmentRequest, error) {
	var out []AdjustmentRequest
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		rows, err := q.ListAllInventoryAdjustmentRequestsDetailed(ctx, businessID)
		if err != nil {
			return err
		}
		out = make([]AdjustmentRequest, 0, len(rows))
		for _, r := range rows {
			out = append(out, toAllAdjustmentRequestDetailed(r))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list all adjustment requests: %w", err)
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

// AdminCorrectBalance directly corrects variantID's balance to
// targetQuantity, for a platform-admin-only case where a balance is wrong
// with no receipt/adjustment/sale trail that explains why (e.g. a bug).
// It never writes inventory_balances directly -- inventory_balances is
// documented as a rebuildable projection over inventory_movements, and
// this preserves that invariant exactly the way every other balance
// change in this codebase does: the target is converted to a signed delta
// (targetQuantity - liveBalance) and posted through the same adjustment-
// request ledger ApproveAdjustmentRequest already uses, with
// reason_category=AdjustmentReasonPlatformCorrection (a reason no business
// request can submit directly) and decidedBy/requestedBy both set to the
// acting platform actor. Reads the live balance and posts the correction
// inside one transaction, so a concurrent movement landing in between
// can never be silently overwritten by a stale target.
//
// This duplicates a few lines of ApproveAdjustmentRequest's own posting
// logic (event/movement/balance-upsert/negative-review) rather than
// calling RequestAdjustment then ApproveAdjustmentRequest as two separate
// calls -- each of those opens its own transaction, so composing them
// that way would mean a request could be created without ever being
// approved (or vice versa) if the second call failed. One transaction
// doing both is the only way to keep "the ledger entry and the balance
// change either both land or neither does" true here too.
func (s *Service) AdminCorrectBalance(ctx context.Context, userID, businessID, locationID, variantID, actorID uuid.UUID, targetQuantity int32, reasonNote string) (AdjustmentRequest, error) {
	requestID, err := newID()
	if err != nil {
		return AdjustmentRequest{}, err
	}
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
		liveBalance, err := q.GetInventoryBalance(ctx, sqlc.GetInventoryBalanceParams{
			BusinessID: businessID, VariantID: variantID, LocationID: locationID,
		})
		if err != nil {
			return fmt.Errorf("read live balance: %w", err)
		}
		delta := targetQuantity - liveBalance
		if delta == 0 {
			return ErrBalanceAlreadyCorrect
		}

		created, err := q.CreateInventoryAdjustmentRequest(ctx, sqlc.CreateInventoryAdjustmentRequestParams{
			ID: requestID, BusinessID: businessID, LocationID: locationID, VariantID: variantID,
			RequestedBy: actorID, IdempotencyKey: uuid.Must(uuid.NewV7()), QuantityDelta: delta,
			ReasonCategory: AdjustmentReasonPlatformCorrection, ReasonNote: reasonNote,
		})
		if err != nil {
			if pgErrorCode(err) == pgForeignKeyViolation {
				return ErrVariantNotFound
			}
			return fmt.Errorf("create platform correction request: %w", err)
		}

		approved, err := q.ApproveInventoryAdjustmentRequest(ctx, sqlc.ApproveInventoryAdjustmentRequestParams{
			BusinessID: businessID, ID: created.ID, DecidedBy: pgUUID(actorID),
			ResolutionNote: pgText("Approved automatically: platform corrections post pre-approved."),
		})
		if err != nil {
			return fmt.Errorf("approve platform correction request: %w", err)
		}

		if _, err := q.CreateInventoryEventForAdjustment(ctx, sqlc.CreateInventoryEventForAdjustmentParams{
			ID: eventID, BusinessID: businessID, ActorID: actorID, AdjustmentRequestID: pgUUID(approved.ID),
		}); err != nil {
			return fmt.Errorf("create inventory event: %w", err)
		}
		if _, err := q.CreateInventoryMovement(ctx, sqlc.CreateInventoryMovementParams{
			ID: movementID, BusinessID: businessID, EventID: eventID,
			VariantID: approved.VariantID, LocationID: approved.LocationID, QuantityDelta: approved.QuantityDelta,
		}); err != nil {
			return fmt.Errorf("create inventory movement: %w", err)
		}
		balance, err := q.UpsertInventoryBalanceDelta(ctx, sqlc.UpsertInventoryBalanceDeltaParams{
			BusinessID: businessID, VariantID: approved.VariantID, LocationID: approved.LocationID,
			Quantity: approved.QuantityDelta,
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
				VariantID: approved.VariantID, LocationID: approved.LocationID,
				RelatedMovementID: pgUUID(movementID),
			}); err != nil {
				return fmt.Errorf("create negative-inventory review: %w", err)
			}
		}

		result = toAdjustmentRequest(approved)
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
//
// resolvedUnitCostKobo is optional and only ever meaningful for a
// negative_inventory review that traces back to a sale's oversell (see
// docs/PHASE_FIFO_COSTING.md §5): when supplied, it closes out that
// sale item's pending FIFO cost allocation at this per-unit price,
// via a new synthetic lot -- never a fabricated cost, always a
// deliberate, reviewed decision. Harmless to supply when it doesn't
// apply (a stale-count review, or a negative_inventory review with
// nothing actually pending) -- resolvePendingAllocation is a no-op then.
func (s *Service) ResolveReview(ctx context.Context, userID, businessID, reviewID, resolvedBy uuid.UUID, note string, resolvedUnitCostKobo *int64) (InventoryReview, error) {
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

		if resolvedUnitCostKobo != nil && updated.Type == ReviewTypeNegativeInventory && updated.SaleItemID.Valid {
			saleItemID := uuid.UUID(updated.SaleItemID.Bytes)
			if err := resolvePendingAllocation(ctx, q, businessID, saleItemID, updated.VariantID, updated.LocationID, *resolvedUnitCostKobo); err != nil {
				return fmt.Errorf("resolve pending cost allocation: %w", err)
			}
		}
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

// GetStockReceipt returns one receipt's full breakdown (lines with variant/
// product names) for the activity feed's detail view, plus whether it has
// already been reversed.
func (s *Service) GetStockReceipt(ctx context.Context, userID, businessID, receiptID uuid.UUID) (ReceiptDetail, error) {
	var result ReceiptDetail
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		receipt, err := q.GetStockReceiptByID(ctx, sqlc.GetStockReceiptByIDParams{BusinessID: businessID, ID: receiptID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrReceiptNotFound
			}
			return fmt.Errorf("get stock receipt: %w", err)
		}
		lines, err := q.ListStockReceiptLinesDetailed(ctx, sqlc.ListStockReceiptLinesDetailedParams{
			BusinessID: businessID, ReceiptID: receiptID,
		})
		if err != nil {
			return fmt.Errorf("list stock receipt lines: %w", err)
		}
		reversalOfThis := uuid.Nil
		if reversal, err := q.GetReversalOfStockReceipt(ctx, sqlc.GetReversalOfStockReceiptParams{
			BusinessID: businessID, ReversalOfReceiptID: pgUUID(receiptID),
		}); err == nil {
			reversalOfThis = reversal.ID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check existing reversal: %w", err)
		}

		result = ReceiptDetail{
			ID: receipt.ID, ReceivedBy: receipt.ReceivedBy, ReceivedAt: toTime(receipt.ReceivedAt),
			ReversalOf: toUUID(receipt.ReversalOfReceiptID), ReversalOfThis: reversalOfThis,
		}
		for _, l := range lines {
			result.Lines = append(result.Lines, ReceiptLineDetail{
				ID: l.ID, VariantID: l.VariantID, VariantName: l.VariantName, ProductName: l.ProductName,
				Quantity: l.Quantity, TotalCostKobo: l.TotalCostKobo,
			})
		}
		return nil
	})
	if err != nil {
		return ReceiptDetail{}, err
	}
	return result, nil
}

// ReverseStockReceipt undoes a whole receipt by posting a new, equal-and-
// opposite receipt (negative quantities/costs) -- the original rows are
// never edited or deleted, same idiom as sales.ReverseSale/
// expenses.ReverseExpense. Only a receipt none of whose stock has been
// sold yet can be reversed (ErrReceiptPartiallyConsumed otherwise): giving
// back quantity to a lot that FIFO has already allocated from would either
// understate real sales or drive a lot negative, neither of which a plain
// undo should silently paper over.
func (s *Service) ReverseStockReceipt(ctx context.Context, userID, businessID, receiptID, actorID uuid.UUID) (ReceiptDetail, error) {
	reversalReceiptID, err := newID()
	if err != nil {
		return ReceiptDetail{}, err
	}
	eventID, err := newID()
	if err != nil {
		return ReceiptDetail{}, err
	}
	now := time.Now()

	var result ReceiptDetail
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		original, err := q.GetStockReceiptByID(ctx, sqlc.GetStockReceiptByIDParams{BusinessID: businessID, ID: receiptID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrReceiptNotFound
			}
			return fmt.Errorf("get stock receipt: %w", err)
		}
		if _, err := q.GetReversalOfStockReceipt(ctx, sqlc.GetReversalOfStockReceiptParams{
			BusinessID: businessID, ReversalOfReceiptID: pgUUID(receiptID),
		}); err == nil {
			return ErrReceiptAlreadyReversed
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check existing reversal: %w", err)
		}

		lines, err := q.ListStockReceiptLinesDetailed(ctx, sqlc.ListStockReceiptLinesDetailedParams{
			BusinessID: businessID, ReceiptID: receiptID,
		})
		if err != nil {
			return fmt.Errorf("list stock receipt lines: %w", err)
		}
		if len(lines) == 0 {
			return ErrReceiptNotFound
		}
		lineIDs := make([]uuid.UUID, len(lines))
		for i, l := range lines {
			lineIDs[i] = l.ID
		}
		lots, err := q.ListStockLotsByReceiptLineIDs(ctx, sqlc.ListStockLotsByReceiptLineIDsParams{
			BusinessID: businessID, ReceiptLineIds: lineIDs,
		})
		if err != nil {
			return fmt.Errorf("list stock lots: %w", err)
		}
		lotByLine := make(map[uuid.UUID]sqlc.StockLot, len(lots))
		for _, lot := range lots {
			lotByLine[toUUID(lot.ReceiptLineID)] = lot
		}
		for _, l := range lines {
			// The lot check catches stock a sale's FIFO allocation has
			// already drawn from (remaining_quantity tracks that, not a
			// plain adjustment). The balance check catches the other way
			// stock can have left since this receipt landed -- a manual/
			// complimentary/broken/spoiled adjustment, which never touches
			// stock_lots at all. Either one alone misses a real case the
			// other covers; a receipt is only safe to fully undo when
			// neither has happened.
			lot, ok := lotByLine[l.ID]
			if !ok || lot.RemainingQuantity != lot.ReceivedQuantity {
				return ErrReceiptPartiallyConsumed
			}
			balance, err := q.GetInventoryBalance(ctx, sqlc.GetInventoryBalanceParams{
				BusinessID: businessID, VariantID: l.VariantID, LocationID: original.LocationID,
			})
			if err != nil {
				return fmt.Errorf("get inventory balance: %w", err)
			}
			if int64(balance)-int64(l.Quantity) < 0 {
				return ErrReceiptPartiallyConsumed
			}
		}

		reversal, err := q.CreateReversalStockReceipt(ctx, sqlc.CreateReversalStockReceiptParams{
			ID: reversalReceiptID, BusinessID: businessID, LocationID: original.LocationID,
			ReceivedBy: actorID, ReceivedAt: pgTimestamptz(now), ReversalOfReceiptID: pgUUID(receiptID),
		})
		if err != nil {
			return fmt.Errorf("create reversal receipt: %w", err)
		}
		if _, err := q.CreateInventoryEventForReceiptReversal(ctx, sqlc.CreateInventoryEventForReceiptReversalParams{
			ID: eventID, BusinessID: businessID, ActorID: actorID, ReceiptID: pgUUID(reversal.ID),
		}); err != nil {
			return fmt.Errorf("create inventory event: %w", err)
		}

		result = ReceiptDetail{
			ID: reversal.ID, ReceivedBy: actorID, ReceivedAt: now, ReversalOf: receiptID,
		}
		for _, l := range lines {
			lot := lotByLine[l.ID]
			lineID, err := newID()
			if err != nil {
				return err
			}
			if _, err := q.CreateStockReceiptLine(ctx, sqlc.CreateStockReceiptLineParams{
				ID: lineID, BusinessID: businessID, ReceiptID: reversal.ID,
				VariantID: l.VariantID, Quantity: -l.Quantity, TotalCostKobo: -l.TotalCostKobo,
			}); err != nil {
				return fmt.Errorf("create reversal receipt line: %w", err)
			}
			movementID, err := newID()
			if err != nil {
				return err
			}
			if _, err := q.CreateInventoryMovement(ctx, sqlc.CreateInventoryMovementParams{
				ID: movementID, BusinessID: businessID, EventID: eventID,
				VariantID: l.VariantID, LocationID: original.LocationID, QuantityDelta: -l.Quantity,
			}); err != nil {
				return fmt.Errorf("create inventory movement: %w", err)
			}
			if _, err := q.UpsertInventoryBalanceDelta(ctx, sqlc.UpsertInventoryBalanceDeltaParams{
				BusinessID: businessID, VariantID: l.VariantID, LocationID: original.LocationID, Quantity: -l.Quantity,
			}); err != nil {
				return fmt.Errorf("update inventory balance: %w", err)
			}
			if _, err := q.DecrementLotRemainingQuantity(ctx, sqlc.DecrementLotRemainingQuantityParams{
				BusinessID: businessID, ID: lot.ID, RemainingQuantity: lot.ReceivedQuantity,
			}); err != nil {
				return fmt.Errorf("decrement lot remaining quantity: %w", err)
			}
			result.Lines = append(result.Lines, ReceiptLineDetail{
				ID: lineID, VariantID: l.VariantID, VariantName: l.VariantName, ProductName: l.ProductName,
				Quantity: -l.Quantity, TotalCostKobo: -l.TotalCostKobo,
			})
		}
		return nil
	})
	if err != nil {
		return ReceiptDetail{}, err
	}
	return result, nil
}

// AdminForceReverseStockReceipt is a platform-admin-only override of
// ReverseStockReceipt's "untouched" guard: it gives back whatever quantity
// of each line is still actually sitting in that line's lot
// (lot.RemainingQuantity), rather than requiring the whole receipt be
// untouched. The negative-balance guard is NOT relaxed -- it is a
// correctness invariant (inventory_balances must never go negative from a
// reversal), not a business-logic preference that only applies to the
// ordinary path, so a line that would drive the balance negative still
// blocks the whole reversal with ErrReceiptPartiallyConsumed, same as
// ReverseStockReceipt. A line with nothing left in its lot (fully
// consumed by sales since) is silently skipped -- "give back whatever's
// left" is the point of this method, not "give back everything or
// refuse." If no line has anything left, ErrReceiptFullyConsumed is
// returned before any row is written, since there would be nothing for
// the reversal to actually do.
//
// A partially-given-back line's reversal cost is computed proportionally
// (lineTotalCost * giveBack / originalQuantity, integer division) rather
// than reversing the line's full original cost -- the one place in this
// codebase a per-unit cost is derived rather than stored, accepted here
// specifically because this is an admin override of an already-unusual
// situation, not the normal receiving/reversal path
// docs/ARCHITECTURE.md §8.4's "store the total, never a rounded per-unit
// cost" rule protects.
func (s *Service) AdminForceReverseStockReceipt(ctx context.Context, userID, businessID, receiptID, actorID uuid.UUID) (ReceiptDetail, error) {
	reversalReceiptID, err := newID()
	if err != nil {
		return ReceiptDetail{}, err
	}
	eventID, err := newID()
	if err != nil {
		return ReceiptDetail{}, err
	}
	now := time.Now()

	type giveBackLine struct {
		line      sqlc.ListStockReceiptLinesDetailedRow
		lot       sqlc.StockLot
		giveBack  int32
		lineTotal int64
	}

	var result ReceiptDetail
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		original, err := q.GetStockReceiptByID(ctx, sqlc.GetStockReceiptByIDParams{BusinessID: businessID, ID: receiptID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrReceiptNotFound
			}
			return fmt.Errorf("get stock receipt: %w", err)
		}
		if _, err := q.GetReversalOfStockReceipt(ctx, sqlc.GetReversalOfStockReceiptParams{
			BusinessID: businessID, ReversalOfReceiptID: pgUUID(receiptID),
		}); err == nil {
			return ErrReceiptAlreadyReversed
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check existing reversal: %w", err)
		}

		lines, err := q.ListStockReceiptLinesDetailed(ctx, sqlc.ListStockReceiptLinesDetailedParams{
			BusinessID: businessID, ReceiptID: receiptID,
		})
		if err != nil {
			return fmt.Errorf("list stock receipt lines: %w", err)
		}
		if len(lines) == 0 {
			return ErrReceiptNotFound
		}
		lineIDs := make([]uuid.UUID, len(lines))
		for i, l := range lines {
			lineIDs[i] = l.ID
		}
		lots, err := q.ListStockLotsByReceiptLineIDs(ctx, sqlc.ListStockLotsByReceiptLineIDsParams{
			BusinessID: businessID, ReceiptLineIds: lineIDs,
		})
		if err != nil {
			return fmt.Errorf("list stock lots: %w", err)
		}
		lotByLine := make(map[uuid.UUID]sqlc.StockLot, len(lots))
		for _, lot := range lots {
			lotByLine[toUUID(lot.ReceiptLineID)] = lot
		}

		// First pass: decide, for every line, how much can safely be given
		// back -- without writing anything yet, so a receipt with nothing
		// left anywhere can be rejected (ErrReceiptFullyConsumed) before any
		// row exists, rather than leaving behind an empty reversal.
		toGiveBack := make([]giveBackLine, 0, len(lines))
		anyGiveBack := false
		for _, l := range lines {
			lot, ok := lotByLine[l.ID]
			if !ok {
				continue
			}
			giveBack := lot.RemainingQuantity
			if giveBack > l.Quantity {
				giveBack = l.Quantity
			}
			if giveBack <= 0 {
				continue
			}
			balance, err := q.GetInventoryBalance(ctx, sqlc.GetInventoryBalanceParams{
				BusinessID: businessID, VariantID: l.VariantID, LocationID: original.LocationID,
			})
			if err != nil {
				return fmt.Errorf("get inventory balance: %w", err)
			}
			if int64(balance)-int64(giveBack) < 0 {
				return ErrReceiptPartiallyConsumed
			}
			anyGiveBack = true
			toGiveBack = append(toGiveBack, giveBackLine{
				line: l, lot: lot, giveBack: giveBack,
				lineTotal: l.TotalCostKobo * int64(giveBack) / int64(l.Quantity),
			})
		}
		if !anyGiveBack {
			return ErrReceiptFullyConsumed
		}

		reversal, err := q.CreateReversalStockReceipt(ctx, sqlc.CreateReversalStockReceiptParams{
			ID: reversalReceiptID, BusinessID: businessID, LocationID: original.LocationID,
			ReceivedBy: actorID, ReceivedAt: pgTimestamptz(now), ReversalOfReceiptID: pgUUID(receiptID),
		})
		if err != nil {
			return fmt.Errorf("create reversal receipt: %w", err)
		}
		if _, err := q.CreateInventoryEventForReceiptReversal(ctx, sqlc.CreateInventoryEventForReceiptReversalParams{
			ID: eventID, BusinessID: businessID, ActorID: actorID, ReceiptID: pgUUID(reversal.ID),
		}); err != nil {
			return fmt.Errorf("create inventory event: %w", err)
		}

		result = ReceiptDetail{ID: reversal.ID, ReceivedBy: actorID, ReceivedAt: now, ReversalOf: receiptID}
		for _, g := range toGiveBack {
			lineID, err := newID()
			if err != nil {
				return err
			}
			if _, err := q.CreateStockReceiptLine(ctx, sqlc.CreateStockReceiptLineParams{
				ID: lineID, BusinessID: businessID, ReceiptID: reversal.ID,
				VariantID: g.line.VariantID, Quantity: -g.giveBack, TotalCostKobo: -g.lineTotal,
			}); err != nil {
				return fmt.Errorf("create reversal receipt line: %w", err)
			}
			movementID, err := newID()
			if err != nil {
				return err
			}
			if _, err := q.CreateInventoryMovement(ctx, sqlc.CreateInventoryMovementParams{
				ID: movementID, BusinessID: businessID, EventID: eventID,
				VariantID: g.line.VariantID, LocationID: original.LocationID, QuantityDelta: -g.giveBack,
			}); err != nil {
				return fmt.Errorf("create inventory movement: %w", err)
			}
			if _, err := q.UpsertInventoryBalanceDelta(ctx, sqlc.UpsertInventoryBalanceDeltaParams{
				BusinessID: businessID, VariantID: g.line.VariantID, LocationID: original.LocationID, Quantity: -g.giveBack,
			}); err != nil {
				return fmt.Errorf("update inventory balance: %w", err)
			}
			if _, err := q.DecrementLotRemainingQuantity(ctx, sqlc.DecrementLotRemainingQuantityParams{
				BusinessID: businessID, ID: g.lot.ID, RemainingQuantity: g.giveBack,
			}); err != nil {
				return fmt.Errorf("decrement lot remaining quantity: %w", err)
			}
			result.Lines = append(result.Lines, ReceiptLineDetail{
				ID: lineID, VariantID: g.line.VariantID, VariantName: g.line.VariantName, ProductName: g.line.ProductName,
				Quantity: -g.giveBack, TotalCostKobo: -g.lineTotal,
			})
		}
		return nil
	})
	if err != nil {
		return ReceiptDetail{}, err
	}
	return result, nil
}

// GetAdjustmentRequest returns one adjustment request (any status) with its
// variant/product names, for the activity feed's detail view.
func (s *Service) GetAdjustmentRequest(ctx context.Context, userID, businessID, requestID uuid.UUID) (AdjustmentRequest, error) {
	var result AdjustmentRequest
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetInventoryAdjustmentRequestDetailed(ctx, sqlc.GetInventoryAdjustmentRequestDetailedParams{
			BusinessID: businessID, ID: requestID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrAdjustmentRequestNotFound
			}
			return fmt.Errorf("get adjustment request: %w", err)
		}
		result = AdjustmentRequest{
			ID: row.ID, LocationID: row.LocationID, VariantID: row.VariantID, RequestedBy: row.RequestedBy,
			QuantityDelta: row.QuantityDelta, ReasonCategory: row.ReasonCategory, ReasonNote: row.ReasonNote,
			SourceCountLineID: toUUID(row.SourceCountLineID), Status: row.Status, CreatedAt: toTime(row.CreatedAt),
			VariantName: row.VariantName, ProductName: row.ProductName,
			DecidedBy: toUUID(row.DecidedBy), DecidedAt: toTime(row.DecidedAt), ResolutionNote: toText(row.ResolutionNote),
		}
		return nil
	})
	if err != nil {
		return AdjustmentRequest{}, err
	}
	return result, nil
}
