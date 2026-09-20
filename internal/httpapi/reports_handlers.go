package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/tenancy"
)

// parseReportRange reads and validates the start_at/end_at RFC 3339 query
// params every report endpoint requires -- a report always has an
// explicit range, unlike the activity feed's default "most recent N".
func parseReportRange(r *http.Request) (start, end time.Time, ok bool) {
	q := r.URL.Query()
	start, startErr := time.Parse(time.RFC3339, q.Get("start_at"))
	end, endErr := time.Parse(time.RFC3339, q.Get("end_at"))
	return start, end, startErr == nil && endErr == nil
}

type salesDayResponse struct {
	Day       string `json:"day"`
	TotalKobo int64  `json:"total_kobo"`
	SaleCount int64  `json:"sale_count"`
}

// reportSales implements GET /api/v1/reports/sales (reports:read) --
// daily-bucketed totals in the business's own timezone, backing the Sales
// report's heatmap and the table underneath it.
func (api *API) reportSales(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("reportSales ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityReportsRead)
	if !ok {
		return
	}
	start, end, valid := parseReportRange(r)
	if !valid {
		api.badRequestResponse(w, r, "start_at and end_at must both be RFC 3339 timestamps.")
		return
	}

	biz, err := api.identity.GetBusiness(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("get business: %w", err))
		return
	}

	days, err := api.reports.SalesByDay(r.Context(), principal.UserID, business.BusinessID, biz.Timezone, start, end)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("sales report: %w", err))
		return
	}
	out := make([]salesDayResponse, 0, len(days))
	for _, d := range days {
		out = append(out, salesDayResponse{Day: d.Day.Format("2006-01-02"), TotalKobo: d.TotalKobo, SaleCount: d.SaleCount})
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write sales report response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type productQuantityResponse struct {
	VariantID   string `json:"variant_id"`
	VariantName string `json:"variant_name"`
	ProductName string `json:"product_name"`
	UnitsSold   int64  `json:"units_sold"`
}

// reportProducts implements GET /api/v1/reports/products (reports:read).
func (api *API) reportProducts(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("reportProducts ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityReportsRead)
	if !ok {
		return
	}
	start, end, valid := parseReportRange(r)
	if !valid {
		api.badRequestResponse(w, r, "start_at and end_at must both be RFC 3339 timestamps.")
		return
	}

	rows, err := api.reports.ProductQuantities(r.Context(), principal.UserID, business.BusinessID, start, end)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("products report: %w", err))
		return
	}
	out := make([]productQuantityResponse, 0, len(rows))
	for _, p := range rows {
		out = append(out, productQuantityResponse{
			VariantID: p.VariantID.String(), VariantName: p.VariantName, ProductName: p.ProductName, UnitsSold: p.UnitsSold,
		})
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write products report response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type staffSalesResponse struct {
	SellerID   string `json:"seller_id"`
	SellerName string `json:"seller_name,omitempty"`
	TotalKobo  int64  `json:"total_kobo"`
	SaleCount  int64  `json:"sale_count"`
}

// reportStaffSales implements GET /api/v1/reports/staff-sales
// (reports:read). Seller names resolved via resolveSellerNames, same as
// every other seller-facing endpoint -- internal/reports itself never
// reaches into internal/identity.
func (api *API) reportStaffSales(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("reportStaffSales ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityReportsRead)
	if !ok {
		return
	}
	start, end, valid := parseReportRange(r)
	if !valid {
		api.badRequestResponse(w, r, "start_at and end_at must both be RFC 3339 timestamps.")
		return
	}

	rows, err := api.reports.StaffSales(r.Context(), principal.UserID, business.BusinessID, start, end)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("staff sales report: %w", err))
		return
	}
	sellerIDs := make([]uuid.UUID, len(rows))
	for i, s := range rows {
		sellerIDs[i] = s.SellerID
	}
	names, err := api.resolveSellerNames(r.Context(), sellerIDs)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve seller names: %w", err))
		return
	}
	out := make([]staffSalesResponse, 0, len(rows))
	for _, s := range rows {
		out = append(out, staffSalesResponse{
			SellerID: s.SellerID.String(), SellerName: names[s.SellerID], TotalKobo: s.TotalKobo, SaleCount: s.SaleCount,
		})
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write staff sales report response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type expensesByCategoryResponse struct {
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
	TotalKobo    int64  `json:"total_kobo"`
	ExpenseCount int64  `json:"expense_count"`
}

// reportExpenses implements GET /api/v1/reports/expenses (reports:read).
func (api *API) reportExpenses(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("reportExpenses ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityReportsRead)
	if !ok {
		return
	}
	start, end, valid := parseReportRange(r)
	if !valid {
		api.badRequestResponse(w, r, "start_at and end_at must both be RFC 3339 timestamps.")
		return
	}

	rows, err := api.reports.ExpensesByCategory(r.Context(), principal.UserID, business.BusinessID, start, end)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("expenses report: %w", err))
		return
	}
	out := make([]expensesByCategoryResponse, 0, len(rows))
	for _, e := range rows {
		out = append(out, expensesByCategoryResponse{
			CategoryID: e.CategoryID.String(), CategoryName: e.CategoryName, TotalKobo: e.TotalKobo, ExpenseCount: e.ExpenseCount,
		})
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write expenses report response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type stockDiscrepancyResponse struct {
	ID               string `json:"id"`
	VariantID        string `json:"variant_id"`
	VariantName      string `json:"variant_name"`
	ProductName      string `json:"product_name"`
	ExpectedQuantity int32  `json:"expected_quantity"`
	PhysicalQuantity int32  `json:"physical_quantity"`
	Variance         int32  `json:"variance"`
	IsStale          bool   `json:"is_stale"`
	CountedAt        string `json:"counted_at"`
	CountedByName    string `json:"counted_by_name,omitempty"`
}

// reportStock implements GET /api/v1/reports/stock (reports:read) --
// reuses Phase 7's own stock-count/variance data directly, no new schema.
func (api *API) reportStock(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("reportStock ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityReportsRead)
	if !ok {
		return
	}
	start, end, valid := parseReportRange(r)
	if !valid {
		api.badRequestResponse(w, r, "start_at and end_at must both be RFC 3339 timestamps.")
		return
	}

	rows, err := api.reports.StockDiscrepancies(r.Context(), principal.UserID, business.BusinessID, start, end)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("stock discrepancies report: %w", err))
		return
	}
	counterIDs := make([]uuid.UUID, len(rows))
	for i, d := range rows {
		counterIDs[i] = d.CountedBy
	}
	names, err := api.resolveSellerNames(r.Context(), counterIDs)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve counter names: %w", err))
		return
	}
	out := make([]stockDiscrepancyResponse, 0, len(rows))
	for _, d := range rows {
		out = append(out, stockDiscrepancyResponse{
			ID: d.ID.String(), VariantID: d.VariantID.String(), VariantName: d.VariantName, ProductName: d.ProductName,
			ExpectedQuantity: d.ExpectedQuantity, PhysicalQuantity: d.PhysicalQuantity, Variance: d.Variance, IsStale: d.IsStale,
			CountedAt: d.CountedAt.UTC().Format(time.RFC3339), CountedByName: names[d.CountedBy],
		})
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write stock discrepancies report response", "request_id", RequestID(r.Context()), "error", err)
	}
}
