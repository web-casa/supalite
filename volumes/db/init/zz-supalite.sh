#!/usr/bin/env bash
# supabase-lite custom init script.
# Runs AFTER the supabase/postgres image's migrate.sh has completed.
# See docs/pg-supabase-compatibility.md §9.3 for rationale.
#
# At this point the image has already:
#   - Created 12 Supabase roles (anon, authenticated, service_role,
#     authenticator, supabase_admin, supabase_auth_admin, supabase_storage_admin,
#     supabase_replication_admin, supabase_read_only_user, dashboard_user,
#     pgbouncer, postgres). Newer images (> 15.8.1.085) also add
#     supabase_etl_admin — our script tolerates its absence with IF EXISTS.
#   - Created base schemas (auth, storage, extensions, realtime, pgbouncer)
#   - Installed extensions uuid-ossp, pgcrypto, pg_stat_statements
#   - Issued GRANT supabase_admin TO authenticator (see ADR-001)
#   - Set supabase_admin password to POSTGRES_PASSWORD
#
# Roles NOT yet created by the image (created by extension triggers):
#   - supabase_functions_admin  (created when CREATE EXTENSION pg_net)
#
# What WE do here (in order, because extensions create roles):
#   1. Enable 9 core Supabase extensions (pg_net creates supabase_functions_admin)
#   2. Create _realtime and supabase_functions schemas
#   3. Unify ALL role passwords to POSTGRES_PASSWORD
#   4. Override auth.uid()/role()/email() for new GUC mode
#   5. Set app.settings.jwt_secret / jwt_exp
#
# Security: NEVER interpolate secrets via bash heredoc. All sensitive values
# are passed through `psql -v name=value` which uses proper SQL literal escaping
# (:'name' syntax), then read inside DO blocks via current_setting() +
# format(%L). This prevents both shell and SQL injection.

set -eu -o pipefail

# --- Validate required env vars ---------------------------------------------
if [[ -z "${POSTGRES_PASSWORD:-}" ]]; then
  echo "[zz-supalite] FATAL: POSTGRES_PASSWORD not set" >&2
  exit 1
fi
if [[ -z "${JWT_SECRET:-}" ]]; then
  echo "[zz-supalite] FATAL: JWT_SECRET not set" >&2
  exit 1
fi

# --- Step tracking + error trap ---------------------------------------------
current_step="init"
trap 'echo "[zz-supalite] FAILED at step: ${current_step} (exit $?)" >&2' ERR

export PGPASSWORD="${POSTGRES_PASSWORD}"
PSQL="psql -v ON_ERROR_STOP=1 --no-psqlrc -U supabase_admin -d postgres"

echo "[zz-supalite] running post-migration customizations..."

# --- 1. Enable 9 core Supabase extensions -----------------------------------
# Order matters:
#   - pgsodium MUST precede supabase_vault
#   - pg_net creates supabase_functions_admin via event trigger (see post-setup.sql)
#   - pg_graphql creates graphql schema
# Fixed-schema extensions (cannot relocate): pg_net (net), pg_cron (cron),
#   pg_graphql (graphql), pgsodium (pgsodium), supabase_vault (vault)
# No sensitive values here — safe to use regular heredoc.
current_step="enable extensions"
$PSQL <<-'EOSQL'
  -- REST API / PostgREST support
  CREATE EXTENSION IF NOT EXISTS pgjwt           WITH SCHEMA extensions;
  CREATE EXTENSION IF NOT EXISTS http            WITH SCHEMA extensions;
  CREATE EXTENSION IF NOT EXISTS pg_jsonschema   WITH SCHEMA extensions;

  -- AI / Vector
  CREATE EXTENSION IF NOT EXISTS vector          WITH SCHEMA extensions;

  -- Fixed-schema extensions
  CREATE EXTENSION IF NOT EXISTS pg_net;
  CREATE EXTENSION IF NOT EXISTS pg_graphql;

  -- Secrets management (order matters: pgsodium before supabase_vault)
  CREATE EXTENSION IF NOT EXISTS pgsodium;
  CREATE EXTENSION IF NOT EXISTS supabase_vault;

  -- Scheduled jobs
  CREATE EXTENSION IF NOT EXISTS pg_cron;
EOSQL

