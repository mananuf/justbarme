-- Platform-owned onboarding suggestions -- not live business data, and not
-- tenant-scoped (no business_id, no RLS), for the same reason `businesses`
-- itself carries no RLS: nothing here is ever written through a tenant
-- request path. A template is copied into a business's own
-- products/variants/prices when an owner selects it during onboarding
-- (internal/catalogue.Service.ApplyTemplates); editing the copy never
-- touches the template it came from.
CREATE TABLE catalogue_templates (
    id            UUID NOT NULL,
    name          TEXT NOT NULL,
    category_name TEXT NOT NULL,
    sort_order    INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (name)
);

CREATE TABLE catalogue_template_variants (
    id                   UUID NOT NULL,
    template_id          UUID NOT NULL REFERENCES catalogue_templates (id),
    name                 TEXT NOT NULL,
    suggested_price_kobo BIGINT NOT NULL CHECK (suggested_price_kobo >= 0),
    sort_order           INTEGER NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (template_id, name)
);

CREATE INDEX catalogue_template_variants_template_idx ON catalogue_template_variants (template_id);
