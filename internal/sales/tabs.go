package sales

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mananuf/justbarme/internal/store"
	"github.com/mananuf/justbarme/internal/store/sqlc"
)

// CreateTable adds a business-configured table label (docs/ARCHITECTURE.md
// §8.3) -- no kitchen/routing behavior, just a name a bill can reference.
func (s *Service) CreateTable(ctx context.Context, userID, businessID uuid.UUID, label string) (Table, error) {
	id, err := newID()
	if err != nil {
		return Table{}, err
	}
	var result Table
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.CreateTable(ctx, sqlc.CreateTableParams{ID: id, BusinessID: businessID, Label: label})
		if err != nil {
			if pgErrorCode(err) == pgUniqueViolation {
				return ErrTableLabelTaken
			}
			return fmt.Errorf("create table: %w", err)
		}
		result = toTable(row)
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrTableLabelTaken) {
			return Table{}, ErrTableLabelTaken
		}
		return Table{}, err
	}
	return result, nil
}

// ListTables returns every active table for a business.
func (s *Service) ListTables(ctx context.Context, userID, businessID uuid.UUID) ([]Table, error) {
	var out []Table
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		rows, err := q.ListTables(ctx, businessID)
		if err != nil {
			return err
		}
		out = make([]Table, 0, len(rows))
		for _, r := range rows {
			out = append(out, toTable(r))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	return out, nil
}

// CreateCustomer adds a lightweight customer record. Name is mandatory
// (docs/ARCHITECTURE.md §8.3); phone/email/notes are optional.
func (s *Service) CreateCustomer(ctx context.Context, userID, businessID uuid.UUID, name, phone, email, notes string) (Customer, error) {
	id, err := newID()
	if err != nil {
		return Customer{}, err
	}
	var result Customer
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.CreateCustomer(ctx, sqlc.CreateCustomerParams{
			ID: id, BusinessID: businessID, Name: name,
			Phone: pgText(phone), Email: pgText(email), Notes: pgText(notes),
		})
		if err != nil {
			return fmt.Errorf("create customer: %w", err)
		}
		result = toCustomer(row)
		return nil
	})
	if err != nil {
		return Customer{}, err
	}
	return result, nil
}

// ListCustomers returns every customer for a business, alphabetically.
func (s *Service) ListCustomers(ctx context.Context, userID, businessID uuid.UUID) ([]Customer, error) {
	var out []Customer
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		rows, err := q.ListCustomers(ctx, businessID)
		if err != nil {
			return err
		}
		out = make([]Customer, 0, len(rows))
		for _, r := range rows {
			out = append(out, toCustomer(r))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list customers: %w", err)
	}
	return out, nil
}

