-- Global identity. Not tenant-owned: membership joins users to businesses.
CREATE TABLE users (
    id             UUID PRIMARY KEY,
    email          TEXT NOT NULL,
    phone          TEXT,
    display_name   TEXT NOT NULL,
    password_hash  TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Case-insensitive uniqueness without a citext dependency.
CREATE UNIQUE INDEX users_email_key ON users (lower(email));
CREATE UNIQUE INDEX users_phone_key ON users (phone) WHERE phone IS NOT NULL;
