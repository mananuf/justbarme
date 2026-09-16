package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/catalogue"
	"github.com/mananuf/justbarme/internal/tenancy"
	"github.com/mananuf/justbarme/internal/validator"
)

type categoryResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SortOrder int32  `json:"sort_order"`
	Active    bool   `json:"active"`
}

type priceResponse struct {
	ID         string  `json:"id"`
	AmountKobo int64   `json:"amount_kobo"`
	ValidFrom  string  `json:"valid_from"`
	ValidTo    *string `json:"valid_to"`
}

type variantResponse struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Active       bool          `json:"active"`
	CurrentPrice priceResponse `json:"current_price"`
	// CurrentStock comes from internal/inventory, merged in here rather
	// than internal/catalogue knowing about inventory at all -- see
	// docs/PHASE_STOCK_RECEIVING.md. A variant with no stock receipts yet
	// is simply absent from the balances map, and reads as 0 here.
	CurrentStock int64 `json:"current_stock"`
}

type productResponse struct {
	ID         string            `json:"id"`
	CategoryID *string           `json:"category_id"`
	Name       string            `json:"name"`
	Active     bool              `json:"active"`
	Variants   []variantResponse `json:"variants,omitempty"`
}

func toCategoryResponse(c catalogue.Category) categoryResponse {
	return categoryResponse{ID: c.ID.String(), Name: c.Name, SortOrder: c.SortOrder, Active: c.Active}
}

func toPriceResponse(p catalogue.Price) priceResponse {
	out := priceResponse{ID: p.ID.String(), AmountKobo: p.AmountKobo, ValidFrom: p.ValidFrom.UTC().Format(time.RFC3339)}
	if p.ValidTo != nil {
		validTo := p.ValidTo.UTC().Format(time.RFC3339)
		out.ValidTo = &validTo
	}
	return out
}

func toVariantResponse(v catalogue.VariantWithPrice, balances map[uuid.UUID]int64) variantResponse {
	return variantResponse{
		ID: v.ID.String(), Name: v.Name, Active: v.Active, CurrentPrice: toPriceResponse(v.CurrentPrice),
		CurrentStock: balances[v.ID],
	}
}

func toProductResponse(p catalogue.ProductWithVariants, balances map[uuid.UUID]int64) productResponse {
	out := productResponse{ID: p.ID.String(), Name: p.Name, Active: p.Active, CategoryID: uuidOrNil(p.CategoryID)}
	for _, v := range p.Variants {
		out.Variants = append(out.Variants, toVariantResponse(v, balances))
	}
	return out
}

// uuidOrNil renders id as a pointer to its string form, or nil for
// uuid.Nil -- catalogue.Product.CategoryID uses uuid.Nil to mean
// "uncategorized" rather than a separate bool, matching how the service
// layer itself represents it.
func uuidOrNil(id uuid.UUID) *string {
	if id == uuid.Nil {
		return nil
	}
	s := id.String()
	return &s
}

// parseOptionalUUID parses raw as a UUID, treating nil or an empty string
// as uuid.Nil (catalogue's "no category" value) rather than an error.
func parseOptionalUUID(raw *string) (uuid.UUID, error) {
	if raw == nil || *raw == "" {
		return uuid.Nil, nil
	}
	return uuid.Parse(*raw)
}

func catalogueErrorResponse(api *API, w http.ResponseWriter, r *http.Request, err error, action string) {
	switch {
	case errors.Is(err, catalogue.ErrCategoryNotFound),
		errors.Is(err, catalogue.ErrProductNotFound),
		errors.Is(err, catalogue.ErrVariantNotFound),
		errors.Is(err, catalogue.ErrNoCurrentPrice):
		api.notFoundResponse(w, r)
	case errors.Is(err, catalogue.ErrCategoryNameTaken),
		errors.Is(err, catalogue.ErrProductNameTaken),
		errors.Is(err, catalogue.ErrVariantNameTaken):
		api.conflictResponse(w, r, err.Error())
	default:
		api.internalErrorResponse(w, r, fmt.Errorf("%s: %w", action, err))
	}
}

