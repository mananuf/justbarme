package sales

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

// CreateSale posts one walk-in sale: a bill, the sale, its items, one
// payment, the matching inventory deduction (negative movements +
// balance), and one inventory_events row, all in a single transaction --
// the same non-negotiable atomicity pattern as
// inventory.Service.ReceiveStock and catalogue.Service.SetVariantPrice.
//
// idempotencyKey makes a retried POST safe: calling this again with the
// same key returns the sale already posted the first time (docs/
// IMPLEMENTATION_PLAN.md Phase 5's "retry does not duplicate it"), rather
// than creating a second sale or erroring. The check happens first, inside
// the transaction, so a genuine retry does no other writes at all.
//
// A sale is never rejected for referencing a deactivated variant, or a
// price that was never actually in effect at occurredAt -- it posts
// exactly as submitted either way, and a Review row opens instead, for the
// owner to look at later; only a variant that does not exist at all is
// rejected (ErrVariantNotFound), since that is a malformed request, not
// staleness.
func (s *Service) CreateSale(
	ctx context.Context,
	userID, businessID, locationID, sellerID uuid.UUID,
	idempotencyKey uuid.UUID,
	occurredAt time.Time,
	items []SaleItemInput,
	payment PaymentInput,
) (Sale, error) {
	if len(items) == 0 {
		return Sale{}, ErrNoItems
	}

	billID, err := newID()
	if err != nil {
		return Sale{}, err
	}
	saleID, err := newID()
	if err != nil {
		return Sale{}, err
	}
	paymentID, err := newID()
	if err != nil {
		return Sale{}, err
	}
	receivedAt := time.Now()

	var result Sale
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		if existing, err := q.GetSaleByIdempotencyKey(ctx, sqlc.GetSaleByIdempotencyKeyParams{
			BusinessID: businessID, IdempotencyKey: idempotencyKey,
		}); err == nil {
			replay, err := loadSaleAggregate(ctx, q, businessID, existing.ID)
			if err != nil {
				return err
			}
			result = replay
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check idempotency key: %w", err)
		}

		bill, err := q.CreateBill(ctx, sqlc.CreateBillParams{
			ID: billID, BusinessID: businessID, LocationID: locationID,
			Status: BillStatusSettled, OpenedBy: userID, OpenedAt: pgTimestamptz(occurredAt),
		})
		if err != nil {
			return fmt.Errorf("create bill: %w", err)
		}

		sale, err := postSaleRound(ctx, q, businessID, bill.ID, sellerID, locationID, saleID, idempotencyKey, occurredAt, receivedAt, items)
		if err != nil {
			return err
		}

		postedPayment, err := q.CreatePayment(ctx, sqlc.CreatePaymentParams{
			ID: paymentID, BusinessID: businessID, BillID: bill.ID,
			AmountKobo: payment.AmountKobo, Method: payment.Method, ActorID: userID,
		})
		if err != nil {
			return fmt.Errorf("create payment: %w", err)
		}

		sale.Payment = toPayment(postedPayment)
		result = sale
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrVariantNotFound) {
			return Sale{}, ErrVariantNotFound
		}
		return Sale{}, err
	}
	return result, nil
}

