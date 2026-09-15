package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakePinger struct {
	err   error
	calls int
	wait  bool
}

func (p *fakePinger) Ping(ctx context.Context) error {
	p.calls++
	if p.wait {
		<-ctx.Done()
		return ctx.Err()
	}
	return p.err
}

func testHandler(database DatabasePinger, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return NewHandler(Dependencies{Logger: logger, Database: database, HealthTimeout: 5 * time.Millisecond, Version: "test-version"})
}

func TestLivenessDoesNotPingDatabase(t *testing.T) {
	database := &fakePinger{}
	response := httptest.NewRecorder()
	testHandler(database, nil).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil))

	if response.Code != http.StatusOK || database.calls != 0 {
		t.Fatalf("code=%d ping calls=%d", response.Code, database.calls)
	}
	assertJSONPath(t, response.Body.Bytes(), "data.status", "available")
	assertJSONPath(t, response.Body.Bytes(), "data.version", "test-version")
}

func TestReadiness(t *testing.T) {
	tests := []struct {
		name       string
		database   DatabasePinger
		wantStatus int
		wantCode   string
	}{
		{name: "ready", database: &fakePinger{}, wantStatus: http.StatusOK},
		{name: "database error", database: &fakePinger{err: errors.New("unavailable")}, wantStatus: http.StatusServiceUnavailable, wantCode: "DEPENDENCY_UNAVAILABLE"},
		{name: "database timeout", database: &fakePinger{wait: true}, wantStatus: http.StatusServiceUnavailable, wantCode: "DEPENDENCY_UNAVAILABLE"},
		{name: "missing checker", database: nil, wantStatus: http.StatusServiceUnavailable, wantCode: "DEPENDENCY_UNAVAILABLE"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			testHandler(test.database, nil).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil))
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if test.wantCode != "" {
				assertJSONPath(t, response.Body.Bytes(), "error.code", test.wantCode)
			}
		})
	}
}

func TestRouterErrorsUseContract(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantCode   string
	}{
		{name: "not found", method: http.MethodGet, path: "/missing", wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
		{name: "method not allowed", method: http.MethodPost, path: "/api/v1/health/live", wantStatus: http.StatusMethodNotAllowed, wantCode: "METHOD_NOT_ALLOWED"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			testHandler(&fakePinger{}, nil).ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			assertJSONPath(t, response.Body.Bytes(), "error.code", test.wantCode)
			requestID := response.Header().Get(requestIDHeader)
			if _, err := uuid.Parse(requestID); err != nil {
				t.Fatalf("request ID %q is invalid: %v", requestID, err)
			}
			assertJSONPath(t, response.Body.Bytes(), "error.request_id", requestID)
		})
	}
}

func TestRequestIDPreservesValidClientID(t *testing.T) {
	requestID := uuid.NewString()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	request.Header.Set(requestIDHeader, requestID)
	response := httptest.NewRecorder()

	testHandler(&fakePinger{}, nil).ServeHTTP(response, request)
	if got := response.Header().Get(requestIDHeader); got != requestID {
		t.Fatalf("request ID=%q, want %q", got, requestID)
	}
}

func TestSecurityHeaders(t *testing.T) {
	response := httptest.NewRecorder()
	testHandler(&fakePinger{}, nil).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil))

	for _, header := range []string{"Cache-Control", "Content-Security-Policy", "Referrer-Policy", "X-Content-Type-Options", "X-Frame-Options"} {
		if response.Header().Get(header) == "" {
			t.Errorf("missing %s header", header)
		}
	}
}

func TestRecoverPanicReturnsSafeError(t *testing.T) {
	var logs bytes.Buffer
	api := &API{logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	handler := api.requestIDMiddleware(api.recoverPanicMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("sensitive panic")
	})))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if response.Code != http.StatusInternalServerError || response.Header().Get("Connection") != "close" {
		t.Fatalf("status=%d headers=%v", response.Code, response.Header())
	}
	assertJSONPath(t, response.Body.Bytes(), "error.code", "INTERNAL_ERROR")
	if strings.Contains(response.Body.String(), "sensitive panic") {
		t.Fatalf("panic leaked in response: %s", response.Body.String())
	}
	if !strings.Contains(logs.String(), "sensitive panic") {
		t.Fatal("internal panic was not logged")
	}
}

func TestAccessLogDoesNotIncludeRequestBody(t *testing.T) {
	var logs bytes.Buffer
	api := &API{logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	handler := api.requestIDMiddleware(api.accessLogMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"password":"do-not-log"}`))
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if strings.Contains(logs.String(), "do-not-log") {
		t.Fatalf("request body leaked to log: %s", logs.String())
	}
	for _, field := range []string{"request_id", "method", "path", "status", "duration"} {
		if !strings.Contains(logs.String(), field) {
			t.Errorf("log missing field %q: %s", field, logs.String())
		}
	}
}

func TestResponseRecorderUsesFirstStatus(t *testing.T) {
	underlying := httptest.NewRecorder()
	recorder := &responseRecorder{ResponseWriter: underlying}
	recorder.WriteHeader(http.StatusCreated)
	recorder.WriteHeader(http.StatusInternalServerError)
	_, _ = recorder.Write([]byte("ok"))

	if recorder.status != http.StatusCreated || underlying.Code != http.StatusCreated || recorder.bytes != 2 {
		t.Fatalf("status=%d underlying=%d bytes=%d", recorder.status, underlying.Code, recorder.bytes)
	}
}

func BenchmarkLivenessRoute(b *testing.B) {
	handler := testHandler(&fakePinger{}, nil)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil))
	}
}

func BenchmarkRequestIDMiddleware(b *testing.B) {
	api := &API{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	handler := api.requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}
}

func assertJSONPath(t *testing.T, body []byte, path, want string) {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, body)
	}
	parts := strings.Split(path, ".")
	var current any = value
	for _, part := range parts {
		object, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("path %q does not contain an object at %q", path, part)
		}
		current, ok = object[part]
		if !ok {
			t.Fatalf("path %q missing %q", path, part)
		}
	}
	if current != want {
		t.Fatalf("%s=%v, want %q", path, current, want)
	}
}
