---
title: Ports & Networking
description: What binds where, why.
---

## Default port map

```
host:8000     ───►  gateway      :8000  (Caddy)
host:5432     ───►  db           :5432  (Postgres, loopback only by default)
                      └─ internal-only network access from rest, gotrue, admin

container-only:
  rest         :3000
  gotrue       :9999
  meta         :8080  (only with COMPOSE_PROFILES=studio)
  studio       :3000  (same)
  admin        :9100
```

The host only sees `:8000` (and `:5432` on loopback). Everything else is on the internal Docker bridge network `supalite_default`.

## Why Postgres is loopback by default

A Postgres binding `0.0.0.0:5432` to the public internet is a common foot-gun — bots scan the port and try to brute-force the password. SupaLite defaults `POSTGRES_BIND_ADDR=127.0.0.1` so only the host itself can connect directly.

Apps inside the Docker network use `db:5432` (resolves via Docker DNS), not the host port. The host port is for `psql` from your shell.

To expose Postgres externally (only if you know why):

```bash
# .env
POSTGRES_BIND_ADDR=0.0.0.0
```

Then either firewall :5432 to a known IP allowlist OR set up `pg_hba.conf` carefully (the supabase/postgres image's hba is permissive by default).

## Ports added when HTTPS is enabled

The `docker-compose.https.yml` override adds:

```
host:80       ───►  gateway      :80   (ACME challenge + HTTP→HTTPS redirect)
host:443      ───►  gateway      :443  (TLS-terminated traffic)
```

`CADDY_HTTPS_BIND` controls the host IP for these. Empty (default) = `0.0.0.0` (required for Let's Encrypt to reach :80 from the public internet).

## Inter-container DNS

Compose creates `supalite_default` network. Containers reach each other by their service name:

| Origin | Target |
|---|---|
| rest → `postgres://...@db:5432/...` | db |
| gotrue → `postgres://...@db:5432/...` | db |
| admin → `http://docker/...` (via docker.sock) | docker daemon |
| admin → DB pool | db (via DATABASE_URL) |
| meta → db | db |
| studio → meta | meta |
| Caddy `reverse_proxy rest:3000` | rest |

If you scale (compose v2 doesn't really, but you could `docker run` a second), service names round-robin via DNS. Not relevant for SupaLite (single-replica architecture).

## Volumes

| Volume | Holds | Lifecycle |
|---|---|---|
| `supalite_db-data` | Postgres data dir | **Persistent.** `docker compose down` keeps it; `down -v` wipes it |
| `caddy-data` | Let's Encrypt certs + ACME state | Persistent (only created with HTTPS enabled) |
| `caddy-config` | Caddy autosave config | Persistent |

Bind mounts (read-only):

- `./volumes/db/init/zz-supalite.sh` → init script for first-start of db
- `./volumes/api/Caddyfile` → gateway config
- `./volumes/api/www` → static landing page at `/`
- `./` → `/project` (admin's `working_dir`, lets it `docker compose` against the same `.env`)

`/var/run/docker.sock` is mounted into admin so it can:
- Tail container logs
- Inspect peer containers
- Exec pgbackrest commands inside db
- Create one-shot containers for restore
- Start/stop containers for scheduled restart

This is **deliberately powerful** — admin needs sibling-container management to do its job. The trust model is "admin-panel access ⇒ root on the host".

## Egress

For outbound traffic from inside containers:

- gotrue → SMTP servers (your configured host)
- gotrue → OAuth providers (GitHub/Google/Apple APIs)
- admin → S3-compatible bucket (for backups + presigned URLs)
- db → S3 (when pgBackRest is enabled, for WAL archive + backups)

If you firewall egress, allowlist:
- Your SMTP host
- `api.github.com`, `oauth2.googleapis.com`, `appleid.apple.com` (the OAuth providers you've enabled)
- Your S3 endpoint

For Let's Encrypt: allow outbound to `acme-v02.api.letsencrypt.org` and the OCSP responder.
