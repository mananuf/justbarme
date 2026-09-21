package httpapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// metricsHandler implements GET /metrics -- the Prometheus text-format
// exposition, docs/PHASE_PILOT_RELEASE.md §6. When api.metricsToken is set
// (JBM_METRICS_TOKEN), a request must carry a matching
// "Authorization: Bearer <token>" or gets 401; left unset, the endpoint is
// open, which is fine for local development. Unlike the API's normal
// error envelope, an unauthorized response here is a bare 401 with no
// body -- a metrics scraper doesn't parse {"error": ...} JSON, and this
// endpoint sits outside /api/v1 entirely.
func (api *API) metricsHandler(w http.ResponseWriter, r *http.Request) {
	if api.metrics == nil {
		http.NotFound(w, r)
		return
	}
	if api.metricsToken != "" {
		const prefix = "Bearer "
		auth := r.Header.Get("Authorization")
		token := ""
		if strings.HasPrefix(auth, prefix) {
			token = auth[len(prefix):]
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(api.metricsToken)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}
	api.metrics.Handler().ServeHTTP(w, r)
}
