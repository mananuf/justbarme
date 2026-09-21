package inventory_test

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
	variant, err := catalogueSvc.CreateVariant(ctx, owner.ID, business.ID, product.ID, "50cl Bottle", 80000, true)
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
				"DELETE FROM inventory_reviews WHERE business_id = $1",
				"DELETE FROM inventory_adjustment_requests WHERE business_id = $1",
				"DELETE FROM stock_count_lines WHERE business_id = $1",
				"DELETE FROM stock_counts WHERE business_id = $1",
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

func TestReceiveStockRejectsUntrackedVariant(t *testing.T) {
	inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, catalogueSvc, identitySvc, pool)

	product, err := catalogueSvc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, uniqueName("Snooker"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant, err := catalogueSvc.CreateVariant(ctx, ownerID, businessID, product.ID, "Game", 50000, false)
	if err != nil {
		t.Fatalf("CreateVariant (untracked): %v", err)
	}

	_, err = inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variant.ID, Quantity: 1, TotalCostKobo: 100},
	})
	if err != inventory.ErrVariantNotTracked {
		t.Fatalf("expected ErrVariantNotTracked, got %v", err)
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

func TestSubmitStockCountWithCorrectExpectedCreatesAdjustmentRequest(t *testing.T) {
	inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, catalogueSvc, identitySvc, pool)

	if _, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variantID, Quantity: 20, TotalCostKobo: 1_000_000},
	}); err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}

	// Counted 18, expected (submitted, matches live balance) 20 --
	// variance -2, not stale, so it should become a pending
	// count_correction adjustment request, not a review.
	count, err := inventorySvc.SubmitStockCount(ctx, ownerID, businessID, locationID, ownerID,
		uuid.New(), time.Now(), []inventory.StockCountLineInput{
			{VariantID: variantID, ExpectedQuantity: 20, PhysicalQuantity: 18},
		})
	if err != nil {
		t.Fatalf("SubmitStockCount: %v", err)
	}
	if len(count.Lines) != 1 || count.Lines[0].Variance != -2 || count.Lines[0].IsStale {
		t.Fatalf("unexpected count line: %+v", count.Lines)
	}

	pending, err := inventorySvc.ListPendingAdjustmentRequests(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("ListPendingAdjustmentRequests: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending adjustment request, got %d", len(pending))
	}
	if pending[0].QuantityDelta != -2 || pending[0].ReasonCategory != inventory.AdjustmentReasonCountCorrection {
		t.Fatalf("unexpected pending request: %+v", pending[0])
	}
	if pending[0].SourceCountLineID != count.Lines[0].ID {
		t.Fatalf("expected SourceCountLineID to reference the count line")
	}

	reviews, err := inventorySvc.ListOpenReviews(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("ListOpenReviews: %v", err)
	}
	if len(reviews) != 0 {
		t.Fatalf("expected no reviews for a non-stale count, got %d", len(reviews))
	}
}

func TestSubmitStockCountMatchingQuantityCreatesNoAdjustment(t *testing.T) {
	inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, catalogueSvc, identitySvc, pool)

	if _, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variantID, Quantity: 10, TotalCostKobo: 500_000},
	}); err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}

	if _, err := inventorySvc.SubmitStockCount(ctx, ownerID, businessID, locationID, ownerID,
		uuid.New(), time.Now(), []inventory.StockCountLineInput{
			{VariantID: variantID, ExpectedQuantity: 10, PhysicalQuantity: 10},
		}); err != nil {
		t.Fatalf("SubmitStockCount: %v", err)
	}

	pending, err := inventorySvc.ListPendingAdjustmentRequests(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("ListPendingAdjustmentRequests: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no adjustment request when physical matches expected, got %d", len(pending))
	}
}

