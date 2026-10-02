package sales_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/catalogue"
	"github.com/mananuf/justbarme/internal/sales"
)

func TestAddSaleRoundAppendsToOpenBillAndAllowsClosedUnpaid(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	customer, err := salesSvc.CreateCustomer(ctx, ownerID, businessID, "Amara", "", "", "")
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	bill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, customer.ID, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}
	if bill.Status != sales.BillStatusOpen {
		t.Fatalf("expected open bill, got %q", bill.Status)
	}

	round1, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 2, UnitPriceKobo: 80000}})
	if err != nil {
		t.Fatalf("first AddSaleRound: %v", err)
	}
	if round1.TotalKobo != 160000 {
		t.Fatalf("expected round total 160000, got %d", round1.TotalKobo)
	}

	round2, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}})
	if err != nil {
		t.Fatalf("second AddSaleRound: %v", err)
	}
	if round2.TotalKobo != 80000 {
		t.Fatalf("expected round total 80000, got %d", round2.TotalKobo)
	}

	detail, err := salesSvc.GetBillDetail(ctx, ownerID, businessID, bill.ID)
	if err != nil {
		t.Fatalf("GetBillDetail: %v", err)
	}
	if detail.BalanceKobo != 240000 {
		t.Fatalf("expected balance 240000 after two unpaid rounds, got %d", detail.BalanceKobo)
	}
	if len(detail.Sales) != 2 {
		t.Fatalf("expected 2 rounds on the bill, got %d", len(detail.Sales))
	}

	if _, err := salesSvc.CloseBill(ctx, ownerID, businessID, bill.ID); err != nil {
		t.Fatalf("CloseBill: %v", err)
	}

	// Nothing has been paid in full yet -- a closed_unpaid bill can still
	// take a correction/extra round.
	round3, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}})
	if err != nil {
		t.Fatalf("AddSaleRound on closed_unpaid bill should succeed: %v", err)
	}
	if round3.TotalKobo != 80000 {
		t.Fatalf("expected round total 80000, got %d", round3.TotalKobo)
	}

	// Fully pay and settle -- now editing is rejected.
	if _, err := salesSvc.RecordPayment(ctx, ownerID, businessID, bill.ID, ownerID, 320000, sales.PaymentMethodCash, uuid.Nil); err != nil {
		t.Fatalf("RecordPayment: %v", err)
	}
	settled, err := salesSvc.GetBillDetail(ctx, ownerID, businessID, bill.ID)
	if err != nil || settled.Status != sales.BillStatusSettled {
		t.Fatalf("expected settled bill, got %+v err=%v", settled.Bill, err)
	}
	if _, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}}); !errors.Is(err, sales.ErrBillNotEditable) {
		t.Fatalf("expected ErrBillNotEditable on a settled bill, got %v", err)
	}
}

