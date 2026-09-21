package catalogue_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/catalogue"
	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/identity"
)

// testServices returns a catalogue.Service and an identity.Service sharing
// one real PostgreSQL pool, or skips the test if JBM_DATABASE_URL is not
// set -- RLS and the price-history constraints below are architectural and
// must be exercised against real PostgreSQL, matching internal/identity and
// internal/store's own tests.
func testServices(t *testing.T) (*catalogue.Service, *identity.Service, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("JBM_DATABASE_URL")
	if url == "" {
		t.Skip("JBM_DATABASE_URL not set; skipping PostgreSQL-backed test")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("database not reachable: %v", err)
	}
	return catalogue.New(pool), identity.New(pool, config.Argon2{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1}), pool
}

func uniqueName(prefix string) string {
	return prefix + "-" + uuid.New().String()
}

// newTenant creates a fresh owner and business for one test, atomically,
// via internal/identity -- the same real path a real signup goes through.
func newTenant(t *testing.T, ctx context.Context, identitySvc *identity.Service, pool *pgxpool.Pool) (ownerID, businessID uuid.UUID) {
	t.Helper()
	owner, err := identitySvc.CreateUser(ctx, uuid.New().String()+"@example.com", "", "Catalogue Test Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	cleanupUser(t, pool, owner.ID)

	business, _, _, err := identitySvc.CreateBusinessWithOwner(ctx, owner.ID, uniqueName("Test Bar"))
	if err != nil {
		t.Fatalf("CreateBusinessWithOwner: %v", err)
	}
	cleanupCatalogue(t, pool, business.ID)
	return owner.ID, business.ID
}

// cleanupUser mirrors internal/identity/service_test.go's own helper: it is
// duplicated here (not imported, since it is unexported there) rather than
// exported from internal/identity purely for a second package's tests.
func cleanupUser(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		conn, err := pool.Acquire(context.Background())
		if err != nil {
			return
		}
		defer conn.Release()
		var businessIDs []uuid.UUID
		rows, err := conn.Query(context.Background(), "SELECT business_id FROM business_memberships WHERE user_id = $1", userID)
		if err == nil {
			for rows.Next() {
				var id uuid.UUID
				if rows.Scan(&id) == nil {
					businessIDs = append(businessIDs, id)
				}
			}
			rows.Close()
		}

		_, _ = conn.Exec(context.Background(), "DELETE FROM sessions WHERE user_id = $1", userID)
		_, _ = conn.Exec(context.Background(), "DELETE FROM devices WHERE user_id = $1", userID)
		_, _ = conn.Exec(context.Background(), "DELETE FROM business_memberships WHERE user_id = $1", userID)
		for _, businessID := range businessIDs {
			_, _ = conn.Exec(context.Background(), "DELETE FROM devices WHERE business_id = $1", businessID)
			_, _ = conn.Exec(context.Background(), "DELETE FROM locations WHERE business_id = $1", businessID)
			_, _ = conn.Exec(context.Background(), "DELETE FROM businesses WHERE id = $1", businessID)
		}
		_, _ = conn.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	})
}

// cleanupCatalogue removes businessID's catalogue rows before cleanupUser
// (registered earlier, so it runs later) deletes the business itself --
// t.Cleanup runs LIFO, and products/categories carry a foreign key to
// businesses that would otherwise block that later delete.
func cleanupCatalogue(t *testing.T, pool *pgxpool.Pool, businessID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		conn, err := pool.Acquire(context.Background())
		if err != nil {
			return
		}
		defer conn.Release()
		for _, stmt := range []string{
			"DELETE FROM product_prices WHERE business_id = $1",
			"DELETE FROM product_variants WHERE business_id = $1",
			"DELETE FROM products WHERE business_id = $1",
			"DELETE FROM categories WHERE business_id = $1",
		} {
			_, _ = conn.Exec(context.Background(), stmt, businessID)
		}
	})
}

// cleanupTemplate removes a platform catalogue template seeded by a test
// (by name, since that is the natural key SeedTemplates upserts on).
// catalogue_templates is platform-wide, not scoped to any one test's
// business, so without this a test that seeds its own template leaks a row
// into it permanently on every run.
func cleanupTemplate(t *testing.T, pool *pgxpool.Pool, templateName string) {
	t.Helper()
	t.Cleanup(func() {
		conn, err := pool.Acquire(context.Background())
		if err != nil {
			return
		}
		defer conn.Release()
		_, _ = conn.Exec(context.Background(),
			"DELETE FROM catalogue_template_variants WHERE template_id IN (SELECT id FROM catalogue_templates WHERE name = $1)", templateName)
		_, _ = conn.Exec(context.Background(), "DELETE FROM catalogue_templates WHERE name = $1", templateName)
	})
}

