package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const requestIDHeader = "X-Request-ID"

type requestIDContextKey struct{}

func RequestID(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func (api *API) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(requestIDHeader)
		if _, err := uuid.Parse(requestID); err != nil {
			generated, generationErr := uuid.NewV7()
			if generationErr != nil {
				generated = uuid.New()
			}
			requestID = generated.String()
		}

		w.Header().Set(requestIDHeader, requestID)
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func (api *API) recoverPanicMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				w.Header().Set("Connection", "close")
				api.internalErrorResponse(w, r, fmt.Errorf("panic: %v", recovered))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	written, err := r.ResponseWriter.Write(data)
	r.bytes += written
	return written, err
}

func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (api *API) accessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		duration := time.Since(started)

		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}

		// chi.RouteContext's RoutePattern is only fully populated once
		// routing has actually matched a handler -- reading it here,
		// after next.ServeHTTP returns, is what makes this the resolved
		// pattern ("/bills/{bill_id}/rounds") rather than the raw path
		// with a live UUID in it. A request that matched no route at all
		// (chi's NotFound handler) leaves it empty, so fall back to the
		// raw path -- still useful for spotting a genuinely wrong URL,
		// and the fallback is exactly why this can't just always be the
		// pattern.
		route := ""
		if routeCtx := chi.RouteContext(r.Context()); routeCtx != nil {
			route = routeCtx.RoutePattern()
		}
		if route == "" {
			route = r.URL.Path
		}

		if api.metrics != nil {
			api.metrics.ObserveHTTPRequest(route, r.Method, status, duration)
		}

		api.logger.LogAttrs(r.Context(), slog.LevelInfo, "HTTP request",
			slog.String("request_id", RequestID(r.Context())),
			slog.String("method", r.Method),
			slog.String("route", route),
			slog.String("path", r.URL.Path),
			slog.Int("status", status),
			slog.Int("response_bytes", recorder.bytes),
			slog.Duration("duration", duration),
		)
	})
}