func TestRemoveBillItemNetsAgainstEarlierRoundsAndCapsAtWhatsPresent(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	bill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}
	if _, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 3, UnitPriceKobo: 80000}}); err != nil {
		t.Fatalf("AddSaleRound: %v", err)
	}

	// Can't remove more than the 3 that were added.
	if _, err := salesSvc.RemoveBillItem(ctx, ownerID, businessID, bill.ID, ownerID, variantID, uuid.New(), 4, 80000, time.Now()); !errors.Is(err, sales.ErrInsufficientQuantityOnBill) {
		t.Fatalf("expected ErrInsufficientQuantityOnBill, got %v", err)
	}

	removal, err := salesSvc.RemoveBillItem(ctx, ownerID, businessID, bill.ID, ownerID, variantID, uuid.New(), 1, 80000, time.Now())
	if err != nil {
		t.Fatalf("RemoveBillItem: %v", err)
	}
	if removal.TotalKobo != -80000 {
		t.Fatalf("expected a -80000 compensating round, got %d", removal.TotalKobo)
	}
	if len(removal.Items) != 1 || removal.Items[0].Quantity != -1 {
		t.Fatalf("expected one -1 quantity item, got %+v", removal.Items)
	}

	detail, err := salesSvc.GetBillDetail(ctx, ownerID, businessID, bill.ID)
	if err != nil {
		t.Fatalf("GetBillDetail: %v", err)
	}
	if detail.BalanceKobo != 160000 {
		t.Fatalf("expected balance 160000 (2 remaining bottles), got %d", detail.BalanceKobo)
	}
	if len(detail.Sales) != 2 {
		t.Fatalf("expected the original round plus the compensating removal round, got %d", len(detail.Sales))
	}

	balances, err := inventorySvc.GetBalances(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	if balances[variantID] != 46 {
		t.Fatalf("expected stock 48-3+1=46 after the removal restocked one bottle, got %d", balances[variantID])
	}

	// Removing the exact remaining quantity brings it to zero and is
	// allowed; a further removal is then rejected as insufficient.
	if _, err := salesSvc.RemoveBillItem(ctx, ownerID, businessID, bill.ID, ownerID, variantID, uuid.New(), 2, 80000, time.Now()); err != nil {
		t.Fatalf("RemoveBillItem down to zero: %v", err)
	}
	if _, err := salesSvc.RemoveBillItem(ctx, ownerID, businessID, bill.ID, ownerID, variantID, uuid.New(), 1, 80000, time.Now()); !errors.Is(err, sales.ErrInsufficientQuantityOnBill) {
		t.Fatalf("expected ErrInsufficientQuantityOnBill once nothing remains, got %v", err)
	}
}

// TestRemoveBillItemRejectsOnceFullyPaid reproduces a real bug: an open
// bill that gets paid in full deliberately stays 'open' (so it can still
// take more rounds later, pay-as-you-go -- see recomputeBillBalance), but
// that meant RemoveBillItem had no check stopping a removal after the
// money was already collected, leaving the bill with a negative "due"
// amount. The fix caps removal at what's still outstanding, not just at
// what's physically present.
func TestRemoveBillItemRejectsOnceFullyPaid(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	bill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}
	if _, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 2, UnitPriceKobo: 80000}}); err != nil {
		t.Fatalf("AddSaleRound: %v", err)
	}
	if _, err := salesSvc.RecordPayment(ctx, ownerID, businessID, bill.ID, ownerID, 160000, sales.PaymentMethodCard, uuid.Nil); err != nil {
		t.Fatalf("RecordPayment: %v", err)
	}

	// Still 'open' by design (pay-as-you-go), but fully paid -- confirm
	// that status directly.
	paid, err := salesSvc.GetBillDetail(ctx, ownerID, businessID, bill.ID)
	if err != nil {
		t.Fatalf("GetBillDetail: %v", err)
	}
	if paid.Status != sales.BillStatusOpen || paid.BalanceKobo != 0 {
		t.Fatalf("expected open bill with zero balance, got status=%q balance=%d", paid.Status, paid.BalanceKobo)
	}

	if _, err := salesSvc.RemoveBillItem(ctx, ownerID, businessID, bill.ID, ownerID, variantID, uuid.New(), 1, 80000, time.Now()); !errors.Is(err, sales.ErrBillFullyPaid) {
		t.Fatalf("expected ErrBillFullyPaid, got %v", err)
	}

	// Adding is unaffected -- only removal is blocked.
	if _, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}}); err != nil {
		t.Fatalf("AddSaleRound after full payment should still succeed: %v", err)
	}

	// Now the bill owes 80000 again -- a removal within that new headroom
	// is fine.
	if _, err := salesSvc.RemoveBillItem(ctx, ownerID, businessID, bill.ID, ownerID, variantID, uuid.New(), 1, 80000, time.Now()); err != nil {
		t.Fatalf("RemoveBillItem within the new outstanding balance should succeed: %v", err)
	}
}

