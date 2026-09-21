package sales_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/sales"
)

func TestCreateOrRotateShareLinkChangesTheTokenEachTime(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	bill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}

	firstToken, _, firstLink, err := salesSvc.CreateOrRotateShareLink(ctx, ownerID, businessID, bill.ID, nil)
	if err != nil {
		t.Fatalf("CreateOrRotateShareLink (first): %v", err)
	}
	if firstToken == "" {
		t.Fatal("expected a non-empty raw token")
	}

	resolvedBusinessID, resolvedBillID, err := salesSvc.ResolveShareLink(ctx, firstToken)
	if err != nil {
		t.Fatalf("ResolveShareLink (first token): %v", err)
	}
	if resolvedBusinessID != businessID || resolvedBillID != bill.ID {
		t.Fatalf("resolved wrong bill: got business=%s bill=%s", resolvedBusinessID, resolvedBillID)
	}

	secondToken, _, secondLink, err := salesSvc.CreateOrRotateShareLink(ctx, ownerID, businessID, bill.ID, nil)
	if err != nil {
		t.Fatalf("CreateOrRotateShareLink (second): %v", err)
	}
	if secondToken == firstToken {
		t.Fatal("expected rotation to produce a different raw token")
	}
	if secondLink.ID != firstLink.ID {
		t.Fatalf("expected rotation to reuse the same link row, got a new ID: %s vs %s", secondLink.ID, firstLink.ID)
	}

	// The old token no longer resolves once rotated away.
	if _, _, err := salesSvc.ResolveShareLink(ctx, firstToken); !errors.Is(err, sales.ErrShareLinkNotFound) {
		t.Fatalf("expected ErrShareLinkNotFound for the rotated-away token, got %v", err)
	}

	// The new token resolves to the same bill.
	resolvedBusinessID, resolvedBillID, err = salesSvc.ResolveShareLink(ctx, secondToken)
	if err != nil {
		t.Fatalf("ResolveShareLink (second token): %v", err)
	}
	if resolvedBusinessID != businessID || resolvedBillID != bill.ID {
		t.Fatalf("resolved wrong bill after rotation: got business=%s bill=%s", resolvedBusinessID, resolvedBillID)
	}
}

func TestResolveShareLinkRejectsUnknownRevokedAndExpiredTokens(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	if _, _, err := salesSvc.ResolveShareLink(ctx, "not-a-real-token"); !errors.Is(err, sales.ErrShareLinkNotFound) {
		t.Fatalf("expected ErrShareLinkNotFound for an unknown token, got %v", err)
	}

	bill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}

	revokedToken, _, _, err := salesSvc.CreateOrRotateShareLink(ctx, ownerID, businessID, bill.ID, nil)
	if err != nil {
		t.Fatalf("CreateOrRotateShareLink: %v", err)
	}
	if err := salesSvc.RevokeShareLink(ctx, ownerID, businessID, bill.ID); err != nil {
		t.Fatalf("RevokeShareLink: %v", err)
	}
	if _, _, err := salesSvc.ResolveShareLink(ctx, revokedToken); !errors.Is(err, sales.ErrShareLinkNotFound) {
		t.Fatalf("expected ErrShareLinkNotFound for a revoked token, got %v", err)
	}
	// Revoking again (no active link left) is a no-op, not an error.
	if err := salesSvc.RevokeShareLink(ctx, ownerID, businessID, bill.ID); err != nil {
		t.Fatalf("RevokeShareLink (already revoked): %v", err)
	}

	pastExpiry := time.Now().Add(-time.Hour)
	expiredToken, _, _, err := salesSvc.CreateOrRotateShareLink(ctx, ownerID, businessID, bill.ID, &pastExpiry)
	if err != nil {
		t.Fatalf("CreateOrRotateShareLink (already-expired): %v", err)
	}
	if _, _, err := salesSvc.ResolveShareLink(ctx, expiredToken); !errors.Is(err, sales.ErrShareLinkNotFound) {
		t.Fatalf("expected ErrShareLinkNotFound for an expired token, got %v", err)
	}
}

func TestCreateOrRotateShareLinkRejectsUnknownBill(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, _, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	if _, _, _, err := salesSvc.CreateOrRotateShareLink(ctx, ownerID, businessID, uuid.New(), nil); !errors.Is(err, sales.ErrBillNotFound) {
		t.Fatalf("expected ErrBillNotFound, got %v", err)
	}
}
