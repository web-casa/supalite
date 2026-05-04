---
title: Architecture
description: How SupaLite's containers fit together.
---

SupaLite is six (or eight, with optional services) containers behind a single Caddy gateway. The diagram below shows the request flow:

```
┌─────────────────────────────────────────────────────────────────┐
│                        :8000  (Caddy gateway)                    │
│                                                                   │
│  /rest/v1/*    ───►  rest    (PostgREST)         ───►  ┌─────┐   │
│  /auth/v1/*    ───►  gotrue  (auth + OAuth)       ───►  │     │   │
│  /studio/*     ───►  studio  (table editor UI)    ───►  │     │   │
│                       └──── meta (postgres-meta)  ───►  │  db │   │
│  /admin/*      ───►  admin   (Go + Next.js SPA)   ───►  │     │   │
│                       └──── docker.sock (peer ops)      └─────┘   │
│  /              ───►  static landing page                          │
└─────────────────────────────────────────────────────────────────┘
                                ▲
                       80 / 443 (opt-in HTTPS)
```

## The seven services

| Service   | Image                                | Purpose |
|-----------|--------------------------------------|---------|
| `db`      | Custom (`supabase/postgres` + pgbackrest) | Postgres 15 with all Supabase roles, schemas, extensions, plus pgbackrest binary for incremental backups. |
| `rest`    | `postgrest/postgrest`                | Auto REST API + pg_graphql. Connects as `authenticator` role. |
| `gotrue`  | `supabase/gotrue`                    | Email + OAuth auth, JWT minting, password reset. Connects as `supabase_auth_admin`. |
| `meta`    | `supabase/postgres-meta`             | Metadata REST API used only by Studio. Profile: `studio`. |
| `studio`  | `supabase/studio`                    | Upstream Supabase UI. Profile: `studio`. |
| `admin`   | Custom Go + Next.js                  | This project's admin panel. Mounts docker.sock for peer ops (restart, exec into db). |
| `gateway` | `caddy:2-alpine`                     | Reverse proxy, CORS, optional auto-HTTPS, admin-cookie gating for `/studio/*`. |

## Network

Single Docker network (`supalite_default`). Containers address each other by service name (`db`, `rest`, etc.). Only the gateway is exposed externally.

## Auth model

Two independent auth layers:

1. **Application auth** (your end users): Caddy's `require_apikey` snippet checks the `apikey` header against `ANON_KEY` or `SERVICE_ROLE_KEY` for `/rest/v1/*` and `/graphql/v1/*`. GoTrue mints user JWTs that PostgREST honors via `auth.uid()` in RLS policies.

2. **Admin panel auth** (you the operator): HMAC-signed session cookies for `/admin/*`, with `SameSite=Strict` and `HttpOnly`. Login uses `ADMIN_TOKEN` once; the cookie keeps you logged in until expiry. Studio's `/studio/*` is gated on cookie presence (cookie value isn't verified at gateway level — Studio's API calls hit PostgREST/GoTrue/postgres-meta which validate keys independently).

A compromised `ANON_KEY` cannot reach the admin API. A compromised `ADMIN_TOKEN` cannot bypass RLS for application data unless the operator explicitly uses `SERVICE_ROLE_KEY` from the admin panel.

## Storage

One Docker named volume:

- `supalite_db-data` — Postgres data dir (`/var/lib/postgresql/data`)

Optional, if you enable HTTPS:

- `caddy-data` — Let's Encrypt certs and ACME account keys
- `caddy-config` — Caddy's runtime config

## Init pipeline

When `db` starts on a fresh volume:

1. The supabase/postgres image runs its own `migrate.sh` which creates the 12 Supabase roles, base schemas (auth, storage, extensions, …), and installs `uuid-ossp`, `pgcrypto`, `pg_stat_statements`.
2. SupaLite's `volumes/db/init/zz-supalite.sh` runs after (the `zz-` prefix sorts last in `/docker-entrypoint-initdb.d/`):
   - Enables 9 more extensions (`pg_graphql`, `pgvector`, `pg_net`, `pg_cron`, `pgsodium`, `supabase_vault`, `pgjwt`, `http`, `pg_jsonschema`)
   - Creates the `_realtime` and `supabase_functions` schemas
   - Unifies all 12 role passwords to `POSTGRES_PASSWORD`
   - Overrides `auth.uid()` / `auth.role()` / `auth.email()` for the new GUC mode (`request.jwt.claims` instead of legacy per-claim GUCs)
   - Sets `app.settings.jwt_secret` at the database level

Subsequent restarts skip the init scripts (Postgres only runs them on empty data dirs).

## See also

- [Multi-frontend setup](/supalite/configuration/multi-frontend/) — running one SupaLite for several apps
- [Compose profiles](/supalite/configuration/compose-profiles/) — minimal vs full stack
- [Ports & networking](/supalite/concepts/ports-networking/) — what binds where, why
