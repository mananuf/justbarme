-- Pending public signups awaiting email verification. Deliberately not rows
-- in `users` -- that table's email uniqueness must stay reserved for real,
-- completed accounts, not an abandoned or still-in-progress signup. One row
-- per email; starting signup again for the same address replaces it with a
-- fresh code rather than erroring (see internal/signup.Service.Start's
-- upsert) -- that is what "resend the code" actually is, not a separate
-- endpoint. No RLS: not tenant-owned, same reasoning as users/platform_staff.
CREATE TABLE signup_verifications (
    id            UUID NOT NULL,
    email         TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    otp_hash      TEXT NOT NULL,
    attempts      INTEGER NOT NULL DEFAULT 0,
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id)
);

CREATE UNIQUE INDEX signup_verifications_email_key ON signup_verifications (lower(email));