func TestCloseBillRequiresCustomerForOutstandingBalance(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	customer, err := salesSvc.CreateCustomer(ctx, ownerID, businessID, "Chidi", "", "", "")
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}

	bill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, customer.ID, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}
	if _, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}}); err != nil {
		t.Fatalf("AddSaleRound: %v", err)
	}

	closed, err := salesSvc.CloseBill(ctx, ownerID, businessID, bill.ID)
	if err != nil {
		t.Fatalf("CloseBill with a named customer should succeed: %v", err)
	}
	if closed.Status != sales.BillStatusClosedUnpaid {
		t.Fatalf("expected closed_unpaid, got %q", closed.Status)
	}
	if closed.BalanceKobo != 80000 {
		t.Fatalf("expected balance 80000, got %d", closed.BalanceKobo)
	}
}

func TestCloseBillSettlesDirectlyWhenFullyPaid(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	bill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}
	if _, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}}); err != nil {
		t.Fatalf("AddSaleRound: %v", err)
	}
	if _, err := salesSvc.RecordPayment(ctx, ownerID, businessID, bill.ID, ownerID, 80000, sales.PaymentMethodCash, uuid.Nil); err != nil {
		t.Fatalf("RecordPayment: %v", err)
	}

	closed, err := salesSvc.CloseBill(ctx, ownerID, businessID, bill.ID)
	if err != nil {
		t.Fatalf("CloseBill: %v", err)
	}
	if closed.Status != sales.BillStatusSettled {
		t.Fatalf("expected settled once fully paid before closing, got %q", closed.Status)
	}
	if closed.BalanceKobo != 0 {
		t.Fatalf("expected zero balance, got %d", closed.BalanceKobo)
	}
}

func TestRecordPaymentRejectsOverpaymentAndSettlesOnFullPayment(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	customer, err := salesSvc.CreateCustomer(ctx, ownerID, businessID, "Ngozi", "", "", "")
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	bill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, customer.ID, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}
	if _, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 2, UnitPriceKobo: 80000}}); err != nil {
		t.Fatalf("AddSaleRound: %v", err)
	}
	if _, err := salesSvc.CloseBill(ctx, ownerID, businessID, bill.ID); err != nil {
		t.Fatalf("CloseBill: %v", err)
	}

	if _, err := salesSvc.RecordPayment(ctx, ownerID, businessID, bill.ID, ownerID, 999999, sales.PaymentMethodCash, uuid.Nil); !errors.Is(err, sales.ErrOverpayment) {
		t.Fatalf("expected ErrOverpayment, got %v", err)
	}

	partial, err := salesSvc.RecordPayment(ctx, ownerID, businessID, bill.ID, ownerID, 80000, sales.PaymentMethodCash, uuid.Nil)
	if err != nil {
		t.Fatalf("partial RecordPayment: %v", err)
	}
	if partial.AmountKobo != 80000 {
		t.Fatalf("expected partial payment of 80000, got %d", partial.AmountKobo)
	}

	detail, err := salesSvc.GetBillDetail(ctx, ownerID, businessID, bill.ID)
	if err != nil {
		t.Fatalf("GetBillDetail: %v", err)
	}
	if detail.Status != sales.BillStatusClosedUnpaid || detail.BalanceKobo != 80000 {
		t.Fatalf("expected closed_unpaid with balance 80000 after partial payment, got status=%q balance=%d", detail.Status, detail.BalanceKobo)
	}

	final, err := salesSvc.RecordPayment(ctx, ownerID, businessID, bill.ID, ownerID, 80000, sales.PaymentMethodTransfer, uuid.Nil)
	if err != nil {
		t.Fatalf("final RecordPayment: %v", err)
	}
	if final.Method != sales.PaymentMethodTransfer {
		t.Fatalf("unexpected method: %q", final.Method)
	}

	settled, err := salesSvc.GetBillDetail(ctx, ownerID, businessID, bill.ID)
	if err != nil {
		t.Fatalf("GetBillDetail: %v", err)
	}
	if settled.Status != sales.BillStatusSettled || settled.BalanceKobo != 0 {
		t.Fatalf("expected settled with zero balance, got status=%q balance=%d", settled.Status, settled.BalanceKobo)
	}
}

