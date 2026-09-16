package inventory_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/catalogue"
	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/inventory"
)

func testServices(t *testing.T) (*inventory.Service, *catalogue.Service, *identity.Service, *pgxpool.Pool) {
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
	return inventory.New(pool), catalogue.New(pool), identity.New(pool, argon2), pool
}

func uniqueName(prefix string) string {
	return prefix + "-" + uuid.New().String()
}

// newTenant creates a fresh owner, business, and one priced variant to
// receive stock against, via the same real internal/identity and
// internal/catalogue paths a real signup and onboarding go through.
func newTenant(t *testing.T, ctx context.Context, catalogueSvc *catalogue.Service, identitySvc *identity.Service, pool *pgxpool.Pool) (ownerID, businessID, locationID, variantID uuid.UUID) {
	t.Helper()
	owner, err := identitySvc.CreateUser(ctx, uuid.New().String()+"@example.com", "", "Inventory Test Owner", "password123")
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

	return owner.ID, business.ID, location.ID, variant.ID
}

// cleanupTenant is the RLS-aware pattern internal/platformadmin/service_test.go
// establishes: a raw DELETE on an RLS-scoped (FORCE ROW LEVEL SECURITY)
// table only actually removes rows when the connecting role is an outright
// Postgres superuser. Setting the same role and app.business_id a real
// request would set (store.WithTenant's own mechanics) makes this correct
// regardless of which kind of role JBM_DATABASE_URL names.
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
				"DELETE FROM inventory_balances WHERE business_id = $1",
				"DELETE FROM inventory_movements WHERE business_id = $1",
				"DELETE FROM inventory_events WHERE business_id = $1",
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

		// businesses/users carry no RLS at all, so a plain DELETE is
		// correct on any role with table privileges.
		_, _ = conn.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1", ownerID)
		_, _ = conn.Exec(ctx, "DELETE FROM businesses WHERE id = $1", businessID)
		_, _ = conn.Exec(ctx, "DELETE FROM users WHERE id = $1", ownerID)
	})
}

func TestReceiveStockCreatesReceiptAndBalance(t *testing.T) {
	inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, catalogueSvc, identitySvc, pool)

	// "4 crates of Trophy Lager at ₦43,000 total, 12 bottles per crate" --
	// one receipt line: quantity = 48, total cost = 4,300,000 kobo. The
	// per-bottle figure (≈896 kobo) is never itself stored.
	receipt, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variantID, Quantity: 48, TotalCostKobo: 4_300_000},
	})
	if err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}
	if receipt.BusinessID != businessID || receipt.LocationID != locationID || receipt.ReceivedBy != ownerID {
		t.Fatalf("unexpected receipt header: %+v", receipt)
	}
	if len(receipt.Lines) != 1 || receipt.Lines[0].Quantity != 48 || receipt.Lines[0].TotalCostKobo != 4_300_000 {
		t.Fatalf("unexpected receipt lines: %+v", receipt.Lines)
	}
	if receipt.Lines[0].NewBalance != 48 {
		t.Fatalf("expected NewBalance 48, got %d", receipt.Lines[0].NewBalance)
	}

	balances, err := inventorySvc.GetBalances(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	if balances[variantID] != 48 {
		t.Fatalf("expected balance 48, got %d", balances[variantID])
	}
}

func TestReceiveStockAccumulatesAcrossReceipts(t *testing.T) {
	inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, catalogueSvc, identitySvc, pool)

	if _, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variantID, Quantity: 12, TotalCostKobo: 875000},
	}); err != nil {
		t.Fatalf("first ReceiveStock: %v", err)
	}
	if _, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variantID, Quantity: 24, TotalCostKobo: 1750000},
	}); err != nil {
		t.Fatalf("second ReceiveStock: %v", err)
	}

	balances, err := inventorySvc.GetBalances(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	if balances[variantID] != 36 {
		t.Fatalf("expected accumulated balance 36, got %d", balances[variantID])
	}
}

func TestReceiveStockRejectsEmptyLines(t *testing.T) {
	inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, catalogueSvc, identitySvc, pool)

	if _, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, nil); err != inventory.ErrNoLines {
		t.Fatalf("expected ErrNoLines, got %v", err)
	}
}

func TestReceiveStockRejectsUnknownVariant(t *testing.T) {
	inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, catalogueSvc, identitySvc, pool)

	_, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: uuid.New(), Quantity: 1, TotalCostKobo: 100},
	})
	if err != inventory.ErrVariantNotFound {
		t.Fatalf("expected ErrVariantNotFound, got %v", err)
	}
}

func TestGetBalancesOmitsVariantsWithNoReceipts(t *testing.T) {
	inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, _, variantID := newTenant(t, ctx, catalogueSvc, identitySvc, pool)

	balances, err := inventorySvc.GetBalances(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	if _, ok := balances[variantID]; ok {
		t.Fatalf("expected no balance entry for a variant with no receipts, got %d", balances[variantID])
	}
}
