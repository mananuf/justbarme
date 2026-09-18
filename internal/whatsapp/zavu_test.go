package whatsapp

// Internal test (package whatsapp, not whatsapp_test) so it can point
// ZavuProvider at an httptest server instead of the real api.zavu.dev.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestZavuProvider(t *testing.T, handler http.HandlerFunc) *ZavuProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &ZavuProvider{
		cfg:        ZavuConfig{APIKey: "zv_test_key", SenderID: "sender-123"},
		httpClient: server.Client(),
		baseURL:    server.URL,
	}
}

func TestZavuProviderSendsExpectedRequestAndReturnsMessageID(t *testing.T) {
	var gotAuth, gotSender, gotMethod, gotPath string
	var gotBody sendMessageRequest

	p := newTestZavuProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotSender = r.Header.Get("Zavu-Sender")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)

		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(sendMessageResponse{
			Message: struct {
				ID string `json:"id"`
			}{ID: "msg-abc123"},
		})
	})

	id, err := p.Send(context.Background(), Message{To: "+2348012345678", Text: "Your code is 123456"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if id != "msg-abc123" {
		t.Fatalf("expected message ID msg-abc123, got %q", id)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/messages" {
		t.Fatalf("expected POST /v1/messages, got %s %s", gotMethod, gotPath)
	}
	if gotAuth != "Bearer zv_test_key" {
		t.Fatalf("expected Authorization: Bearer zv_test_key, got %q", gotAuth)
	}
	if gotSender != "sender-123" {
		t.Fatalf("expected Zavu-Sender: sender-123, got %q", gotSender)
	}
	if gotBody.To != "+2348012345678" || gotBody.Channel != "whatsapp" || gotBody.Text != "Your code is 123456" {
		t.Fatalf("unexpected request body: %+v", gotBody)
	}
	if gotBody.IdempotencyKey == "" {
		t.Fatal("expected a non-empty idempotencyKey")
	}
}

func TestZavuProviderReturnsErrorOnNonSuccessStatus(t *testing.T) {
	p := newTestZavuProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(zavuErrorResponse{Error: struct {
			Message string `json:"message"`
		}{Message: "invalid recipient"}})
	})

	_, err := p.Send(context.Background(), Message{To: "not-a-phone-number", Text: "hi"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}
