-- name: UpsertCatalogueTemplate :one
INSERT INTO catalogue_templates (id, name, category_name, sort_order)
VALUES ($1, $2, $3, $4)
ON CONFLICT (name) DO UPDATE
    SET category_name = EXCLUDED.category_name, sort_order = EXCLUDED.sort_order
RETURNING *;

-- name: UpsertCatalogueTemplateVariant :one
INSERT INTO catalogue_template_variants (id, template_id, name, suggested_price_kobo, sort_order)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (template_id, name) DO UPDATE
    SET suggested_price_kobo = EXCLUDED.suggested_price_kobo, sort_order = EXCLUDED.sort_order
RETURNING *;

-- name: ListCatalogueTemplates :many
SELECT * FROM catalogue_templates ORDER BY sort_order, name;

-- name: ListCatalogueTemplatesByIDs :many
SELECT * FROM catalogue_templates WHERE id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY sort_order, name;

-- name: ListCatalogueTemplateVariantsByTemplateIDs :many
SELECT * FROM catalogue_template_variants
WHERE template_id = ANY(sqlc.arg(template_ids)::uuid[])
ORDER BY template_id, sort_order, name;
