package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/sales"
	"github.com/mananuf/justbarme/internal/tenancy"
	"github.com/mananuf/justbarme/internal/validator"
)

type saleItemRequest struct {
	VariantID     string `json:"variant_id"`
	Quantity      int32  `json:"quantity"`
	UnitPriceKobo int64  `json:"unit_price_kobo"`
}

type paymentRequest struct {
	AmountKobo int64  `json:"amount_kobo"`
	Method     string `json:"method"`
}

type createSaleRequest struct {
	IdempotencyKey string            `json:"idempotency_key"`
	OccurredAt     string            `json:"occurred_at"`
	Items          []saleItemRequest `json:"items"`
	Payment        paymentRequest    `json:"payment"`
}

type saleItemResponse struct {
	VariantID     string `json:"variant_id"`
	Description   string `json:"description"`
	Quantity      int32  `json:"quantity"`
	UnitPriceKobo int64  `json:"unit_price_kobo"`
	LineTotalKobo int64  `json:"line_total_kobo"`
}

type paymentResponse struct {
	AmountKobo int64  `json:"amount_kobo"`
	Method     string `json:"method"`
}

type reviewResponse struct {
	ID         string `json:"id"`
	SaleItemID string `json:"sale_item_id"`
	Reason     string `json:"reason"`
	Status     string `json:"status"`
}

type saleResponse struct {
	ID         string             `json:"id"`
	OccurredAt string             `json:"occurred_at"`
	ReceivedAt string             `json:"received_at"`
	TotalKobo  int64              `json:"total_kobo"`
	ReversalOf *string            `json:"reversal_of,omitempty"`
	Items      []saleItemResponse `json:"items,omitempty"`
	Payment    paymentResponse    `json:"payment"`
	Reviews    []reviewResponse   `json:"reviews,omitempty"`
}

func toSaleResponse(s sales.Sale) saleResponse {
	out := saleResponse{
		ID: s.ID.String(), OccurredAt: s.OccurredAt.UTC().Format(time.RFC3339),
		ReceivedAt: s.ReceivedAt.UTC().Format(time.RFC3339), TotalKobo: s.TotalKobo,
		ReversalOf: uuidOrNil(s.ReversalOf),
		Payment:    paymentResponse{AmountKobo: s.Payment.AmountKobo, Method: s.Payment.Method},
	}
	for _, i := range s.Items {
		out.Items = append(out.Items, saleItemResponse{
			VariantID: i.VariantID.String(), Description: i.Description, Quantity: i.Quantity,
			UnitPriceKobo: i.UnitPriceKobo, LineTotalKobo: i.LineTotalKobo,
		})
	}
	for _, r := range s.Reviews {
		out.Reviews = append(out.Reviews, reviewResponse{
			ID: r.ID.String(), SaleItemID: r.SaleItemID.String(), Reason: r.Reason, Status: r.Status,
		})
	}
	return out
}

func salesErrorResponse(api *API, w http.ResponseWriter, r *http.Request, err error, action string) {
	switch {
	case errors.Is(err, sales.ErrVariantNotFound), errors.Is(err, sales.ErrSaleNotFound), errors.Is(err, sales.ErrReviewNotFound):
		api.notFoundResponse(w, r)
	case errors.Is(err, sales.ErrAlreadyReversed):
		api.conflictResponse(w, r, err.Error())
	default:
		api.internalErrorResponse(w, r, fmt.Errorf("%s: %w", action, err))
	}
}

