package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/activity"
	"github.com/mananuf/justbarme/internal/inventory"
	"github.com/mananuf/justbarme/internal/tenancy"
	"github.com/mananuf/justbarme/internal/validator"
)

// mayOpenActivityEntry reports whether the caller may see one activity
// entry's full breakdown: an Owner may open anything, a Staff member only
// one they themselves caused. Mirrors listActivity's own actor_id
// override -- together these are what makes "Staff sees only their own
// entered activity" true both in the list and for a direct GET by id, so
// a staff member who already knows (or guesses) another entry's UUID still
// gets 404, same "indistinguishable from not found" discipline
// requireBusinessContext already uses for a business the caller isn't a
// member of.
func mayOpenActivityEntry(business tenancy.Business, principal tenancy.Principal, actorID uuid.UUID) bool {
	return business.Role == tenancy.RoleOwner || actorID == principal.UserID
}

// getSale implements GET /api/v1/sales/{sale_id} (activity:read) -- the
// activity feed's "tap to see what happened" breakdown for a sale.
func (api *API) getSale(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("getSale ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityActivityRead)
	if !ok {
		return
	}
	saleID, err := uuid.Parse(chi.URLParam(r, "sale_id"))
	if err != nil {
		api.badRequestResponse(w, r, "sale_id must be a valid UUID.")
		return
	}
	sale, err := api.sales.GetSale(r.Context(), principal.UserID, business.BusinessID, saleID)
	if err != nil {
		salesErrorResponse(api, w, r, err, "get sale")
		return
	}
	if !mayOpenActivityEntry(business, principal, sale.SellerID) {
		api.notFoundResponse(w, r)
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toSaleResponse(sale)}, nil); err != nil {
		api.logger.Error("write get sale response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// getExpense implements GET /api/v1/expenses/{expense_id} (activity:read).
func (api *API) getExpense(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("getExpense ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityActivityRead)
	if !ok {
		return
	}
	expenseID, err := uuid.Parse(chi.URLParam(r, "expense_id"))
	if err != nil {
		api.badRequestResponse(w, r, "expense_id must be a valid UUID.")
		return
	}
	exp, err := api.expenses.GetExpense(r.Context(), principal.UserID, business.BusinessID, expenseID)
	if err != nil {
		expensesErrorResponse(api, w, r, err, "get expense")
		return
	}
	if !mayOpenActivityEntry(business, principal, exp.RecordedBy) {
		api.notFoundResponse(w, r)
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toExpenseResponse(exp)}, nil); err != nil {
		api.logger.Error("write get expense response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// getAdjustmentRequest implements
// GET /api/v1/inventory-adjustments/{adjustment_id} (activity:read).
func (api *API) getAdjustmentRequest(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("getAdjustmentRequest ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityActivityRead)
	if !ok {
		return
	}
	requestID, err := uuid.Parse(chi.URLParam(r, "adjustment_id"))
	if err != nil {
		api.badRequestResponse(w, r, "adjustment_id must be a valid UUID.")
		return
	}
	req, err := api.inventory.GetAdjustmentRequest(r.Context(), principal.UserID, business.BusinessID, requestID)
	if err != nil {
		inventoryErrorResponse(api, w, r, err, "get adjustment request")
		return
	}
	if !mayOpenActivityEntry(business, principal, req.RequestedBy) {
		api.notFoundResponse(w, r)
		return
	}
	names, err := api.resolveActorNames(r.Context(), []uuid.UUID{req.RequestedBy})
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve actor names: %w", err))
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toAdjustmentRequestResponse(req, names[req.RequestedBy])}, nil); err != nil {
		api.logger.Error("write get adjustment request response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type stockReceiptLineResponse struct {
	VariantID     string `json:"variant_id"`
	VariantName   string `json:"variant_name"`
	ProductName   string `json:"product_name"`
	Quantity      int32  `json:"quantity"`
	TotalCostKobo int64  `json:"total_cost_kobo"`
}

type stockReceiptResponse struct {
	ID             string                     `json:"id"`
	ReceivedBy     string                     `json:"received_by"`
	ReceivedAt     string                     `json:"received_at"`
	ReversalOf     string                     `json:"reversal_of,omitempty"`
	ReversalOfThis string                     `json:"reversal_of_this,omitempty"`
	Lines          []stockReceiptLineResponse `json:"lines"`
}

func toStockReceiptResponse(d inventory.ReceiptDetail) stockReceiptResponse {
	out := stockReceiptResponse{
		ID: d.ID.String(), ReceivedBy: d.ReceivedBy.String(), ReceivedAt: d.ReceivedAt.UTC().Format(time.RFC3339),
	}
	if d.ReversalOf != uuid.Nil {
		out.ReversalOf = d.ReversalOf.String()
	}
	if d.ReversalOfThis != uuid.Nil {
		out.ReversalOfThis = d.ReversalOfThis.String()
	}
	for _, l := range d.Lines {
		out.Lines = append(out.Lines, stockReceiptLineResponse{
			VariantID: l.VariantID.String(), VariantName: l.VariantName, ProductName: l.ProductName,
			Quantity: l.Quantity, TotalCostKobo: l.TotalCostKobo,
		})
	}
	return out
}

// getStockReceipt implements GET /api/v1/stock-receipts/{receipt_id}
// (activity:read) -- the activity feed's breakdown for a "Stock received"
// entry: every line, with product/variant names, and whether this receipt
// has already been reversed.
func (api *API) getStockReceipt(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("getStockReceipt ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityActivityRead)
	if !ok {
		return
	}
	receiptID, err := uuid.Parse(chi.URLParam(r, "receipt_id"))
	if err != nil {
		api.badRequestResponse(w, r, "receipt_id must be a valid UUID.")
		return
	}
	receipt, err := api.inventory.GetStockReceipt(r.Context(), principal.UserID, business.BusinessID, receiptID)
	if err != nil {
		inventoryErrorResponse(api, w, r, err, "get stock receipt")
		return
	}
	if !mayOpenActivityEntry(business, principal, receipt.ReceivedBy) {
		api.notFoundResponse(w, r)
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toStockReceiptResponse(receipt)}, nil); err != nil {
		api.logger.Error("write get stock receipt response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// reverseStockReceipt implements
// POST /api/v1/stock-receipts/{receipt_id}/reverse
// (inventory:receipt_reverse, Owner only) -- this is "edit" for a stock
// receipt: the original is never changed, a new equal-and-opposite receipt
// is posted instead (same idiom as sale/expense reversal). Blocked with a
// clear reason if any of the receipt's stock has already been sold.
func (api *API) reverseStockReceipt(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("reverseStockReceipt ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityInventoryReceiptReverse)
	if !ok {
		return
	}
	receiptID, err := uuid.Parse(chi.URLParam(r, "receipt_id"))
	if err != nil {
		api.badRequestResponse(w, r, "receipt_id must be a valid UUID.")
		return
	}
	reversal, err := api.inventory.ReverseStockReceipt(r.Context(), principal.UserID, business.BusinessID, receiptID, principal.UserID)
	if err != nil {
		inventoryErrorResponse(api, w, r, err, "reverse stock receipt")
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toStockReceiptResponse(reversal)}, nil); err != nil {
		api.logger.Error("write reverse stock receipt response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type activityFlagResponse struct {
	ID             string `json:"id"`
	SourceType     string `json:"source_type"`
	SourceID       string `json:"source_id"`
	FlaggedBy      string `json:"flagged_by"`
	FlaggedByName  string `json:"flagged_by_name,omitempty"`
	Reason         string `json:"reason"`
	Status         string `json:"status"`
	ResolutionNote string `json:"resolution_note,omitempty"`
	CreatedAt      string `json:"created_at"`
	ResolvedAt     string `json:"resolved_at,omitempty"`
}

func toActivityFlagResponse(f activity.Flag, flaggedByName string) activityFlagResponse {
	out := activityFlagResponse{
		ID: f.ID.String(), SourceType: f.SourceType, SourceID: f.SourceID.String(),
		FlaggedBy: f.FlaggedBy.String(), FlaggedByName: flaggedByName, Reason: f.Reason,
		Status: f.Status, ResolutionNote: f.ResolutionNote, CreatedAt: f.CreatedAt.UTC().Format(time.RFC3339),
	}
	if !f.ResolvedAt.IsZero() {
		out.ResolvedAt = f.ResolvedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func activityFlagErrorResponse(api *API, w http.ResponseWriter, r *http.Request, err error, action string) {
	switch {
	case errors.Is(err, activity.ErrFlagNotFound):
		api.conflictResponse(w, r, err.Error())
	default:
		api.internalErrorResponse(w, r, fmt.Errorf("%s: %w", action, err))
	}
}

// actorOfActivityEntry resolves sourceType/sourceID back to the user who
// caused it, re-using each domain's own Get method -- this both confirms
// the entry actually exists and is what mayOpenActivityEntry checks
// against, so a flag can never be created pointing at an entry the caller
// could not otherwise open.
func (api *API) actorOfActivityEntry(ctx context.Context, userID, businessID uuid.UUID, sourceType string, sourceID uuid.UUID) (uuid.UUID, error) {
	switch sourceType {
	case activity.FlagSourceSale:
		s, err := api.sales.GetSale(ctx, userID, businessID, sourceID)
		return s.SellerID, err
	case activity.FlagSourceExpense:
		e, err := api.expenses.GetExpense(ctx, userID, businessID, sourceID)
		return e.RecordedBy, err
	case activity.FlagSourceInventoryAdjustment:
		a, err := api.inventory.GetAdjustmentRequest(ctx, userID, businessID, sourceID)
		return a.RequestedBy, err
	case activity.FlagSourceStockReceipt:
		rcpt, err := api.inventory.GetStockReceipt(ctx, userID, businessID, sourceID)
		return rcpt.ReceivedBy, err
	default:
		return uuid.Nil, errActivityFlagUnknownSourceType
	}
}

var errActivityFlagUnknownSourceType = errors.New("unknown activity source type")

// flagActivity implements POST /api/v1/activity-flags (activity:read, both
// roles) -- "staff must request review," generalized: anyone who can open
// an activity entry can flag it for the owner, since Staff's own read
// access is already scoped to entries they caused (see
// mayOpenActivityEntry). It never touches the record being flagged.
func (api *API) flagActivity(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("flagActivity ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityActivityRead)
	if !ok {
		return
	}

	var req struct {
		SourceType string `json:"source_type"`
		SourceID   string `json:"source_id"`
		Reason     string `json:"reason"`
	}
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)

	v := validator.New()
	v.Check(validator.PermittedValue(req.SourceType,
		activity.FlagSourceSale, activity.FlagSourceExpense,
		activity.FlagSourceInventoryAdjustment, activity.FlagSourceStockReceipt,
	), "source_type", "must be sale, expense, inventory_adjustment, or stock_receipt")
	sourceID, idErr := uuid.Parse(req.SourceID)
	v.Check(idErr == nil, "source_id", "must be a valid UUID")
	v.Check(req.Reason != "", "reason", "must be provided")
	v.Check(len(req.Reason) <= 500, "reason", "must be at most 500 characters")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	actorID, err := api.actorOfActivityEntry(r.Context(), principal.UserID, business.BusinessID, req.SourceType, sourceID)
	if err != nil {
		switch req.SourceType {
		case activity.FlagSourceSale:
			salesErrorResponse(api, w, r, err, "look up activity entry")
		case activity.FlagSourceExpense:
			expensesErrorResponse(api, w, r, err, "look up activity entry")
		default:
			inventoryErrorResponse(api, w, r, err, "look up activity entry")
		}
		return
	}
	if !mayOpenActivityEntry(business, principal, actorID) {
		api.notFoundResponse(w, r)
		return
	}

	flag, err := api.activity.CreateFlag(r.Context(), principal.UserID, business.BusinessID, req.SourceType, sourceID, req.Reason)
	if err != nil {
		activityFlagErrorResponse(api, w, r, err, "create activity flag")
		return
	}
	names, err := api.resolveActorNames(r.Context(), []uuid.UUID{principal.UserID})
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve actor names: %w", err))
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toActivityFlagResponse(flag, names[principal.UserID])}, nil); err != nil {
		api.logger.Error("write flag activity response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// listActivityFlags implements GET /api/v1/activity-flags (reviews:read,
// Owner only) -- surfaced in Reviews alongside the existing sale/inventory
// reviews.
func (api *API) listActivityFlags(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("listActivityFlags ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityReviewsRead)
	if !ok {
		return
	}
	flags, err := api.activity.ListOpenFlags(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list activity flags: %w", err))
		return
	}
	actorIDs := make([]uuid.UUID, len(flags))
	for i, f := range flags {
		actorIDs[i] = f.FlaggedBy
	}
	names, err := api.resolveActorNames(r.Context(), actorIDs)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve actor names: %w", err))
		return
	}
	out := make([]activityFlagResponse, 0, len(flags))
	for _, f := range flags {
		out = append(out, toActivityFlagResponse(f, names[f.FlaggedBy]))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list activity flags response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// resolveActivityFlag implements POST /api/v1/activity-flags/{flag_id}/resolve
// (reviews:resolve, Owner only). This only marks the flag resolved -- it
// never performs a fix itself; the owner uses the record's own correction
// mechanism (a reversal, or a new adjustment) separately, the same
// resolve-the-decision-not-the-record shape sale_reviews/inventory_reviews
// already use.
func (api *API) resolveActivityFlag(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("resolveActivityFlag ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityReviewsResolve)
	if !ok {
		return
	}
	flagID, err := uuid.Parse(chi.URLParam(r, "flag_id"))
	if err != nil {
		api.badRequestResponse(w, r, "flag_id must be a valid UUID.")
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Note = strings.TrimSpace(req.Note)

	flag, err := api.activity.ResolveFlag(r.Context(), principal.UserID, business.BusinessID, flagID, principal.UserID, req.Note)
	if err != nil {
		activityFlagErrorResponse(api, w, r, err, "resolve activity flag")
		return
	}
	names, err := api.resolveActorNames(r.Context(), []uuid.UUID{flag.FlaggedBy})
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve actor names: %w", err))
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toActivityFlagResponse(flag, names[flag.FlaggedBy])}, nil); err != nil {
		api.logger.Error("write resolve activity flag response", "request_id", RequestID(r.Context()), "error", err)
	}
}