// postSaleRound resolves and posts one sale round (items, inventory
// movements, staleness reviews) onto an already-existing bill -- it does
// not create the bill or a payment, so it is shared by CreateSale (the
// walk-in path, which wraps a brand-new bill and an immediate payment
// around this) and AddSaleRound (which appends a round to an already-open
// tab, with no payment at all). Resolution happens first, no writes yet:
// every child row below (sale items, the inventory event, movements,
// reviews) has a foreign key back to the sales row, so that row must
// exist before any of them can be inserted -- which means the total it
// needs has to be known first.
func postSaleRound(
	ctx context.Context, q *sqlc.Queries,
	businessID, billID, sellerID, locationID, saleID uuid.UUID,
	idempotencyKey uuid.UUID,
	occurredAt, receivedAt time.Time,
	items []SaleItemInput,
) (Sale, error) {
	eventID, err := newID()
	if err != nil {
		return Sale{}, err
	}

	type resolvedItem struct {
		variantID     uuid.UUID
		description   string
		quantity      int32
		unitPriceKobo int64
		lineTotalKobo int64
		reviewReason  string // "" if none
	}
	resolved := make([]resolvedItem, 0, len(items))
	var totalKobo int64
	for _, item := range items {
		variant, err := q.GetVariantWithProductNameByID(ctx, sqlc.GetVariantWithProductNameByIDParams{BusinessID: businessID, ID: item.VariantID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Sale{}, ErrVariantNotFound
			}
			return Sale{}, fmt.Errorf("look up variant: %w", err)
		}

		lineTotal := int64(item.Quantity) * item.UnitPriceKobo
		totalKobo += lineTotal

		reviewReason := ""
		if !variant.Active {
			reviewReason = ReviewReasonDeactivatedVariant
		} else if price, err := q.GetPriceAt(ctx, sqlc.GetPriceAtParams{
			BusinessID: businessID, VariantID: item.VariantID, ValidFrom: pgTimestamptz(occurredAt),
		}); err != nil || price.AmountKobo != item.UnitPriceKobo {
			reviewReason = ReviewReasonPriceMismatch
		}

		resolved = append(resolved, resolvedItem{
			variantID: item.VariantID, description: variant.ProductName + " — " + variant.Name, quantity: item.Quantity,
			unitPriceKobo: item.UnitPriceKobo, lineTotalKobo: lineTotal, reviewReason: reviewReason,
		})
	}

	sale, err := q.CreateSale(ctx, sqlc.CreateSaleParams{
		ID: saleID, BusinessID: businessID, BillID: billID, SellerID: sellerID,
		IdempotencyKey: idempotencyKey, OccurredAt: pgTimestamptz(occurredAt),
		ReceivedAt: pgTimestamptz(receivedAt), TotalKobo: totalKobo,
	})
	if err != nil {
		return Sale{}, fmt.Errorf("create sale: %w", err)
	}

	if _, err := q.CreateInventoryEventForSale(ctx, sqlc.CreateInventoryEventForSaleParams{
		ID: eventID, BusinessID: businessID, Type: "sale", ActorID: sellerID, SaleID: pgUUID(saleID),
	}); err != nil {
		return Sale{}, fmt.Errorf("create inventory event: %w", err)
	}

	var postedItems []SaleItem
	var reviews []Review
	for _, r := range resolved {
		itemID, err := newID()
		if err != nil {
			return Sale{}, err
		}
		postedItem, err := q.CreateSaleItem(ctx, sqlc.CreateSaleItemParams{
			ID: itemID, BusinessID: businessID, SaleID: saleID, VariantID: r.variantID,
			Description: r.description, Quantity: r.quantity,
			UnitPriceKobo: r.unitPriceKobo, LineTotalKobo: r.lineTotalKobo,
		})
		if err != nil {
			return Sale{}, fmt.Errorf("create sale item: %w", err)
		}
		postedItems = append(postedItems, toSaleItem(postedItem))

		movementID, err := newID()
		if err != nil {
			return Sale{}, err
		}
		if _, err := q.CreateInventoryMovement(ctx, sqlc.CreateInventoryMovementParams{
			ID: movementID, BusinessID: businessID, EventID: eventID,
			VariantID: r.variantID, LocationID: locationID, QuantityDelta: -r.quantity,
		}); err != nil {
			return Sale{}, fmt.Errorf("create inventory movement: %w", err)
		}
		if _, err := q.UpsertInventoryBalanceDelta(ctx, sqlc.UpsertInventoryBalanceDeltaParams{
			BusinessID: businessID, VariantID: r.variantID, LocationID: locationID, Quantity: -r.quantity,
		}); err != nil {
			return Sale{}, fmt.Errorf("update inventory balance: %w", err)
		}

		if r.reviewReason != "" {
			reviewID, err := newID()
			if err != nil {
				return Sale{}, err
			}
			created, err := q.CreateSaleReview(ctx, sqlc.CreateSaleReviewParams{
				ID: reviewID, BusinessID: businessID, SaleID: sale.ID, SaleItemID: pgUUID(postedItem.ID), Reason: r.reviewReason,
			})
			if err != nil {
				return Sale{}, fmt.Errorf("create sale review: %w", err)
			}
			reviews = append(reviews, toReview(created))
		}
	}

	return Sale{
		ID: sale.ID, BusinessID: businessID, BillID: billID, LocationID: locationID, SellerID: sellerID,
		OccurredAt: occurredAt, ReceivedAt: receivedAt, TotalKobo: totalKobo,
		Items: postedItems, Reviews: reviews,
	}, nil
}