func TestReversePaymentRevertsSettledBillAndRejectsDoubleReversal(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	bill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}
	if _, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}}); err != nil {
		t.Fatalf("AddSaleRound: %v", err)
	}
	payment, err := salesSvc.RecordPayment(ctx, ownerID, businessID, bill.ID, ownerID, 80000, sales.PaymentMethodCash, uuid.Nil)
	if err != nil {
		t.Fatalf("RecordPayment: %v", err)
	}
	if _, err := salesSvc.CloseBill(ctx, ownerID, businessID, bill.ID); err != nil {
		t.Fatalf("CloseBill: %v", err)
	}
	settled, err := salesSvc.GetBillDetail(ctx, ownerID, businessID, bill.ID)
	if err != nil || settled.Status != sales.BillStatusSettled {
		t.Fatalf("expected settled bill before reversal, got %+v err=%v", settled.Bill, err)
	}

	reversal, err := salesSvc.ReversePayment(ctx, ownerID, businessID, payment.ID, ownerID)
	if err != nil {
		t.Fatalf("ReversePayment: %v", err)
	}
	if reversal.AmountKobo != -80000 {
		t.Fatalf("expected refund of -80000, got %d", reversal.AmountKobo)
	}

	reverted, err := salesSvc.GetBillDetail(ctx, ownerID, businessID, bill.ID)
	if err != nil {
		t.Fatalf("GetBillDetail: %v", err)
	}
	if reverted.Status != sales.BillStatusClosedUnpaid || reverted.BalanceKobo != 80000 {
		t.Fatalf("expected closed_unpaid with balance 80000 after reversal, got status=%q balance=%d", reverted.Status, reverted.BalanceKobo)
	}

	if _, err := salesSvc.ReversePayment(ctx, ownerID, businessID, payment.ID, ownerID); !errors.Is(err, sales.ErrPaymentAlreadyReversed) {
		t.Fatalf("expected ErrPaymentAlreadyReversed, got %v", err)
	}
}

func TestWriteOffBillRequiresClosedUnpaidAndCannotExceedBalance(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	customer, err := salesSvc.CreateCustomer(ctx, ownerID, businessID, "Emeka", "", "", "")
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	bill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, customer.ID, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}
	if _, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}}); err != nil {
		t.Fatalf("AddSaleRound: %v", err)
	}

	// Still open -- write-off must be rejected.
	if _, err := salesSvc.WriteOffBill(ctx, ownerID, businessID, bill.ID, ownerID, 80000, "goodwill"); !errors.Is(err, sales.ErrBillNotClosedUnpaid) {
		t.Fatalf("expected ErrBillNotClosedUnpaid, got %v", err)
	}

	if _, err := salesSvc.CloseBill(ctx, ownerID, businessID, bill.ID); err != nil {
		t.Fatalf("CloseBill: %v", err)
	}

	if _, err := salesSvc.WriteOffBill(ctx, ownerID, businessID, bill.ID, ownerID, 999999, "too much"); !errors.Is(err, sales.ErrWriteOffExceedsBalance) {
		t.Fatalf("expected ErrWriteOffExceedsBalance, got %v", err)
	}

	written, err := salesSvc.WriteOffBill(ctx, ownerID, businessID, bill.ID, ownerID, 80000, "regular customer, forgiven")
	if err != nil {
		t.Fatalf("WriteOffBill: %v", err)
	}
	if written.Status != sales.BillStatusSettled || written.BalanceKobo != 0 {
		t.Fatalf("expected settled with zero balance after full write-off, got status=%q balance=%d", written.Status, written.BalanceKobo)
	}
}

