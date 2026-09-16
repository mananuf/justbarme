-- Platform-level operators who can oversee businesses across the whole
-- service. A completely separate identity space from users (business
-- members) -- not just a flag on that table -- so a bug in ordinary
-- business-user code can never accidentally grant platform capabilities.
-- Bootstrapped only by cmd/seed-platform-staff, an operator tool: there is
-- no signup endpoint, doubly so for this tier.
CREATE TABLE platform_staff (
    id            UUID PRIMARY KEY,
    email         TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('support', 'superadmin')),
    status        TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Case-insensitive uniqueness without a citext dependency -- same pattern
-- as users_email_key.
CREATE UNIQUE INDEX platform_staff_email_key ON platform_staff (lower(email));
