package metrics_test

import (
	"context"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/catalogue"
	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/inventory"
	"github.com/mananuf/justbarme/internal/metrics"
	"github.com/mananuf/justbarme/internal/sales"
)

func scrape(t *testing.T, r *metrics.Registry) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("scrape: expected 200, got %d", rec.Code)
	}
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read scrape body: %v", err)
	}
	return string(body)
}

func TestObserveHTTPRequestAppearsInScrape(t *testing.T) {
	r := metrics.New()
	r.ObserveHTTPRequest("/api/v1/bills/{bill_id}/rounds", "POST", 201, 42*time.Millisecond)

	body := scrape(t, r)
	if !strings.Contains(body, `jbm_http_requests_total{method="POST",route="/api/v1/bills/{bill_id}/rounds",status="201"} 1`) {
		t.Fatalf("expected a request-count sample in scrape output, got:\n%s", body)
	}
	if !strings.Contains(body, "jbm_http_request_duration_seconds_bucket") {
		t.Fatalf("expected duration histogram buckets in scrape output, got:\n%s", body)
	}
}

func TestIncrementLoginFailureAppearsInScrape(t *testing.T) {
	r := metrics.New()
	r.IncrementLoginFailure()
	r.IncrementLoginFailure()

	body := scrape(t, r)
	if !strings.Contains(body, "jbm_login_failures_total 2") {
		t.Fatalf("expected 2 login failures in scrape output, got:\n%s", body)
	}
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("JBM_DATABASE_URL")
	if url == "" {
		t.Skip("JBM_DATABASE_URL not set; skipping PostgreSQL-backed test")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	cfg.MaxConns = 7 // an arbitrary, distinctive value to assert on below
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("database not reachable: %v", err)
	}
	return pool
}

func TestDBPoolCollectorReportsMaxConns(t *testing.T) {
	pool := testPool(t)

	r := metrics.New()
	r.MustRegister(metrics.NewDBPoolCollector(pool))

	body := scrape(t, r)
	if !strings.Contains(body, "jbm_db_pool_max_conns 7") {
		t.Fatalf("expected jbm_db_pool_max_conns to report the configured MaxConns (7), got:\n%s", body)
	}
}

func uniqueName(prefix string) string {
	return prefix + "-" + uuid.New().String()
}

// TestReviewsCollectorCountsOpenSaleReviews seeds one price-mismatch sale
// review (the same scenario internal/sales/service_test.go's
// TestCreateSaleFlagsPriceMismatchButStillPosts exercises) and confirms
// the collector's global count reflects it. It asserts >= 1, not ==1: the
// collector deliberately sums across every active business in the whole
// database (see ReviewsCollector's own doc comment for why -- avoiding an
// RLS bypass means no way to isolate this to just the business this test
// created), and other packages' tests can be creating their own review
// rows concurrently under `go test ./...`.
func TestReviewsCollectorCountsOpenSaleReviews(t *testing.T) {
	pool := testPool(t)
	argon2 := config.Argon2{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1}
	identitySvc := identity.New(pool, argon2)
	catalogueSvc := catalogue.New(pool)
	inventorySvc := inventory.New(pool)
	salesSvc := sales.New(pool, "")
	ctx := context.Background()

	owner, err := identitySvc.CreateUser(ctx, uuid.New().String()+"@example.com", "", "Metrics Test Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	biz, _, location, err := identitySvc.CreateBusinessWithOwner(ctx, owner.ID, uniqueName("Metrics Test Bar"))
	if err != nil {
		t.Fatalf("CreateBusinessWithOwner: %v", err)
	}
	t.Cleanup(func() { cleanupMetricsTenant(t, pool, owner.ID, biz.ID) })

	product, err := catalogueSvc.CreateProduct(ctx, owner.ID, biz.ID, uuid.Nil, uniqueName("Metrics Test Lager"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant, err := catalogueSvc.CreateVariant(ctx, owner.ID, biz.ID, product.ID, "Bottle", 80000)
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}
	if _, err := inventorySvc.ReceiveStock(ctx, owner.ID, biz.ID, location.ID, []inventory.ReceiptLine{
		{VariantID: variant.ID, Quantity: 10, TotalCostKobo: 500000},
	}); err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}

	if _, err := salesSvc.CreateSale(ctx, owner.ID, biz.ID, location.ID, owner.ID, uuid.New(), time.Now(),
		[]sales.SaleItemInput{{VariantID: variant.ID, Quantity: 1, UnitPriceKobo: 50000}},
		sales.PaymentInput{AmountKobo: 50000, Method: sales.PaymentMethodCash},
	); err != nil {
		t.Fatalf("CreateSale (price mismatch): %v", err)
	}

	r := metrics.New()
	r.MustRegister(metrics.NewReviewsCollector(pool, salesSvc, inventorySvc, nil))

	body := scrape(t, r)
	line := findMetricLine(body, "jbm_open_sale_reviews")
	if line == "" {
		t.Fatalf("expected a jbm_open_sale_reviews sample, got:\n%s", body)
	}
	if strings.HasSuffix(line, " 0") {
		t.Fatalf("expected jbm_open_sale_reviews to be at least 1, got: %s", line)
	}
}

func findMetricLine(body, metricName string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, metricName+" ") {
			return line
		}
	}
	return ""
}

func cleanupMetricsTenant(t *testing.T, pool *pgxpool.Pool, ownerID, businessID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err == nil {
		_, _ = tx.Exec(ctx, "SET LOCAL ROLE jbm_app")
		_, _ = tx.Exec(ctx, "SELECT set_config('app.business_id', $1, true)", businessID.String())
		for _, stmt := range []string{
			"DELETE FROM sale_reviews WHERE business_id = $1",
			"DELETE FROM sale_items WHERE business_id = $1",
			"DELETE FROM inventory_movements WHERE business_id = $1",
			"DELETE FROM inventory_events WHERE business_id = $1",
			"DELETE FROM payments WHERE business_id = $1",
			"DELETE FROM sales WHERE business_id = $1",
			"DELETE FROM bills WHERE business_id = $1",
			"DELETE FROM inventory_balances WHERE business_id = $1",
			"DELETE FROM stock_lots WHERE business_id = $1",
			"DELETE FROM stock_receipt_lines WHERE business_id = $1",
			"DELETE FROM stock_receipts WHERE business_id = $1",
			"DELETE FROM product_prices WHERE business_id = $1",
			"DELETE FROM product_variants WHERE business_id = $1",
			"DELETE FROM products WHERE business_id = $1",
			"DELETE FROM devices WHERE business_id = $1",
			"DELETE FROM locations WHERE business_id = $1",
			"DELETE FROM business_memberships WHERE business_id = $1",
		} {
			_, _ = tx.Exec(ctx, stmt, businessID)
		}
		_ = tx.Commit(ctx)
	}

	_, _ = conn.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1", ownerID)
	_, _ = conn.Exec(ctx, "DELETE FROM businesses WHERE id = $1", businessID)
	_, _ = conn.Exec(ctx, "DELETE FROM users WHERE id = $1", ownerID)
}
