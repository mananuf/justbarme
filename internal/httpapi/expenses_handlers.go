package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/expenses"
	"github.com/mananuf/justbarme/internal/tenancy"
	"github.com/mananuf/justbarme/internal/validator"
)

// expensesErrorResponse centralizes the expenses.Err* -> HTTP status
// mapping, the same pattern inventoryErrorResponse/catalogueErrorResponse
// already use.
func expensesErrorResponse(api *API, w http.ResponseWriter, r *http.Request, err error, action string) {
	switch {
	case errors.Is(err, expenses.ErrCategoryNameTaken):
		api.conflictResponse(w, r, err.Error())
	case errors.Is(err, expenses.ErrExpenseNotFound):
		api.notFoundResponse(w, r)
	case errors.Is(err, expenses.ErrAlreadyReversed):
		api.conflictResponse(w, r, err.Error())
	default:
		api.internalErrorResponse(w, r, fmt.Errorf("%s: %w", action, err))
	}
}

type expenseCategoryResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

func toExpenseCategoryResponse(c expenses.Category) expenseCategoryResponse {
	return expenseCategoryResponse{ID: c.ID.String(), Name: c.Name, Active: c.Active}
}

type createExpenseCategoryRequest struct {
	Name string `json:"name"`
}

// createExpenseCategory implements POST /api/v1/expense-categories
// (expenses:record -- creating a category is part of the ordinary
// recording flow, not a separate administrative action, matching how any
// Staff member using Stock.tsx's "Report issue" flow already picks from
// or extends a shared list).
func (api *API) createExpenseCategory(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("createExpenseCategory ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityExpensesRecord)
	if !ok {
		return
	}

	var req createExpenseCategoryRequest
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

	created, err := api.expenses.CreateCategory(r.Context(), principal.UserID, business.BusinessID, req.Name)
	if err != nil {
		expensesErrorResponse(api, w, r, err, "create expense category")
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toExpenseCategoryResponse(created)}, nil); err != nil {
		api.logger.Error("write create expense category response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// listExpenseCategories implements GET /api/v1/expense-categories
// (expenses:record -- anyone who can record an expense needs the list to
// pick from).
func (api *API) listExpenseCategories(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("listExpenseCategories ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityExpensesRecord)
	if !ok {
		return
	}

	list, err := api.expenses.ListCategories(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list expense categories: %w", err))
		return
	}
	out := make([]expenseCategoryResponse, 0, len(list))
	for _, c := range list {
		out = append(out, toExpenseCategoryResponse(c))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list expense categories response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type expenseResponse struct {
	ID            string `json:"id"`
	CategoryID    string `json:"category_id"`
	CategoryName  string `json:"category_name,omitempty"`
	Description   string `json:"description"`
	AmountKobo    int64  `json:"amount_kobo"`
	PaymentMethod string `json:"payment_method"`
	RecordedBy    string `json:"recorded_by"`
	ReversalOf    string `json:"reversal_of,omitempty"`
	OccurredAt    string `json:"occurred_at"`
}

func toExpenseResponse(e expenses.Expense) expenseResponse {
	out := expenseResponse{
		ID: e.ID.String(), CategoryID: e.CategoryID.String(), CategoryName: e.CategoryName,
		Description: e.Description, AmountKobo: e.AmountKobo, PaymentMethod: e.PaymentMethod,
		RecordedBy: e.RecordedBy.String(), OccurredAt: e.OccurredAt.UTC().Format(time.RFC3339),
	}
	if e.ReversalOf != uuid.Nil {
		out.ReversalOf = e.ReversalOf.String()
	}
	return out
}

type recordExpenseRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	CategoryID     string `json:"category_id"`
	Description    string `json:"description"`
	AmountKobo     int64  `json:"amount_kobo"`
	PaymentMethod  string `json:"payment_method"`
	OccurredAt     string `json:"occurred_at"`
}

// recordExpense implements POST /api/v1/expenses (expenses:record,
// offline-safe -- see docs/PHASE_EXPENSES_DASHBOARD_ACTIVITY_REPORTS.md
// §4). idempotency_key makes a queued-offline submission safe to retry.
func (api *API) recordExpense(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("recordExpense ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityExpensesRecord)
	if !ok {
		return
	}

	var req recordExpenseRequest
	if err := readJSON(w, r, &req, 1<<12); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}

	v := validator.New()
	idempotencyKey, idErr := uuid.Parse(req.IdempotencyKey)
	v.Check(idErr == nil, "idempotency_key", "must be a valid UUID")
	categoryID, categoryErr := uuid.Parse(req.CategoryID)
	v.Check(categoryErr == nil, "category_id", "must be a valid UUID")
	v.Check(req.Description != "", "description", "must be provided")
	v.Check(req.AmountKobo > 0, "amount_kobo", "must be positive")
	v.Check(validator.PermittedValue(req.PaymentMethod, "cash", "transfer", "card"), "payment_method", "must be cash, transfer, or card")
	occurredAt, timeErr := time.Parse(time.RFC3339, req.OccurredAt)
	v.Check(timeErr == nil, "occurred_at", "must be an RFC 3339 timestamp")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	location, err := api.identity.GetDefaultLocation(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve default location: %w", err))
		return
	}

	created, err := api.expenses.RecordExpense(r.Context(), principal.UserID, business.BusinessID,
		location.ID, categoryID, principal.UserID, idempotencyKey, req.Description, req.AmountKobo, req.PaymentMethod, occurredAt)
	if err != nil {
		expensesErrorResponse(api, w, r, err, "record expense")
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toExpenseResponse(created)}, nil); err != nil {
		api.logger.Error("write record expense response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// reverseExpense implements POST /api/v1/expenses/{expense_id}/reverse
// (expenses:reverse, Owner-only, deliberately not offline-safe -- same
// tier as sales:reverse/inventory:adjustment_approve). Creates a new,
// equal-and-opposite expense row; never edits or deletes the original.
func (api *API) reverseExpense(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("reverseExpense ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityExpensesReverse)
	if !ok {
		return
	}

	expenseID, err := uuid.Parse(chi.URLParam(r, "expense_id"))
	if err != nil {
		api.badRequestResponse(w, r, "expense_id must be a valid UUID.")
		return
	}

	reversal, err := api.expenses.ReverseExpense(r.Context(), principal.UserID, business.BusinessID, expenseID, principal.UserID)
	if err != nil {
		expensesErrorResponse(api, w, r, err, "reverse expense")
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toExpenseResponse(reversal)}, nil); err != nil {
		api.logger.Error("write reverse expense response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// listExpenses implements GET /api/v1/expenses (activity:read, same tier
// as listSales -- whether Staff should see their own expense history is
// the same open pilot question already on record for sales, carried
// forward rather than resolved here).
func (api *API) listExpenses(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("listExpenses ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityActivityRead)
	if !ok {
		return
	}

	limit := int32(20)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 32); err == nil && parsed > 0 && parsed <= 200 {
			limit = int32(parsed)
		}
	}
	list, err := api.expenses.ListExpenses(r.Context(), principal.UserID, business.BusinessID, limit)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list expenses: %w", err))
		return
	}
	out := make([]expenseResponse, 0, len(list))
	for _, e := range list {
		out = append(out, toExpenseResponse(e))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list expenses response", "request_id", RequestID(r.Context()), "error", err)
	}
}
