package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/activity"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/platformadmin"
	"github.com/mananuf/justbarme/internal/sales"
	"github.com/mananuf/justbarme/internal/validator"
)

// This file implements docs/PHASE_PLATFORM_ADMIN_DEEP_DRILL.md: full
// line-item read access (CapabilityBusinessesReadDetail, both roles) plus
// a named, reason-required set of superadmin-only corrective actions
// (CapabilityBusinessesAdjust) against one business's tenant data.
//
// Every handler here follows the same shape the existing aggregate
// dashboard (platform_admin_dashboard_handlers.go) already established:
// resolve the business first (404 if it doesn't exist), call straight
// into the same self-contained feature-package Service methods the
// business's own handlers use, with no new RLS policy and no bypass role.
// Reads are audited BEFORE the read happens (an unrecordable read is a
// refused read); writes are audited immediately AFTER the mutation
// succeeds, since the mutation and the audit write cannot share one
// transaction across two packages -- see
// platformadmin.Service.RecordCorrectiveAction's doc comment for why, and
// why a failed audit write after a successful mutation is surfaced as a
// loud error rather than swallowed.
//
// Customer PII is protected by omission, not redaction: no handler here
// ever calls into internal/sales' customer-reading methods
// (ListCustomers/GetCustomer), so a bill response's CustomerID is always
// just the same opaque UUID reference the business's own bill endpoints
// already return -- there is no name/phone/email/notes field anywhere in
// this file's responses to forget to redact.

// platformDetailBusiness resolves business_id from the URL and confirms it
// exists, writing a 404 (not a 500) for an unknown id -- the one thing
// every handler in this file needs before doing anything else.
func (api *API) platformDetailBusiness(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	businessID, err := uuid.Parse(chi.URLParam(r, "business_id"))
	if err != nil {
		api.badRequestResponse(w, r, "business_id must be a valid UUID.")
		return uuid.Nil, false
	}
	if _, err := api.platformAdmin.GetBusiness(r.Context(), businessID); err != nil {
		if errors.Is(err, platformadmin.ErrBusinessNotFound) {
			api.notFoundResponse(w, r)
			return uuid.Nil, false
		}
		api.internalErrorResponse(w, r, fmt.Errorf("get business: %w", err))
		return uuid.Nil, false
	}
	return businessID, true
}

// recordDetailView is the non-best-effort read-audit every GET handler in
// this file calls before touching tenant data, mirroring
// getPlatformBusinessActivity's own discipline exactly.
func (api *API) recordDetailView(w http.ResponseWriter, r *http.Request, staff platformadmin.Staff, businessID uuid.UUID, targetResource string) bool {
	if err := api.platformAdmin.RecordDetailViewed(r.Context(), staff.ID, businessID, targetResource, RequestID(r.Context())); err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("record detail view: %w", err))
		return false
	}
	return true
}

// correctiveActionRequest is the request body every write handler in this
// file shares: a reason is always required, never optional, since an
// unexplained platform-admin mutation against a business's own data is
// not meaningfully auditable.
type correctiveActionRequest struct {
	Reason string `json:"reason"`
}

func readCorrectiveReason(api *API, w http.ResponseWriter, r *http.Request) (string, bool) {
	var req correctiveActionRequest
	if err := readJSON(w, r, &req, 1<<12); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return "", false
	}
	v := validator.New()
	v.Check(req.Reason != "", "reason", "must be provided")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return "", false
	}
	return req.Reason, true
}

// recordCorrectiveAction is the non-best-effort write-audit every POST
// handler in this file calls immediately after its mutation succeeds. A
// failure here is surfaced as a 500 -- the action already happened, so
// this cannot "refuse" it, but it must never be silently swallowed either.
func (api *API) recordCorrectiveAction(w http.ResponseWriter, r *http.Request, staff platformadmin.Staff, businessID uuid.UUID, action, targetResource, reason string) bool {
	if err := api.platformAdmin.RecordCorrectiveAction(r.Context(), staff.ID, businessID, action, targetResource, reason, RequestID(r.Context())); err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("the action succeeded but recording it in the audit log failed: %w", err))
		return false
	}
	return true
}