# --- 2. Create supabase reserved schemas not yet present --------------------
current_step="create reserved schemas"
$PSQL <<-'EOSQL'
  CREATE SCHEMA IF NOT EXISTS _realtime          AUTHORIZATION supabase_admin;
  CREATE SCHEMA IF NOT EXISTS supabase_functions AUTHORIZATION supabase_admin;
  GRANT USAGE ON SCHEMA _realtime          TO postgres, anon, authenticated, service_role;
  GRANT USAGE ON SCHEMA supabase_functions TO postgres, anon, authenticated, service_role;
EOSQL

# --- 3. Unify ALL role passwords --------------------------------------------
# Pass POSTGRES_PASSWORD via psql -v (safe escaping), read inside DO via
# current_setting + format(%L). Heredoc uses 'EOSQL' (quoted) = NO bash
# interpolation of ${POSTGRES_PASSWORD}.
current_step="unify role passwords"
$PSQL -v pwd="${POSTGRES_PASSWORD}" <<-'EOSQL'
  SELECT set_config('supalite.pwd', :'pwd', false);

  DO $$
  DECLARE
    r text;
    pwd text := current_setting('supalite.pwd');
  BEGIN
    FOREACH r IN ARRAY ARRAY[
      'authenticator',
      'supabase_auth_admin',
      'supabase_storage_admin',
      'supabase_functions_admin',
      'supabase_replication_admin',
      'supabase_etl_admin',
      'supabase_read_only_user',
      'dashboard_user',
      'pgbouncer'
    ]
    LOOP
      IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
        EXECUTE format('ALTER USER %I WITH PASSWORD %L', r, pwd);
        RAISE NOTICE 'Set password for %', r;
      ELSE
        RAISE NOTICE 'Role % does not exist, skipping', r;
      END IF;
    END LOOP;
  END
  $$;

  -- Clear the session GUC (not strictly needed — session ends here — but tidy)
  RESET supalite.pwd;
EOSQL

# --- 4. Override auth functions for new GUC mode ----------------------------
# Image creates these using request.jwt.claim.sub (legacy GUC).
# We use PGRST_DB_USE_LEGACY_GUCS=false → whole JWT in request.jwt.claims.
# No sensitive values.
current_step="override auth functions"
$PSQL <<-'EOSQL'
  CREATE OR REPLACE FUNCTION auth.uid()
  RETURNS uuid LANGUAGE sql STABLE AS $func$
    SELECT NULLIF(current_setting('request.jwt.claims', true)::json->>'sub', '')::uuid
  $func$;

  CREATE OR REPLACE FUNCTION auth.role()
  RETURNS text LANGUAGE sql STABLE AS $func$
    SELECT NULLIF(current_setting('request.jwt.claims', true)::json->>'role', '')::text
  $func$;

  CREATE OR REPLACE FUNCTION auth.email()
  RETURNS text LANGUAGE sql STABLE AS $func$
    SELECT NULLIF(current_setting('request.jwt.claims', true)::json->>'email', '')::text
  $func$;

  -- Note: auth.jwt() is created by GoTrue's own migration (20220531120530).
  -- We don't create it here to avoid ownership conflicts (GoTrue connects as
  -- supabase_auth_admin which can't overwrite functions we'd own as supabase_admin).
EOSQL

# --- 5. Set database-level JWT settings -------------------------------------
# ALTER DATABASE ... SET requires a literal value (no expression), so we use
# DO + EXECUTE format(%L) for safe escaping. Values passed via psql -v.
current_step="set jwt database settings"
$PSQL \
  -v jwt_secret="${JWT_SECRET}" \
  -v jwt_exp="${JWT_EXP:-3600}" \
  <<-'EOSQL'
  SELECT set_config('supalite.jwt_secret', :'jwt_secret', false);
  SELECT set_config('supalite.jwt_exp',    :'jwt_exp',    false);

  DO $$
  BEGIN
    EXECUTE format('ALTER DATABASE postgres SET %I TO %L',
                   'app.settings.jwt_secret',
                   current_setting('supalite.jwt_secret'));
    EXECUTE format('ALTER DATABASE postgres SET %I TO %L',
                   'app.settings.jwt_exp',
                   current_setting('supalite.jwt_exp'));
  END
  $$;
EOSQL

current_step="done"
echo "[zz-supalite] done."
