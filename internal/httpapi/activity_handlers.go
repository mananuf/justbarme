package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/activity"
	"github.com/mananuf/justbarme/internal/tenancy"
)

type activityEntryResponse struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	ActorID    string `json:"actor_id"`
	ActorName  string `json:"actor_name,omitempty"`
	OccurredAt string `json:"occurred_at"`
	Summary    string `json:"summary"`
	AmountKobo int64  `json:"amount_kobo"`
}

// listActivity implements GET /api/v1/activity (activity:read) -- the
// unified feed (docs/PHASE_EXPENSES_DASHBOARD_ACTIVITY_REPORTS.md §3),
// with optional actor_id/type/start_at/end_at filters. Query params match
// the RFC 3339 convention every other date-bounded endpoint in this
// codebase uses.
func (api *API) listActivity(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("listActivity ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityActivityRead)
	if !ok {
		return
	}

	var filter activity.Filter
	q := r.URL.Query()
	if raw := q.Get("actor_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			api.badRequestResponse(w, r, "actor_id must be a valid UUID.")
			return
		}
		filter.ActorID = id
	}
	filter.Type = q.Get("type")
	if raw := q.Get("start_at"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			api.badRequestResponse(w, r, "start_at must be an RFC 3339 timestamp.")
			return
		}
		filter.StartAt = t
	}
	if raw := q.Get("end_at"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			api.badRequestResponse(w, r, "end_at must be an RFC 3339 timestamp.")
			return
		}
		filter.EndAt = t
	}

	limit := int32(50)
	if raw := q.Get("limit"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 32); err == nil && parsed > 0 && parsed <= 200 {
			limit = int32(parsed)
		}
	}

	list, err := api.activity.List(r.Context(), principal.UserID, business.BusinessID, filter, limit)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("list activity: %w", err))
		return
	}
	actorIDs := make([]uuid.UUID, len(list))
	for i, e := range list {
		actorIDs[i] = e.ActorID
	}
	names, err := api.resolveSellerNames(r.Context(), actorIDs)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("resolve actor names: %w", err))
		return
	}
	out := make([]activityEntryResponse, 0, len(list))
	for _, e := range list {
		out = append(out, activityEntryResponse{
			ID: e.ID.String(), Type: e.Type, ActorID: e.ActorID.String(), ActorName: names[e.ActorID],
			OccurredAt: e.OccurredAt.UTC().Format(time.RFC3339), Summary: e.Summary, AmountKobo: e.AmountKobo,
		})
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write list activity response", "request_id", RequestID(r.Context()), "error", err)
	}
}

type activityDayCountResponse struct {
	Day   string `json:"day"`
	Count int64  `json:"count"`
}

// activityHeatmap implements GET /api/v1/activity/heatmap (activity:read)
// -- per-day activity counts bucketed in the business's own timezone,
// backing the Activity page's GitHub-style heatmap. start_at/end_at are
// required (a heatmap always has an explicit range, unlike the feed's
// default "most recent N").
func (api *API) activityHeatmap(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, errors.New("activityHeatmap ran without requireAuth"))
		return
	}
	business, ok := api.requireCapability(w, r, tenancy.CapabilityActivityRead)
	if !ok {
		return
	}

	q := r.URL.Query()
	startAt, startErr := time.Parse(time.RFC3339, q.Get("start_at"))
	endAt, endErr := time.Parse(time.RFC3339, q.Get("end_at"))
	if startErr != nil || endErr != nil {
		api.badRequestResponse(w, r, "start_at and end_at must both be RFC 3339 timestamps.")
		return
	}

	biz, err := api.identity.GetBusiness(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("get business: %w", err))
		return
	}

	counts, err := api.activity.CountByDay(r.Context(), principal.UserID, business.BusinessID, biz.Timezone, startAt, endAt)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("count activity by day: %w", err))
		return
	}
	out := make([]activityDayCountResponse, 0, len(counts))
	for _, c := range counts {
		out = append(out, activityDayCountResponse{Day: c.Day.Format("2006-01-02"), Count: c.Count})
	}
	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write activity heatmap response", "request_id", RequestID(r.Context()), "error", err)
	}
}
