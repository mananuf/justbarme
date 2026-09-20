package expenses_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/expenses"
	"github.com/mananuf/justbarme/internal/identity"
)

func testServices(t *testing.T) (*expenses.Service, *identity.Service, *pgxpool.Pool) {
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
	argon2 := config.Argon2{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1}
	return expenses.New(pool), identity.New(pool, argon2), pool
}

func uniqueName(prefix string) string {
	return prefix + "-" + uuid.New().String()
}

func newTenant(t *testing.T, ctx context.Context, identitySvc *identity.Service, pool *pgxpool.Pool) (ownerID, businessID, locationID uuid.UUID) {
	t.Helper()
	owner, err := identitySvc.CreateUser(ctx, uuid.New().String()+"@example.com", "", "Expenses Test Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	business, _, location, err := identitySvc.CreateBusinessWithOwner(ctx, owner.ID, uniqueName("Test Bar"))
	if err != nil {
		t.Fatalf("CreateBusinessWithOwner: %v", err)
	}
	cleanupTenant(t, pool, owner.ID, business.ID)
	return owner.ID, business.ID, location.ID
}

// cleanupTenant mirrors internal/sales/service_test.go's RLS-aware
// pattern: a raw DELETE on an RLS-scoped table only actually removes rows
// when the connecting role is an outright Postgres superuser, so this sets
// the same role and app.business_id a real request would.
func cleanupTenant(t *testing.T, pool *pgxpool.Pool, ownerID, businessID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
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
				"DELETE FROM expenses WHERE business_id = $1",
				"DELETE FROM expense_categories WHERE business_id = $1",
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
	})
}

func TestCreateCategoryRejectsDuplicateName(t *testing.T) {
	svc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, _ := newTenant(t, ctx, identitySvc, pool)

	name := uniqueName("Utilities")
	if _, err := svc.CreateCategory(ctx, ownerID, businessID, name); err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if _, err := svc.CreateCategory(ctx, ownerID, businessID, name); err != expenses.ErrCategoryNameTaken {
		t.Fatalf("expected ErrCategoryNameTaken for a duplicate name, got %v", err)
	}
}

func TestRecordExpensePostsImmediately(t *testing.T) {
	svc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID := newTenant(t, ctx, identitySvc, pool)

	category, err := svc.CreateCategory(ctx, ownerID, businessID, uniqueName("Supplies"))
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}

	expense, err := svc.RecordExpense(ctx, ownerID, businessID, locationID, category.ID, ownerID,
		uuid.New(), "Cleaning supplies", 500000, expenses.PaymentMethodCash, time.Now())
	if err != nil {
		t.Fatalf("RecordExpense: %v", err)
	}
	if expense.AmountKobo != 500000 {
		t.Fatalf("expected AmountKobo=500000, got %d", expense.AmountKobo)
	}
	if expense.ReversalOf != uuid.Nil {
		t.Fatalf("expected a fresh expense to have no ReversalOf, got %v", expense.ReversalOf)
	}

	list, err := svc.ListExpenses(ctx, ownerID, businessID, 10)
	if err != nil {
		t.Fatalf("ListExpenses: %v", err)
	}
	if len(list) != 1 || list[0].ID != expense.ID {
		t.Fatalf("expected the recorded expense to appear in ListExpenses, got %+v", list)
	}
	if list[0].CategoryName != category.Name {
		t.Fatalf("expected category name %q, got %q", category.Name, list[0].CategoryName)
	}
}

func TestRecordExpenseIsIdempotent(t *testing.T) {
	svc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID := newTenant(t, ctx, identitySvc, pool)

	category, err := svc.CreateCategory(ctx, ownerID, businessID, uniqueName("Supplies"))
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	idempotencyKey := uuid.New()

	first, err := svc.RecordExpense(ctx, ownerID, businessID, locationID, category.ID, ownerID,
		idempotencyKey, "Cleaning supplies", 500000, expenses.PaymentMethodCash, time.Now())
	if err != nil {
		t.Fatalf("RecordExpense (first): %v", err)
	}
	second, err := svc.RecordExpense(ctx, ownerID, businessID, locationID, category.ID, ownerID,
		idempotencyKey, "Cleaning supplies", 500000, expenses.PaymentMethodCash, time.Now())
	if err != nil {
		t.Fatalf("RecordExpense (retry): %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected a retried idempotency key to return the same expense, got %v and %v", first.ID, second.ID)
	}

	list, err := svc.ListExpenses(ctx, ownerID, businessID, 10)
	if err != nil {
		t.Fatalf("ListExpenses: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected exactly one expense posted despite the retry, got %d", len(list))
	}
}

func TestReverseExpenseCreatesOppositeRecord(t *testing.T) {
	svc, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID, locationID := newTenant(t, ctx, identitySvc, pool)

	category, err := svc.CreateCategory(ctx, ownerID, businessID, uniqueName("Supplies"))
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	original, err := svc.RecordExpense(ctx, ownerID, businessID, locationID, category.ID, ownerID,
		uuid.New(), "Cleaning supplies", 500000, expenses.PaymentMethodCash, time.Now())
	if err != nil {
		t.Fatalf("RecordExpense: %v", err)
	}

	reversal, err := svc.ReverseExpense(ctx, ownerID, businessID, original.ID, ownerID)
	if err != nil {
		t.Fatalf("ReverseExpense: %v", err)
	}
	if reversal.ReversalOf != original.ID {
		t.Fatalf("expected reversal.ReversalOf=%v, got %v", original.ID, reversal.ReversalOf)
	}
	if reversal.AmountKobo != -original.AmountKobo {
		t.Fatalf("expected reversal AmountKobo=%d, got %d", -original.AmountKobo, reversal.AmountKobo)
	}

	summary, err := svc.SumExpensesSince(ctx, ownerID, businessID, original.OccurredAt.Add(-time.Hour))
	if err != nil {
		t.Fatalf("SumExpensesSince: %v", err)
	}
	if summary.TotalKobo != 0 {
		t.Fatalf("expected net total 0 after a full reversal, got %d", summary.TotalKobo)
	}
	if summary.Count != 1 {
		t.Fatalf("expected count to exclude the reversal row, got %d", summary.Count)
	}

	if _, err := svc.ReverseExpense(ctx, ownerID, businessID, original.ID, ownerID); err != expenses.ErrAlreadyReversed {
		t.Fatalf("expected ErrAlreadyReversed on a second reversal attempt, got %v", err)
	}
}
