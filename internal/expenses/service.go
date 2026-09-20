package expenses

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

// CreateCategory adds a new business-owned expense category. Returns
// ErrCategoryNameTaken if name is already used in this business, same
// convention as catalogue.Service.CreateCategory.
func (s *Service) CreateCategory(ctx context.Context, userID, businessID uuid.UUID, name string) (Category, error) {
	id, err := newID()
	if err != nil {
		return Category{}, err
	}
	var created sqlc.ExpenseCategory
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.CreateExpenseCategory(ctx, sqlc.CreateExpenseCategoryParams{ID: id, BusinessID: businessID, Name: name})
		if err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		if pgErrorCode(err) == pgUniqueViolation {
			return Category{}, ErrCategoryNameTaken
		}
		return Category{}, fmt.Errorf("create expense category: %w", err)
	}
	return toCategory(created), nil
}

func (s *Service) ListCategories(ctx context.Context, userID, businessID uuid.UUID) ([]Category, error) {
	var rows []sqlc.ExpenseCategory
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		r, err := q.ListExpenseCategories(ctx, businessID)
		if err != nil {
			return err
		}
		rows = r
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list expense categories: %w", err)
	}
	out := make([]Category, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCategory(r))
	}
	return out, nil
}

// RecordExpense posts one immutable expense. idempotencyKey makes a
// retried POST safe -- expenses:record is offline-safe (internal/tenancy/
// capabilities.go's offlineSafeCapabilities), so a device that queued this
// while offline may retry it once connectivity returns; calling this again
// with the same key returns the expense already posted the first time,
// same pattern as sales.Service.CreateSale.
func (s *Service) RecordExpense(
	ctx context.Context,
	userID, businessID, locationID, categoryID, recordedBy uuid.UUID,
	idempotencyKey uuid.UUID,
	description string,
	amountKobo int64,
	paymentMethod string,
	occurredAt time.Time,
) (Expense, error) {
	id, err := newID()
	if err != nil {
		return Expense{}, err
	}

	var result Expense
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		if existing, err := q.GetExpenseByIdempotencyKey(ctx, sqlc.GetExpenseByIdempotencyKeyParams{
			BusinessID: businessID, IdempotencyKey: idempotencyKey,
		}); err == nil {
			result = toExpense(existing)
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check idempotency key: %w", err)
		}

		created, err := q.CreateExpense(ctx, sqlc.CreateExpenseParams{
			ID: id, BusinessID: businessID, LocationID: locationID, CategoryID: categoryID,
			Description: description, AmountKobo: amountKobo, PaymentMethod: paymentMethod,
			RecordedBy: recordedBy, IdempotencyKey: idempotencyKey, OccurredAt: pgTimestamptz(occurredAt),
		})
		if err != nil {
			return fmt.Errorf("create expense: %w", err)
		}
		result = toExpense(created)
		return nil
	})
	if err != nil {
		return Expense{}, err
	}
	return result, nil
}

// ReverseExpense creates a new, equal-and-opposite Expense row -- it never
// edits or deletes the original (docs/ARCHITECTURE.md §8.6). Owner-only at
// the HTTP layer (expenses:reverse), same tier as sales:reverse/
// inventory:adjustment_approve -- always requires online server
// authorization, deliberately not offline-safe.
func (s *Service) ReverseExpense(ctx context.Context, userID, businessID, expenseID, actorID uuid.UUID) (Expense, error) {
	reversalID, err := newID()
	if err != nil {
		return Expense{}, err
	}
	idempotencyKey, err := uuid.NewRandom()
	if err != nil {
		return Expense{}, fmt.Errorf("generate reversal idempotency key: %w", err)
	}
	now := time.Now()

	var result Expense
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		original, err := q.GetExpenseByID(ctx, sqlc.GetExpenseByIDParams{BusinessID: businessID, ID: expenseID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrExpenseNotFound
			}
			return fmt.Errorf("get expense: %w", err)
		}

		if _, err := q.GetReversalOfExpense(ctx, sqlc.GetReversalOfExpenseParams{
			BusinessID: businessID, ReversalOfExpenseID: pgUUID(expenseID),
		}); err == nil {
			return ErrAlreadyReversed
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check existing reversal: %w", err)
		}

		created, err := q.CreateExpense(ctx, sqlc.CreateExpenseParams{
			ID: reversalID, BusinessID: businessID, LocationID: original.LocationID, CategoryID: original.CategoryID,
			Description: "Reversal: " + original.Description, AmountKobo: -original.AmountKobo,
			PaymentMethod: original.PaymentMethod, RecordedBy: actorID, IdempotencyKey: idempotencyKey,
			OccurredAt: pgTimestamptz(now), ReversalOfExpenseID: pgUUID(expenseID),
		})
		if err != nil {
			return fmt.Errorf("create reversal expense: %w", err)
		}
		result = toExpense(created)
		return nil
	})
	if err != nil {
		return Expense{}, err
	}
	return result, nil
}

// ListExpenses returns the most recent expenses, newest first, with
// category names resolved via the same join ListPendingInventoryAdjustment
// RequestsDetailed uses for variant/product names -- a category isn't a
// person, so no separate name-resolution helper is needed the way actor
// names go through identity.Service elsewhere.
func (s *Service) ListExpenses(ctx context.Context, userID, businessID uuid.UUID, limit int32) ([]Expense, error) {
	var rows []sqlc.ListExpensesDetailedRow
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		r, err := q.ListExpensesDetailed(ctx, sqlc.ListExpensesDetailedParams{BusinessID: businessID, Limit: limit})
		if err != nil {
			return err
		}
		rows = r
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list expenses: %w", err)
	}
	out := make([]Expense, 0, len(rows))
	for _, r := range rows {
		out = append(out, toExpenseDetailed(r))
	}
	return out, nil
}

// SumExpensesSince mirrors sales.Service.SumSalesTotalSince exactly:
// Total nets every row including reversals, Count excludes reversal rows.
func (s *Service) SumExpensesSince(ctx context.Context, userID, businessID uuid.UUID, since time.Time) (Summary, error) {
	var row sqlc.SumExpensesTotalSinceRow
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		r, err := q.SumExpensesTotalSince(ctx, sqlc.SumExpensesTotalSinceParams{BusinessID: businessID, OccurredAt: pgTimestamptz(since)})
		if err != nil {
			return err
		}
		row = r
		return nil
	})
	if err != nil {
		return Summary{}, fmt.Errorf("sum expenses total: %w", err)
	}
	return Summary{TotalKobo: row.Total, Count: row.Count}, nil
}
