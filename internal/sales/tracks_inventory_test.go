package sales_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/sales"
)

// TestCreateSaleSkipsInventoryForUntrackedVariant covers the "snooker
// game" scenario CLAUDE.md's Stock receiving section documents: a variant
// with tracks_inventory=false (a service, never restocked) must never
// accumulate a negative balance or open a negative_inventory review, no
// matter how many times it's sold.
func TestCreateSaleSkipsInventoryForUntrackedVariant(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	product, err := catalogueSvc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, uniqueName("Snooker"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant, err := catalogueSvc.CreateVariant(ctx, ownerID, businessID, product.ID, "Game", 50000, false)
	if err != nil {
		t.Fatalf("CreateVariant (untracked): %v", err)
	}

	for i := 0; i < 3; i++ {
		if _, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
			[]sales.SaleItemInput{{VariantID: variant.ID, Quantity: 1, UnitPriceKobo: 50000}},
			sales.PaymentInput{AmountKobo: 50000, Method: sales.PaymentMethodCash},
		); err != nil {
			t.Fatalf("CreateSale (untracked, round %d): %v", i, err)
		}
	}

	balances, err := inventorySvc.GetBalances(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	if got, ok := balances[variant.ID]; ok {
		t.Fatalf("expected no balance row at all for an untracked variant, got %d", got)
	}

	reviews, err := inventorySvc.ListOpenReviews(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("ListOpenReviews: %v", err)
	}
	for _, r := range reviews {
		if r.VariantID == variant.ID {
			t.Fatalf("expected no inventory review for an untracked variant, got %+v", r)
		}
	}
}

// TestReverseSaleSkipsInventoryForUntrackedVariant confirms the reversal
// path doesn't manufacture a phantom positive balance for an item that was
// never deducted in the first place.
func TestReverseSaleSkipsInventoryForUntrackedVariant(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	product, err := catalogueSvc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, uniqueName("Pool Table"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant, err := catalogueSvc.CreateVariant(ctx, ownerID, businessID, product.ID, "Hour", 100000, false)
	if err != nil {
		t.Fatalf("CreateVariant (untracked): %v", err)
	}

	sale, err := salesSvc.CreateSale(ctx, ownerID, businessID, locationID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variant.ID, Quantity: 1, UnitPriceKobo: 100000}},
		sales.PaymentInput{AmountKobo: 100000, Method: sales.PaymentMethodCash},
	)
	if err != nil {
		t.Fatalf("CreateSale (untracked): %v", err)
	}

	if _, err := salesSvc.ReverseSale(ctx, ownerID, businessID, sale.ID, ownerID); err != nil {
		t.Fatalf("ReverseSale (untracked): %v", err)
	}

	balances, err := inventorySvc.GetBalances(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	if got, ok := balances[variant.ID]; ok {
		t.Fatalf("expected no balance row at all for an untracked variant after reversal, got %d", got)
	}
}