// OpenBill opens or resolves a tab: a table, a named customer, both, or
// neither (an empty walk-in tab opened before the first round -- distinct
// from CreateSale's one-shot walk-in path, which creates and settles a
// bill in a single call). tableID/customerID are uuid.Nil when not used.
//
// Find-or-open, not blind creation (docs/PHASE_UNIFIED_SELL_BILLS.md):
// picking an already-occupied table or an already-open customer tab
// resumes that bill instead of opening a second, concurrent one for the
// same table/customer -- a real gap in the original Phase 6 behavior, not
// just a nicety for the new unified Sell flow. Table takes priority over
// customer when both are given (a table's own occupancy is the more
// concrete signal); this also makes OpenBill naturally idempotent for
// that case on its own, with no key needed. The pure walk-in case (no
// table, no customer) has no such key to resolve against, so
// idempotencyKey is checked there instead -- uuid.Nil means "don't bother"
// (an interactive, online open has no need for one).
func (s *Service) OpenBill(
	ctx context.Context, userID, businessID, locationID, openedBy, tableID, customerID uuid.UUID,
	idempotencyKey uuid.UUID,
) (Bill, error) {
	id, err := newID()
	if err != nil {
		return Bill{}, err
	}
	var result Bill
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		if tableID != uuid.Nil {
			if _, err := q.GetTableByID(ctx, sqlc.GetTableByIDParams{BusinessID: businessID, ID: tableID}); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrTableNotFound
				}
				return fmt.Errorf("look up table: %w", err)
			}
			if existing, err := q.GetOpenBillByTableID(ctx, sqlc.GetOpenBillByTableIDParams{
				BusinessID: businessID, TableID: pgUUID(tableID),
			}); err == nil {
				result = toBill(existing)
				return nil
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("resolve bill for table: %w", err)
			}
		} else if customerID != uuid.Nil {
			if _, err := q.GetCustomerByID(ctx, sqlc.GetCustomerByIDParams{BusinessID: businessID, ID: customerID}); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrCustomerNotFound
				}
				return fmt.Errorf("look up customer: %w", err)
			}
			if existing, err := q.GetOpenBillByCustomerID(ctx, sqlc.GetOpenBillByCustomerIDParams{
				BusinessID: businessID, CustomerID: pgUUID(customerID),
			}); err == nil {
				result = toBill(existing)
				return nil
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("resolve bill for customer: %w", err)
			}
		} else if idempotencyKey != uuid.Nil {
			if existing, err := q.GetBillByIdempotencyKey(ctx, sqlc.GetBillByIdempotencyKeyParams{
				BusinessID: businessID, IdempotencyKey: pgUUID(idempotencyKey),
			}); err == nil {
				result = toBill(existing)
				return nil
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("check idempotency key: %w", err)
			}
		}

		row, err := q.CreateBill(ctx, sqlc.CreateBillParams{
			ID: id, BusinessID: businessID, LocationID: locationID, Status: BillStatusOpen,
			OpenedBy: openedBy, OpenedAt: pgTimestamptz(time.Now()),
			TableID: pgUUID(tableID), CustomerID: pgUUID(customerID),
			IdempotencyKey: pgUUID(idempotencyKey),
		})
		if err != nil {
			if pgErrorCode(err) == pgUniqueViolation {
				// Lost a race against another request opening a walk-in
				// bill with the same idempotency key between our check and
				// our insert -- re-check and return the winner's row
				// rather than surfacing a spurious conflict.
				if existing, getErr := q.GetBillByIdempotencyKey(ctx, sqlc.GetBillByIdempotencyKeyParams{
					BusinessID: businessID, IdempotencyKey: pgUUID(idempotencyKey),
				}); getErr == nil {
					result = toBill(existing)
					return nil
				}
			}
			return fmt.Errorf("open bill: %w", err)
		}
		result = toBill(row)
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrTableNotFound) || errors.Is(err, ErrCustomerNotFound) {
			return Bill{}, err
		}
		return Bill{}, err
	}
	return result, nil
}

// recomputeBillBalance sums sales/payments/write-offs for billID and
// derives the resulting status from currentStatus and the new balance,
// then persists both in one statement. This is the single place bill
// status transitions happen, called by every bill-mutating method below
// while still holding the row lock GetBillForUpdate took.
//
// Status derivation: 'open' never auto-transitions (a tab may sit at a
// zero balance and still take more rounds); 'closed_unpaid' becomes
// 'settled' once balance reaches zero, and a 'settled' bill reverts to
// 'closed_unpaid' if a payment reversal makes its balance nonzero again;
// 'void' is terminal.
func recomputeBillBalance(ctx context.Context, q *sqlc.Queries, businessID, billID uuid.UUID, currentStatus string) (sqlc.Bill, error) {
	salesTotal, err := q.SumSalesTotalByBillID(ctx, sqlc.SumSalesTotalByBillIDParams{BusinessID: businessID, BillID: billID})
	if err != nil {
		return sqlc.Bill{}, fmt.Errorf("sum sales for bill: %w", err)
	}
	paid, err := q.SumPaymentsByBillID(ctx, sqlc.SumPaymentsByBillIDParams{BusinessID: businessID, BillID: billID})
	if err != nil {
		return sqlc.Bill{}, fmt.Errorf("sum payments for bill: %w", err)
	}
	writtenOff, err := q.SumWriteOffsByBillID(ctx, sqlc.SumWriteOffsByBillIDParams{BusinessID: businessID, BillID: billID})
	if err != nil {
		return sqlc.Bill{}, fmt.Errorf("sum write-offs for bill: %w", err)
	}
	balance := salesTotal - paid - writtenOff

	newStatus := currentStatus
	switch currentStatus {
	case BillStatusClosedUnpaid:
		if balance == 0 {
			newStatus = BillStatusSettled
		}
	case BillStatusSettled:
		if balance != 0 {
			newStatus = BillStatusClosedUnpaid
		}
	}

	return q.UpdateBillBalanceAndStatus(ctx, sqlc.UpdateBillBalanceAndStatusParams{
		BusinessID: businessID, ID: billID, BalanceKobo: balance, Status: newStatus,
	})
}

