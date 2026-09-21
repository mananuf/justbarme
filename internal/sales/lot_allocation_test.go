package sales_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/inventory"
	"github.com/mananuf/justbarme/internal/sales"
)

type testAllocation struct {
	stockLotID        *uuid.UUID
	quantity          int32
	allocatedCostKobo *int64
}

// allocationsForSaleItem reads sale_item_lot_allocations directly -- there
// is no public Service method for this (it's an internal accounting
// ledger, not a read API this phase adds a frontend for yet) -- via the
// same RLS-aware transaction pattern this file's own cleanupTenant uses.
func allocationsForSaleItem(t *testing.T, pool *pgxpool.Pool, businessID, saleItemID uuid.UUID) []testAllocation {
	t.Helper()
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire conn: %v", err)
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, "SET LOCAL ROLE jbm_app"); err != nil {
		t.Fatalf("set role: %v", err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.business_id', $1, true)", businessID.String()); err != nil {
		t.Fatalf("set business_id: %v", err)
	}

	rows, err := tx.Query(ctx,
		"SELECT stock_lot_id, quantity, allocated_cost_kobo FROM sale_item_lot_allocations WHERE business_id = $1 AND sale_item_id = $2 ORDER BY created_at ASC",
		businessID, saleItemID,
	)
	if err != nil {
		t.Fatalf("query allocations: %v", err)
	}
	defer rows.Close()

	var out []testAllocation
	for rows.Next() {
		var a testAllocation
		var lotID *uuid.UUID
		var cost *int64
		if err := rows.Scan(&lotID, &a.quantity, &cost); err != nil {
			t.Fatalf("scan allocation: %v", err)
		}
		a.stockLotID = lotID
		a.allocatedCostKobo = cost
		out = append(out, a)
	}
	return out
}

func lotRemainingQuantity(t *testing.T, pool *pgxpool.Pool, businessID, lotID uuid.UUID) int32 {
	t.Helper()
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire conn: %v", err)
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, "SET LOCAL ROLE jbm_app"); err != nil {
		t.Fatalf("set role: %v", err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.business_id', $1, true)", businessID.String()); err != nil {
		t.Fatalf("set business_id: %v", err)
	}

	var remaining int32
	if err := tx.QueryRow(ctx, "SELECT remaining_quantity FROM stock_lots WHERE business_id = $1 AND id = $2", businessID, lotID).Scan(&remaining); err != nil {
		t.Fatalf("query lot remaining_quantity: %v", err)
	}
	return remaining
}

// TestFIFOAllocatesOldestLotFirst is the scenario the design doc's own
// worked example is built from: 10 bottles received for ₦10,000 (₦1,000/
// bottle), then 5 more for ₦5,700 (₦1,140/bottle), then 12 sold. FIFO
// must draw all 10 from the first (cheaper) lot before touching the
// second, and the two allocations' costs must sum to the sale's real
// cost: ₦10,000 + (2 × ₦1,140) = ₦12,280.
func TestFIFOAllocatesOldestLotFirst(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	product, err := catalogueSvc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, uniqueName("Coke"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant, err := catalogueSvc.CreateVariant(ctx, ownerID, businessID, product.ID, "50cl Bottle", 150000, true)
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}

	lotAReceipt, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variant.ID, Quantity: 10, TotalCostKobo: 1_000_000}, // ₦10,000
	})
	if err != nil {
		t.Fatalf("ReceiveStock (lot A): %v", err)
	}
	if _, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variant.ID, Quantity: 5, TotalCostKobo: 570_000}, // ₦5,700
	}); err != nil {
		t.Fatalf("ReceiveStock (lot B): %v", err)
	}
	_ = lotAReceipt

	sale, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variant.ID, Quantity: 12, UnitPriceKobo: 150000}},
		sales.PaymentInput{AmountKobo: 1_800_000, Method: sales.PaymentMethodCash},
	)
	if err != nil {
		t.Fatalf("CreateSale: %v", err)
	}
	if len(sale.Items) != 1 {
		t.Fatalf("expected 1 sale item, got %d", len(sale.Items))
	}

	allocations := allocationsForSaleItem(t, pool, businessID, sale.Items[0].ID)
	if len(allocations) != 2 {
		t.Fatalf("expected 2 allocations (spanning both lots), got %d: %+v", len(allocations), allocations)
	}

	first, second := allocations[0], allocations[1]
	if first.quantity != 10 || first.allocatedCostKobo == nil || *first.allocatedCostKobo != 1_000_000 {
		t.Fatalf("expected the first allocation to be 10 units for ₦10,000 from the oldest lot, got %+v", first)
	}
	if second.quantity != 2 || second.allocatedCostKobo == nil || *second.allocatedCostKobo != 228_000 {
		t.Fatalf("expected the second allocation to be 2 units for ₦2,280 from the newer lot, got %+v", second)
	}

	var totalCost int64
	for _, a := range allocations {
		totalCost += *a.allocatedCostKobo
	}
	if totalCost != 1_228_000 {
		t.Fatalf("expected total allocated cost 1,228,000 kobo (₦12,280), got %d", totalCost)
	}
}

