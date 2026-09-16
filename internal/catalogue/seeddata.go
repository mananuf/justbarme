package catalogue

// NigerianBarCatalogue is the initial platform catalogue template list --
// common products stocked by a Nigerian bar, grouped into suggested
// categories, each with one suggested variant and starting price. Prices
// are estimates an owner is expected to adjust after applying a template
// (docs/IMPLEMENTATION_PLAN.md Phase 3 "Seed the initial Nigerian product
// catalogue"). Passed to Service.SeedTemplates, which upserts by name, so
// this list can be edited and re-seeded safely at any time.
//
// A SuggestedPriceKobo of 0 means "no researched estimate for this one --
// the owner sets the real price during onboarding," rather than a guessed
// number. The four brands seeded before this list grew (Guinness, Star,
// Heineken, Jameson, Hennessy, Gordon's, Trophy Lager) keep their original
// researched estimates.
var NigerianBarCatalogue = []TemplateSeed{
	// Beer & Stout
	seed("Guinness", "Beer & Stout", 1, "50cl Bottle", 150000),
	seed("Star", "Beer & Stout", 2, "50cl Bottle", 90000),
	seed("Heineken", "Beer & Stout", 3, "50cl Bottle", 120000),
	seed("Trophy Lager", "Beer & Stout", 4, "50cl Bottle", 80000),
	seed("Gulder", "Beer & Stout", 5, "50cl Bottle", 0),
	seed("Life Continental Lager", "Beer & Stout", 6, "50cl Bottle", 0),
	seed("Hero", "Beer & Stout", 7, "50cl Bottle", 0),
	seed("Goldberg", "Beer & Stout", 8, "50cl Bottle", 0),
	seed("Budweiser", "Beer & Stout", 9, "50cl Bottle", 0),
	seed("33 Export", "Beer & Stout", 10, "50cl Bottle", 0),
	seed("Tiger Lager", "Beer & Stout", 11, "50cl Bottle", 0),
	seed("Castle Lager", "Beer & Stout", 12, "50cl Bottle", 0),
	seed("Harp Lager", "Beer & Stout", 13, "50cl Bottle", 0),
	seed("Champion Lager", "Beer & Stout", 14, "50cl Bottle", 0),
	seed("Corona", "Beer & Stout", 15, "35cl Bottle", 0),
	seed("Desperados", "Beer & Stout", 16, "50cl Bottle", 0),
	seed("Lagos Lager", "Beer & Stout", 17, "50cl Bottle", 0),
	seed("Harmattan Haze", "Beer & Stout", 18, "50cl Bottle", 0),
	seed("Legend Extra Stout", "Beer & Stout", 19, "50cl Bottle", 0),
	seed("Castle Milk Stout", "Beer & Stout", 20, "50cl Bottle", 0),
	seed("Trophy Stout", "Beer & Stout", 21, "50cl Bottle", 0),
	seed("Eagle Stout", "Beer & Stout", 22, "50cl Bottle", 0),
	seed("Turbo King", "Beer & Stout", 23, "50cl Bottle", 0),
	seed("Goldberg Black", "Beer & Stout", 24, "50cl Bottle", 0),

	// Gin
	seed("Gordon's", "Gin", 25, "75cl Bottle", 800000),
	seed("Tanqueray", "Gin", 26, "75cl Bottle", 0),
	seed("Bombay Sapphire", "Gin", 27, "75cl Bottle", 0),
	seed("Beefeater", "Gin", 28, "75cl Bottle", 0),
	seed("Chelsea London Dry Gin", "Gin", 29, "75cl Bottle", 0),
	seed("Bull London Dry Gin", "Gin", 30, "75cl Bottle", 0),
	seed("Regal Dry Gin", "Gin", 31, "75cl Bottle", 0),
	seed("Lord's Dry Gin", "Gin", 32, "75cl Bottle", 0),

	// Whiskey
	seed("Jameson", "Whiskey", 33, "70cl Bottle", 1500000),
	seed("Johnnie Walker", "Whiskey", 34, "70cl Bottle", 0),
	seed("Chivas Regal", "Whiskey", 35, "70cl Bottle", 0),
	seed("Macallan", "Whiskey", 36, "70cl Bottle", 0),
	seed("Glenfiddich", "Whiskey", 37, "70cl Bottle", 0),
	seed("Famous Grouse", "Whiskey", 38, "70cl Bottle", 0),
	seed("William Lawson's", "Whiskey", 39, "70cl Bottle", 0),
	seed("Ballantine's Finest", "Whiskey", 40, "70cl Bottle", 0),
	seed("Dewar's", "Whiskey", 41, "70cl Bottle", 0),
	seed("The Dalmore", "Whiskey", 42, "70cl Bottle", 0),
	seed("Tomintoul", "Whiskey", 43, "70cl Bottle", 0),
	seed("Jameson Black Barrel", "Whiskey", 44, "70cl Bottle", 0),
	seed("Jack Daniel's", "Whiskey", 45, "70cl Bottle", 0),
	seed("Jack Daniel's Single Barrel", "Whiskey", 46, "70cl Bottle", 0),
	seed("Wild Turkey American Honey", "Whiskey", 47, "70cl Bottle", 0),
	seed("Hibiki Japanese Harmony", "Whiskey", 48, "70cl Bottle", 0),

	// Spirits (vodka, rum, cognac/brandy, schnapps & bitters)
	seed("Hennessy", "Spirits", 49, "70cl Bottle", 3500000),
	seed("Smirnoff", "Spirits", 50, "70cl Bottle", 0),
	seed("Absolut", "Spirits", 51, "70cl Bottle", 0),
	seed("Grey Goose", "Spirits", 52, "70cl Bottle", 0),
	seed("Cîroc", "Spirits", 53, "70cl Bottle", 0),
	seed("Skyy Vodka", "Spirits", 54, "70cl Bottle", 0),
	seed("Flirt Vodka", "Spirits", 55, "70cl Bottle", 0),
	seed("Magic Moments", "Spirits", 56, "70cl Bottle", 0),
	seed("Russian Standard", "Spirits", 57, "70cl Bottle", 0),
	seed("Bacardi", "Spirits", 58, "70cl Bottle", 0),
	seed("Captain Morgan", "Spirits", 59, "70cl Bottle", 0),
	seed("Squadron Dark Rum", "Spirits", 60, "70cl Bottle", 0),
	seed("Old Pascas", "Spirits", 61, "70cl Bottle", 0),
	seed("Negrita", "Spirits", 62, "70cl Bottle", 0),
	seed("Martell", "Spirits", 63, "70cl Bottle", 0),
	seed("Rémy Martin", "Spirits", 64, "70cl Bottle", 0),
	seed("Courvoisier", "Spirits", 65, "70cl Bottle", 0),
	seed("Napoleon Brandy", "Spirits", 66, "70cl Bottle", 0),
	seed("Viceroy Brandy", "Spirits", 67, "70cl Bottle", 0),
	seed("Seaman's Aromatic Schnapps", "Spirits", 68, "75cl Bottle", 0),
	seed("Eagle Schnapps", "Spirits", 69, "75cl Bottle", 0),
	seed("Action Bitters", "Spirits", 70, "75cl Bottle", 0),
	seed("Alomo Bitters", "Spirits", 71, "75cl Bottle", 0),
	seed("Origin Bitters", "Spirits", 72, "75cl Bottle", 0),

	// Soft Drinks
	seed("Coke", "Soft Drinks", 73, "50cl Bottle", 50000),
	seed("Fanta", "Soft Drinks", 74, "50cl Bottle", 50000),
	seed("Sprite", "Soft Drinks", 75, "50cl Bottle", 50000),

	// Water
	seed("Water", "Water", 76, "75cl Bottle", 30000),
}

// seed builds a single-variant TemplateSeed -- every template in this list
// has exactly one suggested variant, so this is a table-row shorthand for
// the (otherwise heavily repetitive) TemplateSeed/TemplateVariantSeed
// struct literal pair.
func seed(name, categoryName string, sortOrder int32, variantName string, suggestedPriceKobo int64) TemplateSeed {
	return TemplateSeed{
		Name: name, CategoryName: categoryName, SortOrder: sortOrder,
		Variants: []TemplateVariantSeed{{Name: variantName, SuggestedPriceKobo: suggestedPriceKobo, SortOrder: 1}},
	}
}
