package httpapi

import (
	"context"
	"errors"
	"net/http"
)

func (api *API) liveness(w http.ResponseWriter, r *http.Request) {
	payload := envelope{"data": envelope{
		"status":  "available",
		"version": api.version,
	}}
	if err := writeJSON(w, http.StatusOK, payload, nil); err != nil {
		api.logger.Error("write liveness response", "request_id", RequestID(r.Context()), "error", err)
	}
}

func (api *API) readiness(w http.ResponseWriter, r *http.Request) {
	if api.database == nil {
		api.dependencyUnavailableResponse(w, r, errors.New("database health checker is not configured"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), api.healthTimeout)
	defer cancel()
	if err := api.database.Ping(ctx); err != nil {
		api.dependencyUnavailableResponse(w, r, err)
		return
	}

	payload := envelope{"data": envelope{
		"status":  "ready",
		"version": api.version,
	}}
	if err := writeJSON(w, http.StatusOK, payload, nil); err != nil {
		api.logger.Error("write readiness response", "request_id", RequestID(r.Context()), "error", err)
	}
}

func (api *API) dependencyUnavailableResponse(w http.ResponseWriter, r *http.Request, err error) {
	api.logger.Error("readiness dependency unavailable", "request_id", RequestID(r.Context()), "error", err)
	api.errorResponse(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "The service is not ready.", nil)
}
