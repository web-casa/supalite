# Changelog

All notable changes to this project are documented here. Format loosely
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions
use [SemVer](https://semver.org/).

## [0.1.0] — 2026-04-19

Initial public release.

### Added

#### Core stack
- PostgreSQL 15.8 (supabase/postgres image) with 12 Supabase roles, 12
  schemas, 13 extensions pre-wired.
- PostgREST auto REST API + pg_graphql.
- GoTrue auth (email + GitHub / Google / Apple OAuth).
- Supabase Studio + postgres-meta (optional, `COMPOSE_PROFILES=studio`).
- Caddy gateway with cookie-gated `/studio/*`, API-key-gated `/rest/*`
  and `/graphql/*`, and the admin panel at `/admin/*`.

#### Admin panel (Go 1.25 + Next.js 16)
- Dashboard with service-status badges and API keys.
- **Logs** — live SSE tail of any container.
- **DB Ops** — VACUUM / VACUUM FULL / ANALYZE / REINDEX with
  identifier validation + `pgx.Identifier.Sanitize` belt-and-suspenders.
- **Sessions** — `pg_stat_activity` table + `pg_terminate_backend`
  with typed-PID confirmation.
- **Backups**:
  - Logical (pg_dump) → S3-compatible storage. Streaming upload
    (no temp files), `pg_restore --clean --if-exists` with
    typed-name confirmation.
  - Physical (pgBackRest) with full/diff/incr backups, orchestrated
    restore (stop db → one-shot container → start db), orphan-container
    detection, 4-hour timeout with SIGTERM-then-SIGKILL graceful cancel.
- **Settings** — General (SITE_URL, API URL, CORS allow list, OAuth
  redirect allow list), SMTP (with live Send Test), GitHub/Google/Apple
  OAuth (with Test credentials), Backup S3 config.

#### Operations
- HMAC-signed session cookies (`SameSite=Strict`, HttpOnly).
- Per-IP rate limiting on login endpoint.
- SSE for logs and service status (no polling).
- Opt-in auto-HTTPS via Caddy + Let's Encrypt
  (`docker-compose.https.yml` override).
- Multi-frontend CORS + GoTrue URI allow list for indie projects with
  web + mobile + marketing front-ends.

#### DX
- `setup.sh` generates all secrets, writes `.env`, health-checks every
  public endpoint, prints the admin token.
- MIT license.
- GitHub Actions CI (`go vet` / `go build` / `next build` /
  `docker compose config` with default + HTTPS override).
- GitHub Actions release workflow publishes multi-arch images
  (amd64 + arm64) to GitHub Container Registry on `v*` tags.
- Container images with `image` + `build` hybrid: default compose
  pulls pre-built images, falls back to local build if unreachable.

### Security hardening (noteworthy)
- All destructive admin actions (restore, terminate, delete) require
  typed-label confirmation.
- Subprocess lifecycle: SIGTERM + `WaitDelay` graceful cancellation,
  owned copy goroutines to prevent stdin deadlocks.
- `ServeMux` method-prefix patterns so wrong-method requests get
  consistent error responses.
- S3 bucket traversal defended by regex allowlist + key prefixing.
- Presigned download URLs capped at 10-minute TTL.
- CSP on `/admin/*` blocks framing, cross-origin fetch, data
  exfiltration (allows `script-src 'self' 'unsafe-inline'` because
  Next.js static export emits inline bootstrap scripts).

### Known limitations
- Apple OAuth credential validation is not covered (JWT-based secret,
  unlike static-secret providers).
- Scheduled backups and retention policy are manual (pg_dump path;
  pgBackRest has its own retention settings).
- No OpenAPI spec for admin API; endpoints documented inline in code.
- No unit tests on the Go side yet; verification has been end-to-end
  manual + Codex review cycles per phase.
