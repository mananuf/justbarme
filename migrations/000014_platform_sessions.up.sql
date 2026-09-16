-- Opaque online sessions for platform_staff, mirroring sessions exactly but
-- kept in a completely separate table so a platform session can never be
-- confused with (or accidentally accepted as) a business user's session.
-- Only the SHA-256 hash of the session token and of the CSRF token are ever
-- stored.
CREATE TABLE platform_sessions (
    id              UUID PRIMARY KEY,
    staff_id        UUID NOT NULL REFERENCES platform_staff (id),
    token_hash      TEXT NOT NULL,
    csrf_token_hash TEXT NOT NULL,
    user_agent      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,
    revoked_reason  TEXT
);

CREATE UNIQUE INDEX platform_sessions_token_hash_key ON platform_sessions (token_hash);
CREATE INDEX platform_sessions_staff_idx ON platform_sessions (staff_id);
