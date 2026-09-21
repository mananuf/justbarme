package reports

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/store"
	"github.com/mananuf/justbarme/internal/store/sqlc"
)

// Service is deliberately read-only: every report here queries typed
// projection/transaction tables that other packages already post to
// (docs/ARCHITECTURE.md §14's "MVP reports should query typed projection/
// transaction tables, not raw JSON event payloads") -- it never writes
// anything and does not import internal/sales/internal/inventory/
// internal/expenses' Service types, only the shared sqlc layer, per this
// codebase's "share the generated sqlc layer directly rather than calling
// another package's Service" convention.
type Service struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func pgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// SalesByDay backs the Sales report's daily-revenue heatmap. Bucketed in
// timezone (the business's own, per docs/ARCHITECTURE.md §14), not UTC.
func (s *Service) SalesByDay(ctx context.Context, userID, businessID uuid.UUID, timezone string, start, end time.Time) ([]SalesDay, error) {
	var rows []sqlc.SumSalesByDayRow
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		r, err := q.SumSalesByDay(ctx, sqlc.SumSalesByDayParams{
			BusinessID: businessID, Column2: timezone, OccurredAt: pgTimestamptz(start), OccurredAt_2: pgTimestamptz(end),
		})
		if err != nil {
			return err
		}
		rows = r
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("sales by day: %w", err)
	}
	out := make([]SalesDay, 0, len(rows))
	for _, r := range rows {
		out = append(out, SalesDay{Day: r.Day.Time, TotalKobo: r.TotalKobo, SaleCount: r.SaleCount})
	}
	return out, nil
}

// ProductQuantities returns units sold per variant within [start, end).
func (s *Service) ProductQuantities(ctx context.Context, userID, businessID uuid.UUID, start, end time.Time) ([]ProductQuantity, error) {
	var rows []sqlc.ListProductQuantitiesSoldRow
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		r, err := q.ListProductQuantitiesSold(ctx, sqlc.ListProductQuantitiesSoldParams{
			BusinessID: businessID, OccurredAt: pgTimestamptz(start), OccurredAt_2: pgTimestamptz(end),
		})
		if err != nil {
			return err
		}
		rows = r
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("product quantities: %w", err)
	}
	out := make([]ProductQuantity, 0, len(rows))
	for _, r := range rows {
		out = append(out, ProductQuantity{VariantID: r.VariantID, VariantName: r.VariantName, ProductName: r.ProductName, UnitsSold: r.UnitsSold})
	}
	return out, nil
}

// StaffSales returns totals grouped by seller within [start, end).
func (s *Service) StaffSales(ctx context.Context, userID, businessID uuid.UUID, start, end time.Time) ([]StaffSales, error) {
	var rows []sqlc.ListStaffSalesRow
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		r, err := q.ListStaffSales(ctx, sqlc.ListStaffSalesParams{
			BusinessID: businessID, OccurredAt: pgTimestamptz(start), OccurredAt_2: pgTimestamptz(end),
		})
		if err != nil {
			return err
		}
		rows = r
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("staff sales: %w", err)
	}
	out := make([]StaffSales, 0, len(rows))
	for _, r := range rows {
		out = append(out, StaffSales{SellerID: r.SellerID, TotalKobo: r.TotalKobo, SaleCount: r.SaleCount})
	}
	return out, nil
}

// ExpensesByCategory returns totals grouped by category within [start, end).
func (s *Service) ExpensesByCategory(ctx context.Context, userID, businessID uuid.UUID, start, end time.Time) ([]ExpensesByCategory, error) {
	var rows []sqlc.SumExpensesByCategoryRow
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		r, err := q.SumExpensesByCategory(ctx, sqlc.SumExpensesByCategoryParams{
			BusinessID: businessID, OccurredAt: pgTimestamptz(start), OccurredAt_2: pgTimestamptz(end),
		})
		if err != nil {
			return err
		}
		rows = r
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("expenses by category: %w", err)
	}
	out := make([]ExpensesByCategory, 0, len(rows))
	for _, r := range rows {
		out = append(out, ExpensesByCategory{CategoryID: r.CategoryID, CategoryName: r.CategoryName, TotalKobo: r.TotalKobo, ExpenseCount: r.ExpenseCount})
	}
	return out, nil
}

// StockDiscrepancies reuses Phase 7's own count/variance data directly --
// no new schema. A count line's date is its parent count's started_at.
func (s *Service) StockDiscrepancies(ctx context.Context, userID, businessID uuid.UUID, start, end time.Time) ([]StockDiscrepancy, error) {
	var rows []sqlc.ListStockDiscrepanciesRow
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		r, err := q.ListStockDiscrepancies(ctx, sqlc.ListStockDiscrepanciesParams{
			BusinessID: businessID, StartedAt: pgTimestamptz(start), StartedAt_2: pgTimestamptz(end),
		})
		if err != nil {
			return err
		}
		rows = r
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("stock discrepancies: %w", err)
	}
	out := make([]StockDiscrepancy, 0, len(rows))
	for _, r := range rows {
		out = append(out, StockDiscrepancy{
			ID: r.ID, VariantID: r.VariantID, VariantName: r.VariantName, ProductName: r.ProductName,
			ExpectedQuantity: r.ExpectedQuantity, PhysicalQuantity: r.PhysicalQuantity, Variance: r.Variance,
			IsStale: r.IsStale, CountedAt: r.StartedAt.Time, CountedBy: r.CountedBy,
		})
	}
	return out, nil
}

// GrossMarginByProduct backs the FIFO gross-margin report
// (docs/PHASE_FIFO_COSTING.md §8) -- revenue and resolved cost of goods
// sold per product/variant within [start, end), plus the unresolved-unit
// count that must always accompany it. See GrossMargin's own doc comment
// for why a nonzero UnresolvedUnits means the margin figure is a lower
// bound, not a final number.
func (s *Service) GrossMarginByProduct(ctx context.Context, userID, businessID uuid.UUID, start, end time.Time) ([]GrossMargin, error) {
	var rows []sqlc.GrossMarginByProductRow
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		r, err := q.GrossMarginByProduct(ctx, sqlc.GrossMarginByProductParams{
			BusinessID: businessID, OccurredAt: pgTimestamptz(start), OccurredAt_2: pgTimestamptz(end),
		})
		if err != nil {
			return err
		}
		rows = r
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("gross margin by product: %w", err)
	}
	out := make([]GrossMargin, 0, len(rows))
	for _, r := range rows {
		out = append(out, GrossMargin{
			VariantID: r.VariantID, VariantName: r.VariantName, ProductName: r.ProductName,
			RevenueKobo: r.RevenueKobo, ResolvedCogsKobo: r.ResolvedCogsKobo, UnresolvedUnits: r.UnresolvedUnits,
		})
	}
	return out, nil
}
