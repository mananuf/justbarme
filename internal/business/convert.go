package business

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mananuf/justbarme/internal/storage"
	"github.com/mananuf/justbarme/internal/store/sqlc"
)

func pgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func toText(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

// toBusiness converts a raw row into the domain type, deriving LogoURL from
// the stored object key via provider.URL rather than storing a URL
// directly -- see Business's own doc comment. provider is nil-safe: a
// business with no logo yet never calls it.
func toBusiness(b sqlc.Business, provider storage.Provider) Business {
	out := Business{
		ID:                  b.ID,
		Name:                b.Name,
		Timezone:            b.Timezone,
		Currency:            b.Currency,
		Phone:               toText(b.Phone),
		Address:             toText(b.Address),
		ReceiptWording:      toText(b.ReceiptWording),
		ReceiptFooter:       toText(b.ReceiptFooter),
		PaymentInstructions: toText(b.PaymentInstructions),
		Status:              b.Status,
		UpdatedAt:           b.UpdatedAt.Time,
	}
	if key := toText(b.LogoObjectKey); key != "" && provider != nil {
		out.LogoURL = provider.URL(key)
	}
	return out
}