// loadSaleAggregate assembles a sale with its items, payment, and reviews
// -- shared by CreateSale's idempotent-replay path and GetSale/ReverseSale.
func loadSaleAggregate(ctx context.Context, q *sqlc.Queries, businessID, saleID uuid.UUID) (Sale, error) {
	saleRow, err := q.GetSaleByID(ctx, sqlc.GetSaleByIDParams{BusinessID: businessID, ID: saleID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Sale{}, ErrSaleNotFound
		}
		return Sale{}, fmt.Errorf("get sale: %w", err)
	}
	sale := toSale(saleRow)

	bill, err := q.GetBillByID(ctx, sqlc.GetBillByIDParams{BusinessID: businessID, ID: sale.BillID})
	if err != nil {
		return Sale{}, fmt.Errorf("get bill: %w", err)
	}
	sale.LocationID = bill.LocationID

	itemRows, err := q.ListSaleItemsBySaleID(ctx, sqlc.ListSaleItemsBySaleIDParams{BusinessID: businessID, SaleID: saleID})
	if err != nil {
		return Sale{}, fmt.Errorf("list sale items: %w", err)
	}
	for _, i := range itemRows {
		sale.Items = append(sale.Items, toSaleItem(i))
	}

	paymentRows, err := q.ListPaymentsByBillID(ctx, sqlc.ListPaymentsByBillIDParams{BusinessID: businessID, BillID: sale.BillID})
	if err != nil {
		return Sale{}, fmt.Errorf("list payments: %w", err)
	}
	if len(paymentRows) > 0 {
		sale.Payment = toPayment(paymentRows[0])
	}

	return sale, nil
}

// GetSale returns a posted sale with its items, payment, and any reviews.
func (s *Service) GetSale(ctx context.Context, userID, businessID, saleID uuid.UUID) (Sale, error) {
	var result Sale
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		sale, err := loadSaleAggregate(ctx, q, businessID, saleID)
		if err != nil {
			return err
		}
		result = sale
		return nil
	})
	if err != nil {
		return Sale{}, err
	}
	return result, nil
}

// ListSales returns the most recent sales for a business, newest first.
func (s *Service) ListSales(ctx context.Context, userID, businessID uuid.UUID, limit, offset int32) ([]Sale, error) {
	var out []Sale
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		rows, err := q.ListSales(ctx, sqlc.ListSalesParams{BusinessID: businessID, Limit: limit, Offset: offset})
		if err != nil {
			return err
		}
		out = make([]Sale, 0, len(rows))
		for _, r := range rows {
			sale := toSale(r)
			// Each sale has exactly one payment in this slice (no partial/
			// tab payments yet) -- one extra lookup per row, bounded by
			// limit (capped at 200 in the HTTP handler), not worth a
			// second bulk query for a dashboard-sized list.
			payments, err := q.ListPaymentsByBillID(ctx, sqlc.ListPaymentsByBillIDParams{BusinessID: businessID, BillID: sale.BillID})
			if err != nil {
				return fmt.Errorf("list payment for sale %s: %w", sale.ID, err)
			}
			if len(payments) > 0 {
				sale.Payment = toPayment(payments[0])
			}
			out = append(out, sale)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list sales: %w", err)
	}
	return out, nil
}

