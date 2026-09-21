-- Public, revocable bill-sharing links -- docs/ARCHITECTURE.md §15,
-- docs/PHASE_PILOT_RELEASE.md §4. Deliberately not RLS-scoped, same
-- reasoning as invitations/signup_verifications/sessions (see CLAUDE.md's
-- Invitations section): the public GET /public/bills/{token} lookup has
-- no business context to filter by -- the row's own unguessable hashed
-- token is the access control, matching invitations.token_hash exactly.
-- One active link per bill (bill_id is UNIQUE): creating a new link
-- rotates the existing one rather than accumulating several.
CREATE TABLE bill_share_links (
    id          UUID PRIMARY KEY,
    business_id UUID NOT NULL,
    bill_id     UUID NOT NULL UNIQUE,
    token_hash  TEXT NOT NULL UNIQUE,
    created_by  UUID NOT NULL REFERENCES users (id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ,
    revoked_at  TIMESTAMPTZ,
    FOREIGN KEY (business_id, bill_id) REFERENCES bills (business_id, id)
);

CREATE INDEX bill_share_links_business_id_idx ON bill_share_links (business_id);
