# SupaLite

[English](./README.md) · [中文](./README.zh-CN.md)

[![ci](https://github.com/web-casa/supalite/actions/workflows/ci.yml/badge.svg)](https://github.com/web-casa/supalite/actions/workflows/ci.yml)
[![license](https://img.shields.io/badge/license-MIT-blue)](./LICENSE)
[![release](https://img.shields.io/github/v/release/web-casa/supalite?display_name=tag&sort=semver)](https://github.com/web-casa/supalite/releases)
[![docs](https://img.shields.io/badge/docs-web--casa.github.io-3ECF8E)](https://web-casa.github.io/supalite/)

> One-command self-hosted Postgres + Auth + Studio for indie developers.
> Built from upstream Supabase images, minus the parts most small
> deployments don't need (Storage, Edge Functions, Realtime).

---

## One-line install

```bash
git clone https://github.com/web-casa/supalite.git && cd supalite && ./setup.sh
```

`setup.sh` generates all secrets (`POSTGRES_PASSWORD`, `JWT_SECRET`,
`ADMIN_TOKEN`, …), writes `.env` (mode 600), pulls images, starts the
stack, health-checks every public endpoint, and prints the admin token.

When it finishes, open:

- **Landing**: <http://localhost:8000/>
- **Admin panel**: <http://localhost:8000/admin/> (login with `ADMIN_TOKEN`)
- **Studio**: <http://localhost:8000/studio/>

Lost the admin token? It's in `.env` under `ADMIN_TOKEN`.

> **Prerequisites**: Docker + Docker Compose v2, `openssl`, `curl`.
> Linux: add your user to the `docker` group, or prefix commands with `sudo`.

---

## What's inside

| Service    | Image                          | Role                                                  |
|------------|--------------------------------|-------------------------------------------------------|
| `db`       | supabase/postgres + pgbackrest | PG 15 + 12 Supabase roles + 12 extensions + pgBackRest|
| `rest`     | postgrest/postgrest            | Auto REST API derived from the schema                 |
| `gotrue`   | supabase/gotrue                | Email + OAuth auth, JWT minting                       |
| `admin`    | (built from `./admin`)         | Go backend + Next.js SPA — this project's admin panel |
| `meta`     | supabase/postgres-meta         | Metadata API for Studio (profile: `studio`)           |
| `studio`   | supabase/studio                | Upstream Supabase Studio (profile: `studio`)          |
| `gateway`  | caddy:2-alpine                 | Reverse proxy, CORS, auto-HTTPS, admin-cookie gate    |

Studio + postgres-meta run under `COMPOSE_PROFILES=studio` (default).
Remove that line in `.env` for the minimum stack.

Architecture diagram: [`architecture.mmd`](./architecture.mmd) (Mermaid).

---

## Admin panel

`http://localhost:8000/admin/` — cookie-auth, HMAC-signed, SameSite=Strict.

| Page      | What it does                                                   |
|-----------|----------------------------------------------------------------|
| Dashboard | Service status + API keys                                      |
| Logs      | Live SSE tail of any container's stdout/stderr                 |
| DB Ops    | VACUUM / VACUUM FULL / ANALYZE / REINDEX, target-aware         |
| Sessions  | `pg_stat_activity` with `pg_terminate_backend` action          |
| Backups   | pg_dump → S3 **+** pgBackRest (full / diff / incr / restore)   |
| Settings  | General, SMTP, GitHub / Google / Apple OAuth, Backup S3        |

Long-running ops (pg_dump, pg_restore, pgBackRest) run async; the UI
polls status. Destructive actions (restore, terminate, delete) require
a typed-name confirmation.

---

## Enabling HTTPS

Default is http-only on `:8000`. Put Caddy in front with auto-TLS:

1. Point public DNS (A/AAAA) at the host.
2. `.env`: `CADDY_SITE_ADDR=db.example.com` (space-separate for multi-host).
3. `.env`: `API_EXTERNAL_URL=https://db.example.com`.
4. Make ports 80 + 443 reachable (ACME HTTP-01 needs `:80`).
5. Start with the HTTPS override file:
   ```bash
   docker compose -f docker-compose.yml -f docker-compose.https.yml up -d
   ```

Caddy auto-issues and renews Let's Encrypt certs. State persists in the
`caddy-data` volume.

---

## Multiple frontends on one instance

One SupaLite typically serves one indie project — same auth backend
for your Next.js app, mobile app, and marketing site. Configure CORS
+ GoTrue allow list in `.env`:

```env
SITE_URL=https://app.example.com
CORS_ALLOWED_ORIGINS_REGEX=https://app\.example\.com|https://admin\.example\.com
GOTRUE_URI_ALLOW_LIST=https://admin.example.com/auth/callback,myapp://auth
```

For unrelated projects on the same host, run separate compose
projects (`docker compose -p app-a` / `-p app-b`) so data volumes
and admin tokens stay isolated.

Full guide: [docs/Multi-frontend](https://web-casa.github.io/supalite/configuration/multi-frontend/)

---

## Backups

Two paths to the same S3-compatible storage (AWS / R2 / MinIO / B2):

- **Logical** — `pg_dump -Fc` streamed to S3. Always available, click
  "Run Backup" in the admin panel.
- **Physical** — pgBackRest with continuous WAL archiving + incremental
  backups. Opt-in via `PGBACKREST_ARCHIVE_MODE=on`. Restore orchestrates
  `stop db → restore → start db` and stays stopped on failure for
  inspection.

Both share the same `BACKUP_S3_*` credentials in `.env`.

---

## Documentation

Full bilingual docs at <https://web-casa.github.io/supalite/>:

- [Getting Started](https://web-casa.github.io/supalite/getting-started/quick-start/)
- [Configuration](https://web-casa.github.io/supalite/configuration/environment-reference/)
- [Operations](https://web-casa.github.io/supalite/operations/backups/)
- [Concepts: RLS, JWT, ANON vs SERVICE_ROLE](https://web-casa.github.io/supalite/concepts/rls/)
- [Admin API reference](https://web-casa.github.io/supalite/api-reference/admin-api/)
- [Troubleshooting](https://web-casa.github.io/supalite/troubleshooting/common-errors/)

---

## Example project

[`examples/nextjs-todo/`](./examples/nextjs-todo/) — minimal Next.js +
`supabase-js` app showing the full SupaLite handshake: email sign-up,
sign-in, RLS-guarded `todos` table, CRUD via the auto REST API. Fork
it as a starting point.

---

## Development

Admin panel source lives in `./admin`:

- `internal/` — Go packages (handlers, docker client, sse, backup, db, …)
- `web/` — Next.js app (statically exported, embedded via `go:embed`)

Rebuild after edits:

```bash
docker compose up -d --build admin
```

`go vet ./...` and `go build ./...` must be run from `admin/` (not
`admin/web/`, which has no Go code).

---

## License

MIT — see [LICENSE](./LICENSE).
