-- Password-less accounts: a Google-only sign-in has no password at all.
-- internal/identity.CreateUserWithoutPassword (used by internal/oauth)
-- stores an unset password hash for this case; internal/auth.VerifyPassword
-- already fails safely (ErrInvalidHashFormat) against an empty/NULL hash,
-- so password login needs no special case for these accounts.
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;

-- Links a users row to an external identity provider's account. Deliberately
-- its own table rather than columns on users: a user can have both a
-- password and a linked Google identity (auto-linked at sign-in when the
-- emails match, since Google -- not the caller -- already verified that
-- email; see internal/oauth), and a second provider later needs no new
-- migration, just a new value in provider. No RLS: not tenant-owned, same
-- reasoning as users/platform_staff.
CREATE TABLE user_identities (
    id               UUID NOT NULL,
    user_id          UUID NOT NULL REFERENCES users (id),
    provider         TEXT NOT NULL,
    provider_user_id TEXT NOT NULL,
    email            TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id)
);

CREATE UNIQUE INDEX user_identities_provider_key ON user_identities (provider, provider_user_id);
CREATE INDEX user_identities_user_id_idx ON user_identities (user_id);
