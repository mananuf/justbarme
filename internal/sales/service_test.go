package sales_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/catalogue"
	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/inventory"
	"github.com/mananuf/justbarme/internal/sales"
)

func testServices(t *testing.T) (*sales.Service, *inventory.Service, *catalogue.Service, *identity.Service, *pgxpool.Pool) {
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
	return sales.New(pool, ""), inventory.New(pool), catalogue.New(pool), identity.New(pool, argon2), pool
}

func uniqueName(prefix string) string {
	return prefix + "-" + uuid.New().String()
}

// newTenant creates a fresh owner, business, a stocked variant (48 bottles
// received), via the same real internal/identity, internal/catalogue, and
// internal/inventory paths this app itself uses.
func newTenant(
	t *testing.T, ctx context.Context,
	inventorySvc *inventory.Service, catalogueSvc *catalogue.Service, identitySvc *identity.Service, pool *pgxpool.Pool,
) (ownerID, businessID, locationID, variantID uuid.UUID) {
	t.Helper()
	owner, err := identitySvc.CreateUser(ctx, uuid.New().String()+"@example.com", "", "Sales Test Owner", "password123")
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
	variant, err := catalogueSvc.CreateVariant(ctx, owner.ID, business.ID, product.ID, "50cl Bottle", 80000)
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

// cleanupTenant mirrors internal/inventory/service_test.go's and
// internal/platformadmin/service_test.go's RLS-aware pattern: a raw DELETE
// on an RLS-scoped table only actually removes rows when the connecting
// role is an outright Postgres superuser, so this sets the same role and
// app.business_id a real request would (store.WithTenant's own mechanics).
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
			// Order matters: a child must be deleted before anything it
			// has a foreign key to. inventory_events.sale_id (added by
			// migration 000019) means inventory_movements/events must go
			// before sales, not after -- getting this backwards leaves
			// every sale-creating test's rows (and their owning
			// user/business, blocked transitively) permanently leaked
			// under a non-superuser JBM_DATABASE_URL, exactly the
			// CLAUDE.md-documented failure mode for this pattern.
			for _, stmt := range []string{
				"DELETE FROM sale_reviews WHERE business_id = $1",
				"DELETE FROM sale_items WHERE business_id = $1",
				"DELETE FROM inventory_reviews WHERE business_id = $1",
				"DELETE FROM inventory_movements WHERE business_id = $1",
				"DELETE FROM inventory_events WHERE business_id = $1",
				"DELETE FROM payments WHERE business_id = $1",
				"DELETE FROM bill_write_offs WHERE business_id = $1",
				"DELETE FROM sales WHERE business_id = $1",
				// bill_share_links (migration 000024) has its own FK to
				// bills -- must go before it, same reasoning as every
				// other entry in this list.
				"DELETE FROM bill_share_links WHERE business_id = $1",
				"DELETE FROM bills WHERE business_id = $1",
				"DELETE FROM customers WHERE business_id = $1",
				"DELETE FROM tables WHERE business_id = $1",
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

func TestCreateSalePostsAtomicallyAndDeductsStock(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	sale, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 3, UnitPriceKobo: 80000}},
		sales.PaymentInput{AmountKobo: 240000, Method: sales.PaymentMethodCash},
	)
	if err != nil {
		t.Fatalf("CreateSale: %v", err)
	}
	if sale.TotalKobo != 240000 {
		t.Fatalf("expected total 240000, got %d", sale.TotalKobo)
	}
	if len(sale.Items) != 1 || sale.Items[0].LineTotalKobo != 240000 {
		t.Fatalf("unexpected sale items: %+v", sale.Items)
	}
	if sale.Payment.AmountKobo != 240000 || sale.Payment.Method != sales.PaymentMethodCash {
		t.Fatalf("unexpected payment: %+v", sale.Payment)
	}
	if len(sale.Reviews) != 0 {
		t.Fatalf("expected no reviews for a clean sale, got %+v", sale.Reviews)
	}

	balances, err := inventorySvc.GetBalances(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	if balances[variantID] != 45 {
		t.Fatalf("expected stock to drop from 48 to 45, got %d", balances[variantID])
	}
}