func TestSubmitStockCountStaleWhenExpectedDoesNotMatchLiveBalance(t *testing.T) {
	inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, catalogueSvc, identitySvc, pool)

	if _, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variantID, Quantity: 10, TotalCostKobo: 500_000},
	}); err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}

	// The device believes the balance is 5 (e.g. it was offline and this
	// is stale local knowledge), but the live balance is actually 10 --
	// something moved in between, so this line must come back stale with
	// a review, not a blindly-trusted adjustment request.
	count, err := inventorySvc.SubmitStockCount(ctx, ownerID, businessID, locationID, ownerID,
		uuid.New(), time.Now(), []inventory.StockCountLineInput{
			{VariantID: variantID, ExpectedQuantity: 5, PhysicalQuantity: 5},
		})
	if err != nil {
		t.Fatalf("SubmitStockCount: %v", err)
	}
	if !count.Lines[0].IsStale {
		t.Fatalf("expected the count line to be marked stale")
	}

	pending, err := inventorySvc.ListPendingAdjustmentRequests(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("ListPendingAdjustmentRequests: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no adjustment request for a stale line, got %d", len(pending))
	}

	reviews, err := inventorySvc.ListOpenReviews(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("ListOpenReviews: %v", err)
	}
	if len(reviews) != 1 || reviews[0].Type != inventory.ReviewTypeStaleStockCount {
		t.Fatalf("expected 1 stale_stock_count review, got %+v", reviews)
	}
	if reviews[0].RelatedCountLineID != count.Lines[0].ID {
		t.Fatalf("expected the review to reference the stale count line")
	}
}

func TestSubmitStockCountIsIdempotent(t *testing.T) {
	inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, catalogueSvc, identitySvc, pool)

	key := uuid.New()
	lines := []inventory.StockCountLineInput{{VariantID: variantID, ExpectedQuantity: 0, PhysicalQuantity: 3}}

	first, err := inventorySvc.SubmitStockCount(ctx, ownerID, businessID, locationID, ownerID, key, time.Now(), lines)
	if err != nil {
		t.Fatalf("first SubmitStockCount: %v", err)
	}
	second, err := inventorySvc.SubmitStockCount(ctx, ownerID, businessID, locationID, ownerID, key, time.Now(), lines)
	if err != nil {
		t.Fatalf("retried SubmitStockCount: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected the retried submission to return the same count, got %s and %s", first.ID, second.ID)
	}

	pending, err := inventorySvc.ListPendingAdjustmentRequests(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("ListPendingAdjustmentRequests: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected exactly 1 adjustment request despite the retry, got %d", len(pending))
	}
}

func TestRequestAdjustmentIsIdempotent(t *testing.T) {
	inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, catalogueSvc, identitySvc, pool)

	key := uuid.New()
	first, err := inventorySvc.RequestAdjustment(ctx, ownerID, businessID, locationID, variantID, ownerID,
		key, -1, inventory.AdjustmentReasonComplimentary, "Gave one to the building inspector.")
	if err != nil {
		t.Fatalf("first RequestAdjustment: %v", err)
	}
	second, err := inventorySvc.RequestAdjustment(ctx, ownerID, businessID, locationID, variantID, ownerID,
		key, -1, inventory.AdjustmentReasonComplimentary, "Gave one to the building inspector.")
	if err != nil {
		t.Fatalf("retried RequestAdjustment: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected the retried request to return the same row, got %s and %s", first.ID, second.ID)
	}

	pending, err := inventorySvc.ListPendingAdjustmentRequests(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("ListPendingAdjustmentRequests: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected exactly 1 pending request despite the retry, got %d", len(pending))
	}
}

func TestApproveAdjustmentRequestPostsMovementAndUpdatesBalance(t *testing.T) {
	inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, catalogueSvc, identitySvc, pool)

	if _, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variantID, Quantity: 10, TotalCostKobo: 500_000},
	}); err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}

	req, err := inventorySvc.RequestAdjustment(ctx, ownerID, businessID, locationID, variantID, ownerID,
		uuid.New(), -3, inventory.AdjustmentReasonBroken, "Bottle broke during setup.")
	if err != nil {
		t.Fatalf("RequestAdjustment: %v", err)
	}

	approved, err := inventorySvc.ApproveAdjustmentRequest(ctx, ownerID, businessID, req.ID, ownerID, "Confirmed with staff.")
	if err != nil {
		t.Fatalf("ApproveAdjustmentRequest: %v", err)
	}
	if approved.Status != inventory.AdjustmentStatusApproved {
		t.Fatalf("expected status approved, got %q", approved.Status)
	}

	balances, err := inventorySvc.GetBalances(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	if balances[variantID] != 7 {
		t.Fatalf("expected balance 7 after a -3 adjustment on 10, got %d", balances[variantID])
	}

	history, err := inventorySvc.GetHistory(ctx, ownerID, businessID, variantID, 10)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	var found bool
	for _, h := range history {
		if h.EventType == "adjustment" && h.AdjustmentReasonCategory == inventory.AdjustmentReasonBroken {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the history to include the approved adjustment with its reason, got %+v", history)
	}
}

func TestApproveAdjustmentRequestTwiceFails(t *testing.T) {
	inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, catalogueSvc, identitySvc, pool)

	req, err := inventorySvc.RequestAdjustment(ctx, ownerID, businessID, locationID, variantID, ownerID,
		uuid.New(), -1, inventory.AdjustmentReasonManual, "Test.")
	if err != nil {
		t.Fatalf("RequestAdjustment: %v", err)
	}
	if _, err := inventorySvc.ApproveAdjustmentRequest(ctx, ownerID, businessID, req.ID, ownerID, "ok"); err != nil {
		t.Fatalf("first ApproveAdjustmentRequest: %v", err)
	}
	if _, err := inventorySvc.ApproveAdjustmentRequest(ctx, ownerID, businessID, req.ID, ownerID, "ok again"); err != inventory.ErrAdjustmentRequestNotFound {
		t.Fatalf("expected ErrAdjustmentRequestNotFound on double-approval, got %v", err)
	}
}

