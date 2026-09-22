package reports_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/catalogue"
	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/expenses"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/inventory"
	"github.com/mananuf/justbarme/internal/reports"
	"github.com/mananuf/justbarme/internal/sales"
)

func testServices(t *testing.T) (*reports.Service, *sales.Service, *expenses.Service, *inventory.Service, *catalogue.Service, *identity.Service, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("JBM_DATABASE_URL")
	if url == "" {
		t.Skip("JBM_DATABASE_URL not set; skipping PostgreSQL-backed test")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("database not reachable: %v", err)
	}
	argon2 := config.Argon2{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1}
	return reports.New(pool), sales.New(pool, ""), expenses.New(pool), inventory.New(pool), catalogue.New(pool), identity.New(pool, argon2), pool
}

func uniqueName(prefix string) string {
	return prefix + "-" + uuid.New().String()
}

func newTenant(
	t *testing.T, ctx context.Context,
	inventorySvc *inventory.Service, catalogueSvc *catalogue.Service, identitySvc *identity.Service, pool *pgxpool.Pool,
) (ownerID, businessID, locationID, variantID uuid.UUID) {
	t.Helper()
	owner, err := identitySvc.CreateUser(ctx, uuid.New().String()+"@example.com", "", "Reports Test Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	business, _, location, err := identitySvc.CreateBusinessWithOwner(ctx, owner.ID, uniqueName("Test Bar"))
	if err != nil {
		t.Fatalf("CreateBusinessWithOwner: %v", err)
	}
	cleanupTenant(t, pool, owner.ID, business.ID)

	product, err := catalogueSvc.CreateProduct(ctx, owner.ID, business.ID, uuid.Nil, uniqueName("Trophy Lager"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant, err := catalogueSvc.CreateVariant(ctx, owner.ID, business.ID, product.ID, "50cl Bottle", 80000, true)
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}
	if _, err := inventorySvc.ReceiveStock(ctx, owner.ID, business.ID, location.ID, []inventory.ReceiptLine{
		{VariantID: variant.ID, Quantity: 48, TotalCostKobo: 3_600_000},
	}); err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}
	return owner.ID, business.ID, location.ID, variant.ID
}

// cleanupTenant mirrors internal/sales/service_test.go's RLS-aware
// pattern -- see that file's comment for why the delete order matters.
func cleanupTenant(t *testing.T, pool *pgxpool.Pool, ownerID, businessID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return
		}
		defer conn.Release()

		tx, err := conn.Begin(ctx)
		if err == nil {
			_, _ = tx.Exec(ctx, "SET LOCAL ROLE jbm_app")
			_, _ = tx.Exec(ctx, "SELECT set_config('app.business_id', $1, true)", businessID.String())
			for _, stmt := range []string{
				"DELETE FROM expenses WHERE business_id = $1",
				"DELETE FROM expense_categories WHERE business_id = $1",
				"DELETE FROM inventory_reviews WHERE business_id = $1",
				"DELETE FROM inventory_movements WHERE business_id = $1",
				"DELETE FROM inventory_events WHERE business_id = $1",
				"DELETE FROM inventory_adjustment_requests WHERE business_id = $1",
				"DELETE FROM stock_count_lines WHERE business_id = $1",
				"DELETE FROM stock_counts WHERE business_id = $1",
				"DELETE FROM payments WHERE business_id = $1",
				"DELETE FROM sale_item_lot_allocations WHERE business_id = $1",
				"DELETE FROM sale_items WHERE business_id = $1",
				"DELETE FROM sale_reviews WHERE business_id = $1",
				"DELETE FROM sales WHERE business_id = $1",
				"DELETE FROM bill_share_links WHERE business_id = $1",
				"DELETE FROM bills WHERE business_id = $1",
				"DELETE FROM inventory_balances WHERE business_id = $1",
				"DELETE FROM stock_lots WHERE business_id = $1",
				"DELETE FROM stock_receipt_lines WHERE business_id = $1",
				"DELETE FROM stock_receipts WHERE business_id = $1",
				"DELETE FROM product_prices WHERE business_id = $1",
				"DELETE FROM product_variants WHERE business_id = $1",
				"DELETE FROM products WHERE business_id = $1",
				"DELETE FROM categories WHERE business_id = $1",
				"DELETE FROM devices WHERE business_id = $1",
				"DELETE FROM locations WHERE business_id = $1",
				"DELETE FROM business_memberships WHERE business_id = $1",
			} {
				_, _ = tx.Exec(ctx, stmt, businessID)
			}
			_ = tx.Commit(ctx)
		}

		_, _ = conn.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1", ownerID)
		_, _ = conn.Exec(ctx, "DELETE FROM businesses WHERE id = $1", businessID)
		_, _ = conn.Exec(ctx, "DELETE FROM users WHERE id = $1", ownerID)
	})
}