func TestCreateSaleOversellOpensNegativeInventoryReview(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	// newTenant stocks exactly 48 -- selling 50 is a real, offline-capable
	// oversell scenario (docs/ARCHITECTURE.md §8.5): the sale must still
	// post exactly as submitted, never rejected for insufficient stock,
	// with the resulting negative balance surfaced as a review instead.
	sale, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 50, UnitPriceKobo: 80000}},
		sales.PaymentInput{AmountKobo: 4_000_000, Method: sales.PaymentMethodCash},
	)
	if err != nil {
		t.Fatalf("CreateSale: %v", err)
	}
	if sale.TotalKobo != 4_000_000 {
		t.Fatalf("expected the oversold sale to post at its full submitted total, got %d", sale.TotalKobo)
	}

	balances, err := inventorySvc.GetBalances(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	if balances[variantID] != -2 {
		t.Fatalf("expected balance to go negative (48 - 50 = -2), got %d", balances[variantID])
	}

	reviews, err := inventorySvc.ListOpenReviews(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("ListOpenReviews: %v", err)
	}
	if len(reviews) != 1 || reviews[0].Type != inventory.ReviewTypeNegativeInventory {
		t.Fatalf("expected 1 negative_inventory review, got %+v", reviews)
	}
	if reviews[0].VariantID != variantID {
		t.Fatalf("expected the review to reference the oversold variant")
	}
}

func TestCreateSaleIsIdempotent(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)
	key := uuid.New()

	first, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, key, time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 2, UnitPriceKobo: 80000}},
		sales.PaymentInput{AmountKobo: 160000, Method: sales.PaymentMethodCash},
	)
	if err != nil {
		t.Fatalf("first CreateSale: %v", err)
	}

	second, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, key, time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 2, UnitPriceKobo: 80000}},
		sales.PaymentInput{AmountKobo: 160000, Method: sales.PaymentMethodCash},
	)
	if err != nil {
		t.Fatalf("retried CreateSale: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected the retried call to return the same sale, got a different ID: %s vs %s", first.ID, second.ID)
	}

	balances, err := inventorySvc.GetBalances(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	if balances[variantID] != 46 {
		t.Fatalf("expected stock deducted only once (48 -> 46), got %d", balances[variantID])
	}
}

func TestCreateSaleRejectsEmptyItems(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	_, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(), nil,
		sales.PaymentInput{AmountKobo: 0, Method: sales.PaymentMethodCash})
	if err != sales.ErrNoItems {
		t.Fatalf("expected ErrNoItems, got %v", err)
	}
}

func TestCreateSaleRejectsUnknownVariant(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	_, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: uuid.New(), Quantity: 1, UnitPriceKobo: 100}},
		sales.PaymentInput{AmountKobo: 100, Method: sales.PaymentMethodCash},
	)
	if err != sales.ErrVariantNotFound {
		t.Fatalf("expected ErrVariantNotFound, got %v", err)
	}
}

func TestCreateSaleFlagsDeactivatedVariantButStillPosts(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	if _, err := catalogueSvc.UpdateVariant(ctx, ownerID, businessID, variantID, catalogue.UpdateVariantParams{Name: "50cl Bottle", Active: false}); err != nil {
		t.Fatalf("UpdateVariant (deactivate): %v", err)
	}

	sale, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}},
		sales.PaymentInput{AmountKobo: 80000, Method: sales.PaymentMethodCash},
	)
	if err != nil {
		t.Fatalf("expected the sale against a deactivated variant to still post, got error: %v", err)
	}
	if len(sale.Reviews) != 1 || sale.Reviews[0].Reason != sales.ReviewReasonDeactivatedVariant {
		t.Fatalf("expected one deactivated_variant review, got %+v", sale.Reviews)
	}
}