// AddSaleRound appends another immutable sale round to an already-open
// bill (docs/ARCHITECTURE.md §8.3: "More items create another immutable
// sale/round on the bill") -- unlike CreateSale's walk-in path, this never
// creates a bill or a payment. Allowed while the bill is open or
// closed_unpaid (i.e. anything short of settled/void) -- nothing has been
// paid in full yet, so free-form item corrections stay a normal staff
// action; once settled, a correction has to go through the heavier,
// owner-only ReverseSale instead. idempotencyKey makes a retried POST
// safe, same reasoning as CreateSale.
func (s *Service) AddSaleRound(
	ctx context.Context, userID, businessID, billID, sellerID uuid.UUID,
	idempotencyKey uuid.UUID, occurredAt time.Time, items []SaleItemInput,
) (Sale, error) {
	if len(items) == 0 {
		return Sale{}, ErrNoItems
	}
	saleID, err := newID()
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

		bill, err := q.GetBillForUpdate(ctx, sqlc.GetBillForUpdateParams{BusinessID: businessID, ID: billID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrBillNotFound
			}
			return fmt.Errorf("lock bill: %w", err)
		}
		if bill.Status != BillStatusOpen && bill.Status != BillStatusClosedUnpaid {
			return ErrBillNotEditable
		}

		sale, err := postSaleRound(ctx, q, businessID, billID, sellerID, bill.LocationID, saleID, idempotencyKey, occurredAt, receivedAt, items)
		if err != nil {
			return err
		}

		if _, err := recomputeBillBalance(ctx, q, businessID, billID, bill.Status); err != nil {
			return err
		}

		result = sale
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrVariantNotFound) || errors.Is(err, ErrBillNotFound) || errors.Is(err, ErrBillNotEditable) {
			return Sale{}, err
		}
		return Sale{}, err
	}
	return result, nil
}