// TestFIFOOversellCreatesPendingAllocation confirms an oversell past every
// lot's coverage is recorded honestly, never as a fabricated zero cost
// (docs/ARCHITECTURE.md §8.5).
func TestFIFOOversellCreatesPendingAllocation(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	product, err := catalogueSvc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, uniqueName("Coke"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant, err := catalogueSvc.CreateVariant(ctx, ownerID, businessID, product.ID, "50cl Bottle", 150000, true)
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}
	if _, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variant.ID, Quantity: 3, TotalCostKobo: 300_000},
	}); err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}

	sale, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variant.ID, Quantity: 5, UnitPriceKobo: 150000}},
		sales.PaymentInput{AmountKobo: 750000, Method: sales.PaymentMethodCash},
	)
	if err != nil {
		t.Fatalf("CreateSale (oversell): %v", err)
	}

	allocations := allocationsForSaleItem(t, pool, businessID, sale.Items[0].ID)
	if len(allocations) != 2 {
		t.Fatalf("expected 2 allocations (1 real + 1 pending), got %d: %+v", len(allocations), allocations)
	}
	real, pending := allocations[0], allocations[1]
	if real.stockLotID == nil || real.quantity != 3 || real.allocatedCostKobo == nil || *real.allocatedCostKobo != 300_000 {
		t.Fatalf("expected a real allocation for the 3 covered units, got %+v", real)
	}
	if pending.stockLotID != nil || pending.quantity != 2 || pending.allocatedCostKobo != nil {
		t.Fatalf("expected a pending allocation (no lot, no cost) for the 2 uncovered units, got %+v", pending)
	}
}

// TestFIFOKoboRoundingSumsExactly confirms a lot whose total cost doesn't
// divide evenly by its quantity never drifts across partial consumptions
// -- the sum of every allocation drawn from one lot must exactly equal
// that lot's total_cost_kobo (docs/PHASE_FIFO_COSTING.md §7).
func TestFIFOKoboRoundingSumsExactly(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	product, err := catalogueSvc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, uniqueName("Coke"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant, err := catalogueSvc.CreateVariant(ctx, ownerID, businessID, product.ID, "50cl Bottle", 150000, true)
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}
	// 10,000 kobo / 3 units = 3333.33... kobo/unit -- deliberately not
	// evenly divisible.
	if _, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variant.ID, Quantity: 3, TotalCostKobo: 10_000},
	}); err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}

	var totalAllocated int64
	for i := 0; i < 3; i++ {
		sale, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
			[]sales.SaleItemInput{{VariantID: variant.ID, Quantity: 1, UnitPriceKobo: 150000}},
			sales.PaymentInput{AmountKobo: 150000, Method: sales.PaymentMethodCash},
		)
		if err != nil {
			t.Fatalf("CreateSale (round %d): %v", i, err)
		}
		allocs := allocationsForSaleItem(t, pool, businessID, sale.Items[0].ID)
		if len(allocs) != 1 || allocs[0].allocatedCostKobo == nil {
			t.Fatalf("expected 1 real allocation on round %d, got %+v", i, allocs)
		}
		totalAllocated += *allocs[0].allocatedCostKobo
	}

	if totalAllocated != 10_000 {
		t.Fatalf("expected the 3 single-unit allocations to sum to exactly 10,000 kobo, got %d", totalAllocated)
	}
}

