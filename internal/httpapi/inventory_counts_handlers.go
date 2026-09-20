package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/inventory"
	"github.com/mananuf/justbarme/internal/tenancy"
	"github.com/mananuf/justbarme/internal/validator"
)

// inventoryErrorResponse centralizes the inventory.Err* -> HTTP status
// mapping for every count/adjustment/review handler in this file, the
// same pattern tabsErrorResponse/catalogueErrorResponse already use.
func inventoryErrorResponse(api *API, w http.ResponseWriter, r *http.Request, err error, action string) {
	switch {
	case errors.Is(err, inventory.ErrVariantNotFound):
		api.notFoundResponse(w, r)
	case errors.Is(err, inventory.ErrAdjustmentRequestNotFound), errors.Is(err, inventory.ErrInventoryReviewNotFound):
		api.conflictResponse(w, r, err.Error())
	default:
		api.internalErrorResponse(w, r, fmt.Errorf("%s: %w", action, err))
	}
}

// resolveActorNames looks up each distinct actor's display name -- same
// dedupe-then-lookup shape as resolveSellerNames (tabs_handlers.go), kept
// as its own small function since callers here pass a mix of requester/
// actor/decider IDs from several different tables, not one sale-shaped
// list. Both ultimately call identity.Service.GetUserByID the same way;
// not worth merging into one signature that would need a mapping function
// at every call site anyway.
func (api *API) resolveActorNames(ctx context.Context, actorIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	return api.resolveSellerNames(ctx, actorIDs)
}

type stockCountLineRequest struct {
	VariantID        string `json:"variant_id"`
	ExpectedQuantity int32  `json:"expected_quantity"`
	PhysicalQuantity int32  `json:"physical_quantity"`
}

type submitStockCountRequest struct {
	IdempotencyKey string                  `json:"idempotency_key"`
	StartedAt      string                  `json:"started_at"`
	Lines          []stockCountLineRequest `json:"lines"`
}

type stockCountLineResponse struct {
	ID               string `json:"id"`
	VariantID        string `json:"variant_id"`
	ExpectedQuantity int32  `json:"expected_quantity"`
	PhysicalQuantity int32  `json:"physical_quantity"`
	Variance         int32  `json:"variance"`
	IsStale          bool   `json:"is_stale"`
}

type stockCountResponse struct {
	ID    string                   `json:"id"`
	Lines []stockCountLineResponse `json:"lines"`
}

func toStockCountResponse(c inventory.StockCount) stockCountResponse {
	out := stockCountResponse{ID: c.ID.String()}
	for _, l := range c.Lines {
		out.Lines = append(out.Lines, stockCountLineResponse{
			ID: l.ID.String(), VariantID: l.VariantID.String(), ExpectedQuantity: l.ExpectedQuantity,
			PhysicalQuantity: l.PhysicalQuantity, Variance: l.Variance, IsStale: l.IsStale,
		})
	}
	return out
}