// RemoveBillItem posts a compensating negative-quantity round that removes
// quantity units of variantID from billID -- never edits or deletes a
// previously posted round, matching every other correction in this
// codebase (ReverseSale, ReversePayment). Allowed under the same
// open/closed_unpaid rule as AddSaleRound (see its comment) -- this is the
// "the customer changed their mind" / "wrong drink added" correction
// path, capped so you can never remove more than is currently net-present
// on the bill. unitPriceKobo is the current catalogue price, the same
// source AddSaleRound's own caller already uses, so an add-then-remove of
// the same drink nets to exactly zero in the common case. idempotencyKey
// makes a retried POST safe, same reasoning as AddSaleRound.
func (s *Service) RemoveBillItem(
	ctx context.Context, userID, businessID, billID, sellerID, variantID uuid.UUID,
	idempotencyKey uuid.UUID, quantity int32, unitPriceKobo int64, occurredAt time.Time,
) (Sale, error) {
	if quantity <= 0 {
		return Sale{}, ErrNoItems
	}
	saleID, err := newID()
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

		bill, err := q.GetBillForUpdate(ctx, sqlc.GetBillForUpdateParams{BusinessID: businessID, ID: billID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrBillNotFound
			}
			return fmt.Errorf("lock bill: %w", err)
		}
		if bill.Status != BillStatusOpen && bill.Status != BillStatusClosedUnpaid {
			return ErrBillNotEditable
		}

		present, err := q.SumBillItemQuantityByVariant(ctx, sqlc.SumBillItemQuantityByVariantParams{
			BusinessID: businessID, BillID: billID, VariantID: variantID,
		})
		if err != nil {
			return fmt.Errorf("sum bill item quantity: %w", err)
		}
		if quantity > present {
			return ErrInsufficientQuantityOnBill
		}

		sale, err := postSaleRound(ctx, q, businessID, billID, sellerID, bill.LocationID, saleID, idempotencyKey, occurredAt, receivedAt,
			[]SaleItemInput{{VariantID: variantID, Quantity: -quantity, UnitPriceKobo: unitPriceKobo}},
		)
		if err != nil {
			return err
		}

		// An open bill deliberately never auto-settles just because its
		// balance hit zero (see recomputeBillBalance) -- a tab can sit at
		// zero and still take more rounds later (pay-as-you-go). But that
		// means status alone can't gate removal: once a payment has
		// already covered what's currently on the bill, taking an item
		// back off would push the balance negative -- a refund owed, not
		// a correction, and never what this action is for. Checked here,
		// after postSaleRound, against sale.TotalKobo (the real amount
		// just computed -- always <= 0 for a removal) rather than a
		// client-estimated int64(quantity)*unitPriceKobo beforehand: for a
		// multi-buy-priced variant (docs/PHASE_MULTIBUY_PRICING.md) that
		// estimate would use the wrong basis entirely, since the true
		// refund is the pack-aware marginal amount, not a flat per-unit
		// guess. Returning a non-nil error here still rolls back
		// everything postSaleRound just wrote -- we're still inside the
		// same transaction, nothing has committed yet. This also still
		// correctly falls through for an item that was never actually paid
		// for (balance reached zero purely because nothing net remains,
		// not because money changed hands), since sale.TotalKobo would
		// already be constrained to what's present by the quantity check
		// above. Adding is unaffected -- it only ever increases what's
		// owed, so it stays fine at any balance.
		if bill.BalanceKobo+sale.TotalKobo < 0 {
			return ErrBillFullyPaid
		}

		if _, err := recomputeBillBalance(ctx, q, businessID, billID, bill.Status); err != nil {
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

// CloseBill transitions an open bill to closed_unpaid (balance remains) or
// directly to settled (balance already zero) -- "stop adding items", never
// implying payment (docs/ARCHITECTURE.md §8.3). Rejects closing with a
// remaining balance unless the bill already has a named customer attached
// -- "a customer name is mandatory whenever credit/outstanding debt is
// recorded", and customers.name is NOT NULL, so attaching one at all
// satisfies that.
func (s *Service) CloseBill(ctx context.Context, userID, businessID, billID uuid.UUID) (Bill, error) {
	var result Bill
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		bill, err := q.GetBillForUpdate(ctx, sqlc.GetBillForUpdateParams{BusinessID: businessID, ID: billID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrBillNotFound
			}
			return fmt.Errorf("lock bill: %w", err)
		}
		if bill.Status != BillStatusOpen {
			return ErrBillNotOpen
		}

		salesTotal, err := q.SumSalesTotalByBillID(ctx, sqlc.SumSalesTotalByBillIDParams{BusinessID: businessID, BillID: billID})
		if err != nil {
			return fmt.Errorf("sum sales for bill: %w", err)
		}
		paid, err := q.SumPaymentsByBillID(ctx, sqlc.SumPaymentsByBillIDParams{BusinessID: businessID, BillID: billID})
		if err != nil {
			return fmt.Errorf("sum payments for bill: %w", err)
		}
		balance := salesTotal - paid
		if balance != 0 && !bill.CustomerID.Valid {
			return ErrCreditRequiresCustomer
		}

		status := BillStatusClosedUnpaid
		if balance == 0 {
			status = BillStatusSettled
		}
		row, err := q.UpdateBillBalanceAndStatus(ctx, sqlc.UpdateBillBalanceAndStatusParams{
			BusinessID: businessID, ID: billID, BalanceKobo: balance, Status: status,
		})
		if err != nil {
			return fmt.Errorf("close bill: %w", err)
		}
		result = toBill(row)
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrBillNotFound) || errors.Is(err, ErrBillNotOpen) || errors.Is(err, ErrCreditRequiresCustomer) {
			return Bill{}, err
		}
		return Bill{}, err
	}
	return result, nil
}

