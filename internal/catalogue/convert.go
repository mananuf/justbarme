package catalogue

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mananuf/justbarme/internal/store/sqlc"
)

func pgUUID(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

func toUUID(id pgtype.UUID) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}
	return uuid.UUID(id.Bytes)
}

func toTime(t pgtype.Timestamptz) time.Time {
	return t.Time
}

func pgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func toTimePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	out := t.Time
	return &out
}

func toTemplate(t sqlc.CatalogueTemplate) Template {
	return Template{ID: t.ID, Name: t.Name, CategoryName: t.CategoryName, SortOrder: t.SortOrder}
}

func toTemplateVariant(v sqlc.CatalogueTemplateVariant) TemplateVariant {
	return TemplateVariant{
		ID:                 v.ID,
		TemplateID:         v.TemplateID,
		Name:               v.Name,
		SuggestedPriceKobo: v.SuggestedPriceKobo,
		SortOrder:          v.SortOrder,
	}
}

func toCategory(c sqlc.Category) Category {
	return Category{ID: c.ID, BusinessID: c.BusinessID, Name: c.Name, SortOrder: c.SortOrder, Active: c.Active}
}

func toProduct(p sqlc.Product) Product {
	return Product{
		ID:         p.ID,
		BusinessID: p.BusinessID,
		CategoryID: toUUID(p.CategoryID),
		Name:       p.Name,
		Active:     p.Active,
		TemplateID: toUUID(p.TemplateID),
	}
}

func toVariant(v sqlc.ProductVariant) Variant {
	return Variant{
		ID: v.ID, BusinessID: v.BusinessID, ProductID: v.ProductID, Name: v.Name, Active: v.Active,
		TracksInventory: v.TracksInventory,
	}
}

func toPrice(p sqlc.ProductPrice) Price {
	return Price{
		ID:         p.ID,
		BusinessID: p.BusinessID,
		VariantID:  p.VariantID,
		AmountKobo: p.AmountKobo,
		ValidFrom:  toTime(p.ValidFrom),
		ValidTo:    toTimePtr(p.ValidTo),
		CreatedBy:  p.CreatedBy,
	}
}
