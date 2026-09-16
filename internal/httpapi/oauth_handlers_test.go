package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestGoogleSignInRouteAbsentWhenOAuthNotConfigured guards the conditional
// registration in NewHandler: leaving Dependencies.OAuth nil (the default
// when JBM_GOOGLE_OAUTH_CLIENT_ID is unset -- see internal/app.Run) must
// leave POST /auth/google entirely unregistered, not present-but-broken.
func TestGoogleSignInRouteAbsentWhenOAuthNotConfigured(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewHandler(Dependencies{
		Logger: logger, Database: &fakePinger{}, HealthTimeout: 5 * time.Millisecond, Version: "test-version",
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/auth/google", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when OAuth is not configured, got %d", response.Code)
	}
}
