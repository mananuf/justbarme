package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/sales"
	"github.com/mananuf/justbarme/internal/tenancy"
	"github.com/mananuf/justbarme/internal/validator"
)

// tabsErrorResponse centralizes the sales.Err* -> HTTP status mapping for
// every tables/customers/bill/payment handler in this file, the same
// pattern catalogueErrorResponse and salesErrorResponse already use.
func tabsErrorResponse(api *API, w http.ResponseWriter, r *http.Request, err error, action string) {
	switch {
	case errors.Is(err, sales.ErrTableNotFound), errors.Is(err, sales.ErrCustomerNotFound),
		errors.Is(err, sales.ErrBillNotFound), errors.Is(err, sales.ErrPaymentNotFound),
		errors.Is(err, sales.ErrVariantNotFound):
		api.notFoundResponse(w, r)
	case errors.Is(err, sales.ErrTableLabelTaken), errors.Is(err, sales.ErrBillNotOpen),
		errors.Is(err, sales.ErrBillNotClosedUnpaid), errors.Is(err, sales.ErrBillNotPayable),
		errors.Is(err, sales.ErrBillNotEditable), errors.Is(err, sales.ErrInsufficientQuantityOnBill),
		errors.Is(err, sales.ErrBillFullyPaid),
		errors.Is(err, sales.ErrCreditRequiresCustomer), errors.Is(err, sales.ErrOverpayment),
		errors.Is(err, sales.ErrWriteOffExceedsBalance), errors.Is(err, sales.ErrBillHasOutstandingActivity),
		errors.Is(err, sales.ErrPaymentAlreadyReversed), errors.Is(err, sales.ErrPaymentIsReversal):
		api.conflictResponse(w, r, err.Error())
	default:
		api.internalErrorResponse(w, r, fmt.Errorf("%s: %w", action, err))
	}
}

type tableResponse struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Active bool   `json:"active"`
}

func toTableResponse(t sales.Table) tableResponse {
	return tableResponse{ID: t.ID.String(), Label: t.Label, Active: t.Active}
}

type customerResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Phone string `json:"phone,omitempty"`
	Email string `json:"email,omitempty"`
	Notes string `json:"notes,omitempty"`
}

func toCustomerResponse(c sales.Customer) customerResponse {
	return customerResponse{ID: c.ID.String(), Name: c.Name, Phone: c.Phone, Email: c.Email, Notes: c.Notes}
}

type billResponse struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	TableID     *string `json:"table_id,omitempty"`
	CustomerID  *string `json:"customer_id,omitempty"`
	BalanceKobo int64   `json:"balance_kobo"`
	OpenedAt    string  `json:"opened_at"`
}

func toBillResponse(b sales.Bill) billResponse {
	return billResponse{
		ID: b.ID.String(), Status: b.Status, TableID: uuidOrNil(b.TableID), CustomerID: uuidOrNil(b.CustomerID),
		BalanceKobo: b.BalanceKobo, OpenedAt: b.OpenedAt.UTC().Format(time.RFC3339),
	}
}

type writeOffResponse struct {
	ID         string `json:"id"`
	AmountKobo int64  `json:"amount_kobo"`
	Reason     string `json:"reason"`
	CreatedAt  string `json:"created_at"`
}

func toWriteOffResponse(w sales.WriteOff) writeOffResponse {
	return writeOffResponse{ID: w.ID.String(), AmountKobo: w.AmountKobo, Reason: w.Reason, CreatedAt: w.CreatedAt.UTC().Format(time.RFC3339)}
}

type billDetailResponse struct {
	billResponse
	Sales     []saleResponse     `json:"sales,omitempty"`
	Payments  []paymentResponse  `json:"payments,omitempty"`
	WriteOffs []writeOffResponse `json:"write_offs,omitempty"`
}

func toBillDetailResponse(d sales.BillDetail) billDetailResponse {
	out := billDetailResponse{billResponse: toBillResponse(d.Bill)}
	for _, s := range d.Sales {
		out.Sales = append(out.Sales, toSaleResponse(s))
	}
	for _, p := range d.Payments {
		out.Payments = append(out.Payments, paymentResponse{AmountKobo: p.AmountKobo, Method: p.Method})
	}
	for _, w := range d.WriteOffs {
		out.WriteOffs = append(out.WriteOffs, toWriteOffResponse(w))
	}
	return out
}

