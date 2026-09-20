package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/mananuf/justbarme/internal/activity"
	"github.com/mananuf/justbarme/internal/tenancy"
)

type dashboardResponse struct {
	TodayTotalKobo    *int64                  `json:"today_total_kobo,omitempty"`
	TodaySaleCount    *int64                  `json:"today_sale_count,omitempty"`
	ItemsSoldToday    *int64                  `json:"items_sold_today,omitempty"`
	TodayExpensesKobo *int64                  `json:"today_expenses_kobo,omitempty"`
	OutstandingKobo   *int64                  `json:"outstanding_kobo,omitempty"`
	AlertsCount       *int64                  `json:"alerts_count,omitempty"`
	RecentActivity    []activityEntryResponse `json:"recent_activity,omitempty"`
}

// getDashboard implements GET /api/v1/dashboard (docs/ARCHITECTURE.md
// §12, docs/PHASE_EXPENSES_DASHBOARD_ACTIVITY_REPORTS.md §3) -- replaces
// the frontend's previous three separate calls (sales summary, sales
// list, outstanding bills) with one round trip.
//
// Deliberately capability-aware per section rather than gated behind one
// blanket capability: today_total_kobo/today_sale_count/items_sold_today/
// today_expenses_kobo need reports:read, recent_activity needs
// activity:read, outstanding_kobo needs bills:read, and alerts_count
// needs reviews:read. A caller missing one of these simply doesn't get
// that field, rather than the whole endpoint 403ing. This also fixes a
// real, previously-live gap: Staff has neither reports:read nor
// activity:read, so the old three-separate-call Dashboard.tsx silently
// 403'd two of three calls for a Staff viewer and rendered blank cards via
// its own swallowed .catch().
func (api *API) getDashboard(w http.ResponseWriter, r *http.Request) {
	principal, ok := tenancy.PrincipalFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, fmt.Errorf("getDashboard ran without requireAuth"))
		return
	}
	business, ok := tenancy.BusinessFromContext(r.Context())
	if !ok {
		api.internalErrorResponse(w, r, fmt.Errorf("getDashboard ran without requireBusinessContext"))
		return
	}

	biz, err := api.identity.GetBusiness(r.Context(), principal.UserID, business.BusinessID)
	if err != nil {
		api.internalErrorResponse(w, r, fmt.Errorf("get business: %w", err))
		return
	}
	loc, err := time.LoadLocation(biz.Timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		out dashboardResponse
		// The first error from any section wins -- a genuine failure
		// (not a missing capability, which is handled by simply not
		// running that section) should still surface as a real error
		// rather than being silently swallowed.
		firstErr error
	)
	setErr := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}

	if business.Has(tenancy.CapabilityReportsRead) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			total, count, err := api.sales.SumSalesTotalSince(r.Context(), principal.UserID, business.BusinessID, startOfDay)
			if err != nil {
				setErr(fmt.Errorf("sum sales total: %w", err))
				return
			}
			units, err := api.sales.SumItemsSoldSince(r.Context(), principal.UserID, business.BusinessID, startOfDay)
			if err != nil {
				setErr(fmt.Errorf("sum items sold: %w", err))
				return
			}
			expenseSummary, err := api.expenses.SumExpensesSince(r.Context(), principal.UserID, business.BusinessID, startOfDay)
			if err != nil {
				setErr(fmt.Errorf("sum expenses: %w", err))
				return
			}
			mu.Lock()
			out.TodayTotalKobo = &total
			out.TodaySaleCount = &count
			out.ItemsSoldToday = &units
			out.TodayExpensesKobo = &expenseSummary.TotalKobo
			mu.Unlock()
		}()
	}

	if business.Has(tenancy.CapabilityActivityRead) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entries, err := api.activity.List(r.Context(), principal.UserID, business.BusinessID, activity.Filter{}, 5)
			if err != nil {
				setErr(fmt.Errorf("list activity: %w", err))
				return
			}
			actorIDs := make([]uuid.UUID, len(entries))
			for i, e := range entries {
				actorIDs[i] = e.ActorID
			}
			names, err := api.resolveSellerNames(r.Context(), actorIDs)
			if err != nil {
				setErr(fmt.Errorf("resolve actor names: %w", err))
				return
			}
			responses := make([]activityEntryResponse, 0, len(entries))
			for _, e := range entries {
				responses = append(responses, activityEntryResponse{
					ID: e.ID.String(), Type: e.Type, ActorID: e.ActorID.String(), ActorName: names[e.ActorID],
					OccurredAt: e.OccurredAt.UTC().Format(time.RFC3339), Summary: e.Summary, AmountKobo: e.AmountKobo,
				})
			}
			mu.Lock()
			out.RecentActivity = responses
			mu.Unlock()
		}()
	}

	if business.Has(tenancy.CapabilityBillsRead) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bills, err := api.sales.ListOutstandingBills(r.Context(), principal.UserID, business.BusinessID)
			if err != nil {
				setErr(fmt.Errorf("list outstanding bills: %w", err))
				return
			}
			var total int64
			for _, b := range bills {
				total += b.BalanceKobo
			}
			mu.Lock()
			out.OutstandingKobo = &total
			mu.Unlock()
		}()
	}

	if business.Has(tenancy.CapabilityReviewsRead) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			count, err := api.countOpenAlerts(r.Context(), principal.UserID, business.BusinessID)
			if err != nil {
				setErr(err)
				return
			}
			mu.Lock()
			out.AlertsCount = &count
			mu.Unlock()
		}()
	}

	wg.Wait()
	if firstErr != nil {
		api.internalErrorResponse(w, r, firstErr)
		return
	}

	if err := writeJSON(w, http.StatusOK, envelope{"data": out}, nil); err != nil {
		api.logger.Error("write dashboard response", "request_id", RequestID(r.Context()), "error", err)
	}
}

// countOpenAlerts sums open inventory_reviews, open sale_reviews, and
// pending inventory_adjustment_requests -- a real number the dashboard's
// "alerts" card can show, replacing the hardcoded "3 items low" mock. It
// counts *flagged reviews and pending decisions*, not stock-level
// thresholds -- Phase 7 explicitly left low-stock alerting out of scope,
// and this doesn't quietly reintroduce it.
func (api *API) countOpenAlerts(ctx context.Context, userID, businessID uuid.UUID) (int64, error) {
	inventoryReviews, err := api.inventory.ListOpenReviews(ctx, userID, businessID)
	if err != nil {
		return 0, fmt.Errorf("list open inventory reviews: %w", err)
	}
	saleReviews, err := api.sales.ListOpenReviews(ctx, userID, businessID)
	if err != nil {
		return 0, fmt.Errorf("list open sale reviews: %w", err)
	}
	pendingAdjustments, err := api.inventory.ListPendingAdjustmentRequests(ctx, userID, businessID)
	if err != nil {
		return 0, fmt.Errorf("list pending adjustment requests: %w", err)
	}
	return int64(len(inventoryReviews) + len(saleReviews) + len(pendingAdjustments)), nil
}