// --- categories ---

type createCategoryRequest struct {
	Name      string `json:"name"`
	SortOrder int32  `json:"sort_order"`
}

// listCategories implements GET /api/v1/categories. Staff may read the
// catalogue (catalogue:read); only an owner may change it.
func (api *API) listCategories(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("listCategories ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityCatalogueRead)
	if !ok {
		return
	}

	categories, err := api.catalogue.ListCategories(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list categories: %w", err))
		return
	}
	out := make([]categoryResponse, 0, len(categories))
	for _, c := range categories {
		out = append(out, toCategoryResponse(c))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list categories response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// createCategory implements POST /api/v1/categories. Owner-only
// (catalogue:manage).
func (api *API) createCategory(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("createCategory ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityCatalogueManage)
	if !ok {
		return
	}

	var req createCategoryRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)

	v := validator.New()
	v.Check(req.Name != "", "name", "must be provided")
	v.Check(len(req.Name) <= 200, "name", "must be at most 200 characters")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	category, err := api.catalogue.CreateCategory(r.Context(), principal.UserID, business.BusinessID, req.Name, req.SortOrder)
	if err != nil {
		catalogueErrorResponse(api, w, r, err, "create category")
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toCategoryResponse(category)}, nil); err != nil {
		api.logger.Error("write create category response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type updateCategoryRequest struct {
	Name      string `json:"name"`
	SortOrder int32  `json:"sort_order"`
	Active    bool   `json:"active"`
}

// updateCategory implements PATCH /api/v1/categories/{category_id}. A full
// replacement of the mutable fields (matching catalogue.UpdateCategoryParams):
// the caller supplies the category's current values for any field it is
// not changing. Owner-only (catalogue:manage). Deactivating a category is
// the only removal path — there is no delete endpoint.
func (api *API) updateCategory(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("updateCategory ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityCatalogueManage)
	if !ok {
		return
	}

	categoryID, err := uuid.Parse(chi.URLParam(r, "category_id"))
	if err != nil {
		api.badRequestResponse(w, r, "category_id must be a valid UUID.")
		return
	}

	var req updateCategoryRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)

	v := validator.New()
	v.Check(req.Name != "", "name", "must be provided")
	v.Check(len(req.Name) <= 200, "name", "must be at most 200 characters")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	category, err := api.catalogue.UpdateCategory(r.Context(), principal.UserID, business.BusinessID, categoryID, catalogue.UpdateCategoryParams{
		Name: req.Name, SortOrder: req.SortOrder, Active: req.Active,
	})
	if err != nil {
		catalogueErrorResponse(api, w, r, err, "update category")
		return
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": toCategoryResponse(category)}, nil); err != nil {
		api.logger.Error("write update category response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// --- products ---

// listProducts implements GET /api/v1/products, returning every product
// with its variants and each variant's current price — the shape a
// catalogue management screen or a device's offline replication read
// needs in one call. Staff may read (catalogue:read).
func (api *API) listProducts(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("listProducts ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityCatalogueRead)
	if !ok {
		return
	}

	products, err := api.catalogue.ListCatalogue(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list products: %w", err))
		return
	}
	balances, err := api.inventory.GetBalances(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list inventory balances: %w", err))
		return
	}
	out := make([]productResponse, 0, len(products))
	for _, p := range products {
		out = append(out, toProductResponse(p, balances))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list products response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type createProductRequest struct {
	Name       string  `json:"name"`
	CategoryID *string `json:"category_id"`
}

// createProduct implements POST /api/v1/products. Owner-only
// (catalogue:manage).
func (api *API) createProduct(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("createProduct ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityCatalogueManage)
	if !ok {
		return
	}

	var req createProductRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	categoryID, parseErr := parseOptionalUUID(req.CategoryID)

	v := validator.New()
	v.Check(req.Name != "", "name", "must be provided")
	v.Check(len(req.Name) <= 200, "name", "must be at most 200 characters")
	v.Check(parseErr == nil, "category_id", "must be a valid UUID")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	product, err := api.catalogue.CreateProduct(r.Context(), principal.UserID, business.BusinessID, categoryID, req.Name)
	if err != nil {
		catalogueErrorResponse(api, w, r, err, "create product")
		return
	}
	payload := toProductResponse(catalogue.ProductWithVariants{Product: product}, nil)
	if err := writeJSON(w, http.StatusCreated, envelope{"data": payload}, nil); err != nil {
		api.logger.Error("write create product response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// getProduct implements GET /api/v1/products/{product_id}. Staff may read
// (catalogue:read).
func (api *API) getProduct(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("getProduct ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityCatalogueRead)
	if !ok {
		return
	}

	productID, err := uuid.Parse(chi.URLParam(r, "product_id"))
	if err != nil {
		api.badRequestResponse(w, r, "product_id must be a valid UUID.")
		return
	}

	product, err := api.catalogue.GetProduct(r.Context(), principal.UserID, business.BusinessID, productID)
	if err != nil {
		catalogueErrorResponse(api, w, r, err, "get product")
		return
	}
	payload := toProductResponse(catalogue.ProductWithVariants{Product: product}, nil)
	if err := writeJSON(w, http.StatusOK, envelope{"data": payload}, nil); err != nil {
		api.logger.Error("write get product response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type updateProductRequest struct {
	Name       string  `json:"name"`
	CategoryID *string `json:"category_id"`
	Active     bool    `json:"active"`
}

// updateProduct implements PATCH /api/v1/products/{product_id}. A full
// replacement of the mutable fields, matching updateCategory's convention.
// Owner-only (catalogue:manage). Deactivating a product is the only
// removal path — there is no delete endpoint.
func (api *API) updateProduct(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("updateProduct ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityCatalogueManage)
	if !ok {
		return
	}

	productID, err := uuid.Parse(chi.URLParam(r, "product_id"))
	if err != nil {
		api.badRequestResponse(w, r, "product_id must be a valid UUID.")
		return
	}

	var req updateProductRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	categoryID, parseErr := parseOptionalUUID(req.CategoryID)

	v := validator.New()
	v.Check(req.Name != "", "name", "must be provided")
	v.Check(len(req.Name) <= 200, "name", "must be at most 200 characters")
	v.Check(parseErr == nil, "category_id", "must be a valid UUID")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	product, err := api.catalogue.UpdateProduct(r.Context(), principal.UserID, business.BusinessID, productID, catalogue.UpdateProductParams{
		Name: req.Name, CategoryID: categoryID, Active: req.Active,
	})
	if err != nil {
		catalogueErrorResponse(api, w, r, err, "update product")
		return
	}
	payload := toProductResponse(catalogue.ProductWithVariants{Product: product}, nil)
	if err := writeJSON(w, http.StatusOK, envelope{"data": payload}, nil); err != nil {
		api.logger.Error("write update product response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// --- variants ---

type createVariantRequest struct {
	Name             string `json:"name"`
	InitialPriceKobo int64  `json:"initial_price_kobo"`
}

// createVariant implements POST /api/v1/products/{product_id}/variants,
// creating the variant and its first price atomically — a variant is never
// without a current price (docs/IMPLEMENTATION_PLAN.md Phase 3 acceptance
// criterion "Exactly one current price exists per variant"). Owner-only
// (catalogue:manage).
func (api *API) createVariant(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("createVariant ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityCatalogueManage)
	if !ok {
		return
	}

	productID, err := uuid.Parse(chi.URLParam(r, "product_id"))
	if err != nil {
		api.badRequestResponse(w, r, "product_id must be a valid UUID.")
		return
	}

	var req createVariantRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)

	v := validator.New()
	v.Check(req.Name != "", "name", "must be provided")
	v.Check(len(req.Name) <= 200, "name", "must be at most 200 characters")
	v.Check(req.InitialPriceKobo >= 0, "initial_price_kobo", "must not be negative")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	variant, err := api.catalogue.CreateVariant(r.Context(), principal.UserID, business.BusinessID, productID, req.Name, req.InitialPriceKobo)
	if err != nil {
		catalogueErrorResponse(api, w, r, err, "create variant")
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toVariantResponse(variant, nil)}, nil); err != nil {
		api.logger.Error("write create variant response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type updateVariantRequest struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// updateVariant implements PATCH /api/v1/variants/{variant_id}. Owner-only
// (catalogue:manage). Deactivating a variant is the only removal path —
// there is no delete endpoint, and it never touches price history.
func (api *API) updateVariant(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("updateVariant ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityCatalogueManage)
	if !ok {
		return
	}

	variantID, err := uuid.Parse(chi.URLParam(r, "variant_id"))
	if err != nil {
		api.badRequestResponse(w, r, "variant_id must be a valid UUID.")
		return
	}

	var req updateVariantRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)

	v := validator.New()
	v.Check(req.Name != "", "name", "must be provided")
	v.Check(len(req.Name) <= 200, "name", "must be at most 200 characters")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	variant, err := api.catalogue.UpdateVariant(r.Context(), principal.UserID, business.BusinessID, variantID, catalogue.UpdateVariantParams{
		Name: req.Name, Active: req.Active,
	})
	if err != nil {
		catalogueErrorResponse(api, w, r, err, "update variant")
		return
	}

	history, err := api.catalogue.ListPriceHistory(r.Context(), principal.UserID, business.BusinessID, variantID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("load current price after update: %w", err))
		return
	}
	payload := variantResponse{ID: variant.ID.String(), Name: variant.Name, Active: variant.Active}
	if len(history) > 0 {
		payload.CurrentPrice = toPriceResponse(history[0])
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": payload}, nil); err != nil {
		api.logger.Error("write update variant response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// --- prices ---

// listVariantPrices implements GET /api/v1/variants/{variant_id}/prices,
// the full effective-dated price history, newest first. Staff may read
// (catalogue:read).
func (api *API) listVariantPrices(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("listVariantPrices ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityCatalogueRead)
	if !ok {
		return
	}

	variantID, err := uuid.Parse(chi.URLParam(r, "variant_id"))
	if err != nil {
		api.badRequestResponse(w, r, "variant_id must be a valid UUID.")
		return
	}

	history, err := api.catalogue.ListPriceHistory(r.Context(), principal.UserID, business.BusinessID, variantID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list variant prices: %w", err))
		return
	}
	out := make([]priceResponse, 0, len(history))
	for _, p := range history {
		out = append(out, toPriceResponse(p))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list variant prices response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type setVariantPriceRequest struct {
	AmountKobo int64 `json:"amount_kobo"`
}

// setVariantPrice implements POST /api/v1/variants/{variant_id}/prices: a
// price change, never an edit of history — it closes the current price row
// and inserts this one atomically (catalogue.Service.SetVariantPrice).
// Owner-only (catalogue:manage) — docs/ARCHITECTURE.md §11.2 lists product
// price changes among the actions that always require online server
// authorization, never permitted from a cached offline lease.
func (api *API) setVariantPrice(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("setVariantPrice ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityCatalogueManage)
	if !ok {
		return
	}

	variantID, err := uuid.Parse(chi.URLParam(r, "variant_id"))
	if err != nil {
		api.badRequestResponse(w, r, "variant_id must be a valid UUID.")
		return
	}

	var req setVariantPriceRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}

	v := validator.New()
	v.Check(req.AmountKobo >= 0, "amount_kobo", "must not be negative")
	if !v.IsValid() {
		api.validationFailedResponse(w, r, v.Errors)
		return
	}

	price, err := api.catalogue.SetVariantPrice(r.Context(), principal.UserID, business.BusinessID, variantID, req.AmountKobo)
	if err != nil {
		catalogueErrorResponse(api, w, r, err, "set variant price")
		return
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": toPriceResponse(price)}, nil); err != nil {
		api.logger.Error("write set variant price response", "request_id", RequestID(r.Context()), "error", err)
	}
}