// SumSalesTotalSince nets every sale's total_kobo since since (inclusive)
// -- a reversal's total is negative by construction (see migration
// 000019's comment on sales.total_kobo), so this is always the correct net
// figure with no case-by-case sign handling here.
// SumSalesTotalSince returns the net total (reversals included, since
// their total_kobo is negative by construction) and a count of ordinary
// sales since since (excluding reversal rows, which aren't "one more
// sale" for display purposes).
func (s *Service) SumSalesTotalSince(ctx context.Context, userID, businessID uuid.UUID, since time.Time) (total int64, count int64, err error) {
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.SumSalesTotalSince(ctx, sqlc.SumSalesTotalSinceParams{BusinessID: businessID, OccurredAt: pgTimestamptz(since)})
		if err != nil {
			return err
		}
		total, count = row.Total, row.Count
		return nil
	})
	if err != nil {
		return 0, 0, fmt.Errorf("sum sales total: %w", err)
	}
	return total, count, nil
}

// ReverseSale posts a new sale that exactly negates saleID -- opposite-sign
// items, a compensating positive inventory movement per item (putting
// stock back), and a negative-amount payment (a refund) -- rather than
// editing or deleting the original, matching every other immutable-history
// convention in this codebase. Returns ErrAlreadyReversed if saleID has
// already been reversed once; a sale is reversed at most once in this
// slice (no partial reversal of individual items).
func (s *Service) ReverseSale(ctx context.Context, userID, businessID, saleID, actorID uuid.UUID) (Sale, error) {
	reversalID, err := newID()
	if err != nil {
		return Sale{}, err
	}
	billID, err := newID()
	if err != nil {
		return Sale{}, err
	}
	eventID, err := newID()
	if err != nil {
		return Sale{}, err
	}
	paymentID, err := newID()
	if err != nil {
		return Sale{}, err
	}
	now := time.Now()

	var result Sale
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		original, err := loadSaleAggregate(ctx, q, businessID, saleID)
		if err != nil {
			return err
		}

		if _, err := q.GetReversalOfSale(ctx, sqlc.GetReversalOfSaleParams{BusinessID: businessID, ReversalOfSaleID: pgUUID(saleID)}); err == nil {
			return ErrAlreadyReversed
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check existing reversal: %w", err)
		}

		original.ReversalOf = saleID
		bill, err := q.CreateBill(ctx, sqlc.CreateBillParams{
			ID: billID, BusinessID: businessID, LocationID: original.LocationID, Status: "settled",
			OpenedBy: actorID, OpenedAt: pgTimestamptz(now),
		})
		if err != nil {
			return fmt.Errorf("create reversal bill: %w", err)
		}

		// Every original item's quantity/price is already known -- no
		// lookups needed, just negate. Computed here, before CreateSale,
		// for the same reason CreateSale itself resolves before inserting:
		// sale_items/inventory_events both have a foreign key back to this
		// row, so it must exist first, which means its total must be known
		// first.
		var totalKobo int64
		for _, item := range original.Items {
			totalKobo += int64(-item.Quantity) * item.UnitPriceKobo
		}

		idempotencyKey, err := uuid.NewRandom()
		if err != nil {
			return fmt.Errorf("generate reversal idempotency key: %w", err)
		}
		sale, err := q.CreateSale(ctx, sqlc.CreateSaleParams{
			ID: reversalID, BusinessID: businessID, BillID: bill.ID, SellerID: actorID,
			IdempotencyKey: idempotencyKey, OccurredAt: pgTimestamptz(now), ReceivedAt: pgTimestamptz(now),
			TotalKobo: totalKobo, ReversalOfSaleID: pgUUID(saleID),
		})
		if err != nil {
			return fmt.Errorf("create reversal sale: %w", err)
		}

		if _, err := q.CreateInventoryEventForSale(ctx, sqlc.CreateInventoryEventForSaleParams{
			ID: eventID, BusinessID: businessID, Type: "sale_reversal", ActorID: actorID, SaleID: pgUUID(reversalID),
		}); err != nil {
			return fmt.Errorf("create inventory event: %w", err)
		}

		var postedItems []SaleItem
		for _, item := range original.Items {
			itemID, err := newID()
			if err != nil {
				return err
			}
			quantity := -item.Quantity
			lineTotal := int64(quantity) * item.UnitPriceKobo
			postedItem, err := q.CreateSaleItem(ctx, sqlc.CreateSaleItemParams{
				ID: itemID, BusinessID: businessID, SaleID: reversalID, VariantID: item.VariantID,
				Description: item.Description, Quantity: quantity, UnitPriceKobo: item.UnitPriceKobo, LineTotalKobo: lineTotal,
			})
			if err != nil {
				return fmt.Errorf("create reversal sale item: %w", err)
			}
			postedItems = append(postedItems, toSaleItem(postedItem))

			// Putting stock back: the original sale moved -quantity, so the
			// reversal moves +quantity of the original (== -quantity here,
			// since quantity is already negated above).
			movementID, err := newID()
			if err != nil {
				return err
			}
			if _, err := q.CreateInventoryMovement(ctx, sqlc.CreateInventoryMovementParams{
				ID: movementID, BusinessID: businessID, EventID: eventID,
				VariantID: item.VariantID, LocationID: original.LocationID, QuantityDelta: -quantity,
			}); err != nil {
				return fmt.Errorf("create reversal inventory movement: %w", err)
			}
			if _, err := q.UpsertInventoryBalanceDelta(ctx, sqlc.UpsertInventoryBalanceDeltaParams{
				BusinessID: businessID, VariantID: item.VariantID, LocationID: original.LocationID, Quantity: -quantity,
			}); err != nil {
				return fmt.Errorf("update inventory balance: %w", err)
			}
		}

		refundPayment, err := q.CreatePayment(ctx, sqlc.CreatePaymentParams{
			ID: paymentID, BusinessID: businessID, BillID: bill.ID,
			AmountKobo: -original.Payment.AmountKobo, Method: original.Payment.Method, ActorID: actorID,
			ReversalOfPaymentID: pgUUID(original.Payment.ID),
		})
		if err != nil {
			return fmt.Errorf("create refund payment: %w", err)
		}

		result = Sale{
			ID: sale.ID, BusinessID: businessID, BillID: bill.ID, SellerID: actorID,
			OccurredAt: now, ReceivedAt: now, TotalKobo: totalKobo, ReversalOf: saleID,
			Items: postedItems, Payment: toPayment(refundPayment),
		}
		return nil
	})
	if err != nil {
		return Sale{}, err
	}
	return result, nil
}