// RecordPayment posts an immutable partial or full payment against a
// bill's outstanding balance. Overpayment is rejected in this MVP
// (docs/ARCHITECTURE.md §8.3's explicit "Reject overpayment").
// idempotencyKey makes a retried/replayed POST safe -- uuid.Nil skips the
// check (an interactive, online payment has no need for one), same
// opt-in shape OpenBill's walk-in path uses.
func (s *Service) RecordPayment(
	ctx context.Context, userID, businessID, billID, actorID uuid.UUID,
	amountKobo int64, method string, idempotencyKey uuid.UUID,
) (Payment, error) {
	if amountKobo <= 0 {
		return Payment{}, ErrInvalidPaymentAmt
	}
	paymentID, err := newID()
	if err != nil {
		return Payment{}, err
	}

	var result Payment
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		if idempotencyKey != uuid.Nil {
			if existing, err := q.GetPaymentByIdempotencyKey(ctx, sqlc.GetPaymentByIdempotencyKeyParams{
				BusinessID: businessID, IdempotencyKey: pgUUID(idempotencyKey),
			}); err == nil {
				result = toPayment(existing)
				return nil
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("check idempotency key: %w", err)
			}
		}

		bill, err := q.GetBillForUpdate(ctx, sqlc.GetBillForUpdateParams{BusinessID: businessID, ID: billID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrBillNotFound
			}
			return fmt.Errorf("lock bill: %w", err)
		}
		if bill.Status != BillStatusOpen && bill.Status != BillStatusClosedUnpaid {
			return ErrBillNotPayable
		}
		if amountKobo > bill.BalanceKobo {
			return ErrOverpayment
		}

		row, err := q.CreatePayment(ctx, sqlc.CreatePaymentParams{
			ID: paymentID, BusinessID: businessID, BillID: billID,
			AmountKobo: amountKobo, Method: method, ActorID: actorID,
			IdempotencyKey: pgUUID(idempotencyKey),
		})
		if err != nil {
			return fmt.Errorf("record payment: %w", err)
		}

		if _, err := recomputeBillBalance(ctx, q, businessID, billID, bill.Status); err != nil {
			return err
		}

		result = toPayment(row)
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrBillNotFound) || errors.Is(err, ErrBillNotPayable) || errors.Is(err, ErrOverpayment) {
			return Payment{}, err
		}
		return Payment{}, err
	}
	return result, nil
}

// ReversePayment posts a new, negative-amount payment that exactly negates
// paymentID -- never edits or deletes the original (docs/ARCHITECTURE.md
// §8.3's "Implement payment reversal as a new event"). At most one
// reversal per payment.
func (s *Service) ReversePayment(ctx context.Context, userID, businessID, paymentID, actorID uuid.UUID) (Payment, error) {
	reversalID, err := newID()
	if err != nil {
		return Payment{}, err
	}

	var result Payment
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		original, err := q.GetPaymentByID(ctx, sqlc.GetPaymentByIDParams{BusinessID: businessID, ID: paymentID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrPaymentNotFound
			}
			return fmt.Errorf("get payment: %w", err)
		}
		if original.ReversalOfPaymentID.Valid {
			return ErrPaymentIsReversal
		}

		bill, err := q.GetBillForUpdate(ctx, sqlc.GetBillForUpdateParams{BusinessID: businessID, ID: original.BillID})
		if err != nil {
			return fmt.Errorf("lock bill: %w", err)
		}
		if bill.Status == BillStatusVoid {
			return ErrBillNotPayable
		}

		if _, err := q.GetReversalOfPayment(ctx, sqlc.GetReversalOfPaymentParams{BusinessID: businessID, ReversalOfPaymentID: pgUUID(paymentID)}); err == nil {
			return ErrPaymentAlreadyReversed
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check existing payment reversal: %w", err)
		}

		row, err := q.CreatePayment(ctx, sqlc.CreatePaymentParams{
			ID: reversalID, BusinessID: businessID, BillID: original.BillID,
			AmountKobo: -original.AmountKobo, Method: original.Method, ActorID: actorID,
			ReversalOfPaymentID: pgUUID(paymentID),
		})
		if err != nil {
			return fmt.Errorf("reverse payment: %w", err)
		}

		if _, err := recomputeBillBalance(ctx, q, businessID, original.BillID, bill.Status); err != nil {
			return err
		}

		result = toPayment(row)
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrPaymentNotFound) || errors.Is(err, ErrPaymentIsReversal) ||
			errors.Is(err, ErrPaymentAlreadyReversed) || errors.Is(err, ErrBillNotPayable) {
			return Payment{}, err
		}
		return Payment{}, err
	}
	return result, nil
}