func TestApproveAdjustmentRequestDrivingBalanceNegativeOpensReview(t *testing.T) {
	inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, catalogueSvc, identitySvc, pool)

	// No stock received at all -- balance starts at 0 (absent), so any
	// negative adjustment immediately drives it negative.
	req, err := inventorySvc.RequestAdjustment(ctx, ownerID, businessID, locationID, variantID, ownerID,
		uuid.New(), -2, inventory.AdjustmentReasonManual, "Test.")
	if err != nil {
		t.Fatalf("RequestAdjustment: %v", err)
	}
	if _, err := inventorySvc.ApproveAdjustmentRequest(ctx, ownerID, businessID, req.ID, ownerID, "ok"); err != nil {
		t.Fatalf("ApproveAdjustmentRequest: %v", err)
	}

	reviews, err := inventorySvc.ListOpenReviews(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("ListOpenReviews: %v", err)
	}
	if len(reviews) != 1 || reviews[0].Type != inventory.ReviewTypeNegativeInventory {
		t.Fatalf("expected 1 negative_inventory review, got %+v", reviews)
	}
}

func TestRejectAdjustmentRequestPostsNoMovement(t *testing.T) {
	inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, catalogueSvc, identitySvc, pool)

	if _, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variantID, Quantity: 5, TotalCostKobo: 250_000},
	}); err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}

	req, err := inventorySvc.RequestAdjustment(ctx, ownerID, businessID, locationID, variantID, ownerID,
		uuid.New(), -5, inventory.AdjustmentReasonManual, "Test.")
	if err != nil {
		t.Fatalf("RequestAdjustment: %v", err)
	}
	rejected, err := inventorySvc.RejectAdjustmentRequest(ctx, ownerID, businessID, req.ID, ownerID, "Not approved.")
	if err != nil {
		t.Fatalf("RejectAdjustmentRequest: %v", err)
	}
	if rejected.Status != inventory.AdjustmentStatusRejected {
		t.Fatalf("expected status rejected, got %q", rejected.Status)
	}

	balances, err := inventorySvc.GetBalances(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	if balances[variantID] != 5 {
		t.Fatalf("expected balance to remain 5 after a rejected adjustment, got %d", balances[variantID])
	}
}

func TestResolveInventoryReviewMarksResolved(t *testing.T) {
	inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, catalogueSvc, identitySvc, pool)

	req, err := inventorySvc.RequestAdjustment(ctx, ownerID, businessID, locationID, variantID, ownerID,
		uuid.New(), -1, inventory.AdjustmentReasonManual, "Test.")
	if err != nil {
		t.Fatalf("RequestAdjustment: %v", err)
	}
	if _, err := inventorySvc.ApproveAdjustmentRequest(ctx, ownerID, businessID, req.ID, ownerID, "ok"); err != nil {
		t.Fatalf("ApproveAdjustmentRequest: %v", err)
	}

	open, err := inventorySvc.ListOpenReviews(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("ListOpenReviews: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("expected 1 open review, got %d", len(open))
	}

	resolved, err := inventorySvc.ResolveReview(ctx, ownerID, businessID, open[0].ID, ownerID, "Checked, restocking tomorrow.")
	if err != nil {
		t.Fatalf("ResolveReview: %v", err)
	}
	if resolved.Status != "resolved" {
		t.Fatalf("expected status resolved, got %q", resolved.Status)
	}

	stillOpen, err := inventorySvc.ListOpenReviews(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("ListOpenReviews after resolve: %v", err)
	}
	if len(stillOpen) != 0 {
		t.Fatalf("expected 0 open reviews after resolving, got %d", len(stillOpen))
	}
}
