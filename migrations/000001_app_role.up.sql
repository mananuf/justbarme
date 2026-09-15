-- The application connects and runs ordinary request-serving queries as
-- jbm_app, a role with no BYPASSRLS and no schema-ownership rights. Schema
-- changes (this migration included) run as whatever role owns the database
-- connection used to apply migrations — a separate, more privileged role in
-- any real environment. Locally that's the same connecting role, which is
-- why this migration grants itself membership in jbm_app via CURRENT_USER.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'jbm_app') THEN
    CREATE ROLE jbm_app NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS NOREPLICATION;
  END IF;
END
$$;

GRANT jbm_app TO CURRENT_USER;

GRANT USAGE ON SCHEMA public TO jbm_app;

ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO jbm_app;

ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO jbm_app;