func TestVoidBillRejectsActivityAndSucceedsWhenClean(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	billWithActivity, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}
	if _, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, billWithActivity.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}}); err != nil {
		t.Fatalf("AddSaleRound: %v", err)
	}
	if _, err := salesSvc.VoidBill(ctx, ownerID, businessID, billWithActivity.ID); !errors.Is(err, sales.ErrBillHasOutstandingActivity) {
		t.Fatalf("expected ErrBillHasOutstandingActivity, got %v", err)
	}

	emptyBill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}
	voided, err := salesSvc.VoidBill(ctx, ownerID, businessID, emptyBill.ID)
	if err != nil {
		t.Fatalf("VoidBill on an empty tab should succeed: %v", err)
	}
	if voided.Status != sales.BillStatusVoid {
		t.Fatalf("expected void, got %q", voided.Status)
	}
}

// TestConcurrentPaymentsNeverExceedBalance mirrors
// internal/catalogue.TestConcurrentSetVariantPriceNeverProducesTwoCurrentPrices:
// two goroutines record a payment against the same bill at the same time,
// each for the bill's full balance. GetBillForUpdate's row lock must
// serialize them so only one can possibly succeed -- the loser must see
// the winner's already-applied balance and be rejected as an overpayment,
// never both succeeding and leaving the bill overpaid.
func TestConcurrentPaymentsNeverExceedBalance(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	bill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}
	if _, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}}); err != nil {
		t.Fatalf("AddSaleRound: %v", err)
	}

	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = salesSvc.RecordPayment(ctx, ownerID, businessID, bill.ID, ownerID, 80000, sales.PaymentMethodCash, uuid.Nil)
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, sales.ErrOverpayment) {
			t.Fatalf("expected either success or ErrOverpayment, got %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one payment to win the race, got %d successes", successes)
	}

	detail, err := salesSvc.GetBillDetail(ctx, ownerID, businessID, bill.ID)
	if err != nil {
		t.Fatalf("GetBillDetail: %v", err)
	}
	if detail.BalanceKobo != 0 {
		t.Fatalf("expected exactly one payment to have applied, balance 0, got %d", detail.BalanceKobo)
	}
	if len(detail.Payments) != 1 {
		t.Fatalf("expected exactly one payment row, got %d", len(detail.Payments))
	}
}

// TestReverseSaleRejectsWhenBillHasOtherSales covers a real bug found live:
// Activity's generic "Fix" on an individual round of a shared tab used to
// either crash (no payment yet to refund) or, worse, quietly succeed by
// opening a disconnected new bill that never touched the actual tab. Both
// are wrong for a sale that isn't the sole sale on its bill.
func TestReverseSaleRejectsWhenBillHasOtherSales(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	bill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}
	round1, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}})
	if err != nil {
		t.Fatalf("first AddSaleRound: %v", err)
	}
	if _, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}}); err != nil {
		t.Fatalf("second AddSaleRound: %v", err)
	}

	loaded, err := salesSvc.GetSale(ctx, ownerID, businessID, round1.ID)
	if err != nil {
		t.Fatalf("GetSale: %v", err)
	}
	if !loaded.OtherSalesOnBill {
		t.Fatal("expected OtherSalesOnBill=true once a second round exists on the same bill")
	}

	if _, err := salesSvc.ReverseSale(ctx, ownerID, businessID, round1.ID, ownerID); err != sales.ErrSaleSharesBillWithOthers {
		t.Fatalf("expected ErrSaleSharesBillWithOthers, got %v", err)
	}
}

// TestReverseSaleWithNoPaymentYetDoesNotCrash covers the exact live bug:
// a tab with exactly one round and nothing paid yet has no payment row at
// all (payments.amount_kobo has a non-zero CHECK, so there's genuinely
// nothing to post a 0-kobo "refund" of) -- reversing it used to hit that
// constraint and 500. It should succeed cleanly with no payment on the
// reversal either.
func TestReverseSaleWithNoPaymentYetDoesNotCrash(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	bill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}
	round, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 80000}})
	if err != nil {
		t.Fatalf("AddSaleRound: %v", err)
	}

	loaded, err := salesSvc.GetSale(ctx, ownerID, businessID, round.ID)
	if err != nil {
		t.Fatalf("GetSale: %v", err)
	}
	if loaded.OtherSalesOnBill {
		t.Fatal("expected OtherSalesOnBill=false with only one round on the bill")
	}
	if loaded.Payment.ID != uuid.Nil {
		t.Fatalf("expected no payment yet, got %+v", loaded.Payment)
	}

	reversal, err := salesSvc.ReverseSale(ctx, ownerID, businessID, round.ID, ownerID)
	if err != nil {
		t.Fatalf("ReverseSale: %v", err)
	}
	if reversal.Payment.ID != uuid.Nil || reversal.Payment.AmountKobo != 0 {
		t.Fatalf("expected no refund payment posted, got %+v", reversal.Payment)
	}
	if reversal.TotalKobo != -80000 {
		t.Fatalf("expected reversal total -80000, got %d", reversal.TotalKobo)
	}
}

