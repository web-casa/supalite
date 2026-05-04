---
title: Admin API Reference
description: Every endpoint exposed by the admin panel.
---

All admin endpoints live under `/admin/api/`. Authentication is one of:

- **Cookie**: HMAC-signed `sbl_auth` set after login (used by the SPA itself).
- **Bearer**: `Authorization: Bearer <ADMIN_TOKEN>` (used by scripts/CLI).

Two endpoints bypass the auth middleware: `POST /api/auth/verify` and `POST /api/auth/logout`.

Wrong-method requests (e.g. `PUT /api/keys`) return JSON 404 (the `/` catch-all swallows them rather than 405; behaves like a generic "not found").

## Auth

### `POST /api/auth/verify`

Login. Send `Authorization: Bearer <ADMIN_TOKEN>`. On success, sets `sbl_auth` cookie. Rate-limited to 10/min per IP.

### `POST /api/auth/logout`

Clears the `sbl_auth` cookie.

### `POST /api/auth/smtp-test`

Body: `{"to": "user@example.com"}`. Sends a test email using the currently saved SMTP config. Returns 200 / 4xx / 502 with diagnostic message.

### `POST /api/auth/oauth-test`

Body: `{"provider": "github"|"google"}`. Probes the provider's API with the currently saved client_id/secret.

## Config

### `GET /api/config` / `POST /api/config`

GET returns the editable subset of `.env` as a `{section: {key: value}}` object; secret values are replaced with `__SECRET_UNCHANGED__`. POST takes the same shape; reposting the placeholder leaves the existing value untouched.

## Status / Logs

### `GET /api/status`

List of containers in the SupaLite project with their state.

### `GET /api/status/stream`

Server-Sent Events stream of `snapshot` events whenever container state changes. Heartbeat every 15s.

### `GET /api/logs?service=<name>&lines=<n>`

One-shot fetch of the last N lines (default 100, max 1000).

### `GET /api/logs/stream?service=<name>&tail=<n>`

SSE stream of log lines as they arrive.

## DB

### `POST /api/dbops/maintenance`

Body: `{"op":"vacuum"|"analyze"|"reindex","target":"schema.table","full":bool}`.

### `GET /api/sessions?system=true`

`pg_stat_activity` rows; `system=true` includes background workers.

### `POST /api/sessions/terminate`

Body: `{"pid": <int>}`. Calls `pg_terminate_backend(pid)`. Refuses to terminate the admin's own backend.

## Restart

### `POST /api/restart`

Restarts `gotrue` + `gateway` via `docker compose up -d --no-deps`. 409 if already running.

## Backups (pg_dump)

### `POST /api/backup/run`

Body (optional): `{"name": "..."}`. Streams pg_dump → S3.

### `GET /api/backup/list`

List of backup objects in S3. Paginated server-side; returns all entries.

### `POST /api/backup/delete`

Body: `{"name": "..."}`.

### `GET /api/backup/download?name=...`

Returns `{"url": "<presigned URL>"}`. URL valid for 10 minutes.

### `POST /api/backup/restore`

Body: `{"name": "...", "clean": true}`. Streams S3 → `pg_restore`. Mutex-guarded; 409 if another restore is running.

## pgBackRest

### `GET /api/pgbackrest/info`

Runs `pgbackrest info --output=json` inside the db container; returns the parsed JSON.

### `POST /api/pgbackrest/stanza-create`

Idempotent stanza initialization.

### `POST /api/pgbackrest/backup`

Body: `{"type": "full"|"diff"|"incr"}`. Async; returns 202.

### `GET /api/pgbackrest/status`

Current backup status + last result.

### `POST /api/pgbackrest/restore`

Body: `{"set": "<backup-label>"}`. Orchestrates stop→one-shot restore→start. Async.

### `GET /api/pgbackrest/restore/status`

Current restore phase + last result.

## Secrets

### `GET /api/secrets`

Returns the catalog of rotatable secrets with metadata.

### `POST /api/secrets/rotate`

Body: `{"key": "ADMIN_TOKEN"|"COOKIE_SIGNING_KEY"|"PG_META_CRYPTO_KEY"|"JWT_SECRET"}`. Generates new value(s), atomically writes `.env`, returns `{updated, restart, notes}`.

## Keys

### `GET /api/keys`

Returns the public `api_url` and `anon_key`. Safe to display.

### `POST /api/keys/service_role`

Returns the `service_role_key`. Wrapped in POST (rather than GET) to discourage accidental exposure in browser history.

## Error format

All error responses are JSON:

```json
{"error": "<message>"}
```

with HTTP status reflecting the failure mode (400 for client error, 401 for auth, 409 for conflict, 500 for server error).
