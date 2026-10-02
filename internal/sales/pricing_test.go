package sales

import (
	"testing"

	"github.com/mananuf/justbarme/internal/store/sqlc"
)

func pack(quantity int32, priceKobo int64) sqlc.ProductVariantPricePack {
	return sqlc.ProductVariantPricePack{PackQuantity: quantity, PackPriceKobo: priceKobo}
}

// TestPriceForQuantitySnookerExample pins the exact example this feature
// was built for: 1 game = 300, 2 games for 500 (discounted), repeating
// indefinitely -- 3 = 500+300, 4 = 500+500, and so on.
func TestPriceForQuantitySnookerExample(t *testing.T) {
	packs := []sqlc.ProductVariantPricePack{pack(2, 50000)}
	cases := []struct {
		quantity int32
		want     int64
	}{
		{0, 0},
		{1, 30000},
		{2, 50000},
		{3, 80000},
		{4, 100000},
		{5, 130000},
		{6, 150000},
	}
	for _, c := range cases {
		if got := priceForQuantity(30000, packs, c.quantity); got != c.want {
			t.Errorf("priceForQuantity(30000, [2:50000], %d) = %d, want %d", c.quantity, got, c.want)
		}
	}
}

// TestPriceForQuantityNoPacksIsPlainLinear confirms a variant with no
// defined packs behaves exactly as flat per-unit pricing always has --
// the "purely additive, nothing changes for an existing product" claim.
func TestPriceForQuantityNoPacksIsPlainLinear(t *testing.T) {
	for q := int32(0); q <= 10; q++ {
		want := int64(q) * 150000
		if got := priceForQuantity(150000, nil, q); got != want {
			t.Errorf("priceForQuantity(150000, nil, %d) = %d, want %d", q, got, want)
		}
	}
}

// TestPriceForQuantityNegativeIsZero guards the "a removal's delta is
// computed as a difference of two calls" contract -- priceForQuantity
// itself only ever needs to answer for a non-negative count.
func TestPriceForQuantityNegativeIsZero(t *testing.T) {
	if got := priceForQuantity(30000, []sqlc.ProductVariantPricePack{pack(2, 50000)}, -3); got != 0 {
		t.Errorf("priceForQuantity with negative quantity = %d, want 0", got)
	}
}

// TestPriceForQuantityPicksTrueOptimumNotGreedyShortcut uses a pack shape
// where "always take the biggest pack first" picks the wrong combination:
// a pack of 3 at 100/unit (10000 total) and a pack of 5 at 80/unit (40000
// total, a better headline per-unit rate). For quantity 6, the greedy
// "biggest/best-rate pack first" strategy takes one 5-pack plus one
// leftover single unit (40000 + 10000 = 50000); the true cheapest
// combination is two 3-packs (10000 + 10000 = 20000). Only a real
// dynamic-programming minimum finds this; this is exactly the class of
// mistake priceForQuantity's own doc comment says a shortcut would make.
func TestPriceForQuantityPicksTrueOptimumNotGreedyShortcut(t *testing.T) {
	packs := []sqlc.ProductVariantPricePack{pack(3, 10000), pack(5, 40000)}
	got := priceForQuantity(10000, packs, 6)
	want := int64(20000) // two 3-packs, not one 5-pack + one leftover single (50000)
	if got != want {
		t.Errorf("priceForQuantity picked %d, want the true optimum %d (two 3-packs)", got, want)
	}
}