// TestOpenBillResolvesToExistingOpenBillForSameTable exercises the
// find-or-open behavior docs/PHASE_UNIFIED_SELL_BILLS.md requires: picking
// an already-occupied table must resume its tab, never open a second,
// concurrent one for the same table.
func TestOpenBillResolvesToExistingOpenBillForSameTable(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	table, err := salesSvc.CreateTable(ctx, ownerID, businessID, "T1")
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	first, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, table.ID, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill (first): %v", err)
	}
	second, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, table.ID, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill (second): %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected picking the same table to resume bill %s, got a new bill %s", first.ID, second.ID)
	}

	// Once the bill is settled (terminal), the table is free again -- a
	// third open against the same table must create a new bill, not
	// resolve to the now-closed one.
	if _, err := salesSvc.CloseBill(ctx, ownerID, businessID, first.ID); err != nil {
		t.Fatalf("CloseBill: %v", err)
	}
	third, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, table.ID, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill (third): %v", err)
	}
	if third.ID == first.ID {
		t.Fatal("expected a new bill once the previous one settled, got the same bill back")
	}
}

// TestOpenBillResolvesToExistingOpenBillForSameCustomer mirrors the table
// case for a named customer tab with no table attached.
func TestOpenBillResolvesToExistingOpenBillForSameCustomer(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	customer, err := salesSvc.CreateCustomer(ctx, ownerID, businessID, "Amara", "", "", "")
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}

	first, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, customer.ID, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill (first): %v", err)
	}
	second, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, customer.ID, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill (second): %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected picking the same customer to resume bill %s, got a new bill %s", first.ID, second.ID)
	}
}

// TestOpenBillWalkInIsIdempotent exercises the plain walk-in path's own
// idempotency key (no table/customer to resolve against) -- a retried
// queued "open a new walk-in bill" must not create a duplicate.
func TestOpenBillWalkInIsIdempotent(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	key := uuid.New()
	first, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil, key)
	if err != nil {
		t.Fatalf("OpenBill (first): %v", err)
	}
	second, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil, key)
	if err != nil {
		t.Fatalf("OpenBill (second): %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected a replayed idempotency key to return the same bill %s, got %s", first.ID, second.ID)
	}

	// A different key (or none at all) must still open a genuinely new
	// walk-in bill -- the key scopes the replay, it doesn't make every
	// walk-in open collapse into one.
	third, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil, uuid.New())
	if err != nil {
		t.Fatalf("OpenBill (third): %v", err)
	}
	if third.ID == first.ID {
		t.Fatal("expected a different idempotency key to open a new bill")
	}
}

