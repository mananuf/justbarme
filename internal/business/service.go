package business

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // decode-only, registers "jpeg" with image.Decode
	"image/png"

	_ "golang.org/x/image/webp" // decode-only, registers "webp" with image.Decode

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/storage"
	"github.com/mananuf/justbarme/internal/store"
	"github.com/mananuf/justbarme/internal/store/sqlc"
)

// maxLogoUploadBytes bounds the raw upload before it's even decoded --
// docs/PHASE_PILOT_RELEASE.md §3's 2MB limit.
const maxLogoUploadBytes = 2 << 20

type Service struct {
	pool    *pgxpool.Pool
	storage storage.Provider
}

func New(pool *pgxpool.Pool, storageProvider storage.Provider) *Service {
	return &Service{pool: pool, storage: storageProvider}
}

func newID() (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("generate id: %w", err)
	}
	return id, nil
}

func (s *Service) getRaw(ctx context.Context, userID, businessID uuid.UUID) (sqlc.Business, error) {
	var found sqlc.Business
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetBusinessByID(ctx, businessID)
		if err != nil {
			return err
		}
		found = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.Business{}, ErrBusinessNotFound
		}
		return sqlc.Business{}, fmt.Errorf("look up business: %w", err)
	}
	return found, nil
}

func (s *Service) Get(ctx context.Context, userID, businessID uuid.UUID) (Business, error) {
	found, err := s.getRaw(ctx, userID, businessID)
	if err != nil {
		return Business{}, err
	}
	return toBusiness(found, s.storage), nil
}

// UpdateBranding replaces the mutable branding fields in one statement --
// see UpdateBrandingParams's own doc comment for why this is a full
// replacement, not a partial merge.
func (s *Service) UpdateBranding(ctx context.Context, userID, businessID uuid.UUID, params UpdateBrandingParams) (Business, error) {
	var updated sqlc.Business
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.UpdateBusinessBranding(ctx, sqlc.UpdateBusinessBrandingParams{
			ID:                  businessID,
			Phone:               pgText(params.Phone),
			Address:             pgText(params.Address),
			ReceiptWording:      pgText(params.ReceiptWording),
			ReceiptFooter:       pgText(params.ReceiptFooter),
			PaymentInstructions: pgText(params.PaymentInstructions),
		})
		if err != nil {
			return err
		}
		updated = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Business{}, ErrBusinessNotFound
		}
		return Business{}, fmt.Errorf("update business branding: %w", err)
	}
	return toBusiness(updated, s.storage), nil
}

// UpdateLogo validates, decodes, and re-encodes an uploaded image (never
// trusting the caller's declared content type alone -- decoding is what
// actually proves it's a real PNG/JPEG/WebP image, and re-encoding strips
// anything else the original file bytes might carry), uploads it under a
// fresh opaque key, and points businesses.logo_object_key at it. The
// previous object (if any) is deleted afterward on a best-effort basis --
// an orphaned object left behind by a failed delete is a harmless storage
// cost, not a correctness problem, so it does not fail the request.
func (s *Service) UpdateLogo(ctx context.Context, userID, businessID uuid.UUID, data []byte) (Business, error) {
	if len(data) > maxLogoUploadBytes {
		return Business{}, ErrImageTooLarge
	}

	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Business{}, fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, decoded); err != nil {
		return Business{}, fmt.Errorf("re-encode logo: %w", err)
	}

	current, err := s.getRaw(ctx, userID, businessID)
	if err != nil {
		return Business{}, err
	}
	oldKey := toText(current.LogoObjectKey)

	objectID, err := newID()
	if err != nil {
		return Business{}, err
	}
	key := "logos/" + businessID.String() + "/" + objectID.String() + ".png"

	if _, err := s.storage.Put(ctx, key, "image/png", encoded.Bytes()); err != nil {
		return Business{}, fmt.Errorf("upload logo: %w", err)
	}

	var updated sqlc.Business
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.UpdateBusinessLogo(ctx, sqlc.UpdateBusinessLogoParams{ID: businessID, LogoObjectKey: pgText(key)})
		if err != nil {
			return err
		}
		updated = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Business{}, ErrBusinessNotFound
		}
		return Business{}, fmt.Errorf("update business logo: %w", err)
	}

	if oldKey != "" && oldKey != key {
		_ = s.storage.Delete(ctx, oldKey)
	}

	return toBusiness(updated, s.storage), nil
}
