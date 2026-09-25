package catalogue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/store"
	"github.com/mananuf/justbarme/internal/store/sqlc"
)

type Service struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func newID() (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("generate id: %w", err)
	}
	return id, nil
}

// TemplateSeed is one platform catalogue template to seed, with its
// suggested variants and starting prices.
type TemplateSeed struct {
	Name         string
	CategoryName string
	SortOrder    int32
	Variants     []TemplateVariantSeed
}

type TemplateVariantSeed struct {
	Name               string
	SuggestedPriceKobo int64
	SortOrder          int32
}

// SeedTemplates idempotently upserts the platform catalogue templates
// (keyed by name) and their suggested variants (keyed by template + name),
// so running it again with an updated list is always safe to repeat -- see
// docs/IMPLEMENTATION_PLAN.md Phase 3's "repeatable seed mechanism".
func (s *Service) SeedTemplates(ctx context.Context, seeds []TemplateSeed) error {
	return store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		for _, seed := range seeds {
			templateID, err := newID()
			if err != nil {
				return err
			}
			template, err := q.UpsertCatalogueTemplate(ctx, sqlc.UpsertCatalogueTemplateParams{
				ID: templateID, Name: seed.Name, CategoryName: seed.CategoryName, SortOrder: seed.SortOrder,
			})
			if err != nil {
				return fmt.Errorf("upsert template %q: %w", seed.Name, err)
			}
			for _, v := range seed.Variants {
				variantID, err := newID()
				if err != nil {
					return err
				}
				if _, err := q.UpsertCatalogueTemplateVariant(ctx, sqlc.UpsertCatalogueTemplateVariantParams{
					ID: variantID, TemplateID: template.ID, Name: v.Name,
					SuggestedPriceKobo: v.SuggestedPriceKobo, SortOrder: v.SortOrder,
				}); err != nil {
					return fmt.Errorf("upsert template variant %q/%q: %w", seed.Name, v.Name, err)
				}
			}
		}
		return nil
	})
}

// ListTemplates returns every platform catalogue template with its
// suggested variants, for the "choose what you sell" onboarding step.
func (s *Service) ListTemplates(ctx context.Context) ([]TemplateWithVariants, error) {
	var (
		templates []sqlc.CatalogueTemplate
		variants  []sqlc.CatalogueTemplateVariant
	)
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		templates, err = q.ListCatalogueTemplates(ctx)
		if err != nil {
			return err
		}
		ids := make([]uuid.UUID, len(templates))
		for i, t := range templates {
			ids[i] = t.ID
		}
		variants, err = q.ListCatalogueTemplateVariantsByTemplateIDs(ctx, ids)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list catalogue templates: %w", err)
	}

	byTemplate := make(map[uuid.UUID][]TemplateVariant, len(templates))
	for _, v := range variants {
		byTemplate[v.TemplateID] = append(byTemplate[v.TemplateID], toTemplateVariant(v))
	}
	out := make([]TemplateWithVariants, 0, len(templates))
	for _, t := range templates {
		out = append(out, TemplateWithVariants{Template: toTemplate(t), Variants: byTemplate[t.ID]})
	}
	return out, nil
}

// getOrCreateCategory matches categoryName to an existing business category
// (case-sensitive, same as the unique constraint) or creates one. Called
// only from within an existing WithTenant transaction.
func getOrCreateCategory(ctx context.Context, q *sqlc.Queries, businessID uuid.UUID, name string) (uuid.UUID, error) {
	existing, err := q.GetCategoryByName(ctx, sqlc.GetCategoryByNameParams{BusinessID: businessID, Name: name})
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}
	id, err := newID()
	if err != nil {
		return uuid.Nil, err
	}
	created, err := q.CreateCategory(ctx, sqlc.CreateCategoryParams{ID: id, BusinessID: businessID, Name: name})
	if err != nil {
		return uuid.Nil, err
	}
	return created.ID, nil
}