// TestRecordPaymentIsIdempotent mirrors CreateSale's own idempotency-key
// contract for RecordPayment -- the real backend gap
// docs/PHASE_UNIFIED_SELL_BILLS.md identified before offline queuing of a
// bill's payment step could be made safe.
func TestRecordPaymentIsIdempotent(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, variantID := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	bill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}
	if _, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variantID, Quantity: 1, UnitPriceKobo: 100000}}); err != nil {
		t.Fatalf("AddSaleRound: %v", err)
	}

	key := uuid.New()
	first, err := salesSvc.RecordPayment(ctx, ownerID, businessID, bill.ID, ownerID, 100000, sales.PaymentMethodCash, key)
	if err != nil {
		t.Fatalf("RecordPayment (first): %v", err)
	}
	second, err := salesSvc.RecordPayment(ctx, ownerID, businessID, bill.ID, ownerID, 100000, sales.PaymentMethodCash, key)
	if err != nil {
		t.Fatalf("RecordPayment (second): %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected a replayed idempotency key to return the same payment %s, got %s", first.ID, second.ID)
	}

	detail, err := salesSvc.GetBillDetail(ctx, ownerID, businessID, bill.ID)
	if err != nil {
		t.Fatalf("GetBillDetail: %v", err)
	}
	if len(detail.Payments) != 1 {
		t.Fatalf("expected exactly one payment to have landed (not double-applied), got %d", len(detail.Payments))
	}
	if detail.Bill.BalanceKobo != 0 {
		t.Fatalf("expected balance 0 after the (single, deduped) payment, got %d", detail.Bill.BalanceKobo)
	}
}

// TestAddSaleRoundAppliesMultiBuyPricingAcrossRounds exercises the exact
// example docs/PHASE_MULTIBUY_PRICING.md was built for: 1 game = 300, 2
// games for 500 (discounted), repeating indefinitely. Every tap still
// posts its own live, quantity-1 round (unchanged); what varies is the
// server-computed marginal price charged for each one, based on how many
// of this variant are already on the bill -- never the client's own
// submitted unit price, which this test deliberately keeps wrong (300 on
// every call) to prove it is ignored for a multi-buy-priced variant.
func TestAddSaleRoundAppliesMultiBuyPricingAcrossRounds(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	product, err := catalogueSvc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, uniqueName("Snooker"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant, err := catalogueSvc.CreateVariant(ctx, ownerID, businessID, product.ID, "Game", 30000, false)
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}
	if _, err := catalogueSvc.SetVariantPricePacks(ctx, ownerID, businessID, variant.ID, []catalogue.PricePackInput{
		{Quantity: 2, PriceKobo: 50000},
	}); err != nil {
		t.Fatalf("SetVariantPricePacks: %v", err)
	}

	bill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}

	wantMarginals := []int64{30000, 20000, 30000, 20000}
	wantCumulative := []int64{30000, 50000, 80000, 100000}
	for i, wantMarginal := range wantMarginals {
		round, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
			[]sales.SaleItemInput{{VariantID: variant.ID, Quantity: 1, UnitPriceKobo: 30000}},
		)
		if err != nil {
			t.Fatalf("AddSaleRound #%d: %v", i+1, err)
		}
		if round.TotalKobo != wantMarginal {
			t.Fatalf("round #%d: got marginal %d, want %d", i+1, round.TotalKobo, wantMarginal)
		}
		detail, err := salesSvc.GetBillDetail(ctx, ownerID, businessID, bill.ID)
		if err != nil {
			t.Fatalf("GetBillDetail: %v", err)
		}
		if detail.Bill.BalanceKobo != wantCumulative[i] {
			t.Fatalf("after round #%d: got balance %d, want cumulative %d", i+1, detail.Bill.BalanceKobo, wantCumulative[i])
		}
	}
}