// ListOpenReviews returns every unresolved sale review for a business.
func (s *Service) ListOpenReviews(ctx context.Context, userID, businessID uuid.UUID) ([]Review, error) {
	var out []Review
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		rows, err := q.ListSaleReviewsDetailed(ctx, sqlc.ListSaleReviewsDetailedParams{BusinessID: businessID, Status: "open"})
		if err != nil {
			return err
		}
		out = make([]Review, 0, len(rows))
		for _, r := range rows {
			out = append(out, toReviewDetailed(r))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list sale reviews: %w", err)
	}
	return out, nil
}

// ResolveReview marks a review acknowledged, with a required note -- the
// sale itself is never changed by this; a review is purely informational.
func (s *Service) ResolveReview(ctx context.Context, userID, businessID, reviewID, resolvedBy uuid.UUID, note string) (Review, error) {
	var result Review
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.ResolveSaleReview(ctx, sqlc.ResolveSaleReviewParams{
			BusinessID: businessID, ID: reviewID, ResolvedBy: pgUUID(resolvedBy), ResolvedNote: pgText(note),
		})
		if err != nil {
			return err
		}
		result = toReview(row)
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Review{}, ErrReviewNotFound
		}
		return Review{}, fmt.Errorf("resolve sale review: %w", err)
	}
	return result, nil
}