// ApplyTemplates copies the selected platform templates' products,
// variants, and starting prices into businessID's own catalogue in one
// transaction -- see docs/ARCHITECTURE.md §8.2 and the Phase 3 acceptance
// criterion "Selecting a template creates business-owned editable records."
// Each template's category is matched by name to an existing business
// category or created. It returns ErrTemplateNotFound if any requested ID
// does not exist, and the whole selection is rolled back together with it.
func (s *Service) ApplyTemplates(ctx context.Context, userID, businessID uuid.UUID, templateIDs []uuid.UUID) ([]ProductWithVariants, error) {
	var created []ProductWithVariants
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		templates, err := q.ListCatalogueTemplatesByIDs(ctx, templateIDs)
		if err != nil {
			return err
		}
		if len(templates) != len(templateIDs) {
			return ErrTemplateNotFound
		}

		ids := make([]uuid.UUID, len(templates))
		for i, t := range templates {
			ids[i] = t.ID
		}
		templateVariants, err := q.ListCatalogueTemplateVariantsByTemplateIDs(ctx, ids)
		if err != nil {
			return err
		}
		variantsByTemplate := make(map[uuid.UUID][]sqlc.CatalogueTemplateVariant, len(templates))
		for _, v := range templateVariants {
			variantsByTemplate[v.TemplateID] = append(variantsByTemplate[v.TemplateID], v)
		}

		categoryIDByName := make(map[string]uuid.UUID, len(templates))
		for _, t := range templates {
			categoryID, ok := categoryIDByName[t.CategoryName]
			if !ok {
				categoryID, err = getOrCreateCategory(ctx, q, businessID, t.CategoryName)
				if err != nil {
					return fmt.Errorf("resolve category %q: %w", t.CategoryName, err)
				}
				categoryIDByName[t.CategoryName] = categoryID
			}

			productID, err := newID()
			if err != nil {
				return err
			}
			product, err := q.CreateProduct(ctx, sqlc.CreateProductParams{
				ID: productID, BusinessID: businessID, CategoryID: pgUUID(categoryID), Name: t.Name,
				TemplateID: pgUUID(t.ID),
			})
			if err != nil {
				return fmt.Errorf("create product %q from template: %w", t.Name, err)
			}

			out := ProductWithVariants{Product: toProduct(product)}
			for _, tv := range variantsByTemplate[t.ID] {
				variantID, err := newID()
				if err != nil {
					return err
				}
				variant, err := q.CreateVariant(ctx, sqlc.CreateVariantParams{
					ID: variantID, BusinessID: businessID, ProductID: product.ID, Name: tv.Name, TracksInventory: true,
				})
				if err != nil {
					return fmt.Errorf("create variant %q from template: %w", tv.Name, err)
				}
				priceID, err := newID()
				if err != nil {
					return err
				}
				price, err := q.CreatePrice(ctx, sqlc.CreatePriceParams{
					ID: priceID, BusinessID: businessID, VariantID: variant.ID,
					AmountKobo: tv.SuggestedPriceKobo, ValidFrom: pgTimestamptz(time.Now()), CreatedBy: userID,
				})
				if err != nil {
					return fmt.Errorf("set starting price for %q: %w", tv.Name, err)
				}
				out.Variants = append(out.Variants, VariantWithPrice{Variant: toVariant(variant), CurrentPrice: toPrice(price)})
			}
			created = append(created, out)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// CreateCategory adds a new business-owned category. It returns
// ErrCategoryNameTaken if name is already used in this business.
func (s *Service) CreateCategory(ctx context.Context, userID, businessID uuid.UUID, name string, sortOrder int32) (Category, error) {
	id, err := newID()
	if err != nil {
		return Category{}, err
	}
	var created sqlc.Category
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.CreateCategory(ctx, sqlc.CreateCategoryParams{ID: id, BusinessID: businessID, Name: name, SortOrder: sortOrder})
		if err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		if pgErrorCode(err) == pgUniqueViolation {
			return Category{}, ErrCategoryNameTaken
		}
		return Category{}, fmt.Errorf("create category: %w", err)
	}
	return toCategory(created), nil
}

func (s *Service) ListCategories(ctx context.Context, userID, businessID uuid.UUID) ([]Category, error) {
	var rows []sqlc.Category
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		rows, err = q.ListCategories(ctx, businessID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	out := make([]Category, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCategory(r))
	}
	return out, nil
}

// UpdateCategoryParams is a full replacement of the mutable fields: callers
// supply the category's current values for any field they are not
// changing, matching PATCH /api/v1/... semantics once the HTTP handler for
// this exists.
type UpdateCategoryParams struct {
	Name      string
	SortOrder int32
	Active    bool
}

// UpdateCategory renames, reorders, and/or (de)activates categoryID. A
// category is never hard-deleted -- deactivating it is the only removal
// path, so products referencing it never dangle.
func (s *Service) UpdateCategory(ctx context.Context, userID, businessID, categoryID uuid.UUID, params UpdateCategoryParams) (Category, error) {
	var updated sqlc.Category
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.UpdateCategory(ctx, sqlc.UpdateCategoryParams{
			BusinessID: businessID, ID: categoryID, Name: params.Name, SortOrder: params.SortOrder, Active: params.Active,
		})
		if err != nil {
			return err
		}
		updated = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Category{}, ErrCategoryNotFound
		}
		if pgErrorCode(err) == pgUniqueViolation {
			return Category{}, ErrCategoryNameTaken
		}
		return Category{}, fmt.Errorf("update category: %w", err)
	}
	return toCategory(updated), nil
}