// TestRemoveBillItemRefundsMultiBuyMarginal exercises the inverse: taking
// one game off a 4-game bill refunds exactly the marginal cost of that
// 4th unit (200, not the client's flat 300 guess), leaving the bill at
// the correct 3-game price (800), not a naive 4-game-price-minus-300.
func TestRemoveBillItemRefundsMultiBuyMarginal(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	product, err := catalogueSvc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, uniqueName("Snooker"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant, err := catalogueSvc.CreateVariant(ctx, ownerID, businessID, product.ID, "Game", 30000, false)
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}
	if _, err := catalogueSvc.SetVariantPricePacks(ctx, ownerID, businessID, variant.ID, []catalogue.PricePackInput{
		{Quantity: 2, PriceKobo: 50000},
	}); err != nil {
		t.Fatalf("SetVariantPricePacks: %v", err)
	}

	bill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}
	for i := 0; i < 4; i++ {
		if _, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
			[]sales.SaleItemInput{{VariantID: variant.ID, Quantity: 1, UnitPriceKobo: 30000}},
		); err != nil {
			t.Fatalf("AddSaleRound #%d: %v", i+1, err)
		}
	}

	removal, err := salesSvc.RemoveBillItem(ctx, ownerID, businessID, bill.ID, ownerID, variant.ID,
		uuid.New(), 1, 30000, time.Now(),
	)
	if err != nil {
		t.Fatalf("RemoveBillItem: %v", err)
	}
	if removal.TotalKobo != -20000 {
		t.Fatalf("expected the refund to be the 4th unit's marginal (-20000), got %d", removal.TotalKobo)
	}
	detail, err := salesSvc.GetBillDetail(ctx, ownerID, businessID, bill.ID)
	if err != nil {
		t.Fatalf("GetBillDetail: %v", err)
	}
	if detail.Bill.BalanceKobo != 80000 {
		t.Fatalf("expected balance back to the correct 3-game price 80000, got %d", detail.Bill.BalanceKobo)
	}
}

// TestRemoveBillItemFullyPaidCheckUsesTrueMultiBuyRefund confirms
// ErrBillFullyPaid is judged against the real, pack-aware refund amount,
// not the client's flat per-unit guess -- the restructuring that moved
// this check to after postSaleRound (docs/PHASE_MULTIBUY_PRICING.md).
func TestRemoveBillItemFullyPaidCheckUsesTrueMultiBuyRefund(t *testing.T) {
	salesSvc, inventorySvc, catalogueSvc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID, _ := newTenant(t, ctx, inventorySvc, catalogueSvc, identitySvc, pool)

	product, err := catalogueSvc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, uniqueName("Snooker"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant, err := catalogueSvc.CreateVariant(ctx, ownerID, businessID, product.ID, "Game", 30000, false)
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}
	if _, err := catalogueSvc.SetVariantPricePacks(ctx, ownerID, businessID, variant.ID, []catalogue.PricePackInput{
		{Quantity: 2, PriceKobo: 50000},
	}); err != nil {
		t.Fatalf("SetVariantPricePacks: %v", err)
	}

	bill, err := salesSvc.OpenBill(ctx, ownerID, businessID, locationID, ownerID, uuid.Nil, uuid.Nil, uuid.Nil)
	if err != nil {
		t.Fatalf("OpenBill: %v", err)
	}
	for i := 0; i < 4; i++ {
		if _, err := salesSvc.AddSaleRound(ctx, ownerID, businessID, bill.ID, ownerID, uuid.New(), time.Now(),
			[]sales.SaleItemInput{{VariantID: variant.ID, Quantity: 1, UnitPriceKobo: 30000}},
		); err != nil {
			t.Fatalf("AddSaleRound #%d: %v", i+1, err)
		}
	}
	// Bill is now 100000 (4 games). Pay it off in full.
	if _, err := salesSvc.RecordPayment(ctx, ownerID, businessID, bill.ID, ownerID, 100000, sales.PaymentMethodCash, uuid.Nil); err != nil {
		t.Fatalf("RecordPayment: %v", err)
	}

	// Removing one game now would refund 20000 (the 4th unit's true
	// marginal) against a balance of 0 -- correctly rejected. A naive
	// client-guess-based check using the submitted 30000 would also
	// reject this particular case, so this alone wouldn't distinguish the
	// fix from the old behavior -- the real proof is
	// TestRemoveBillItemRefundsMultiBuyMarginal above succeeding with a
	// non-30000 refund at all while this one still correctly blocks an
	// over-the-balance removal.
	if _, err := salesSvc.RemoveBillItem(ctx, ownerID, businessID, bill.ID, ownerID, variant.ID,
		uuid.New(), 1, 30000, time.Now(),
	); !errors.Is(err, sales.ErrBillFullyPaid) {
		t.Fatalf("expected ErrBillFullyPaid, got %v", err)
	}
}