// submitStockCount implements POST /api/v1/stock-counts (inventory:count,
// offline-safe -- see docs/PHASE_INVENTORY_COUNTS_AND_ADJUSTMENTS.md §4).
// idempotency_key makes a queued-offline submission safe to retry.
func (api *API) submitStockCount(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("submitStockCount ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityInventoryCount)
	if !ok {
		return
	}

	var req submitStockCountRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}

	v := validator.New()
	idempotencyKey, idErr := uuid.Parse(req.IdempotencyKey)
	v.Check(idErr == nil, "idempotency_key", "must be a valid UUID")
	startedAt, timeErr := time.Parse(time.RFC3339, req.StartedAt)
	v.Check(timeErr == nil, "started_at", "must be an RFC 3339 timestamp")
	v.Check(len(req.Lines) > 0, "lines", "must include at least one line")

	lines := make([]inventory.StockCountLineInput, 0, len(req.Lines))
	for i, raw := range req.Lines {
		field := fmt.Sprintf("lines[%d]", i)
		variantID, err := uuid.Parse(raw.VariantID)
		v.Check(err == nil, field+".variant_id", "must be a valid UUID")
		v.Check(raw.ExpectedQuantity >= 0, field+".expected_quantity", "must not be negative")
		v.Check(raw.PhysicalQuantity >= 0, field+".physical_quantity", "must not be negative")
		if err == nil {
			lines = append(lines, inventory.StockCountLineInput{
				VariantID: variantID, ExpectedQuantity: raw.ExpectedQuantity, PhysicalQuantity: raw.PhysicalQuantity,
			})
		}
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

	count, err := api.inventory.SubmitStockCount(r.Context(), principal.UserID, business.BusinessID,
		location.ID, principal.UserID, idempotencyKey, startedAt, lines)
	if err != nil {
		inventoryErrorResponse(api, w, r, err, "submit stock count")
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toStockCountResponse(count)}, nil); err != nil {
		api.logger.Error("write submit stock count response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// permittedAdjustmentReasons excludes count_correction deliberately --
// that category only ever comes from SubmitStockCount itself, never a
// direct client request.
var permittedAdjustmentReasons = []string{
	inventory.AdjustmentReasonComplimentary, inventory.AdjustmentReasonBroken,
	inventory.AdjustmentReasonSpoiled, inventory.AdjustmentReasonStaffUse, inventory.AdjustmentReasonManual,
}

type requestAdjustmentRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	VariantID      string `json:"variant_id"`
	QuantityDelta  int32  `json:"quantity_delta"`
	ReasonCategory string `json:"reason_category"`
	ReasonNote     string `json:"reason_note"`
}

type adjustmentRequestResponse struct {
	ID              string `json:"id"`
	VariantID       string `json:"variant_id"`
	VariantName     string `json:"variant_name,omitempty"`
	ProductName     string `json:"product_name,omitempty"`
	QuantityDelta   int32  `json:"quantity_delta"`
	ReasonCategory  string `json:"reason_category"`
	ReasonNote      string `json:"reason_note"`
	Status          string `json:"status"`
	RequestedByName string `json:"requested_by_name,omitempty"`
	CreatedAt       string `json:"created_at"`
}

func toAdjustmentRequestResponse(a inventory.AdjustmentRequest, requestedByName string) adjustmentRequestResponse {
	return adjustmentRequestResponse{
		ID: a.ID.String(), VariantID: a.VariantID.String(), VariantName: a.VariantName, ProductName: a.ProductName,
		QuantityDelta: a.QuantityDelta, ReasonCategory: a.ReasonCategory, ReasonNote: a.ReasonNote,
		Status: a.Status, RequestedByName: requestedByName, CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// requestAdjustment implements POST /api/v1/inventory-adjustments
// (inventory:adjustment_request, offline-safe -- Staff may submit, only an
// Owner may later approve). Complimentary/broken/spoiled/staff-use
// consumption all go through this one endpoint -- reason_category is what
// keeps "what happened" unambiguous, not a separate mechanism (docs/
// PHASE_INVENTORY_COUNTS_AND_ADJUSTMENTS.md §7).
func (api *API) requestAdjustment(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("requestAdjustment ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityInventoryAdjustmentRequest)
	if !ok {
		return
	}

	var req requestAdjustmentRequest
	if err := readJSON(w, r, &req, 1<<12); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}

	v := validator.New()
	idempotencyKey, idErr := uuid.Parse(req.IdempotencyKey)
	v.Check(idErr == nil, "idempotency_key", "must be a valid UUID")
	variantID, variantErr := uuid.Parse(req.VariantID)
	v.Check(variantErr == nil, "variant_id", "must be a valid UUID")
	v.Check(req.QuantityDelta != 0, "quantity_delta", "must not be zero")
	v.Check(validator.PermittedValue(req.ReasonCategory, permittedAdjustmentReasons...), "reason_category",
		"must be one of complimentary, broken, spoiled, staff_use, manual")
	v.Check(req.ReasonNote != "", "reason_note", "must be provided")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	location, err := api.identity.GetDefaultLocation(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve default location: %w", err))
		return
	}

	created, err := api.inventory.RequestAdjustment(r.Context(), principal.UserID, business.BusinessID,
		location.ID, variantID, principal.UserID, idempotencyKey, req.QuantityDelta, req.ReasonCategory, req.ReasonNote)
	if err != nil {
		inventoryErrorResponse(api, w, r, err, "request adjustment")
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toAdjustmentRequestResponse(created, "")}, nil); err != nil {
		api.logger.Error("write request adjustment response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// listPendingAdjustments implements GET /api/v1/inventory-adjustments
// (inventory:adjustment_approve, Owner-only -- this is the approval queue,
// not a general activity feed).
func (api *API) listPendingAdjustments(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("listPendingAdjustments ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityInventoryAdjustmentApprove)
	if !ok {
		return
	}

	list, err := api.inventory.ListPendingAdjustmentRequests(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list pending adjustments: %w", err))
		return
	}
	requesterIDs := make([]uuid.UUID, len(list))
	for i, a := range list {
		requesterIDs[i] = a.RequestedBy
	}
	names, err := api.resolveActorNames(r.Context(), requesterIDs)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve requester names: %w", err))
		return
	}
	out := make([]adjustmentRequestResponse, 0, len(list))
	for _, a := range list {
		out = append(out, toAdjustmentRequestResponse(a, names[a.RequestedBy]))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list pending adjustments response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type decideAdjustmentRequest struct {
	Note string `json:"note"`
}

// approveAdjustment implements POST
// /api/v1/inventory-adjustments/{adjustment_id}/approve
// (inventory:adjustment_approve, Owner-only, deliberately not offline-safe
// -- approval always requires online server authorization).
func (api *API) approveAdjustment(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("approveAdjustment ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityInventoryAdjustmentApprove)
	if !ok {
		return
	}

	requestID, err := uuid.Parse(chi.URLParam(r, "adjustment_id"))
	if err != nil {
		api.badRequestResponse(w, r, "adjustment_id must be a valid UUID.")
		return
	}
	var req decideAdjustmentRequest
	if err := readJSON(w, r, &req, 1<<12); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	v := validator.New()
	v.Check(req.Note != "", "note", "must be provided")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	updated, err := api.inventory.ApproveAdjustmentRequest(r.Context(), principal.UserID, business.BusinessID, requestID, principal.UserID, req.Note)
	if err != nil {
		inventoryErrorResponse(api, w, r, err, "approve adjustment")
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toAdjustmentRequestResponse(updated, "")}, nil); err != nil {
		api.logger.Error("write approve adjustment response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// rejectAdjustment implements POST
// /api/v1/inventory-adjustments/{adjustment_id}/reject
// (inventory:adjustment_approve, Owner-only). No movement ever posts.
func (api *API) rejectAdjustment(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("rejectAdjustment ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityInventoryAdjustmentApprove)
	if !ok {
		return
	}

	requestID, err := uuid.Parse(chi.URLParam(r, "adjustment_id"))
	if err != nil {
		api.badRequestResponse(w, r, "adjustment_id must be a valid UUID.")
		return
	}
	var req decideAdjustmentRequest
	if err := readJSON(w, r, &req, 1<<12); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	v := validator.New()
	v.Check(req.Note != "", "note", "must be provided")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	updated, err := api.inventory.RejectAdjustmentRequest(r.Context(), principal.UserID, business.BusinessID, requestID, principal.UserID, req.Note)
	if err != nil {
		inventoryErrorResponse(api, w, r, err, "reject adjustment")
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toAdjustmentRequestResponse(updated, "")}, nil); err != nil {
		api.logger.Error("write reject adjustment response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type inventoryReviewResponse struct {
	ID                    string `json:"id"`
	Type                  string `json:"type"`
	VariantID             string `json:"variant_id"`
	VariantName           string `json:"variant_name,omitempty"`
	ProductName           string `json:"product_name,omitempty"`
	Status                string `json:"status"`
	CreatedAt             string `json:"created_at"`
	CountExpectedQuantity int32  `json:"count_expected_quantity,omitempty"`
	CountPhysicalQuantity int32  `json:"count_physical_quantity,omitempty"`
}

func toInventoryReviewResponse(r inventory.InventoryReview) inventoryReviewResponse {
	return inventoryReviewResponse{
		ID: r.ID.String(), Type: r.Type, VariantID: r.VariantID.String(),
		VariantName: r.VariantName, ProductName: r.ProductName,
		Status: r.Status, CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339),
		CountExpectedQuantity: r.CountExpectedQuantity, CountPhysicalQuantity: r.CountPhysicalQuantity,
	}
}

// listInventoryReviews implements GET /api/v1/inventory-reviews
// (reviews:read) -- negative-inventory and stale-count reviews, the
// inventory-side counterpart of GET /sale-reviews.
func (api *API) listInventoryReviews(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilityReviewsRead)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	list, err := api.inventory.ListOpenReviews(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list inventory reviews: %w", err))
		return
	}
	out := make([]inventoryReviewResponse, 0, len(list))
	for _, rv := range list {
		out = append(out, toInventoryReviewResponse(rv))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list inventory reviews response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type resolveInventoryReviewRequest struct {
	Note string `json:"note"`
}

// resolveInventoryReview implements POST
// /api/v1/inventory-reviews/{review_id}/resolve (reviews:resolve).
func (api *API) resolveInventoryReview(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilityReviewsResolve)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	reviewID, err := uuid.Parse(chi.URLParam(r, "review_id"))
	if err != nil {
		api.badRequestResponse(w, r, "review_id must be a valid UUID.")
		return
	}
	var req resolveInventoryReviewRequest
	if err := readJSON(w, r, &req, 1<<12); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	v := validator.New()
	v.Check(req.Note != "", "note", "must be provided")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	updated, err := api.inventory.ResolveReview(r.Context(), principal.UserID, business.BusinessID, reviewID, principal.UserID, req.Note)
	if err != nil {
		inventoryErrorResponse(api, w, r, err, "resolve inventory review")
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toInventoryReviewResponse(updated)}, nil); err != nil {
		api.logger.Error("write resolve inventory review response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type historyEntryResponse struct {
	ID                       string `json:"id"`
	QuantityDelta            int32  `json:"quantity_delta"`
	CreatedAt                string `json:"created_at"`
	EventType                string `json:"event_type"`
	ActorName                string `json:"actor_name,omitempty"`
	AdjustmentReasonCategory string `json:"adjustment_reason_category,omitempty"`
	AdjustmentReasonNote     string `json:"adjustment_reason_note,omitempty"`
	AdjustmentDecidedByName  string `json:"adjustment_decided_by_name,omitempty"`
}

// getInventoryHistory implements
// GET /api/v1/products/variants/{variant_id}/history (inventory:read) --
// stock history: a flat, chronological, human-readable feed of every
// movement for one variant (docs/PHASE_INVENTORY_COUNTS_AND_ADJUSTMENTS.md
// §3).
func (api *API) getInventoryHistory(w http.ResponseWriter, r *http.Request) {
	business, ok := api.requireCapability(w, r, tenancy.CapabilityInventoryRead)
	if !ok {
		return
	}
	principal, _ := tenancy.PrincipalFromContext(r.Context())

	variantID, err := uuid.Parse(chi.URLParam(r, "variant_id"))
	if err != nil {
		api.badRequestResponse(w, r, "variant_id must be a valid UUID.")
		return
	}
	limit := int32(50)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 32); err == nil && parsed > 0 && parsed <= 200 {
			limit = int32(parsed)
		}
	}

	list, err := api.inventory.GetHistory(r.Context(), principal.UserID, business.BusinessID, variantID, limit)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("get inventory history: %w", err))
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
		api.logger.Error("write inventory history response", "request_id", RequestID(r.Context()), "error", err)
	}
}