// createSale implements POST /api/v1/sales: any active member may sell
// (sales:record) -- see docs/PHASE_STOCK_RECEIVING.md's sibling doc,
// docs/ARCHITECTURE.md §8.3. occurred_at is the client's own claimed time
// (this may be an offline sale synced later); the server's own receipt
// time is recorded separately and never confused with it.
func (api *API) createSale(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("createSale ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilitySalesRecord)
	if !ok {
		return
	}

	var req createSaleRequest
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
	v.Check(validator.PermittedValue(req.Payment.Method, "cash", "transfer", "card"), "payment.method", "must be cash, transfer, or card")
	v.Check(req.Payment.AmountKobo > 0, "payment.amount_kobo", "must be positive")

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

	location, err := api.identity.GetDefaultLocation(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve default location: %w", err))
		return
	}

	sale, err := api.sales.CreateSale(
		r.Context(), principal.UserID, business.BusinessID, location.ID, principal.UserID,
		idempotencyKey, occurredAt, items,
		sales.PaymentInput{AmountKobo: req.Payment.AmountKobo, Method: req.Payment.Method},
	)
	if err != nil {
		salesErrorResponse(api, w, r, err, "create sale")
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toSaleResponse(sale)}, nil); err != nil {
		api.logger.Error("write create sale response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// reverseSale implements POST /api/v1/sales/{sale_id}/reverse: owner-only
// (sales:reverse).
func (api *API) reverseSale(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("reverseSale ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilitySalesReverse)
	if !ok {
		return
	}

	saleID, err := uuid.Parse(chi.URLParam(r, "sale_id"))
	if err != nil {
		api.badRequestResponse(w, r, "sale_id must be a valid UUID.")
		return
	}

	reversal, err := api.sales.ReverseSale(r.Context(), principal.UserID, business.BusinessID, saleID, principal.UserID)
	if err != nil {
		salesErrorResponse(api, w, r, err, "reverse sale")
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toSaleResponse(reversal)}, nil); err != nil {
		api.logger.Error("write reverse sale response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// listSales implements GET /api/v1/sales: recent activity, newest first.
// Owner-only (activity:read) for now -- whether Staff should see this feed
// of their own sales is an open pilot question, same shape as the
// inventory:receive Staff-scoping decision already flagged elsewhere in
// this codebase.
func (api *API) listSales(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("listSales ran without requireAuth"))
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

	list, err := api.sales.ListSales(r.Context(), principal.UserID, business.BusinessID, limit, 0)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list sales: %w", err))
		return
	}
	out := make([]saleResponse, 0, len(list))
	for _, s := range list {
		out = append(out, toSaleResponse(s))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list sales response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type salesSummaryResponse struct {
	TodayTotalKobo int64 `json:"today_total_kobo"`
	TodaySaleCount int64 `json:"today_sale_count"`
}

// salesSummary implements GET /api/v1/sales/summary -- "today" is
// computed in the business's own timezone (docs/ARCHITECTURE.md's
// businesses.timezone), not the server's, so a sale just before local
// midnight is never misattributed to the wrong day. Owner-only
// (reports:read).
func (api *API) salesSummary(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("salesSummary ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityReportsRead)
	if !ok {
		return
	}

	biz, err := api.identity.GetBusiness(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("get business: %w", err))
		return
	}
	loc, err := time.LoadLocation(biz.Timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	total, count, err := api.sales.SumSalesTotalSince(r.Context(), principal.UserID, business.BusinessID, startOfDay)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("sum sales total: %w", err))
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": salesSummaryResponse{TodayTotalKobo: total, TodaySaleCount: count}}, nil); err != nil {
		api.logger.Error("write sales summary response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// listSaleReviews implements GET /api/v1/sale-reviews. Owner-only
// (activity:read) -- same tier as listSales.
func (api *API) listSaleReviews(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("listSaleReviews ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityActivityRead)
	if !ok {
		return
	}

	list, err := api.sales.ListOpenReviews(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list sale reviews: %w", err))
		return
	}
	out := make([]reviewResponse, 0, len(list))
	for _, rv := range list {
		out = append(out, reviewResponse{ID: rv.ID.String(), SaleItemID: rv.SaleItemID.String(), Reason: rv.Reason, Status: rv.Status})
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list sale reviews response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type resolveSaleReviewRequest struct {
	Note string `json:"note"`
}

// resolveSaleReview implements POST /api/v1/sale-reviews/{review_id}/resolve
// -- owner-only (sales:reverse, the same tier as reversing a sale: an
// owner-level judgment call). A note is required, matching this
// codebase's consistent pattern for audit-trail actions (platform admin's
// suspend, Phase 7's adjustment reasons).
func (api *API) resolveSaleReview(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("resolveSaleReview ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilitySalesReverse)
	if !ok {
		return
	}

	reviewID, err := uuid.Parse(chi.URLParam(r, "review_id"))
	if err != nil {
		api.badRequestResponse(w, r, "review_id must be a valid UUID.")
		return
	}

	var req resolveSaleReviewRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}

	v := validator.New()
	v.Check(req.Note != "", "note", "must be provided")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	review, err := api.sales.ResolveReview(r.Context(), principal.UserID, business.BusinessID, reviewID, principal.UserID, req.Note)
	if err != nil {
		salesErrorResponse(api, w, r, err, "resolve sale review")
		return
	}
	out := reviewResponse{ID: review.ID.String(), SaleItemID: review.SaleItemID.String(), Reason: review.Reason, Status: review.Status}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write resolve sale review response", "request_id", RequestID(r.Context()), "error", err)
	}
}