// WriteOffBill forgives part or all of a closed, unpaid bill's balance --
// owner-only at the HTTP layer (bills:write_off), reason required
// (docs/ARCHITECTURE.md §8.3's explicit acceptance criterion). Never a
// payment: no money changes hands. Restricted to closed_unpaid bills, not
// still-open ones, keeping the status machine linear: a tab is written off
// only after it has stopped taking rounds.
func (s *Service) WriteOffBill(ctx context.Context, userID, businessID, billID, actorID uuid.UUID, amountKobo int64, reason string) (Bill, error) {
	if amountKobo <= 0 {
		return Bill{}, ErrWriteOffExceedsBalance
	}
	if reason == "" {
		return Bill{}, ErrWriteOffReasonRequired
	}
	writeOffID, err := newID()
	if err != nil {
		return Bill{}, err
	}

	var result Bill
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		bill, err := q.GetBillForUpdate(ctx, sqlc.GetBillForUpdateParams{BusinessID: businessID, ID: billID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrBillNotFound
			}
			return fmt.Errorf("lock bill: %w", err)
		}
		if bill.Status != BillStatusClosedUnpaid {
			return ErrBillNotClosedUnpaid
		}
		if amountKobo > bill.BalanceKobo {
			return ErrWriteOffExceedsBalance
		}

		if _, err := q.CreateBillWriteOff(ctx, sqlc.CreateBillWriteOffParams{
			ID: writeOffID, BusinessID: businessID, BillID: billID, AmountKobo: amountKobo, Reason: reason, ActorID: actorID,
		}); err != nil {
			return fmt.Errorf("create write-off: %w", err)
		}

		row, err := recomputeBillBalance(ctx, q, businessID, billID, bill.Status)
		if err != nil {
			return err
		}
		result = toBill(row)
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrBillNotFound) || errors.Is(err, ErrBillNotClosedUnpaid) || errors.Is(err, ErrWriteOffExceedsBalance) || errors.Is(err, ErrWriteOffReasonRequired) {
			return Bill{}, err
		}
		return Bill{}, err
	}
	return result, nil
}

// VoidBill marks a bill void: "no effective sales remain" -- only allowed
// when its net sales total and outstanding balance are both zero (every
// round ever added has since been fully reversed, or none were ever
// added), never as a way to discard real, unpaid activity.
func (s *Service) VoidBill(ctx context.Context, userID, businessID, billID uuid.UUID) (Bill, error) {
	var result Bill
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		bill, err := q.GetBillForUpdate(ctx, sqlc.GetBillForUpdateParams{BusinessID: businessID, ID: billID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrBillNotFound
			}
			return fmt.Errorf("lock bill: %w", err)
		}
		if bill.Status != BillStatusOpen && bill.Status != BillStatusClosedUnpaid {
			return ErrBillNotOpen
		}

		salesTotal, err := q.SumSalesTotalByBillID(ctx, sqlc.SumSalesTotalByBillIDParams{BusinessID: businessID, BillID: billID})
		if err != nil {
			return fmt.Errorf("sum sales for bill: %w", err)
		}
		if salesTotal != 0 || bill.BalanceKobo != 0 {
			return ErrBillHasOutstandingActivity
		}

		row, err := q.UpdateBillBalanceAndStatus(ctx, sqlc.UpdateBillBalanceAndStatusParams{
			BusinessID: businessID, ID: billID, BalanceKobo: 0, Status: BillStatusVoid,
		})
		if err != nil {
			return fmt.Errorf("void bill: %w", err)
		}
		result = toBill(row)
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrBillNotFound) || errors.Is(err, ErrBillNotOpen) || errors.Is(err, ErrBillHasOutstandingActivity) {
			return Bill{}, err
		}
		return Bill{}, err
	}
	return result, nil
}

