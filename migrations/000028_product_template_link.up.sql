-- Canonical brand identity: a business's product remembers which platform
-- catalogue template it came from, by id, so "Trophy Stout" in one bar and a
-- renamed "Trophy" in another still roll up to the same brand for any future
-- cross-business aggregate. Nullable: a genuinely custom product has no
-- template. Size is deliberately NOT linked here -- an owner can pick a size
-- different from the template's suggestion, so a variant-level link would
-- often be wrong; the size lives in the variant name (a standard-size
-- string) and can be normalized separately.
ALTER TABLE products
    ADD COLUMN template_id UUID REFERENCES catalogue_templates (id) ON DELETE SET NULL;

CREATE INDEX products_template_idx ON products (template_id) WHERE template_id IS NOT NULL;

-- Backfill existing products whose name exactly matches a template (case-
-- insensitive) -- how ApplyTemplates named them. products is FORCE ROW LEVEL
-- SECURITY, which applies even to the table owner, so without a business
-- context this UPDATE would silently touch zero rows. Lifting FORCE for the
-- duration of this one statement (same file, one implicit transaction) lets
-- the owner role run it; FORCE is restored immediately after.
ALTER TABLE products NO FORCE ROW LEVEL SECURITY;

UPDATE products p
SET template_id = (
    SELECT t.id FROM catalogue_templates t
    WHERE lower(t.name) = lower(p.name)
    ORDER BY t.name
    LIMIT 1
)
WHERE p.template_id IS NULL;

ALTER TABLE products FORCE ROW LEVEL SECURITY;
