package catalogue

// NigerianBarCatalogue is the initial platform catalogue template list --
// common products stocked by a Nigerian bar, grouped into suggested
// categories, each with one suggested variant and starting price. Prices
// are estimates an owner is expected to adjust after applying a template
// (docs/IMPLEMENTATION_PLAN.md Phase 3 "Seed the initial Nigerian product
// catalogue"). Passed to Service.SeedTemplates, which upserts by name, so
// this list can be edited and re-seeded safely at any time.
var NigerianBarCatalogue = []TemplateSeed{
	{
		Name: "Guinness", CategoryName: "Beer & Stout", SortOrder: 1,
		Variants: []TemplateVariantSeed{{Name: "50cl Bottle", SuggestedPriceKobo: 150000, SortOrder: 1}},
	},
	{
		Name: "Star", CategoryName: "Beer & Stout", SortOrder: 2,
		Variants: []TemplateVariantSeed{{Name: "50cl Bottle", SuggestedPriceKobo: 90000, SortOrder: 1}},
	},
	{
		Name: "Heineken", CategoryName: "Beer & Stout", SortOrder: 3,
		Variants: []TemplateVariantSeed{{Name: "50cl Bottle", SuggestedPriceKobo: 120000, SortOrder: 1}},
	},
	{
		Name: "Trophy", CategoryName: "Beer & Stout", SortOrder: 4,
		Variants: []TemplateVariantSeed{{Name: "50cl Bottle", SuggestedPriceKobo: 80000, SortOrder: 1}},
	},
	{
		Name: "Jameson", CategoryName: "Spirits", SortOrder: 5,
		Variants: []TemplateVariantSeed{{Name: "70cl Bottle", SuggestedPriceKobo: 1500000, SortOrder: 1}},
	},
	{
		Name: "Hennessy", CategoryName: "Spirits", SortOrder: 6,
		Variants: []TemplateVariantSeed{{Name: "70cl Bottle", SuggestedPriceKobo: 3500000, SortOrder: 1}},
	},
	{
		Name: "Gordon's", CategoryName: "Spirits", SortOrder: 7,
		Variants: []TemplateVariantSeed{{Name: "75cl Bottle", SuggestedPriceKobo: 800000, SortOrder: 1}},
	},
	{
		Name: "Coke", CategoryName: "Soft Drinks", SortOrder: 8,
		Variants: []TemplateVariantSeed{{Name: "50cl Bottle", SuggestedPriceKobo: 50000, SortOrder: 1}},
	},
	{
		Name: "Fanta", CategoryName: "Soft Drinks", SortOrder: 9,
		Variants: []TemplateVariantSeed{{Name: "50cl Bottle", SuggestedPriceKobo: 50000, SortOrder: 1}},
	},
	{
		Name: "Sprite", CategoryName: "Soft Drinks", SortOrder: 10,
		Variants: []TemplateVariantSeed{{Name: "50cl Bottle", SuggestedPriceKobo: 50000, SortOrder: 1}},
	},
	{
		Name: "Water", CategoryName: "Water", SortOrder: 11,
		Variants: []TemplateVariantSeed{{Name: "75cl Bottle", SuggestedPriceKobo: 30000, SortOrder: 1}},
	},
}
