package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/inventory"
	"github.com/mananuf/justbarme/internal/tenancy"
	"github.com/mananuf/justbarme/internal/validator"
)

type receiveStockLineRequest struct {
	VariantID     string `json:"variant_id"`
	Quantity      int32  `json:"quantity"`
	TotalCostKobo int64  `json:"total_cost_kobo"`
}

type receiveStockRequest struct {
	Lines []receiveStockLineRequest `json:"lines"`
}

type receiptLineResponse struct {
	VariantID     string `json:"variant_id"`
	Quantity      int32  `json:"quantity"`
	TotalCostKobo int64  `json:"total_cost_kobo"`
	NewBalance    int64  `json:"new_balance"`
}

type receiptResponse struct {
	ID         string                `json:"id"`
	ReceivedAt string                `json:"received_at"`
	Lines      []receiptLineResponse `json:"lines"`
}

// receiveStock implements POST /api/v1/stock-receipts: owner-only
// (inventory:receive) since it's the only mutation this thin inventory
// slice has. The client never supplies a location -- it's resolved
// server-side from the business's default location, since multi-location
// isn't a pilot concern (see docs/PHASE_STOCK_RECEIVING.md).
func (api *API) receiveStock(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("receiveStock ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityInventoryReceive)
	if !ok {
		return
	}

	var req receiveStockRequest
	if err := readJSON(w, r, &req, 1<<16); err != nil {
		api.badRequestResponse(w, r, err.Error())
		return
	}

	v := validator.New()
	v.Check(len(req.Lines) > 0, "lines", "must include at least one line")
	lines := make([]inventory.ReceiptLine, 0, len(req.Lines))
	for i, raw := range req.Lines {
		field := fmt.Sprintf("lines[%d]", i)
		variantID, err := uuid.Parse(raw.VariantID)
		v.Check(err == nil, field+".variant_id", "must be a valid UUID")
		v.Check(raw.Quantity > 0, field+".quantity", "must be positive")
		v.Check(raw.TotalCostKobo >= 0, field+".total_cost_kobo", "must not be negative")
		if err == nil {
			lines = append(lines, inventory.ReceiptLine{
				VariantID: variantID, Quantity: raw.Quantity, TotalCostKobo: raw.TotalCostKobo,
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

	receipt, err := api.inventory.ReceiveStock(r.Context(), principal.UserID, business.BusinessID, location.ID, lines)
	if err != nil {
		if errors.Is(err, inventory.ErrVariantNotFound) {
			api.badRequestResponse(w, r, "One or more variant IDs do not exist.")
			return
		}
		if errors.Is(err, inventory.ErrVariantNotTracked) {
			api.conflictResponse(w, r, err.Error())
			return
		}
		api.internalErrorResponse(w, r, fmt.Errorf("receive stock: %w", err))
		return
	}

	out := receiptResponse{ID: receipt.ID.String(), ReceivedAt: receipt.ReceivedAt.UTC().Format(time.RFC3339)}
	for _, l := range receipt.Lines {
		out.Lines = append(out.Lines, receiptLineResponse{
			VariantID: l.VariantID.String(), Quantity: l.Quantity, TotalCostKobo: l.TotalCostKobo, NewBalance: l.NewBalance,
		})
	}
	if err := writeJSON(w, http.StatusCreated, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write receive stock response", "request_id", RequestID(r.Context()), "error", err)
	}
}
