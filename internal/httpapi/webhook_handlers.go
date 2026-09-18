package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/mananuf/justbarme/internal/whatsapp"
)

type zavuWebhookEvent struct {
	Event   string `json:"event"`
	Message struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"message"`
}

// zavuWebhook implements POST /api/v1/webhooks/zavu: public but
// signature-verified (X-Zavu-Signature, HMAC-SHA256 -- see
// internal/whatsapp.VerifySignature), per docs/PHASE_INVITATIONS_
// WHATSAPP.md §8's confirmed decision to build delivery-status visibility
// in this pass rather than deferring it. This v1 slice only verifies and
// logs the event -- there is no per-message delivery-status storage yet
// to update (that would need messageID correlation storage, a further
// slice once something actually consumes delivery status).
func (api *API) zavuWebhook(w http.ResponseWriter, r *http.Request) {
	if api.zavuWebhookSecret == "" {
		// Zavu isn't configured (dev, or WhatsApp not yet connected) --
		// there is nothing to verify against, and no real webhook will
		// ever be sent here in that case. Fail closed rather than
		// accepting unverifiable events.
		api.notFoundResponse(w, r)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		api.badRequestResponse(w, r, "could not read request body")
		return
	}

	if err := whatsapp.VerifySignature(r.Header.Get("X-Zavu-Signature"), body, api.zavuWebhookSecret); err != nil {
		api.logger.Warn("rejected zavu webhook with invalid signature", "request_id", RequestID(r.Context()), "error", err)
		api.errorResponse(w, r, http.StatusUnauthorized, "INVALID_SIGNATURE", "Webhook signature verification failed.", nil)
		return
	}

	// Only parsed, non-PII fields are logged (message ID and status, never
	// recipient/content) -- a phone number or message text has no reason
	// to sit in application logs indefinitely.
	var event zavuWebhookEvent
	_ = json.Unmarshal(body, &event)
	api.logger.Info("received zavu webhook event", "request_id", RequestID(r.Context()),
		"event", event.Event, "message_id", event.Message.ID, "status", event.Message.Status)
	w.WriteHeader(http.StatusOK)
}
