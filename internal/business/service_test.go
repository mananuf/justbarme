package business_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/business"
	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/identity"
)

// fakeStorage is an in-memory storage.Provider, so these tests exercise
// business.Service's own logic (decode/re-encode, key generation, old-key
// cleanup) without depending on real object storage.
type fakeStorage struct {
	mu      sync.Mutex
	objects map[string][]byte
	deleted []string
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{objects: map[string][]byte{}}
}

func (f *fakeStorage) Put(_ context.Context, key, _ string, data []byte) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = data
	return f.URL(key), nil
}

func (f *fakeStorage) URL(key string) string {
	return "https://fake.local/" + key
}

func (f *fakeStorage) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
	f.deleted = append(f.deleted, key)
	return nil
}

func (f *fakeStorage) has(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.objects[key]
	return ok
}

func testServices(t *testing.T) (*business.Service, *fakeStorage, *identity.Service, *pgxpool.Pool) {
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
	storageFake := newFakeStorage()
	argon2 := config.Argon2{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1}
	return business.New(pool, storageFake), storageFake, identity.New(pool, argon2), pool
}

func uniqueName(prefix string) string {
	return prefix + "-" + uuid.New().String()
}

func newTenant(t *testing.T, ctx context.Context, identitySvc *identity.Service, pool *pgxpool.Pool) (ownerID, businessID uuid.UUID) {
	t.Helper()
	owner, err := identitySvc.CreateUser(ctx, uuid.New().String()+"@example.com", "", "Business Test Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	biz, _, _, err := identitySvc.CreateBusinessWithOwner(ctx, owner.ID, uniqueName("Test Bar"))
	if err != nil {
		t.Fatalf("CreateBusinessWithOwner: %v", err)
	}
	cleanupTenant(t, pool, owner.ID, biz.ID)
	return owner.ID, biz.ID
}

// cleanupTenant mirrors internal/expenses/service_test.go's RLS-aware
// pattern -- see that file's own comment for why a plain DELETE silently
// affects zero rows under a non-superuser JBM_DATABASE_URL role.
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

func onePixelPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 200, G: 30, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test PNG: %v", err)
	}
	return buf.Bytes()
}

func TestUpdateBrandingIsFullReplacement(t *testing.T) {
	svc, _, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID := newTenant(t, ctx, identitySvc, pool)

	updated, err := svc.UpdateBranding(ctx, ownerID, businessID, business.UpdateBrandingParams{
		Phone: "+2348012345678", Address: "12 Marina Road", ReceiptWording: "Thank you!",
		ReceiptFooter: "Come again", PaymentInstructions: "Pay at the bar",
	})
	if err != nil {
		t.Fatalf("UpdateBranding: %v", err)
	}
	if updated.Phone != "+2348012345678" || updated.Address != "12 Marina Road" {
		t.Fatalf("branding fields not applied: %+v", updated)
	}

	// A second call with an empty field genuinely clears it -- full
	// replacement, not a partial merge.
	updated, err = svc.UpdateBranding(ctx, ownerID, businessID, business.UpdateBrandingParams{
		Phone: "", Address: "12 Marina Road", ReceiptWording: "Thank you!",
		ReceiptFooter: "Come again", PaymentInstructions: "Pay at the bar",
	})
	if err != nil {
		t.Fatalf("UpdateBranding (clear phone): %v", err)
	}
	if updated.Phone != "" {
		t.Fatalf("expected phone to be cleared, got %q", updated.Phone)
	}
}

func TestUpdateLogoUploadsAndDeletesPrevious(t *testing.T) {
	svc, storageFake, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID := newTenant(t, ctx, identitySvc, pool)

	first, err := svc.UpdateLogo(ctx, ownerID, businessID, onePixelPNG(t))
	if err != nil {
		t.Fatalf("UpdateLogo (first): %v", err)
	}
	if first.LogoURL == "" {
		t.Fatal("expected a non-empty LogoURL after first upload")
	}

	second, err := svc.UpdateLogo(ctx, ownerID, businessID, onePixelPNG(t))
	if err != nil {
		t.Fatalf("UpdateLogo (second): %v", err)
	}
	if second.LogoURL == first.LogoURL {
		t.Fatal("expected a fresh object key/URL on re-upload, got the same one")
	}

	firstKey := first.LogoURL[len("https://fake.local/"):]
	if storageFake.has(firstKey) {
		t.Fatal("expected the first logo object to be deleted after replacement")
	}
}

func TestUpdateLogoRejectsInvalidImage(t *testing.T) {
	svc, _, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID := newTenant(t, ctx, identitySvc, pool)

	_, err := svc.UpdateLogo(ctx, ownerID, businessID, []byte("this is not an image"))
	if !errors.Is(err, business.ErrInvalidImage) {
		t.Fatalf("expected ErrInvalidImage, got %v", err)
	}
}

func TestUpdateLogoRejectsOversizedUpload(t *testing.T) {
	svc, _, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, businessID := newTenant(t, ctx, identitySvc, pool)

	oversized := make([]byte, 2<<20+1)
	_, err := svc.UpdateLogo(ctx, ownerID, businessID, oversized)
	if !errors.Is(err, business.ErrImageTooLarge) {
		t.Fatalf("expected ErrImageTooLarge, got %v", err)
	}
}

func TestGetReturnsNotFoundForUnknownBusiness(t *testing.T) {
	svc, _, identitySvc, pool := testServices(t)
	ctx := context.Background()
	ownerID, _ := newTenant(t, ctx, identitySvc, pool)

	_, err := svc.Get(ctx, ownerID, uuid.New())
	if !errors.Is(err, business.ErrBusinessNotFound) {
		t.Fatalf("expected ErrBusinessNotFound, got %v", err)
	}
}
