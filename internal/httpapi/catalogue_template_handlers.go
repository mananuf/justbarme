package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/catalogue"
	"github.com/mananuf/justbarme/internal/tenancy"
)

type templateVariantResponse struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	SuggestedPriceKobo int64  `json:"suggested_price_kobo"`
}

type templateResponse struct {
	ID           string                    `json:"id"`
	Name         string                    `json:"name"`
	CategoryName string                    `json:"category_name"`
	Variants     []templateVariantResponse `json:"variants"`
}

// listCatalogueTemplates implements GET /api/v1/catalogue-templates. Any
// authenticated principal may read these — they are platform reference
// data, not tenant-owned, and are shown during onboarding before a
// business necessarily exists yet.
func (api *API) listCatalogueTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := api.catalogue.ListTemplates(r.Context())
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list catalogue templates: %w", err))
		return
	}

	out := make([]templateResponse, 0, len(templates))
	for _, t := range templates {
		out = append(out, toTemplateResponse(t))
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write catalogue templates response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type applyCatalogueTemplatesRequest struct {
	TemplateIDs []string `json:"template_ids"`
}

// applyCatalogueTemplates implements POST /api/v1/catalogue-templates/apply:
// owner-only (catalogue:manage) since it creates business-owned catalogue
// records — see docs/ARCHITECTURE.md §8.2 "Selecting a template creates
// business-owned editable records."
func (api *API) applyCatalogueTemplates(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("applyCatalogueTemplates ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityCatalogueManage)
	if !ok {
		return
	}

	var req applyCatalogueTemplatesRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}

	if len(req.TemplateIDs) == 0 {
		api.validationFailedResponse(w, r, map[string]string{"template_ids": "must include at least one template ID"})
		return
	}
	templateIDs := make([]uuid.UUID, len(req.TemplateIDs))
	for i, raw := range req.TemplateIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			api.validationFailedResponse(w, r, map[string]string{"template_ids": "must all be valid UUIDs"})
			return
		}
		templateIDs[i] = id
	}

	products, err := api.catalogue.ApplyTemplates(r.Context(), principal.UserID, business.BusinessID, templateIDs)
	if err != nil {
		if errors.Is(err, catalogue.ErrTemplateNotFound) {
			api.badRequestResponse(w, r, "One or more template IDs do not exist.")
			return
		}
		api.internalErrorResponse(w, r, fmt.Errorf("apply catalogue templates: %w", err))
		return
	}

	out := make([]productResponse, 0, len(products))
	for _, p := range products {
		// Freshly applied templates have no stock receipts yet.
		out = append(out, toProductResponse(p, nil))
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write apply catalogue templates response", "request_id", RequestID(r.Context()), "error", err)
	}
}

func toTemplateResponse(t catalogue.TemplateWithVariants) templateResponse {
	variants := make([]templateVariantResponse, 0, len(t.Variants))
	for _, v := range t.Variants {
		variants = append(variants, templateVariantResponse{
			ID: v.ID.String(), Name: v.Name, SuggestedPriceKobo: v.SuggestedPriceKobo,
		})
	}
	return templateResponse{ID: t.ID.String(), Name: t.Name, CategoryName: t.CategoryName, Variants: variants}
}
