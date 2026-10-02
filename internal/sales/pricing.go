package sales

import "github.com/mananuf/justbarme/internal/store/sqlc"

// priceForQuantity computes the cheapest possible total cost of exactly
// quantity units of a variant, given its flat single-unit price and any
// currently-defined multi-buy packs (each usable any number of times, in
// any combination) -- docs/PHASE_MULTIBUY_PRICING.md's generalization of
// "2 games for 500, otherwise 300 each, repeating indefinitely" to any
// pack shape a different service might define.
//
// This is a genuine dynamic-programming minimum, not an "always use the
// biggest pack first" shortcut -- the shortcut is not guaranteed optimal
// once pack sizes/prices stop being as simple as a single 2-for-X deal
// (the classic coin-change counterexample), and correctness here is
// non-negotiable: this is what actually gets charged. cost[k] is the
// cheapest way to cover exactly k units; it is always at least as cheap
// as buying k single units at basePriceKobo, since that is always one of
// the options considered at every step.
//
// quantity <= 0 returns 0. This package's own duplication of pricing math
// that could in principle live in internal/catalogue mirrors a convention
// already established for negative-inventory detection (see CLAUDE.md's
// Phase 7 section: "duplicated by design, not shared... per this
// codebase's each feature package stays self-contained convention") --
// internal/sales reads product_variant_price_packs directly through the
// shared sqlc layer rather than calling into catalogue.Service.
func priceForQuantity(basePriceKobo int64, packs []sqlc.ProductVariantPricePack, quantity int32) int64 {
	if quantity <= 0 {
		return 0
	}
	cost := make([]int64, quantity+1)
	for k := int32(1); k <= quantity; k++ {
		best := cost[k-1] + basePriceKobo // always available: one more at the plain unit price
		for _, p := range packs {
			if p.PackQuantity <= k {
				if candidate := cost[k-p.PackQuantity] + p.PackPriceKobo; candidate < best {
					best = candidate
				}
			}
		}
		cost[k] = best
	}
	return cost[quantity]
}
