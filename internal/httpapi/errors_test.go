package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestErrorResponseIncludesDetailsAndRequestID(t *testing.T) {
	api := &API{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx := context.WithValue(context.Background(), requestIDContextKey{}, "request-123")
	request := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
	response := httptest.NewRecorder()

	api.errorResponse(response, request, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Invalid input provided.", map[string]string{"email": "invalid"})

	var payload struct {
		Error errorBody `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != http.StatusUnprocessableEntity || payload.Error.Code != "VALIDATION_FAILED" || payload.Error.RequestID != "request-123" || payload.Error.Details["email"] != "invalid" {
		t.Fatalf("unexpected response: status=%d payload=%+v", response.Code, payload)
	}
}

func TestErrorResponseUsesEmptyDetailsObject(t *testing.T) {
	api := &API{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	api.errorResponse(response, request, http.StatusNotFound, "NOT_FOUND", "Not found.", nil)

	var payload map[string]map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if details, ok := payload["error"]["details"].(map[string]any); !ok || len(details) != 0 {
		t.Fatalf("details=%#v, want empty object", payload["error"]["details"])
	}
}

func BenchmarkErrorResponse(b *testing.B) {
	api := &API{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		api.errorResponse(httptest.NewRecorder(), request, http.StatusNotFound, "NOT_FOUND", "Not found.", nil)
	}
}