func TestSalesByDayBucketsInBusinessTimezone(t *testing.T) {
	reportsSvc, salesSvc, _, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	if _, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 2, UnitPriceKobo: 80000}},
		sales.PaymentInput{AmountKobo: 160000, Method: "cash"}); err != nil {
		t.Fatalf("CreateSale: %v", err)
	}

	days, err := reportsSvc.SalesByDay(ctx, ownerID, businessID, "Africa/Lagos", time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("SalesByDay: %v", err)
	}
	if len(days) != 1 {
		t.Fatalf("expected exactly one bucketed day, got %d: %+v", len(days), days)
	}
	if days[0].TotalKobo != 160000 || days[0].SaleCount != 1 {
		t.Fatalf("expected total=160000 count=1, got %+v", days[0])
	}
}

func TestProductQuantitiesSoldExcludesReversals(t *testing.T) {
	reportsSvc, salesSvc, _, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	sale, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 3, UnitPriceKobo: 80000}},
		sales.PaymentInput{AmountKobo: 240000, Method: "cash"})
	if err != nil {
		t.Fatalf("CreateSale: %v", err)
	}
	if _, err := salesSvc.ReverseSale(ctx, ownerID, businessID, sale.ID, ownerID); err != nil {
		t.Fatalf("ReverseSale: %v", err)
	}

	products, err := reportsSvc.ProductQuantities(ctx, ownerID, businessID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("ProductQuantities: %v", err)
	}
	if len(products) != 1 {
		t.Fatalf("expected one product row, got %d: %+v", len(products), products)
	}
	if products[0].UnitsSold != 3 {
		t.Fatalf("expected units_sold=3 (reversal's negative quantity excluded), got %d", products[0].UnitsSold)
	}
}

func TestExpensesByCategorySumsPerCategory(t *testing.T) {
	reportsSvc, _, expensesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	category, err := expensesSvc.CreateCategory(ctx, ownerID, businessID, uniqueName("Utilities"))
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if _, err := expensesSvc.RecordExpense(ctx, ownerID, businessID, locationID, category.ID, ownerID,
		uuid.New(), "Electricity", 300000, expenses.PaymentMethodTransfer, time.Now()); err != nil {
		t.Fatalf("RecordExpense: %v", err)
	}
	if _, err := expensesSvc.RecordExpense(ctx, ownerID, businessID, locationID, category.ID, ownerID,
		uuid.New(), "Water", 100000, expenses.PaymentMethodCash, time.Now()); err != nil {
		t.Fatalf("RecordExpense: %v", err)
	}

	byCategory, err := reportsSvc.ExpensesByCategory(ctx, ownerID, businessID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("ExpensesByCategory: %v", err)
	}
	if len(byCategory) != 1 {
		t.Fatalf("expected one category row, got %d: %+v", len(byCategory), byCategory)
	}
	if byCategory[0].TotalKobo != 400000 || byCategory[0].ExpenseCount != 2 {
		t.Fatalf("expected total=400000 count=2, got %+v", byCategory[0])
	}
}

func TestStockDiscrepanciesReusesPhase7Data(t *testing.T) {
	reportsSvc, _, _, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	if _, err := inventorySvc.SubmitStockCount(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]inventory.StockCountLineInput{{VariantID: variantID, ExpectedQuantity: 48, PhysicalQuantity: 46}}); err != nil {
		t.Fatalf("SubmitStockCount: %v", err)
	}

	discrepancies, err := reportsSvc.StockDiscrepancies(ctx, ownerID, businessID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("StockDiscrepancies: %v", err)
	}
	if len(discrepancies) != 1 {
		t.Fatalf("expected one discrepancy row, got %d: %+v", len(discrepancies), discrepancies)
	}
	if discrepancies[0].Variance != -2 {
		t.Fatalf("expected variance=-2, got %d", discrepancies[0].Variance)
	}
}