// CreateProduct adds a new business-owned product. categoryID may be
// uuid.Nil for an uncategorized product. It returns ErrProductNameTaken if
// name is already used in this business.
func (s *Service) CreateProduct(ctx context.Context, userID, businessID, categoryID uuid.UUID, name string) (Product, error) {
	id, err := newID()
	if err != nil {
		return Product{}, err
	}
	var created sqlc.Product
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		// A product typed in by hand whose name exactly matches a platform
		// template is still that brand -- link it, so an owner who didn't
		// pick from the suggestions isn't silently invisible to any
		// brand-level rollup. No match (or any lookup miss) just means a
		// genuinely custom product with no link.
		templateID := uuid.Nil
		if found, err := q.FindCatalogueTemplateIDByName(ctx, name); err == nil {
			templateID = found
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		row, err := q.CreateProduct(ctx, sqlc.CreateProductParams{
			ID: id, BusinessID: businessID, CategoryID: pgUUID(categoryID), Name: name, TemplateID: pgUUID(templateID),
		})
		if err != nil {
			return err
		}
		created = row
		return nil
	})
	if err != nil {
		if pgErrorCode(err) == pgUniqueViolation {
			return Product{}, ErrProductNameTaken
		}
		return Product{}, fmt.Errorf("create product: %w", err)
	}
	return toProduct(created), nil
}

func (s *Service) GetProduct(ctx context.Context, userID, businessID, productID uuid.UUID) (Product, error) {
	var found sqlc.Product
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetProductByID(ctx, sqlc.GetProductByIDParams{BusinessID: businessID, ID: productID})
		if err != nil {
			return err
		}
		found = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Product{}, ErrProductNotFound
		}
		return Product{}, fmt.Errorf("get product: %w", err)
	}
	return toProduct(found), nil
}

// UpdateProductParams is a full replacement of the mutable fields, matching
// UpdateCategoryParams's PATCH-shaped convention. CategoryID of uuid.Nil
// clears the product's category.
type UpdateProductParams struct {
	Name       string
	CategoryID uuid.UUID
	Active     bool
}

// UpdateProduct renames, recategorizes, and/or (de)activates productID. A
// product is never hard-deleted -- deactivating it is the only removal
// path, so its variants and price history never dangle.
func (s *Service) UpdateProduct(ctx context.Context, userID, businessID, productID uuid.UUID, params UpdateProductParams) (Product, error) {
	var updated sqlc.Product
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.UpdateProduct(ctx, sqlc.UpdateProductParams{
			BusinessID: businessID, ID: productID, Name: params.Name, CategoryID: pgUUID(params.CategoryID), Active: params.Active,
		})
		if err != nil {
			return err
		}
		updated = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Product{}, ErrProductNotFound
		}
		if pgErrorCode(err) == pgUniqueViolation {
			return Product{}, ErrProductNameTaken
		}
		return Product{}, fmt.Errorf("update product: %w", err)
	}
	return toProduct(updated), nil
}

// CreateVariant adds a variant to productID with an initial price, both in
// one transaction -- a variant is never without a current price from the
// moment it exists (Phase 3 acceptance criterion "Exactly one current
// price exists per variant"). It returns ErrVariantNameTaken if name is
// already used for this product.
func (s *Service) CreateVariant(ctx context.Context, userID, businessID, productID uuid.UUID, name string, initialPriceKobo int64, tracksInventory bool) (VariantWithPrice, error) {
	variantID, err := newID()
	if err != nil {
		return VariantWithPrice{}, err
	}
	priceID, err := newID()
	if err != nil {
		return VariantWithPrice{}, err
	}

	var (
		variant sqlc.ProductVariant
		price   sqlc.ProductPrice
	)
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		variant, err = q.CreateVariant(ctx, sqlc.CreateVariantParams{
			ID: variantID, BusinessID: businessID, ProductID: productID, Name: name, TracksInventory: tracksInventory,
		})
		if err != nil {
			return fmt.Errorf("create variant: %w", err)
		}
		price, err = q.CreatePrice(ctx, sqlc.CreatePriceParams{
			ID: priceID, BusinessID: businessID, VariantID: variant.ID,
			AmountKobo: initialPriceKobo, ValidFrom: pgTimestamptz(time.Now()), CreatedBy: userID,
		})
		if err != nil {
			return fmt.Errorf("set initial price: %w", err)
		}
		return nil
	})
	if err != nil {
		if pgErrorCode(err) == pgUniqueViolation {
			return VariantWithPrice{}, ErrVariantNameTaken
		}
		return VariantWithPrice{}, err
	}
	return VariantWithPrice{Variant: toVariant(variant), CurrentPrice: toPrice(price)}, nil
}

type UpdateVariantParams struct {
	Name   string
	Active bool
	// TracksInventory -- see Variant's own doc comment. Full-replacement,
	// matching this codebase's PATCH convention: the caller resends the
	// whole mutable set, not a partial merge.
	TracksInventory bool
}

