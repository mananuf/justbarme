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