// TestGrossMarginByProductComputesRealCostAndFlagsUnresolvedUnits covers
// both halves of docs/PHASE_FIFO_COSTING.md §8: a clean sale's resolved
// COGS/margin, and an oversold sale's unresolved-unit count (never a
// fabricated cost).
func TestGrossMarginByProductComputesRealCostAndFlagsUnresolvedUnits(t *testing.T) {
	reportsSvc, salesSvc, _, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	// newTenant already received 48 units at a total of 3,600,000 kobo
	// (75,000 kobo/unit).
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	if _, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 10, UnitPriceKobo: 80000}},
		sales.PaymentInput{AmountKobo: 800000, Method: "cash"},
	); err != nil {
		t.Fatalf("CreateSale (clean, within stock): %v", err)
	}

	// Oversell the remaining 38 by 5 -- 5 units end up with no known cost.
	if _, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 43, UnitPriceKobo: 80000}},
		sales.PaymentInput{AmountKobo: 3_440_000, Method: "cash"},
	); err != nil {
		t.Fatalf("CreateSale (oversell): %v", err)
	}

	rows, err := reportsSvc.GrossMarginByProduct(ctx, ownerID, businessID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("GrossMarginByProduct: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one product row, got %d: %+v", len(rows), rows)
	}
	row := rows[0]

	// Revenue: (10 + 43) * 80,000 = 4,240,000.
	if row.RevenueKobo != 4_240_000 {
		t.Fatalf("expected revenue 4,240,000, got %d", row.RevenueKobo)
	}
	// Resolved COGS: only the 48 units actually covered by the real lot,
	// at 75,000 kobo/unit = 3,600,000 (the lot's exact total, kobo-exact).
	if row.ResolvedCogsKobo != 3_600_000 {
		t.Fatalf("expected resolved COGS 3,600,000 (the lot's exact total), got %d", row.ResolvedCogsKobo)
	}
	if row.UnresolvedUnits != 5 {
		t.Fatalf("expected 5 unresolved units (the oversold amount), got %d", row.UnresolvedUnits)
	}
	if row.GrossMarginKobo() != 4_240_000-3_600_000 {
		t.Fatalf("expected gross margin 640,000, got %d", row.GrossMarginKobo())
	}
}

// TestStockPurchasesByProductSumsAcrossReceiptsAndVariants confirms the
// restocking-spend report answers "how much did I spend restocking?" --
// distinct from GrossMarginByProduct's COGS-of-sold-units-only figure.
func TestStockPurchasesByProductSumsAcrossReceiptsAndVariants(t *testing.T) {
	reportsSvc, _, _, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	// newTenant already received 48 units of one variant at 3,600,000 kobo.
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	// A second receipt for the same variant should sum into one row, not
	// create a duplicate.
	if _, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variantID, Quantity: 12, TotalCostKobo: 900_000},
	}); err != nil {
		t.Fatalf("ReceiveStock (second receipt): %v", err)
	}

	// A second variant's own receipt should appear as its own row.
	product2, err := catalogueSvc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, uniqueName("Star Lager"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant2, err := catalogueSvc.CreateVariant(ctx, ownerID, businessID, product2.ID, "60cl Bottle", 90000, true)
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}
	if _, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variant2.ID, Quantity: 24, TotalCostKobo: 1_200_000},
	}); err != nil {
		t.Fatalf("ReceiveStock (second variant): %v", err)
	}

	rows, err := reportsSvc.StockPurchasesByProduct(ctx, ownerID, businessID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("StockPurchasesByProduct: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two product rows, got %d: %+v", len(rows), rows)
	}

	byVariant := make(map[uuid.UUID]reports.StockPurchase, len(rows))
	for _, r := range rows {
		byVariant[r.VariantID] = r
	}

	first, ok := byVariant[variantID]
	if !ok {
		t.Fatalf("missing row for first variant: %+v", rows)
	}
	if first.QuantityReceived != 60 { // 48 + 12
		t.Fatalf("expected quantity 60, got %d", first.QuantityReceived)
	}
	if first.TotalKobo != 4_500_000 { // 3,600,000 + 900,000
		t.Fatalf("expected total 4,500,000, got %d", first.TotalKobo)
	}
	if first.ReceiptCount != 2 {
		t.Fatalf("expected 2 receipts, got %d", first.ReceiptCount)
	}

	second, ok := byVariant[variant2.ID]
	if !ok {
		t.Fatalf("missing row for second variant: %+v", rows)
	}
	if second.QuantityReceived != 24 {
		t.Fatalf("expected quantity 24, got %d", second.QuantityReceived)
	}
	if second.TotalKobo != 1_200_000 {
		t.Fatalf("expected total 1,200,000, got %d", second.TotalKobo)
	}
	if second.ReceiptCount != 1 {
		t.Fatalf("expected 1 receipt, got %d", second.ReceiptCount)
	}
}