// GetBillDetail assembles a bill with its rounds, payments, and
// write-offs, for a bill detail screen.
func (s *Service) GetBillDetail(ctx context.Context, userID, businessID, billID uuid.UUID) (BillDetail, error) {
	var result BillDetail
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		billRow, err := q.GetBillByID(ctx, sqlc.GetBillByIDParams{BusinessID: businessID, ID: billID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrBillNotFound
			}
			return fmt.Errorf("get bill: %w", err)
		}

		saleRows, err := q.ListSalesByBillID(ctx, sqlc.ListSalesByBillIDParams{BusinessID: businessID, BillID: billID})
		if err != nil {
			return fmt.Errorf("list sales for bill: %w", err)
		}
		var salesOut []Sale
		for _, sr := range saleRows {
			sale := toSale(sr)
			itemRows, err := q.ListSaleItemsBySaleID(ctx, sqlc.ListSaleItemsBySaleIDParams{BusinessID: businessID, SaleID: sale.ID})
			if err != nil {
				return fmt.Errorf("list sale items: %w", err)
			}
			for _, i := range itemRows {
				sale.Items = append(sale.Items, toSaleItem(i))
			}
			salesOut = append(salesOut, sale)
		}

		paymentRows, err := q.ListPaymentsByBillID(ctx, sqlc.ListPaymentsByBillIDParams{BusinessID: businessID, BillID: billID})
		if err != nil {
			return fmt.Errorf("list payments for bill: %w", err)
		}
		var paymentsOut []Payment
		for _, p := range paymentRows {
			paymentsOut = append(paymentsOut, toPayment(p))
		}

		writeOffRows, err := q.ListWriteOffsByBillID(ctx, sqlc.ListWriteOffsByBillIDParams{BusinessID: businessID, BillID: billID})
		if err != nil {
			return fmt.Errorf("list write-offs for bill: %w", err)
		}
		var writeOffsOut []WriteOff
		for _, wo := range writeOffRows {
			writeOffsOut = append(writeOffsOut, toWriteOff(wo))
		}

		result = BillDetail{Bill: toBill(billRow), Sales: salesOut, Payments: paymentsOut, WriteOffs: writeOffsOut}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrBillNotFound) {
			return BillDetail{}, err
		}
		return BillDetail{}, err
	}
	return result, nil
}

// ListOutstandingBills returns every bill with a nonzero balance --
// "outstanding bill/customer queries" per docs/IMPLEMENTATION_PLAN.md
// Phase 6.
func (s *Service) ListOutstandingBills(ctx context.Context, userID, businessID uuid.UUID) ([]Bill, error) {
	var out []Bill
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		rows, err := q.ListOutstandingBills(ctx, businessID)
		if err != nil {
			return err
		}
		out = make([]Bill, 0, len(rows))
		for _, r := range rows {
			out = append(out, toBill(r))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list outstanding bills: %w", err)
	}
	return out, nil
}

// ListBillsByStatus returns every bill in exactly one status, most
// recently opened first -- backs the unified Bills screen's status filter
// (docs/PHASE_UNIFIED_SELL_BILLS.md), which unlike ListOpenBills/
// ListOutstandingBills needs to show settled/void history too, not just
// what's currently actionable.
func (s *Service) ListBillsByStatus(ctx context.Context, userID, businessID uuid.UUID, status string) ([]Bill, error) {
	var out []Bill
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		rows, err := q.ListBillsByStatus(ctx, sqlc.ListBillsByStatusParams{BusinessID: businessID, Status: status})
		if err != nil {
			return err
		}
		out = make([]Bill, 0, len(rows))
		for _, r := range rows {
			out = append(out, toBill(r))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list bills by status: %w", err)
	}
	return out, nil
}

// ListAllBills returns every bill regardless of status, most recently
// opened first -- the unified Bills screen's "All" filter.
func (s *Service) ListAllBills(ctx context.Context, userID, businessID uuid.UUID) ([]Bill, error) {
	var out []Bill
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		rows, err := q.ListAllBills(ctx, businessID)
		if err != nil {
			return err
		}
		out = make([]Bill, 0, len(rows))
		for _, r := range rows {
			out = append(out, toBill(r))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list all bills: %w", err)
	}
	return out, nil
}

// ListOpenBills returns every bill still open or closed-unpaid (i.e. not
// yet settled or voided) -- the table grid / open tabs view.
func (s *Service) ListOpenBills(ctx context.Context, userID, businessID uuid.UUID) ([]Bill, error) {
	var out []Bill
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		rows, err := q.ListOpenBills(ctx, businessID)
		if err != nil {
			return err
		}
		out = make([]Bill, 0, len(rows))
		for _, r := range rows {
			out = append(out, toBill(r))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list open bills: %w", err)
	}
	return out, nil
}