func parseLimit(r *http.Request, def, max int32) int32 {
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 32); err == nil && parsed > 0 && parsed <= int64(max) {
			return int32(parsed)
		}
	}
	return def
}

// --- Reads ---

// platformListSales implements
// GET /platform/businesses/{business_id}/sales (CapabilityBusinessesReadDetail,
// both roles).
func (api *API) platformListSales(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesReadDetail)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	if !api.recordDetailView(w, r, staff, businessID, "sales") {
		return
	}
	limit := parseLimit(r, 50, 200)
	list, err := api.sales.ListSales(r.Context(), uuid.Nil, businessID, limit, 0)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list sales: %w", err))
		return
	}
	out := make([]saleResponse, 0, len(list))
	for _, s := range list {
		out = append(out, toSaleResponse(s))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write platform list sales response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformGetSale implements
// GET /platform/businesses/{business_id}/sales/{sale_id}.
func (api *API) platformGetSale(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesReadDetail)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	saleID, err := uuid.Parse(chi.URLParam(r, "sale_id"))
	if err != nil {
		api.badRequestResponse(w, r, "sale_id must be a valid UUID.")
		return
	}
	if !api.recordDetailView(w, r, staff, businessID, "sale:"+saleID.String()) {
		return
	}
	sale, err := api.sales.GetSale(r.Context(), uuid.Nil, businessID, saleID)
	if err != nil {
		salesErrorResponse(api, w, r, err, "get sale")
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toSaleResponse(sale)}, nil); err != nil {
		api.logger.Error("write platform get sale response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformReverseSale implements
// POST /platform/businesses/{business_id}/sales/{sale_id}/reverse
// (CapabilityBusinessesAdjust, superadmin only) -- the same
// sales.Service.ReverseSale an owner's own reversal button calls, acting
// as the reserved Platform Support actor.
func (api *API) platformReverseSale(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesAdjust)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	saleID, err := uuid.Parse(chi.URLParam(r, "sale_id"))
	if err != nil {
		api.badRequestResponse(w, r, "sale_id must be a valid UUID.")
		return
	}
	reason, ok := readCorrectiveReason(api, w, r)
	if !ok {
		return
	}
	reversal, err := api.sales.ReverseSale(r.Context(), identity.PlatformSupportUserID, businessID, saleID, identity.PlatformSupportUserID)
	if err != nil {
		salesErrorResponse(api, w, r, err, "platform reverse sale")
		return
	}
	if !api.recordCorrectiveAction(w, r, staff, businessID, platformadmin.ActionBusinessSaleReversed, "sale:"+saleID.String(), reason) {
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toSaleResponse(reversal)}, nil); err != nil {
		api.logger.Error("write platform reverse sale response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformListExpenses implements
// GET /platform/businesses/{business_id}/expenses.
func (api *API) platformListExpenses(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesReadDetail)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	if !api.recordDetailView(w, r, staff, businessID, "expenses") {
		return
	}
	limit := parseLimit(r, 50, 200)
	list, err := api.expenses.ListExpenses(r.Context(), uuid.Nil, businessID, limit)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list expenses: %w", err))
		return
	}
	out := make([]expenseResponse, 0, len(list))
	for _, e := range list {
		out = append(out, toExpenseResponse(e))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write platform list expenses response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformGetExpense implements
// GET /platform/businesses/{business_id}/expenses/{expense_id}.
func (api *API) platformGetExpense(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesReadDetail)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	expenseID, err := uuid.Parse(chi.URLParam(r, "expense_id"))
	if err != nil {
		api.badRequestResponse(w, r, "expense_id must be a valid UUID.")
		return
	}
	if !api.recordDetailView(w, r, staff, businessID, "expense:"+expenseID.String()) {
		return
	}
	exp, err := api.expenses.GetExpense(r.Context(), uuid.Nil, businessID, expenseID)
	if err != nil {
		expensesErrorResponse(api, w, r, err, "get expense")
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toExpenseResponse(exp)}, nil); err != nil {
		api.logger.Error("write platform get expense response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformReverseExpense implements
// POST /platform/businesses/{business_id}/expenses/{expense_id}/reverse.
func (api *API) platformReverseExpense(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesAdjust)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	expenseID, err := uuid.Parse(chi.URLParam(r, "expense_id"))
	if err != nil {
		api.badRequestResponse(w, r, "expense_id must be a valid UUID.")
		return
	}
	reason, ok := readCorrectiveReason(api, w, r)
	if !ok {
		return
	}
	reversal, err := api.expenses.ReverseExpense(r.Context(), identity.PlatformSupportUserID, businessID, expenseID, identity.PlatformSupportUserID)
	if err != nil {
		expensesErrorResponse(api, w, r, err, "platform reverse expense")
		return
	}
	if !api.recordCorrectiveAction(w, r, staff, businessID, platformadmin.ActionBusinessExpenseReversed, "expense:"+expenseID.String(), reason) {
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toExpenseResponse(reversal)}, nil); err != nil {
		api.logger.Error("write platform reverse expense response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformListBills implements
// GET /platform/businesses/{business_id}/bills, optionally filtered by
// ?status= (any real bill status, or "all" -- same contract
// GET /api/v1/bills already offers an owner).
func (api *API) platformListBills(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesReadDetail)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	if !api.recordDetailView(w, r, staff, businessID, "bills") {
		return
	}
	status := r.URL.Query().Get("status")
	var bills []sales.Bill
	var err error
	if status == "" || status == "all" {
		bills, err = api.sales.ListAllBills(r.Context(), uuid.Nil, businessID)
	} else {
		bills, err = api.sales.ListBillsByStatus(r.Context(), uuid.Nil, businessID, status)
	}
	if err != nil {
		tabsErrorResponse(api, w, r, err, "list bills")
		return
	}
	out := make([]billResponse, 0, len(bills))
	for _, b := range bills {
		out = append(out, toBillResponse(b))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write platform list bills response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformGetBillDetail implements
// GET /platform/businesses/{business_id}/bills/{bill_id}.
func (api *API) platformGetBillDetail(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesReadDetail)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	billID, err := uuid.Parse(chi.URLParam(r, "bill_id"))
	if err != nil {
		api.badRequestResponse(w, r, "bill_id must be a valid UUID.")
		return
	}
	if !api.recordDetailView(w, r, staff, businessID, "bill:"+billID.String()) {
		return
	}
	detail, err := api.sales.GetBillDetail(r.Context(), uuid.Nil, businessID, billID)
	if err != nil {
		tabsErrorResponse(api, w, r, err, "get bill detail")
		return
	}
	sellerIDs := make([]uuid.UUID, len(detail.Sales))
	for i, s := range detail.Sales {
		sellerIDs[i] = s.SellerID
	}
	sellerNames, err := api.resolveSellerNames(r.Context(), sellerIDs)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve seller names: %w", err))
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toBillDetailResponse(detail, sellerNames)}, nil); err != nil {
		api.logger.Error("write platform get bill detail response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformCloseBill implements
// POST /platform/businesses/{business_id}/bills/{bill_id}/close.
func (api *API) platformCloseBill(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesAdjust)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	billID, err := uuid.Parse(chi.URLParam(r, "bill_id"))
	if err != nil {
		api.badRequestResponse(w, r, "bill_id must be a valid UUID.")
		return
	}
	reason, ok := readCorrectiveReason(api, w, r)
	if !ok {
		return
	}
	bill, err := api.sales.CloseBill(r.Context(), identity.PlatformSupportUserID, businessID, billID)
	if err != nil {
		tabsErrorResponse(api, w, r, err, "platform close bill")
		return
	}
	if !api.recordCorrectiveAction(w, r, staff, businessID, platformadmin.ActionBusinessBillClosed, "bill:"+billID.String(), reason) {
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toBillResponse(bill)}, nil); err != nil {
		api.logger.Error("write platform close bill response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformVoidBill implements
// POST /platform/businesses/{business_id}/bills/{bill_id}/void.
func (api *API) platformVoidBill(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesAdjust)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	billID, err := uuid.Parse(chi.URLParam(r, "bill_id"))
	if err != nil {
		api.badRequestResponse(w, r, "bill_id must be a valid UUID.")
		return
	}
	reason, ok := readCorrectiveReason(api, w, r)
	if !ok {
		return
	}
	bill, err := api.sales.VoidBill(r.Context(), identity.PlatformSupportUserID, businessID, billID)
	if err != nil {
		tabsErrorResponse(api, w, r, err, "platform void bill")
		return
	}
	if !api.recordCorrectiveAction(w, r, staff, businessID, platformadmin.ActionBusinessBillVoided, "bill:"+billID.String(), reason) {
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toBillResponse(bill)}, nil); err != nil {
		api.logger.Error("write platform void bill response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformGetStockReceipt implements
// GET /platform/businesses/{business_id}/stock-receipts/{receipt_id}.
func (api *API) platformGetStockReceipt(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesReadDetail)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	receiptID, err := uuid.Parse(chi.URLParam(r, "receipt_id"))
	if err != nil {
		api.badRequestResponse(w, r, "receipt_id must be a valid UUID.")
		return
	}
	if !api.recordDetailView(w, r, staff, businessID, "stock_receipt:"+receiptID.String()) {
		return
	}
	receipt, err := api.inventory.GetStockReceipt(r.Context(), uuid.Nil, businessID, receiptID)
	if err != nil {
		inventoryErrorResponse(api, w, r, err, "get stock receipt")
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toStockReceiptResponse(receipt)}, nil); err != nil {
		api.logger.Error("write platform get stock receipt response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformReverseStockReceipt implements the ordinary (guarded) reversal:
// POST /platform/businesses/{business_id}/stock-receipts/{receipt_id}/reverse.
// Blocked with ErrReceiptPartiallyConsumed exactly like an owner's own
// reversal -- use platformForceReverseStockReceipt for the override.
func (api *API) platformReverseStockReceipt(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesAdjust)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	receiptID, err := uuid.Parse(chi.URLParam(r, "receipt_id"))
	if err != nil {
		api.badRequestResponse(w, r, "receipt_id must be a valid UUID.")
		return
	}
	reason, ok := readCorrectiveReason(api, w, r)
	if !ok {
		return
	}
	reversal, err := api.inventory.ReverseStockReceipt(r.Context(), identity.PlatformSupportUserID, businessID, receiptID, identity.PlatformSupportUserID)
	if err != nil {
		inventoryErrorResponse(api, w, r, err, "platform reverse stock receipt")
		return
	}
	if !api.recordCorrectiveAction(w, r, staff, businessID, platformadmin.ActionBusinessReceiptReversed, "stock_receipt:"+receiptID.String(), reason) {
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toStockReceiptResponse(reversal)}, nil); err != nil {
		api.logger.Error("write platform reverse stock receipt response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformForceReverseStockReceipt implements
// POST /platform/businesses/{business_id}/stock-receipts/{receipt_id}/force-reverse
// -- the one new mechanism a business owner does not have (inventory.Service.
// AdminForceReverseStockReceipt's own doc comment covers exactly what it
// relaxes and what it still refuses).
func (api *API) platformForceReverseStockReceipt(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesAdjust)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	receiptID, err := uuid.Parse(chi.URLParam(r, "receipt_id"))
	if err != nil {
		api.badRequestResponse(w, r, "receipt_id must be a valid UUID.")
		return
	}
	reason, ok := readCorrectiveReason(api, w, r)
	if !ok {
		return
	}
	reversal, err := api.inventory.AdminForceReverseStockReceipt(r.Context(), identity.PlatformSupportUserID, businessID, receiptID, identity.PlatformSupportUserID)
	if err != nil {
		inventoryErrorResponse(api, w, r, err, "platform force reverse stock receipt")
		return
	}
	if !api.recordCorrectiveAction(w, r, staff, businessID, platformadmin.ActionBusinessReceiptForceReversed, "stock_receipt:"+receiptID.String(), reason) {
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toStockReceiptResponse(reversal)}, nil); err != nil {
		api.logger.Error("write platform force reverse stock receipt response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformListAdjustmentRequests implements
// GET /platform/businesses/{business_id}/inventory-adjustments -- every
// request regardless of status (inventory.Service.ListAllAdjustmentRequests),
// unlike the business-facing endpoint which only ever shows the pending
// approval queue.
func (api *API) platformListAdjustmentRequests(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesReadDetail)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	if !api.recordDetailView(w, r, staff, businessID, "inventory_adjustments") {
		return
	}
	list, err := api.inventory.ListAllAdjustmentRequests(r.Context(), uuid.Nil, businessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list all adjustment requests: %w", err))
		return
	}
	actorIDs := make([]uuid.UUID, 0, len(list)*2)
	for _, a := range list {
		actorIDs = append(actorIDs, a.RequestedBy)
		if a.DecidedBy != uuid.Nil {
			actorIDs = append(actorIDs, a.DecidedBy)
		}
	}
	names, err := api.resolveActorNames(r.Context(), actorIDs)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve actor names: %w", err))
		return
	}
	out := make([]adjustmentRequestResponse, 0, len(list))
	for _, a := range list {
		out = append(out, toAdjustmentRequestResponse(a, names[a.RequestedBy], names[a.DecidedBy]))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write platform list adjustment requests response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformGetAdjustmentRequest implements
// GET /platform/businesses/{business_id}/inventory-adjustments/{adjustment_id}.
func (api *API) platformGetAdjustmentRequest(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesReadDetail)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	requestID, err := uuid.Parse(chi.URLParam(r, "adjustment_id"))
	if err != nil {
		api.badRequestResponse(w, r, "adjustment_id must be a valid UUID.")
		return
	}
	if !api.recordDetailView(w, r, staff, businessID, "inventory_adjustment:"+requestID.String()) {
		return
	}
	req, err := api.inventory.GetAdjustmentRequest(r.Context(), uuid.Nil, businessID, requestID)
	if err != nil {
		inventoryErrorResponse(api, w, r, err, "get adjustment request")
		return
	}
	names, err := api.resolveActorNames(r.Context(), []uuid.UUID{req.RequestedBy, req.DecidedBy})
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve actor names: %w", err))
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toAdjustmentRequestResponse(req, names[req.RequestedBy], names[req.DecidedBy])}, nil); err != nil {
		api.logger.Error("write platform get adjustment request response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformApproveAdjustmentRequest implements
// POST /platform/businesses/{business_id}/inventory-adjustments/{adjustment_id}/approve.
func (api *API) platformApproveAdjustmentRequest(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesAdjust)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	requestID, err := uuid.Parse(chi.URLParam(r, "adjustment_id"))
	if err != nil {
		api.badRequestResponse(w, r, "adjustment_id must be a valid UUID.")
		return
	}
	reason, ok := readCorrectiveReason(api, w, r)
	if !ok {
		return
	}
	updated, err := api.inventory.ApproveAdjustmentRequest(r.Context(), identity.PlatformSupportUserID, businessID, requestID, identity.PlatformSupportUserID, reason)
	if err != nil {
		inventoryErrorResponse(api, w, r, err, "platform approve adjustment request")
		return
	}
	if !api.recordCorrectiveAction(w, r, staff, businessID, platformadmin.ActionBusinessAdjustmentApproved, "inventory_adjustment:"+requestID.String(), reason) {
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toAdjustmentRequestResponse(updated, "", "")}, nil); err != nil {
		api.logger.Error("write platform approve adjustment request response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformRejectAdjustmentRequest implements
// POST /platform/businesses/{business_id}/inventory-adjustments/{adjustment_id}/reject.
func (api *API) platformRejectAdjustmentRequest(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesAdjust)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	requestID, err := uuid.Parse(chi.URLParam(r, "adjustment_id"))
	if err != nil {
		api.badRequestResponse(w, r, "adjustment_id must be a valid UUID.")
		return
	}
	reason, ok := readCorrectiveReason(api, w, r)
	if !ok {
		return
	}
	updated, err := api.inventory.RejectAdjustmentRequest(r.Context(), identity.PlatformSupportUserID, businessID, requestID, identity.PlatformSupportUserID, reason)
	if err != nil {
		inventoryErrorResponse(api, w, r, err, "platform reject adjustment request")
		return
	}
	if !api.recordCorrectiveAction(w, r, staff, businessID, platformadmin.ActionBusinessAdjustmentRejected, "inventory_adjustment:"+requestID.String(), reason) {
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toAdjustmentRequestResponse(updated, "", "")}, nil); err != nil {
		api.logger.Error("write platform reject adjustment request response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformGetStockHistory implements
// GET /platform/businesses/{business_id}/stock/{variant_id}/history.
func (api *API) platformGetStockHistory(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesReadDetail)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	variantID, err := uuid.Parse(chi.URLParam(r, "variant_id"))
	if err != nil {
		api.badRequestResponse(w, r, "variant_id must be a valid UUID.")
		return
	}
	if !api.recordDetailView(w, r, staff, businessID, "stock_history:"+variantID.String()) {
		return
	}
	limit := parseLimit(r, 50, 200)
	list, err := api.inventory.GetHistory(r.Context(), uuid.Nil, businessID, variantID, limit)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("get stock history: %w", err))
		return
	}
	actorIDs := make([]uuid.UUID, 0, len(list)*2)
	for _, h := range list {
		actorIDs = append(actorIDs, h.ActorID)
		if h.AdjustmentDecidedBy != uuid.Nil {
			actorIDs = append(actorIDs, h.AdjustmentDecidedBy)
		}
	}
	names, err := api.resolveActorNames(r.Context(), actorIDs)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve actor names: %w", err))
		return
	}
	out := make([]historyEntryResponse, 0, len(list))
	for _, h := range list {
		out = append(out, historyEntryResponse{
			ID: h.ID.String(), QuantityDelta: h.QuantityDelta, CreatedAt: h.CreatedAt.UTC().Format(time.RFC3339),
			EventType: h.EventType, ActorName: names[h.ActorID],
			AdjustmentReasonCategory: h.AdjustmentReasonCategory, AdjustmentReasonNote: h.AdjustmentReasonNote,
			AdjustmentDecidedByName: names[h.AdjustmentDecidedBy],
		})
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write platform stock history response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformCorrectBalance implements
// POST /platform/businesses/{business_id}/stock/{variant_id}/correct-balance
// -- the other new mechanism a business owner does not have
// (inventory.Service.AdminCorrectBalance's own doc comment covers how this
// stays a ledger-derived change, never a raw overwrite).
func (api *API) platformCorrectBalance(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesAdjust)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	variantID, err := uuid.Parse(chi.URLParam(r, "variant_id"))
	if err != nil {
		api.badRequestResponse(w, r, "variant_id must be a valid UUID.")
		return
	}
	var req struct {
		TargetQuantity int32  `json:"target_quantity"`
		Reason         string `json:"reason"`
	}
	if err := readJSON(w, r, &req, 1<<12); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	v := validator.New()
	v.Check(req.Reason != "", "reason", "must be provided")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}
	location, err := api.identity.GetDefaultLocation(r.Context(), identity.PlatformSupportUserID, businessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("get default location: %w", err))
		return
	}
	updated, err := api.inventory.AdminCorrectBalance(r.Context(), identity.PlatformSupportUserID, businessID,
		location.ID, variantID, identity.PlatformSupportUserID, req.TargetQuantity, req.Reason)
	if err != nil {
		inventoryErrorResponse(api, w, r, err, "platform correct balance")
		return
	}
	if !api.recordCorrectiveAction(w, r, staff, businessID, platformadmin.ActionBusinessBalanceCorrected, "variant:"+variantID.String(), req.Reason) {
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toAdjustmentRequestResponse(updated, "", "")}, nil); err != nil {
		api.logger.Error("write platform correct balance response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformListSaleReviews implements
// GET /platform/businesses/{business_id}/sale-reviews.
func (api *API) platformListSaleReviews(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesReadDetail)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	if !api.recordDetailView(w, r, staff, businessID, "sale_reviews") {
		return
	}
	list, err := api.sales.ListOpenReviews(r.Context(), uuid.Nil, businessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list open sale reviews: %w", err))
		return
	}
	sellerIDs := make([]uuid.UUID, len(list))
	for i, rv := range list {
		sellerIDs[i] = rv.SellerID
	}
	sellerNames, err := api.resolveSellerNames(r.Context(), sellerIDs)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve seller names: %w", err))
		return
	}
	out := make([]reviewResponse, 0, len(list))
	for _, rv := range list {
		out = append(out, reviewResponse{
			ID: rv.ID.String(), SaleID: rv.SaleID.String(), SaleItemID: rv.SaleItemID.String(),
			Reason: rv.Reason, Status: rv.Status,
			SaleOccurredAt: rv.SaleOccurredAt.UTC().Format(time.RFC3339), SellerName: sellerNames[rv.SellerID],
			ItemDescription:   rv.ItemDescription,
			ItemQuantity:      rv.ItemQuantity,
			ItemUnitPriceKobo: rv.ItemUnitPriceKobo, ItemLineTotalKobo: rv.ItemLineTotalKobo,
		})
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write platform list sale reviews response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformResolveSaleReview implements
// POST /platform/businesses/{business_id}/sale-reviews/{review_id}/resolve.
func (api *API) platformResolveSaleReview(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesAdjust)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	reviewID, err := uuid.Parse(chi.URLParam(r, "review_id"))
	if err != nil {
		api.badRequestResponse(w, r, "review_id must be a valid UUID.")
		return
	}
	reason, ok := readCorrectiveReason(api, w, r)
	if !ok {
		return
	}
	updated, err := api.sales.ResolveReview(r.Context(), identity.PlatformSupportUserID, businessID, reviewID, identity.PlatformSupportUserID, reason)
	if err != nil {
		salesErrorResponse(api, w, r, err, "platform resolve sale review")
		return
	}
	if !api.recordCorrectiveAction(w, r, staff, businessID, platformadmin.ActionBusinessSaleReviewResolved, "sale_review:"+reviewID.String(), reason) {
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": reviewResponse{
		ID: updated.ID.String(), SaleID: updated.SaleID.String(), SaleItemID: updated.SaleItemID.String(),
		Reason: updated.Reason, Status: updated.Status,
	}}, nil); err != nil {
		api.logger.Error("write platform resolve sale review response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformListInventoryReviews implements
// GET /platform/businesses/{business_id}/inventory-reviews.
func (api *API) platformListInventoryReviews(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesReadDetail)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	if !api.recordDetailView(w, r, staff, businessID, "inventory_reviews") {
		return
	}
	list, err := api.inventory.ListOpenReviews(r.Context(), uuid.Nil, businessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list open inventory reviews: %w", err))
		return
	}
	out := make([]inventoryReviewResponse, 0, len(list))
	for _, rv := range list {
		out = append(out, toInventoryReviewResponse(rv))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write platform list inventory reviews response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformResolveInventoryReview implements
// POST /platform/businesses/{business_id}/inventory-reviews/{review_id}/resolve.
func (api *API) platformResolveInventoryReview(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesAdjust)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	reviewID, err := uuid.Parse(chi.URLParam(r, "review_id"))
	if err != nil {
		api.badRequestResponse(w, r, "review_id must be a valid UUID.")
		return
	}
	reason, ok := readCorrectiveReason(api, w, r)
	if !ok {
		return
	}
	updated, err := api.inventory.ResolveReview(r.Context(), identity.PlatformSupportUserID, businessID, reviewID, identity.PlatformSupportUserID, reason, nil)
	if err != nil {
		inventoryErrorResponse(api, w, r, err, "platform resolve inventory review")
		return
	}
	if !api.recordCorrectiveAction(w, r, staff, businessID, platformadmin.ActionBusinessInventoryReviewResolved, "inventory_review:"+reviewID.String(), reason) {
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toInventoryReviewResponse(updated)}, nil); err != nil {
		api.logger.Error("write platform resolve inventory review response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// platformListActivityFeed implements
// GET /platform/businesses/{business_id}/activity/feed -- the line-item
// unified activity feed, distinct from GET .../activity (the pre-existing
// aggregate summary route). Unlike the business-facing
// GET /api/v1/activity, this is never actor-scoped: a platform admin's
// read here is not "my own entries," it's the whole business.
func (api *API) platformListActivityFeed(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesReadDetail)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	if !api.recordDetailView(w, r, staff, businessID, "activity_feed") {
		return
	}
	var filter activity.Filter
	q := r.URL.Query()
	filter.Type = q.Get("type")
	if raw := q.Get("start_at"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			api.badRequestResponse(w, r, "start_at must be an RFC 3339 timestamp.")
			return
		}
		filter.StartAt = t
	}
	if raw := q.Get("end_at"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			api.badRequestResponse(w, r, "end_at must be an RFC 3339 timestamp.")
			return
		}
		filter.EndAt = t
	}
	limit := parseLimit(r, 50, 200)

	list, err := api.activity.List(r.Context(), uuid.Nil, businessID, filter, limit)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list activity: %w", err))
		return
	}
	actorIDs := make([]uuid.UUID, len(list))
	for i, e := range list {
		actorIDs[i] = e.ActorID
	}
	names, err := api.resolveSellerNames(r.Context(), actorIDs)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve actor names: %w", err))
		return
	}
	out := make([]activityEntryResponse, 0, len(list))
	for _, e := range list {
		out = append(out, activityEntryResponse{
			ID: e.ID.String(), Type: e.Type, ActorID: e.ActorID.String(), ActorName: names[e.ActorID],
			OccurredAt: e.OccurredAt.UTC().Format(time.RFC3339), Summary: e.Summary, AmountKobo: e.AmountKobo,
		})
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write platform activity feed response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type platformMemberResponse struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	JoinedAt string `json:"joined_at"`
}

// platformListMembers implements
// GET /platform/businesses/{business_id}/members -- full member names and
// roles, widening the aggregate dashboard's owner/staff *counts* into the
// actual list (docs/PHASE_PLATFORM_ADMIN_DEEP_DRILL.md's "full line items
// including staff identity" read-depth decision).
func (api *API) platformListMembers(w http.ResponseWriter, r *http.Request) {
	staff, ok := api.requirePlatformCapability(w, r, platformadmin.CapabilityBusinessesReadDetail)
	if !ok {
		return
	}
	businessID, ok := api.platformDetailBusiness(w, r)
	if !ok {
		return
	}
	if !api.recordDetailView(w, r, staff, businessID, "members") {
		return
	}
	list, err := api.identity.ListMembersForBusiness(r.Context(), uuid.Nil, businessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list members: %w", err))
		return
	}
	out := make([]platformMemberResponse, 0, len(list))
	for _, m := range list {
		out = append(out, platformMemberResponse{Name: m.Name, Role: m.Role, JoinedAt: m.JoinedAt.UTC().Format(time.RFC3339)})
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write platform list members response", "request_id", RequestID(r.Context()), "error", err)
	}
}
