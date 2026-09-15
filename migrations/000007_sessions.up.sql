-- Opaque online sessions. Global like users: a session is not scoped to a
-- business, since one user can hold memberships in more than one. Only the
-- SHA-256 hash of the session token and of the CSRF token are ever stored.
CREATE TABLE sessions (
    id               UUID PRIMARY KEY,
    user_id          UUID NOT NULL REFERENCES users (id),
    token_hash       TEXT NOT NULL,
    csrf_token_hash  TEXT NOT NULL,
    user_agent       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at       TIMESTAMPTZ NOT NULL,
    revoked_at       TIMESTAMPTZ,
    revoked_reason   TEXT
);

CREATE UNIQUE INDEX sessions_token_hash_key ON sessions (token_hash);
CREATE INDEX sessions_user_idx ON sessions (user_id);
