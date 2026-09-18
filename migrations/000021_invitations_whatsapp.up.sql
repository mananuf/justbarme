-- Invitations + WhatsApp messaging -- docs/PHASE_INVITATIONS_WHATSAPP.md.
-- Dual identity (phone and email either usable to sign up/log in/be
-- invited), staff invitations, and identity_verifications (the
-- verify-before-link mechanism for attaching a new identifier to an
-- already-existing account, whether from account settings or an
-- invitation match).

-- users.phone already exists (migration 000002), unused until now. email
-- becomes optional so a WhatsApp-only account is possible; the existing
-- unique index on lower(email) needs no change -- Postgres already treats
-- multiple NULLs as non-conflicting for uniqueness.
ALTER TABLE users ALTER COLUMN email DROP NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_email_or_phone_required
    CHECK (email IS NOT NULL OR phone IS NOT NULL);

-- signup_verifications gains the same optionality plus a channel column,
-- so Start/Verify handle either an email or a WhatsApp pending signup
-- through one code path. Exactly one of email/phone is set, matching
-- whichever channel is pending.
ALTER TABLE signup_verifications ALTER COLUMN email DROP NOT NULL;
ALTER TABLE signup_verifications ADD COLUMN phone TEXT;
ALTER TABLE signup_verifications ADD COLUMN channel TEXT NOT NULL DEFAULT 'email'
    CHECK (channel IN ('email', 'whatsapp'));
ALTER TABLE signup_verifications ADD CONSTRAINT signup_verifications_channel_identifier
    CHECK (
        (channel = 'email' AND email IS NOT NULL AND phone IS NULL)
        OR (channel = 'whatsapp' AND phone IS NOT NULL AND email IS NULL)
    );
CREATE UNIQUE INDEX signup_verifications_phone_key ON signup_verifications (phone) WHERE phone IS NOT NULL;

-- Staff invitations (docs/IMPLEMENTATION_PLAN.md Phase 2 deferred
-- POST /invitations from day one). Deliberately NOT RLS-scoped, unlike
-- most business-owned tables: the public landing-page lookup and accept
-- endpoints (internal/httpapi) are hit by an anonymous invitee who has no
-- tenant context at all -- an RLS policy keyed on app.business_id would
-- return zero rows for that request, since the GUC is never set outside
-- an authenticated tenant transaction. Same reasoning as
-- signup_verifications/sessions: the row is looked up by its own
-- unguessable hashed token, which is itself the access control, not RLS.
-- The authenticated owner-facing queries (list/create/revoke) filter by
-- business_id explicitly in application SQL instead, the same discipline
-- already used for the (also unscoped) businesses table.
CREATE TABLE invitations (
    id          UUID NOT NULL,
    business_id UUID NOT NULL REFERENCES businesses (id),
    invited_by  UUID NOT NULL REFERENCES users (id),
    phone       TEXT,
    email       TEXT,
    role        TEXT NOT NULL CHECK (role IN ('owner', 'staff')),
    token_hash  TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'revoked', 'expired')),
    expires_at  TIMESTAMPTZ NOT NULL,
    accepted_by UUID REFERENCES users (id),
    accepted_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (business_id, id),
    CHECK (phone IS NOT NULL OR email IS NOT NULL),
    CHECK ((status = 'accepted') = (accepted_by IS NOT NULL AND accepted_at IS NOT NULL))
);

CREATE UNIQUE INDEX invitations_token_hash_key ON invitations (token_hash);
-- One pending invite per identifier per business at a time.
CREATE UNIQUE INDEX invitations_business_phone_pending_key ON invitations (business_id, phone)
    WHERE status = 'pending' AND phone IS NOT NULL;
CREATE UNIQUE INDEX invitations_business_email_pending_key ON invitations (business_id, lower(email))
    WHERE status = 'pending' AND email IS NOT NULL;
CREATE INDEX invitations_business_status_idx ON invitations (business_id, status);

-- Verify-before-link: proves control of a phone/email before it is
-- attached to an ALREADY EXISTING user account -- distinct from
-- signup_verifications (a brand-new account) and invitations (a brand-new
-- membership). Used both by account settings ("add a phone/email to my
-- account") and by an invitation match where the invitee already has an
-- account under a different identifier. Not tenant-owned (a user, not a
-- business, owns this), same reasoning as users/signup_verifications --
-- no RLS.
CREATE TABLE identity_verifications (
    id          UUID NOT NULL,
    user_id     UUID NOT NULL REFERENCES users (id),
    channel     TEXT NOT NULL CHECK (channel IN ('email', 'whatsapp')),
    identifier  TEXT NOT NULL,
    otp_hash    TEXT NOT NULL,
    purpose     TEXT NOT NULL CHECK (purpose IN ('add_identifier', 'invitation_link')),
    attempts    INTEGER NOT NULL DEFAULT 0,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id)
);

CREATE INDEX identity_verifications_user_id_idx ON identity_verifications (user_id);