func TestCreateSaleFlagsPriceMismatchButStillPosts(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	// Current price is 80000 (set in newTenant); submit a price that was
	// never actually in effect for this variant.
	sale, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 50000}},
		sales.PaymentInput{AmountKobo: 50000, Method: sales.PaymentMethodCash},
	)
	if err != nil {
		t.Fatalf("expected the sale with a mismatched price to still post, got error: %v", err)
	}
	if sale.Items[0].UnitPriceKobo != 50000 {
		t.Fatalf("expected the submitted price to be preserved as the actual charge, got %d", sale.Items[0].UnitPriceKobo)
	}
	if len(sale.Reviews) != 1 || sale.Reviews[0].Reason != sales.ReviewReasonPriceMismatch {
		t.Fatalf("expected one price_mismatch review, got %+v", sale.Reviews)
	}
}

func TestReverseSaleNegatesAndRestocksExactly(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	sale, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 5, UnitPriceKobo: 80000}},
		sales.PaymentInput{AmountKobo: 400000, Method: sales.PaymentMethodTransfer},
	)
	if err != nil {
		t.Fatalf("CreateSale: %v", err)
	}

	reversal, err := salesSvc.ReverseSale(ctx, ownerID, businessID, sale.ID, ownerID)
	if err != nil {
		t.Fatalf("ReverseSale: %v", err)
	}
	if reversal.ReversalOf != sale.ID {
		t.Fatalf("expected ReversalOf to point at the original sale")
	}
	if reversal.TotalKobo != -400000 {
		t.Fatalf("expected the reversal's total to be -400000, got %d", reversal.TotalKobo)
	}
	if reversal.Payment.AmountKobo != -400000 {
		t.Fatalf("expected a -400000 refund payment, got %d", reversal.Payment.AmountKobo)
	}

	balances, err := inventorySvc.GetBalances(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	if balances[variantID] != 48 {
		t.Fatalf("expected stock restored to 48 after reversal, got %d", balances[variantID])
	}

	net, count, err := salesSvc.SumSalesTotalSince(ctx, ownerID, businessID, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("SumSalesTotalSince: %v", err)
	}
	if net != 0 {
		t.Fatalf("expected the sale and its reversal to net to 0, got %d", net)
	}
	if count != 1 {
		t.Fatalf("expected the reversal to be excluded from the count (just the 1 original sale), got %d", count)
	}
}

func TestReverseSaleRejectsDoubleReversal(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	sale, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}},
		sales.PaymentInput{AmountKobo: 80000, Method: sales.PaymentMethodCash},
	)
	if err != nil {
		t.Fatalf("CreateSale: %v", err)
	}
	if _, err := salesSvc.ReverseSale(ctx, ownerID, businessID, sale.ID, ownerID); err != nil {
		t.Fatalf("first ReverseSale: %v", err)
	}
	if _, err := salesSvc.ReverseSale(ctx, ownerID, businessID, sale.ID, ownerID); err != sales.ErrAlreadyReversed {
		t.Fatalf("expected ErrAlreadyReversed, got %v", err)
	}
}

func TestResolveReviewMarksResolved(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	sale, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 12345}},
		sales.PaymentInput{AmountKobo: 12345, Method: sales.PaymentMethodCash},
	)
	if err != nil {
		t.Fatalf("CreateSale: %v", err)
	}
	if len(sale.Reviews) != 1 {
		t.Fatalf("expected one review to open, got %+v", sale.Reviews)
	}

	open, err := salesSvc.ListOpenReviews(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("ListOpenReviews: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("expected exactly one open review, got %d", len(open))
	}

	resolved, err := salesSvc.ResolveReview(ctx, ownerID, businessID, open[0].ID, ownerID, "Checked with staff, price was correct at the time.")
	if err != nil {
		t.Fatalf("ResolveReview: %v", err)
	}
	if resolved.Status != "resolved" {
		t.Fatalf("expected status resolved, got %q", resolved.Status)
	}

	stillOpen, err := salesSvc.ListOpenReviews(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("ListOpenReviews after resolve: %v", err)
	}
	if len(stillOpen) != 0 {
		t.Fatalf("expected no open reviews left, got %+v", stillOpen)
	}
}