// createTable implements POST /api/v1/tables (bills:manage).
func (api *API) createTable(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilityBillsManage)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	var req struct {
		Label string `json:"label"`
	}
	if err := readJSON(w, r, &req, 1<<12); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	v := validator.New()
	v.Check(req.Label != "", "label", "must be provided")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	table, err := api.sales.CreateTable(r.Context(), principal.UserID, business.BusinessID, req.Label)
	if err != nil {
		tabsErrorResponse(api, w, r, err, "create table")
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toTableResponse(table)}, nil); err != nil {
		api.logger.Error("write create table response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// listTables implements GET /api/v1/tables (bills:read).
func (api *API) listTables(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilityBillsRead)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	list, err := api.sales.ListTables(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list tables: %w", err))
		return
	}
	out := make([]tableResponse, 0, len(list))
	for _, t := range list {
		out = append(out, toTableResponse(t))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list tables response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// createCustomer implements POST /api/v1/customers (bills:manage -- Staff
// are allowed to grant credit, docs/ARCHITECTURE.md §8.3).
func (api *API) createCustomer(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilityBillsManage)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	var req struct {
		Name  string `json:"name"`
		Phone string `json:"phone"`
		Email string `json:"email"`
		Notes string `json:"notes"`
	}
	if err := readJSON(w, r, &req, 1<<12); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	v := validator.New()
	v.Check(req.Name != "", "name", "must be provided")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	customer, err := api.sales.CreateCustomer(r.Context(), principal.UserID, business.BusinessID, req.Name, req.Phone, req.Email, req.Notes)
	if err != nil {
		tabsErrorResponse(api, w, r, err, "create customer")
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toCustomerResponse(customer)}, nil); err != nil {
		api.logger.Error("write create customer response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// listCustomers implements GET /api/v1/customers (bills:read).
func (api *API) listCustomers(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilityBillsRead)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	list, err := api.sales.ListCustomers(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list customers: %w", err))
		return
	}
	out := make([]customerResponse, 0, len(list))
	for _, c := range list {
		out = append(out, toCustomerResponse(c))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list customers response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// openBill implements POST /api/v1/bills (bills:manage): opens a tab
// against an optional table and/or named customer -- neither is a plain
// walk-in tab opened ahead of its first round.
func (api *API) openBill(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilityBillsManage)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	var req struct {
		TableID    string `json:"table_id"`
		CustomerID string `json:"customer_id"`
	}
	if err := readJSON(w, r, &req, 1<<12); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}

	v := validator.New()
	var tableID, customerID uuid.UUID
	if req.TableID != "" {
		var err error
		tableID, err = uuid.Parse(req.TableID)
		v.Check(err == nil, "table_id", "must be a valid UUID")
	}
	if req.CustomerID != "" {
		var err error
		customerID, err = uuid.Parse(req.CustomerID)
		v.Check(err == nil, "customer_id", "must be a valid UUID")
	}
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	location, err := api.identity.GetDefaultLocation(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve default location: %w", err))
		return
	}

	bill, err := api.sales.OpenBill(r.Context(), principal.UserID, business.BusinessID, location.ID, principal.UserID, tableID, customerID)
	if err != nil {
		tabsErrorResponse(api, w, r, err, "open bill")
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toBillResponse(bill)}, nil); err != nil {
		api.logger.Error("write open bill response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// listBills implements GET /api/v1/bills?status=open|outstanding
// (bills:read). Defaults to open/closed-unpaid tabs (the table grid / open
// tabs view); ?status=outstanding lists every bill with a nonzero balance
// regardless of status.
func (api *API) listBills(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilityBillsRead)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	var list []sales.Bill
	var err error
	if r.URL.Query().Get("status") == "outstanding" {
		list, err = api.sales.ListOutstandingBills(r.Context(), principal.UserID, business.BusinessID)
	} else {
		list, err = api.sales.ListOpenBills(r.Context(), principal.UserID, business.BusinessID)
	}
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list bills: %w", err))
		return
	}
	out := make([]billResponse, 0, len(list))
	for _, b := range list {
		out = append(out, toBillResponse(b))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list bills response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// getBillDetail implements GET /api/v1/bills/{bill_id} (bills:read).
func (api *API) getBillDetail(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilityBillsRead)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	billID, err := uuid.Parse(chi.URLParam(r, "bill_id"))
	if err != nil {
		api.badRequestResponse(w, r, "bill_id must be a valid UUID.")
		return
	}

	detail, err := api.sales.GetBillDetail(r.Context(), principal.UserID, business.BusinessID, billID)
	if err != nil {
		tabsErrorResponse(api, w, r, err, "get bill")
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toBillDetailResponse(detail)}, nil); err != nil {
		api.logger.Error("write get bill response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// addSaleRound implements POST /api/v1/bills/{bill_id}/rounds
// (sales:record) -- appends another round of items to an already-open
// tab. Same idempotency-key contract as POST /sales.
func (api *API) addSaleRound(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilitySalesRecord)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	billID, err := uuid.Parse(chi.URLParam(r, "bill_id"))
	if err != nil {
		api.badRequestResponse(w, r, "bill_id must be a valid UUID.")
		return
	}

	var req struct {
		IdempotencyKey string            `json:"idempotency_key"`
		OccurredAt     string            `json:"occurred_at"`
		Items          []saleItemRequest `json:"items"`
	}
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}

	v := validator.New()
	idempotencyKey, idErr := uuid.Parse(req.IdempotencyKey)
	v.Check(idErr == nil, "idempotency_key", "must be a valid UUID")
	occurredAt, timeErr := time.Parse(time.RFC3339, req.OccurredAt)
	v.Check(timeErr == nil, "occurred_at", "must be an RFC 3339 timestamp")
	v.Check(len(req.Items) > 0, "items", "must include at least one item")

	items := make([]sales.SaleItemInput, 0, len(req.Items))
	for i, raw := range req.Items {
		field := fmt.Sprintf("items[%d]", i)
		variantID, err := uuid.Parse(raw.VariantID)
		v.Check(err == nil, field+".variant_id", "must be a valid UUID")
		v.Check(raw.Quantity > 0, field+".quantity", "must be positive")
		v.Check(raw.UnitPriceKobo >= 0, field+".unit_price_kobo", "must not be negative")
		if err == nil {
			items = append(items, sales.SaleItemInput{VariantID: variantID, Quantity: raw.Quantity, UnitPriceKobo: raw.UnitPriceKobo})
		}
	}
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	sale, err := api.sales.AddSaleRound(r.Context(), principal.UserID, business.BusinessID, billID, principal.UserID, idempotencyKey, occurredAt, items)
	if err != nil {
		tabsErrorResponse(api, w, r, err, "add sale round")
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toSaleResponse(sale)}, nil); err != nil {
		api.logger.Error("write add sale round response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// removeBillItem implements POST /api/v1/bills/{bill_id}/items/remove
// (sales:record) -- posts a compensating negative round that removes
// quantity units of one variant from an open/closed_unpaid bill (never
// edits/deletes a previously posted round). Same idempotency-key contract
// as addSaleRound.
func (api *API) removeBillItem(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilitySalesRecord)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	billID, err := uuid.Parse(chi.URLParam(r, "bill_id"))
	if err != nil {
		api.badRequestResponse(w, r, "bill_id must be a valid UUID.")
		return
	}

	var req struct {
		IdempotencyKey string `json:"idempotency_key"`
		OccurredAt     string `json:"occurred_at"`
		VariantID      string `json:"variant_id"`
		Quantity       int32  `json:"quantity"`
		UnitPriceKobo  int64  `json:"unit_price_kobo"`
	}
	if err := readJSON(w, r, &req, 1<<12); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}

	v := validator.New()
	idempotencyKey, idErr := uuid.Parse(req.IdempotencyKey)
	v.Check(idErr == nil, "idempotency_key", "must be a valid UUID")
	occurredAt, timeErr := time.Parse(time.RFC3339, req.OccurredAt)
	v.Check(timeErr == nil, "occurred_at", "must be an RFC 3339 timestamp")
	variantID, variantErr := uuid.Parse(req.VariantID)
	v.Check(variantErr == nil, "variant_id", "must be a valid UUID")
	v.Check(req.Quantity > 0, "quantity", "must be positive")
	v.Check(req.UnitPriceKobo >= 0, "unit_price_kobo", "must not be negative")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	sale, err := api.sales.RemoveBillItem(
		r.Context(), principal.UserID, business.BusinessID, billID, principal.UserID, variantID,
		idempotencyKey, req.Quantity, req.UnitPriceKobo, occurredAt,
	)
	if err != nil {
		tabsErrorResponse(api, w, r, err, "remove bill item")
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toSaleResponse(sale)}, nil); err != nil {
		api.logger.Error("write remove bill item response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// closeBill implements POST /api/v1/bills/{bill_id}/close (bills:manage):
// "stop adding items" -- never implies payment.
func (api *API) closeBill(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilityBillsManage)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	billID, err := uuid.Parse(chi.URLParam(r, "bill_id"))
	if err != nil {
		api.badRequestResponse(w, r, "bill_id must be a valid UUID.")
		return
	}

	bill, err := api.sales.CloseBill(r.Context(), principal.UserID, business.BusinessID, billID)
	if err != nil {
		tabsErrorResponse(api, w, r, err, "close bill")
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toBillResponse(bill)}, nil); err != nil {
		api.logger.Error("write close bill response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// voidBill implements POST /api/v1/bills/{bill_id}/void (sales:reverse --
// owner-only, matching reversal's caution tier).
func (api *API) voidBill(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilitySalesReverse)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	billID, err := uuid.Parse(chi.URLParam(r, "bill_id"))
	if err != nil {
		api.badRequestResponse(w, r, "bill_id must be a valid UUID.")
		return
	}

	bill, err := api.sales.VoidBill(r.Context(), principal.UserID, business.BusinessID, billID)
	if err != nil {
		tabsErrorResponse(api, w, r, err, "void bill")
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toBillResponse(bill)}, nil); err != nil {
		api.logger.Error("write void bill response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// writeOffBill implements POST /api/v1/bills/{bill_id}/write-off
// (bills:write_off -- owner-only). Reason required.
func (api *API) writeOffBill(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilityBillsWriteOff)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	billID, err := uuid.Parse(chi.URLParam(r, "bill_id"))
	if err != nil {
		api.badRequestResponse(w, r, "bill_id must be a valid UUID.")
		return
	}

	var req struct {
		AmountKobo int64  `json:"amount_kobo"`
		Reason     string `json:"reason"`
	}
	if err := readJSON(w, r, &req, 1<<12); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	v := validator.New()
	v.Check(req.AmountKobo > 0, "amount_kobo", "must be positive")
	v.Check(req.Reason != "", "reason", "must be provided")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	bill, err := api.sales.WriteOffBill(r.Context(), principal.UserID, business.BusinessID, billID, principal.UserID, req.AmountKobo, req.Reason)
	if err != nil {
		tabsErrorResponse(api, w, r, err, "write off bill")
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toBillResponse(bill)}, nil); err != nil {
		api.logger.Error("write write-off bill response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// recordPayment implements POST /api/v1/bills/{bill_id}/payments
// (payments:record). Overpayment is rejected (409) rather than clamped.
func (api *API) recordPayment(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilityPaymentsRecord)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	billID, err := uuid.Parse(chi.URLParam(r, "bill_id"))
	if err != nil {
		api.badRequestResponse(w, r, "bill_id must be a valid UUID.")
		return
	}

	var req paymentRequest
	if err := readJSON(w, r, &req, 1<<12); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	v := validator.New()
	v.Check(req.AmountKobo > 0, "amount_kobo", "must be positive")
	v.Check(validator.PermittedValue(req.Method, "cash", "transfer", "card"), "method", "must be cash, transfer, or card")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	payment, err := api.sales.RecordPayment(r.Context(), principal.UserID, business.BusinessID, billID, principal.UserID, req.AmountKobo, req.Method)
	if err != nil {
		tabsErrorResponse(api, w, r, err, "record payment")
		return
	}
	out := paymentResponse{AmountKobo: payment.AmountKobo, Method: payment.Method}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write record payment response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// reversePayment implements POST /api/v1/payments/{payment_id}/reverse
// (payments:reverse -- owner-only).
func (api *API) reversePayment(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilityPaymentsReverse)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	paymentID, err := uuid.Parse(chi.URLParam(r, "payment_id"))
	if err != nil {
		api.badRequestResponse(w, r, "payment_id must be a valid UUID.")
		return
	}

	payment, err := api.sales.ReversePayment(r.Context(), principal.UserID, business.BusinessID, paymentID, principal.UserID)
	if err != nil {
		tabsErrorResponse(api, w, r, err, "reverse payment")
		return
	}
	out := paymentResponse{AmountKobo: payment.AmountKobo, Method: payment.Method}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write reverse payment response", "request_id", RequestID(r.Context()), "error", err)
	}
}