func TestCreateVariantSetsInitialPriceAtomically(t *testing.T) {
	svc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID := newTenant(t, ctx, identitySvc, pool)

	product, err := svc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, uniqueName("Guinness"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	variant, err := svc.CreateVariant(ctx, ownerID, businessID, product.ID, "50cl Bottle", 150000, true)
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}
	if variant.CurrentPrice.AmountKobo != 150000 {
		t.Fatalf("expected initial price 150000 kobo, got %d", variant.CurrentPrice.AmountKobo)
	}
	if variant.CurrentPrice.ValidTo != nil {
		t.Fatalf("expected the initial price to be current (ValidTo nil), got %v", variant.CurrentPrice.ValidTo)
	}

	history, err := svc.ListPriceHistory(ctx, ownerID, businessID, variant.ID)
	if err != nil {
		t.Fatalf("ListPriceHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected exactly one price row right after creation, got %d", len(history))
	}
}

func TestSetVariantPriceClosesOldRowAndInsertsNewAtomically(t *testing.T) {
	svc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID := newTenant(t, ctx, identitySvc, pool)

	product, err := svc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, uniqueName("Star"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant, err := svc.CreateVariant(ctx, ownerID, businessID, product.ID, "50cl Bottle", 90000, true)
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}

	newPrice, err := svc.SetVariantPrice(ctx, ownerID, businessID, variant.ID, 100000)
	if err != nil {
		t.Fatalf("SetVariantPrice: %v", err)
	}
	if newPrice.AmountKobo != 100000 {
		t.Fatalf("expected new price 100000 kobo, got %d", newPrice.AmountKobo)
	}
	if newPrice.ValidTo != nil {
		t.Fatalf("expected the new price to be current (ValidTo nil), got %v", newPrice.ValidTo)
	}

	history, err := svc.ListPriceHistory(ctx, ownerID, businessID, variant.ID)
	if err != nil {
		t.Fatalf("ListPriceHistory: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected two price rows (old + new) after one price change, got %d", len(history))
	}

	// history is ordered valid_from DESC: [0] is the new current row, [1]
	// is the closed old row -- the old row's amount must be untouched.
	current, old := history[0], history[1]
	if current.AmountKobo != 100000 || current.ValidTo != nil {
		t.Fatalf("expected history[0] to be the new current row, got %+v", current)
	}
	if old.AmountKobo != 90000 {
		t.Fatalf("expected the old price row's amount to remain 90000 kobo (never altered), got %d", old.AmountKobo)
	}
	if old.ValidTo == nil {
		t.Fatal("expected the old price row to be closed (ValidTo set), got nil")
	}

	// Exactly one current price must exist for the variant -- the acceptance
	// criterion this table's partial unique index and this method jointly
	// guarantee.
	currentCount := 0
	for _, p := range history {
		if p.ValidTo == nil {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Fatalf("expected exactly one current price, found %d", currentCount)
	}
}

// TestConcurrentSetVariantPriceNeverProducesTwoCurrentPrices exercises the
// atomicity guarantee directly under real concurrent writers, not just
// single-threaded logic. Two transactions racing to change the same
// variant's price may interleave either way, and both are legitimate:
//   - Postgres may serialize them entirely (the second's SELECT only starts
//     once the first has already committed), so both succeed and the
//     variant ends up with three price rows.
//   - Or they may genuinely overlap: both read the same "current" row
//     before either commits, the loser blocks on the winner's row lock,
//     and once it wakes, Postgres re-checks its UPDATE's WHERE clause
//     against the now-committed row (no longer valid_to IS NULL) and
//     leaves it untouched, then the loser's own INSERT of a second
//     "current" row collides with the database's partial unique index
//     (product_prices_one_current_per_variant) and the whole transaction
//     rolls back.
//
// What must hold under either interleaving -- and is NOT guaranteed by
// application code, only by that index -- is that exactly one current
// price exists afterward. Never zero, never two.
func TestConcurrentSetVariantPriceNeverProducesTwoCurrentPrices(t *testing.T) {
	svc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID := newTenant(t, ctx, identitySvc, pool)

	product, err := svc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, uniqueName("Concurrent Product"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant, err := svc.CreateVariant(ctx, ownerID, businessID, product.ID, "50cl Bottle", 100000, true)
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	race := func(amount int64) {
		<-start
		_, err := svc.SetVariantPrice(ctx, ownerID, businessID, variant.ID, amount)
		results <- err
	}
	go race(200000)
	go race(300000)
	close(start)

	successes := 0
	for i := 0; i < 2; i++ {
		if err := <-results; err == nil {
			successes++
		}
	}
	if successes == 0 {
		t.Fatal("expected at least one of the two concurrent price changes to succeed, got zero")
	}

	history, err := svc.ListPriceHistory(ctx, ownerID, businessID, variant.ID)
	if err != nil {
		t.Fatalf("ListPriceHistory: %v", err)
	}
	currentCount := 0
	for _, p := range history {
		if p.ValidTo == nil {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Fatalf("expected exactly one current price after the race, found %d across %d rows", currentCount, len(history))
	}
	// One successful row-close-and-insert adds exactly one row to the
	// history (the original row, now closed, plus the new current one) --
	// so the row count must match how many of the two calls actually won.
	if wantRows := successes + 1; len(history) != wantRows {
		t.Fatalf("expected %d price rows for %d successful change(s), got %d", wantRows, successes, len(history))
	}
}

func TestSetVariantPriceUnknownVariantFails(t *testing.T) {
	svc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID := newTenant(t, ctx, identitySvc, pool)

	if _, err := svc.SetVariantPrice(ctx, ownerID, businessID, uuid.New(), 100000); err != catalogue.ErrNoCurrentPrice {
		t.Fatalf("expected ErrNoCurrentPrice for a variant with no current price, got %v", err)
	}
}

func TestCategoryDuplicateNameRejected(t *testing.T) {
	svc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID := newTenant(t, ctx, identitySvc, pool)

	name := uniqueName("Drinks")
	if _, err := svc.CreateCategory(ctx, ownerID, businessID, name, 1); err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if _, err := svc.CreateCategory(ctx, ownerID, businessID, name, 2); err != catalogue.ErrCategoryNameTaken {
		t.Fatalf("expected ErrCategoryNameTaken for a duplicate category name, got %v", err)
	}
}

func TestProductDuplicateNameRejected(t *testing.T) {
	svc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID := newTenant(t, ctx, identitySvc, pool)

	name := uniqueName("Guinness")
	if _, err := svc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, name); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := svc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, name); err != catalogue.ErrProductNameTaken {
		t.Fatalf("expected ErrProductNameTaken for a duplicate product name, got %v", err)
	}
}

func TestVariantDuplicateNameRejected(t *testing.T) {
	svc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID := newTenant(t, ctx, identitySvc, pool)

	product, err := svc.CreateProduct(ctx, ownerID, businessID, uuid.Nil, uniqueName("Heineken"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := svc.CreateVariant(ctx, ownerID, businessID, product.ID, "50cl Bottle", 120000, true); err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}
	if _, err := svc.CreateVariant(ctx, ownerID, businessID, product.ID, "50cl Bottle", 125000, true); err != catalogue.ErrVariantNameTaken {
		t.Fatalf("expected ErrVariantNameTaken for a duplicate variant name on the same product, got %v", err)
	}
}

func TestDeactivateProductCategoryAndVariantPreservesRows(t *testing.T) {
	svc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID := newTenant(t, ctx, identitySvc, pool)

	category, err := svc.CreateCategory(ctx, ownerID, businessID, uniqueName("Beer"), 1)
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	product, err := svc.CreateProduct(ctx, ownerID, businessID, category.ID, uniqueName("Trophy"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant, err := svc.CreateVariant(ctx, ownerID, businessID, product.ID, "50cl Bottle", 80000, true)
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}

	deactivatedCategory, err := svc.UpdateCategory(ctx, ownerID, businessID, category.ID,
		catalogue.UpdateCategoryParams{Name: category.Name, SortOrder: category.SortOrder, Active: false})
	if err != nil {
		t.Fatalf("UpdateCategory: %v", err)
	}
	if deactivatedCategory.Active {
		t.Fatal("expected category to be inactive after deactivation")
	}

	deactivatedProduct, err := svc.UpdateProduct(ctx, ownerID, businessID, product.ID,
		catalogue.UpdateProductParams{Name: product.Name, CategoryID: category.ID, Active: false})
	if err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	if deactivatedProduct.Active {
		t.Fatal("expected product to be inactive after deactivation")
	}

	deactivatedVariant, err := svc.UpdateVariant(ctx, ownerID, businessID, variant.ID,
		catalogue.UpdateVariantParams{Name: variant.Name, Active: false})
	if err != nil {
		t.Fatalf("UpdateVariant: %v", err)
	}
	if deactivatedVariant.Active {
		t.Fatal("expected variant to be inactive after deactivation")
	}

	// Deactivation must never remove the row -- price history stays intact
	// and readable.
	if _, err := svc.GetProduct(ctx, ownerID, businessID, product.ID); err != nil {
		t.Fatalf("expected a deactivated product to remain readable, got %v", err)
	}
	history, err := svc.ListPriceHistory(ctx, ownerID, businessID, variant.ID)
	if err != nil {
		t.Fatalf("ListPriceHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected the deactivated variant's price history to remain intact, got %d rows", len(history))
	}
}

func TestCrossTenantCatalogueAccessBlocked(t *testing.T) {
	svc, identitySvc, pool := testServices(t)
	ctx := context.Background()

	_, businessA := newTenant(t, ctx, identitySvc, pool)
	ownerB, businessB := newTenant(t, ctx, identitySvc, pool)

	product, err := svc.CreateProduct(ctx, ownerB, businessB, uuid.Nil, uniqueName("B's Product"))
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	// Scoped to business A, ask directly (and correctly, by ID) for
	// business B's product. RLS -- not application logic -- must be what
	// blocks this.
	if _, err := svc.GetProduct(ctx, ownerB, businessA, product.ID); err != catalogue.ErrProductNotFound {
		t.Fatalf("expected ErrProductNotFound when reading business B's product under business A's context, got %v", err)
	}

	catalogueA, err := svc.ListCatalogue(ctx, ownerB, businessA)
	if err != nil {
		t.Fatalf("ListCatalogue for business A: %v", err)
	}
	for _, p := range catalogueA {
		if p.ID == product.ID {
			t.Fatal("business B's product leaked into business A's catalogue listing")
		}
	}
}

func TestApplyTemplatesCreatesBusinessOwnedRecordsAtomically(t *testing.T) {
	svc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID := newTenant(t, ctx, identitySvc, pool)

	templateName := uniqueName("Test Guinness")
	categoryName := uniqueName("Test Beer & Stout")
	cleanupTemplate(t, pool, templateName)
	if err := svc.SeedTemplates(ctx, []catalogue.TemplateSeed{
		{
			Name: templateName, CategoryName: categoryName, SortOrder: 1,
			Variants: []catalogue.TemplateVariantSeed{{Name: "50cl Bottle", SuggestedPriceKobo: 150000, SortOrder: 1}},
		},
	}); err != nil {
		t.Fatalf("SeedTemplates: %v", err)
	}

	templates, err := svc.ListTemplates(ctx)
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	var templateID uuid.UUID
	for _, tpl := range templates {
		if tpl.Name == templateName {
			templateID = tpl.ID
		}
	}
	if templateID == uuid.Nil {
		t.Fatal("seeded template not found in ListTemplates")
	}

	products, err := svc.ApplyTemplates(ctx, ownerID, businessID, []uuid.UUID{templateID})
	if err != nil {
		t.Fatalf("ApplyTemplates: %v", err)
	}
	if len(products) != 1 {
		t.Fatalf("expected one product created from the template, got %d", len(products))
	}
	got := products[0]
	if got.Name != templateName {
		t.Fatalf("expected product name %q, got %q", templateName, got.Name)
	}
	if len(got.Variants) != 1 || got.Variants[0].CurrentPrice.AmountKobo != 150000 {
		t.Fatalf("expected one variant priced at 150000 kobo, got %+v", got.Variants)
	}

	categories, err := svc.ListCategories(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	found := false
	for _, c := range categories {
		if c.Name == categoryName {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a category named %q to be created for the template's category", categoryName)
	}
}

func TestApplyTemplatesUnknownIDRollsBackEverything(t *testing.T) {
	svc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID := newTenant(t, ctx, identitySvc, pool)

	templateName := uniqueName("Test Rollback Product")
	cleanupTemplate(t, pool, templateName)
	if err := svc.SeedTemplates(ctx, []catalogue.TemplateSeed{
		{Name: templateName, CategoryName: uniqueName("Rollback Category"), SortOrder: 1},
	}); err != nil {
		t.Fatalf("SeedTemplates: %v", err)
	}
	templates, err := svc.ListTemplates(ctx)
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	var realID uuid.UUID
	for _, tpl := range templates {
		if tpl.Name == templateName {
			realID = tpl.ID
		}
	}
	if realID == uuid.Nil {
		t.Fatal("seeded template not found in ListTemplates")
	}

	// One real template ID plus one nonexistent one: the whole call must
	// fail, and the real one's product must not have been created either.
	_, err = svc.ApplyTemplates(ctx, ownerID, businessID, []uuid.UUID{realID, uuid.New()})
	if err != catalogue.ErrTemplateNotFound {
		t.Fatalf("expected ErrTemplateNotFound, got %v", err)
	}

	products, err := svc.ListCatalogue(ctx, ownerID, businessID)
	if err != nil {
		t.Fatalf("ListCatalogue: %v", err)
	}
	if len(products) != 0 {
		t.Fatalf("expected no products committed after a failed ApplyTemplates call, got %d", len(products))
	}
}