// UpdateVariant renames, (de)activates, and/or changes the
// tracks-inventory flag of variantID. A variant is never hard-deleted --
// deactivating it is the only removal path, so its price history never
// dangles.
func (s *Service) UpdateVariant(ctx context.Context, userID, businessID, variantID uuid.UUID, params UpdateVariantParams) (Variant, error) {
	var updated sqlc.ProductVariant
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.UpdateVariant(ctx, sqlc.UpdateVariantParams{
			BusinessID: businessID, ID: variantID, Name: params.Name, Active: params.Active,
			TracksInventory: params.TracksInventory,
		})
		if err != nil {
			return err
		}
		updated = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Variant{}, ErrVariantNotFound
		}
		if pgErrorCode(err) == pgUniqueViolation {
			return Variant{}, ErrVariantNameTaken
		}
		return Variant{}, fmt.Errorf("update variant: %w", err)
	}
	return toVariant(updated), nil
}

// SetVariantPrice closes variantID's current price row and inserts a new
// one, atomically, in the same transaction -- see docs/ARCHITECTURE.md §13
// "One active price per variant" and the Phase 3 acceptance criterion
// "Price change never alters an old price row." Returns ErrNoCurrentPrice
// if the variant has no current price to close (should not happen given
// CreateVariant's invariant, but checked rather than assumed).
func (s *Service) SetVariantPrice(ctx context.Context, userID, businessID, variantID uuid.UUID, amountKobo int64) (Price, error) {
	newPriceID, err := newID()
	if err != nil {
		return Price{}, err
	}

	var created sqlc.ProductPrice
	err = store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		if _, err := q.GetCurrentPrice(ctx, sqlc.GetCurrentPriceParams{BusinessID: businessID, VariantID: variantID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNoCurrentPrice
			}
			return err
		}

		now := time.Now()
		if err := q.CloseCurrentPrice(ctx, sqlc.CloseCurrentPriceParams{
			BusinessID: businessID, VariantID: variantID, ValidTo: pgTimestamptz(now),
		}); err != nil {
			return fmt.Errorf("close current price: %w", err)
		}

		var err error
		created, err = q.CreatePrice(ctx, sqlc.CreatePriceParams{
			ID: newPriceID, BusinessID: businessID, VariantID: variantID,
			AmountKobo: amountKobo, ValidFrom: pgTimestamptz(now), CreatedBy: userID,
		})
		if err != nil {
			return fmt.Errorf("insert new price: %w", err)
		}
		return nil
	})
	if err != nil {
		return Price{}, err
	}
	return toPrice(created), nil
}

func (s *Service) ListPriceHistory(ctx context.Context, userID, businessID, variantID uuid.UUID) ([]Price, error) {
	var rows []sqlc.ProductPrice
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		rows, err = q.ListPriceHistory(ctx, sqlc.ListPriceHistoryParams{BusinessID: businessID, VariantID: variantID})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list price history: %w", err)
	}
	out := make([]Price, 0, len(rows))
	for _, r := range rows {
		out = append(out, toPrice(r))
	}
	return out, nil
}

// ListCatalogue assembles every product, its variants, and each variant's
// current price for businessID -- the shape both an owner's catalogue
// management screen and a device's offline replication read need.
func (s *Service) ListCatalogue(ctx context.Context, userID, businessID uuid.UUID) ([]ProductWithVariants, error) {
	var (
		products []sqlc.Product
		variants []sqlc.ProductVariant
		prices   []sqlc.ProductPrice
	)
	err := store.WithTenant(ctx, s.pool, userID, businessID, func(ctx context.Context, q *sqlc.Queries) error {
		var err error
		products, err = q.ListProducts(ctx, businessID)
		if err != nil {
			return err
		}
		variants, err = q.ListVariantsByBusiness(ctx, businessID)
		if err != nil {
			return err
		}
		prices, err = q.ListCurrentPricesByBusiness(ctx, businessID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list catalogue: %w", err)
	}

	priceByVariant := make(map[uuid.UUID]sqlc.ProductPrice, len(prices))
	for _, p := range prices {
		priceByVariant[p.VariantID] = p
	}
	variantsByProduct := make(map[uuid.UUID][]VariantWithPrice, len(variants))
	for _, v := range variants {
		vp := VariantWithPrice{Variant: toVariant(v)}
		if p, ok := priceByVariant[v.ID]; ok {
			vp.CurrentPrice = toPrice(p)
		}
		variantsByProduct[v.ProductID] = append(variantsByProduct[v.ProductID], vp)
	}

	out := make([]ProductWithVariants, 0, len(products))
	for _, p := range products {
		out = append(out, ProductWithVariants{Product: toProduct(p), Variants: variantsByProduct[p.ID]})
	}
	return out, nil
}
