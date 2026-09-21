package activity_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/activity"
	"github.com/mananuf/justbarme/internal/catalogue"
	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/expenses"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/inventory"
	"github.com/mananuf/justbarme/internal/sales"
)

func testServices(t *testing.T) (*activity.Service, *sales.Service, *expenses.Service, *inventory.Service, *catalogue.Service, *identity.Service, *pgxpool.Pool) {
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
	return activity.New(pool), sales.New(pool, ""), expenses.New(pool), inventory.New(pool), catalogue.New(pool), identity.New(pool, argon2), pool
}

func uniqueName(prefix string) string {
	return prefix + "-" + uuid.New().String()
}

func newTenant(
	t *testing.T, ctx context.Context,
	inventorySvc *inventory.Service, catalogueSvc *catalogue.Service, identitySvc *identity.Service, pool *pgxpool.Pool,
) (ownerID, businessID, locationID, variantID uuid.UUID) {
	t.Helper()
	owner, err := identitySvc.CreateUser(ctx, uuid.New().String()+"@example.com", "", "Activity Test Owner", "password123")
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

func TestListActivityCombinesSalesAndExpenses(t *testing.T) {
	activitySvc, salesSvc, expensesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	sale, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}},
		sales.PaymentInput{AmountKobo: 80000, Method: "cash"})
	if err != nil {
		t.Fatalf("CreateSale: %v", err)
	}

	category, err := expensesSvc.CreateCategory(ctx, ownerID, businessID, uniqueName("Supplies"))
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	expense, err := expensesSvc.RecordExpense(ctx, ownerID, businessID, locationID, category.ID, ownerID,
		uuid.New(), "Cleaning supplies", 500000, expenses.PaymentMethodCash, time.Now())
	if err != nil {
		t.Fatalf("RecordExpense: %v", err)
	}

	entries, err := activitySvc.List(ctx, ownerID, businessID, activity.Filter{}, 20)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// newTenant itself posts a stock receipt (to stock the variant sold
	// below), so the feed also carries that -- 3 entries, not 2.
	if len(entries) != 3 {
		t.Fatalf("expected 3 activity entries (1 receipt + 1 sale + 1 expense), got %d: %+v", len(entries), entries)
	}

	var sawSale, sawExpense bool
	for _, e := range entries {
		switch e.ID {
		case sale.ID:
			sawSale = true
			if e.Type != activity.TypeSale {
				t.Fatalf("expected sale entry type %q, got %q", activity.TypeSale, e.Type)
			}
			if e.AmountKobo != sale.TotalKobo {
				t.Fatalf("expected sale amount_kobo=%d (money in), got %d", sale.TotalKobo, e.AmountKobo)
			}
		case expense.ID:
			sawExpense = true
			if e.Type != activity.TypeExpense {
				t.Fatalf("expected expense entry type %q, got %q", activity.TypeExpense, e.Type)
			}
			if e.AmountKobo != -expense.AmountKobo {
				t.Fatalf("expected expense amount_kobo=%d (money out, flipped sign), got %d", -expense.AmountKobo, e.AmountKobo)
			}
		}
	}
	if !sawSale || !sawExpense {
		t.Fatalf("expected both the sale and the expense in the feed, sawSale=%v sawExpense=%v", sawSale, sawExpense)
	}
}

func TestListActivityFiltersByType(t *testing.T) {
	activitySvc, salesSvc, expensesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	if _, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}},
		sales.PaymentInput{AmountKobo: 80000, Method: "cash"}); err != nil {
		t.Fatalf("CreateSale: %v", err)
	}
	category, err := expensesSvc.CreateCategory(ctx, ownerID, businessID, uniqueName("Supplies"))
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if _, err := expensesSvc.RecordExpense(ctx, ownerID, businessID, locationID, category.ID, ownerID,
		uuid.New(), "Cleaning supplies", 500000, expenses.PaymentMethodCash, time.Now()); err != nil {
		t.Fatalf("RecordExpense: %v", err)
	}

	entries, err := activitySvc.List(ctx, ownerID, businessID, activity.Filter{Type: activity.TypeExpense}, 20)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 || entries[0].Type != activity.TypeExpense {
		t.Fatalf("expected exactly one expense entry when filtered by type, got %+v", entries)
	}
}

func TestCountByDayCombinesSources(t *testing.T) {
	activitySvc, salesSvc, expensesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	if _, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}},
		sales.PaymentInput{AmountKobo: 80000, Method: "cash"}); err != nil {
		t.Fatalf("CreateSale: %v", err)
	}
	category, err := expensesSvc.CreateCategory(ctx, ownerID, businessID, uniqueName("Supplies"))
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if _, err := expensesSvc.RecordExpense(ctx, ownerID, businessID, locationID, category.ID, ownerID,
		uuid.New(), "Cleaning supplies", 500000, expenses.PaymentMethodCash, time.Now()); err != nil {
		t.Fatalf("RecordExpense: %v", err)
	}
	// ReceiveStock in newTenant already posted a stock receipt too, so
	// today's count should be 3: 1 sale + 1 expense + 1 receipt.

	counts, err := activitySvc.CountByDay(ctx, ownerID, businessID, "Africa/Lagos", time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("CountByDay: %v", err)
	}
	var total int64
	for _, c := range counts {
		total += c.Count
	}
	if total != 3 {
		t.Fatalf("expected 3 combined activity events today (sale+expense+receipt), got %d: %+v", total, counts)
	}
}
