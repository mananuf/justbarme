-- Every platform-scoped mutation is recorded here -- this is what makes a
-- platform admin's cross-business reach defensible rather than
-- undetectable. Append-only: never updated or deleted by the app role.
-- target_business_id is nullable for an action with no single business
-- target (e.g. staff account management, once that exists).
CREATE TABLE platform_audit_log (
    id                  UUID PRIMARY KEY,
    platform_staff_id   UUID NOT NULL REFERENCES platform_staff (id),
    action              TEXT NOT NULL,
    target_business_id  UUID REFERENCES businesses (id),
    reason              TEXT,
    request_id          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX platform_audit_log_staff_idx ON platform_audit_log (platform_staff_id, created_at DESC);
CREATE INDEX platform_audit_log_business_idx
    ON platform_audit_log (target_business_id, created_at DESC) WHERE target_business_id IS NOT NULL;
