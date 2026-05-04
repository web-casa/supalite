---
title: Secret Rotation
description: When and how to rotate each credential.
---

The Secrets page (`/admin/secrets/`) handles rotation for the four secrets that have safe one-click rotation paths. POSTGRES_PASSWORD is intentionally NOT in the wizard — see below.

## Rotatable secrets

| Secret | Restart needed | Blast radius |
|---|---|---|
| `ADMIN_TOKEN` | None | Existing browser sessions keep working until cookie expires; bearer-token API callers (CLI/scripts) need new value. |
| `COOKIE_SIGNING_KEY` | `admin` | All admin browser sessions invalidated immediately; you log in again with current ADMIN_TOKEN. |
| `PG_META_CRYPTO_KEY` | `meta` + `studio` | Studio's saved project credentials become unreadable; you re-enter in Studio's UI. |
| `JWT_SECRET` | `rest` + `gotrue` + `admin` + `gateway` | **Cascading.** ANON_KEY and SERVICE_ROLE_KEY automatically re-minted with new secret. Every signed-in user logged out. Every client app needs new ANON_KEY. |

## Why POSTGRES_PASSWORD is not rotatable

It's used by all 12 supabase roles + every connecting service. Rotation requires:

1. `ALTER USER <each_role> WITH PASSWORD '<new>'` against the live DB
2. Update `.env`
3. Restart every service that connects to PG

Step 1 has many partial-failure modes — if half the ALTER USER calls succeed and half fail, you're left with a broken cluster. Worth a careful manual procedure (and a backup beforehand), not a one-click button.

If you really need to rotate it:

```sql
-- Run in psql as supabase_admin (or any superuser)
ALTER USER anon WITH PASSWORD 'new-password';
ALTER USER authenticated WITH PASSWORD 'new-password';
ALTER USER service_role WITH PASSWORD 'new-password';
ALTER USER authenticator WITH PASSWORD 'new-password';
ALTER USER supabase_admin WITH PASSWORD 'new-password';
ALTER USER supabase_auth_admin WITH PASSWORD 'new-password';
ALTER USER supabase_storage_admin WITH PASSWORD 'new-password';
ALTER USER supabase_replication_admin WITH PASSWORD 'new-password';
ALTER USER supabase_read_only_user WITH PASSWORD 'new-password';
ALTER USER supabase_functions_admin WITH PASSWORD 'new-password';
ALTER USER dashboard_user WITH PASSWORD 'new-password';
ALTER USER pgbouncer WITH PASSWORD 'new-password';
```

Then update `POSTGRES_PASSWORD` in `.env`, then `docker compose up -d` to restart everything.

## How a rotation goes

1. Open `/admin/secrets/`.
2. Click **Rotate** on the card you want.
3. Type the secret key name (e.g. `JWT_SECRET`) in the confirm dialog.
4. Click **Rotate**.
5. The page shows a green banner with:
   - List of env keys that changed
   - List of services to restart
   - One-line `docker compose up -d --no-deps ...` you can copy-paste

The wizard does NOT auto-restart services — you confirm by running the restart yourself, so you stay in control of when downtime happens.

## After JWT rotation

Three things to do immediately:

1. **Restart the listed services** — `rest`, `gotrue`, `admin`, `gateway`. Old user JWTs start failing on next request.
2. **Copy new ANON_KEY** from the Dashboard. Update every client app (mobile, web, scripts).
3. **Notify users** that they need to log in again — every signed-in session is now invalid.

The Dashboard's keys query is auto-invalidated after rotation, so refreshing the Dashboard tab shows the new ANON_KEY immediately.

:::caution[Restart timing]
The wizard does NOT auto-restart services. You see exactly which services need restart and run the command yourself — so a multi-service rotation doesn't take down everything in a single uncontrolled wave.
:::

## When to rotate

- **`ADMIN_TOKEN`** — after sharing in chat / log file / screenshot. Quarterly as hygiene.
- **`COOKIE_SIGNING_KEY`** — annual rotation; emergency if you suspect a session theft.
- **`PG_META_CRYPTO_KEY`** — annual; only matters if Studio's encrypted project list could be exfiltrated.
- **`JWT_SECRET`** — annual at most (forces all users to re-login). Emergency rotation if your existing JWT_SECRET ever leaks.

There's no audit log of past rotations yet — your `git log` of `.env` (if you commit it, which you shouldn't) or your secret-manager's history is the source of truth.
