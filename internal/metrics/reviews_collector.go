package metrics

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/mananuf/justbarme/internal/inventory"
	"github.com/mananuf/justbarme/internal/sales"
	"github.com/mananuf/justbarme/internal/store"
	"github.com/mananuf/justbarme/internal/store/sqlc"
)

// reviewsScrapeTimeout bounds how long one /metrics scrape may spend
// querying every active business's open reviews -- a slow or hanging
// scrape must never be able to pile up indefinitely.
const reviewsScrapeTimeout = 5 * time.Second

// ReviewsCollector reports open review counts by type --
// docs/ARCHITECTURE.md §18's "open review count by type". There is
// deliberately no global, cross-tenant query here: per this codebase's
// "no RLS bypass, ever" rule (see CLAUDE.md's Platform admin section),
// this instead lists active businesses (businesses itself carries no RLS,
// same reasoning platformadmin.ListBusinesses already relies on) and runs
// one ordinary tenant-scoped query per business, summing the results in
// Go -- every read still goes through the normal jbm_app role with
// app.business_id set, exactly like any other request. Fine for a pilot's
// business count; would need rethinking at real scale.
type ReviewsCollector struct {
	pool   *pgxpool.Pool
	sales  *sales.Service
	invSvc *inventory.Service
	logger *slog.Logger

	saleReviews        *prometheus.Desc
	inventoryReviews   *prometheus.Desc
	pendingAdjustments *prometheus.Desc
}

func NewReviewsCollector(pool *pgxpool.Pool, salesSvc *sales.Service, inventorySvc *inventory.Service, logger *slog.Logger) *ReviewsCollector {
	if logger == nil {
		logger = slog.Default()
	}
	return &ReviewsCollector{
		pool: pool, sales: salesSvc, invSvc: inventorySvc, logger: logger,
		saleReviews:        prometheus.NewDesc("jbm_open_sale_reviews", "Open sale reviews (stale price or deactivated variant) across all active businesses.", nil, nil),
		inventoryReviews:   prometheus.NewDesc("jbm_open_inventory_reviews", "Open inventory reviews across all active businesses, by type.", []string{"type"}, nil),
		pendingAdjustments: prometheus.NewDesc("jbm_pending_adjustment_requests", "Pending staff inventory-adjustment requests across all active businesses.", nil, nil),
	}
}

func (c *ReviewsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.saleReviews
	ch <- c.inventoryReviews
	ch <- c.pendingAdjustments
}

func (c *ReviewsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), reviewsScrapeTimeout)
	defer cancel()

	businessIDs, err := c.listActiveBusinessIDs(ctx)
	if err != nil {
		c.logger.Error("metrics: list active businesses for review counts", "error", err)
		return
	}

	var saleReviewCount, pendingAdjustmentCount int
	inventoryReviewCountsByType := map[string]int{}

	for _, businessID := range businessIDs {
		reviews, err := c.sales.ListOpenReviews(ctx, uuid.Nil, businessID)
		if err != nil {
			c.logger.Error("metrics: list open sale reviews", "business_id", businessID, "error", err)
			continue
		}
		saleReviewCount += len(reviews)

		invReviews, err := c.invSvc.ListOpenReviews(ctx, uuid.Nil, businessID)
		if err != nil {
			c.logger.Error("metrics: list open inventory reviews", "business_id", businessID, "error", err)
			continue
		}
		for _, r := range invReviews {
			inventoryReviewCountsByType[r.Type]++
		}

		adjustments, err := c.invSvc.ListPendingAdjustmentRequests(ctx, uuid.Nil, businessID)
		if err != nil {
			c.logger.Error("metrics: list pending adjustment requests", "business_id", businessID, "error", err)
			continue
		}
		pendingAdjustmentCount += len(adjustments)
	}

	ch <- prometheus.MustNewConstMetric(c.saleReviews, prometheus.GaugeValue, float64(saleReviewCount))
	ch <- prometheus.MustNewConstMetric(c.pendingAdjustments, prometheus.GaugeValue, float64(pendingAdjustmentCount))
	for reviewType, count := range inventoryReviewCountsByType {
		ch <- prometheus.MustNewConstMetric(c.inventoryReviews, prometheus.GaugeValue, float64(count), reviewType)
	}
}

func (c *ReviewsCollector) listActiveBusinessIDs(ctx context.Context) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := store.WithApp(ctx, c.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		businesses, err := q.ListAllBusinesses(ctx)
		if err != nil {
			return err
		}
		for _, b := range businesses {
			if b.Status == "active" {
				ids = append(ids, b.ID)
			}
		}
		return nil
	})
	return ids, err
}