// TestReverseSaleGivesBackToExactLots confirms a reversal restores
// remaining_quantity on the exact lots the original sale drew from, and
// records mirrored negative allocations rather than re-running FIFO.
func TestReverseSaleGivesBackToExactLots(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	product, err := catalogueSvc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, uniqueName("Coke"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant, err := catalogueSvc.CreateVariant(ctx, ownerID, businessID, product.ID, "50cl Bottle", 150000, true)
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}
	if _, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variant.ID, Quantity: 10, TotalCostKobo: 1_000_000},
	}); err != nil {
		t.Fatalf("ReceiveStock (lot A): %v", err)
	}
	if _, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variant.ID, Quantity: 5, TotalCostKobo: 570_000},
	}); err != nil {
		t.Fatalf("ReceiveStock (lot B): %v", err)
	}

	sale, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variant.ID, Quantity: 12, UnitPriceKobo: 150000}},
		sales.PaymentInput{AmountKobo: 1_800_000, Method: sales.PaymentMethodCash},
	)
	if err != nil {
		t.Fatalf("CreateSale: %v", err)
	}
	originalAllocations := allocationsForSaleItem(t, pool, businessID, sale.Items[0].ID)
	if len(originalAllocations) != 2 {
		t.Fatalf("expected 2 original allocations, got %d", len(originalAllocations))
	}
	lotAID, lotBID := *originalAllocations[0].stockLotID, *originalAllocations[1].stockLotID

	if lotRemainingQuantity(t, pool, businessID, lotAID) != 0 {
		t.Fatalf("expected lot A fully depleted before reversal")
	}
	if lotRemainingQuantity(t, pool, businessID, lotBID) != 3 {
		t.Fatalf("expected lot B to have 3 left before reversal")
	}

	reversal, err := salesSvc.ReverseSale(ctx, ownerID, businessID, sale.ID, ownerID)
	if err != nil {
		t.Fatalf("ReverseSale: %v", err)
	}

	if lotRemainingQuantity(t, pool, businessID, lotAID) != 10 {
		t.Fatalf("expected lot A restored to 10 after reversal")
	}
	if lotRemainingQuantity(t, pool, businessID, lotBID) != 5 {
		t.Fatalf("expected lot B restored to 5 after reversal")
	}

	reversalAllocations := allocationsForSaleItem(t, pool, businessID, reversal.Items[0].ID)
	if len(reversalAllocations) != 2 {
		t.Fatalf("expected 2 mirrored reversal allocations, got %d", len(reversalAllocations))
	}
	var totalReversedQty int32
	var totalReversedCost int64
	for _, a := range reversalAllocations {
		totalReversedQty += a.quantity
		if a.allocatedCostKobo != nil {
			totalReversedCost += *a.allocatedCostKobo
		}
	}
	if totalReversedQty != -12 {
		t.Fatalf("expected reversal allocations to sum to -12 units, got %d", totalReversedQty)
	}
	if totalReversedCost != -1_228_000 {
		t.Fatalf("expected reversal allocations to sum to -1,228,000 kobo, got %d", totalReversedCost)
	}
}

// TestResolvePendingAllocationClosesOutOversell exercises the full
// resolution path (docs/PHASE_FIFO_COSTING.md §5): an oversell creates a
// pending allocation and a negative_inventory review; resolving that
// review with a supplied per-unit cost closes the pending amount out via
// a synthetic, review-sourced lot -- never mutating the original pending
// row, always append-only.
func TestResolvePendingAllocationClosesOutOversell(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	product, err := catalogueSvc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, uniqueName("Coke"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant, err := catalogueSvc.CreateVariant(ctx, ownerID, businessID, product.ID, "50cl Bottle", 150000, true)
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}
	if _, err := inventorySvc.ReceiveStock(ctx, ownerID, businessID, locationID, []inventory.ReceiptLine{
		{VariantID: variant.ID, Quantity: 3, TotalCostKobo: 300_000},
	}); err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}

	sale, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variant.ID, Quantity: 5, UnitPriceKobo: 150000}},
		sales.PaymentInput{AmountKobo: 750000, Method: sales.PaymentMethodCash},
	)
	if err != nil {
		t.Fatalf("CreateSale (oversell): %v", err)
	}
	saleItemID := sale.Items[0].ID

	reviews, err := inventorySvc.ListOpenReviews(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("ListOpenReviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("expected 1 open review, got %d", len(reviews))
	}
	reviewID := reviews[0].ID

	resolvedUnitCostKobo := int64(90_000) // ₦900/unit
	if _, err := inventorySvc.ResolveReview(ctx, ownerID, businessID, reviewID, ownerID, "Late receipt found.", &resolvedUnitCostKobo); err != nil {
		t.Fatalf("ResolveReview with cost: %v", err)
	}

	allocations := allocationsForSaleItem(t, pool, businessID, saleItemID)
	if len(allocations) != 4 {
		t.Fatalf("expected 4 allocation rows (1 real, 1 original pending, 1 close-out, 1 resolved), got %d: %+v", len(allocations), allocations)
	}

	var netQuantity int32
	var netCost int64
	var netPendingQuantity int32
	for _, a := range allocations {
		netQuantity += a.quantity
		if a.allocatedCostKobo != nil {
			netCost += *a.allocatedCostKobo
		} else {
			netPendingQuantity += a.quantity
		}
	}
	if netQuantity != 5 {
		t.Fatalf("expected net allocated quantity 5 (matching units sold), got %d", netQuantity)
	}
	if netPendingQuantity != 0 {
		t.Fatalf("expected the pending quantity to net to 0 (fully closed out), got %d in %+v", netPendingQuantity, allocations)
	}
	if netCost != 300_000+2*90_000 {
		t.Fatalf("expected net cost 300,000 (real) + 180,000 (resolved) = 480,000, got %d", netCost)
	}
}
