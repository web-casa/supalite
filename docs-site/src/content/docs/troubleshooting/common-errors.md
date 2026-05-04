---
title: Common Errors
description: Symptoms, likely causes, fixes.
---

## Setup

### `setup.sh` fails: `port is already allocated`

Something else is using port 8000 (or 5432, or both). Find it:

```bash
sudo lsof -i :8000
```

Either stop the conflicting process, or change `GATEWAY_HTTP_BIND` and the port mapping in `docker-compose.yml`.

### `setup.sh` fails dependency check

Need `docker`, `docker compose` (or `docker-compose`), `openssl`, and `curl`. Install via your package manager. On Linux, ensure your user is in the `docker` group OR prefix commands with `sudo`.

### "permission denied" on `/var/run/docker.sock`

Either add your user to the `docker` group:

```bash
sudo usermod -aG docker $USER
# log out + back in
```

…or always invoke compose with `sudo`. The admin panel needs to read this socket; running everything as your user is more idiomatic.

## Container health

### `db` container shows `unhealthy`

Common causes:

1. **Volume permissions** (especially on rootless docker / OrbStack):
   ```
   chmod: changing permissions of '/var/lib/postgresql/data': Operation not permitted
   ```
   The user inside the container can't chmod the data dir. Restart Docker / OrbStack and retry. If persistent, recreate the volume: `docker compose down -v && docker compose up -d`.

2. **Healthcheck can't find pg_isready** — when the custom db image was built, `apt install pgbackrest` brought in a `postgresql-common` shim that overrode `/usr/bin/pg_isready` with one that doesn't know about the Nix-installed Postgres. Verify `/nix/var/nix/profiles/default/bin` is first in PATH inside the container:
   ```bash
   docker exec supalite-db-1 sh -c 'echo $PATH'
   ```
   Should show `/nix/var/...` before `/usr/bin`. If not, the Dockerfile needs the `ENV PATH=` fix.

3. **Postgres logs show real failures**:
   ```bash
   docker logs supalite-db-1 --tail 50
   ```

### `admin` container restarts in a loop

```bash
docker logs supalite-admin-1 --tail 20
```

Common: missing required env (`ADMIN_TOKEN`, `JWT_SECRET`, `DATABASE_URL`, `COOKIE_SIGNING_KEY`). The error message says exactly which one. Run `setup.sh` again to regenerate `.env`.

If logs show `regexp: Compile(... ) error`, an admin code bug — open an issue with the full error.

### `gateway` container 502 on `/admin/`

`admin` not running or not yet ready. Check `docker compose ps`. Wait 10s and retry.

## CORS / API

### Browser console: "blocked by CORS policy"

The frontend origin isn't in `CORS_ALLOWED_ORIGINS_REGEX`. Check it:

```bash
grep CORS /path/to/supalite/.env
```

Default falls back to `SITE_URL`. To allow more, set the regex (see [Multi-frontend](/supalite/configuration/multi-frontend/)). Restart gateway after any change.

### `/rest/v1/...` returns `401 Missing or invalid API key`

Frontend isn't sending the `apikey` header. Most common with raw `fetch` — `supabase-js` does this for you. Manual fix:

```js
fetch('http://localhost:8000/rest/v1/your_table', {
  headers: {
    'apikey': '<ANON_KEY>',
    'Authorization': 'Bearer <ANON_KEY>'   // or user JWT
  }
});
```

### `/rest/v1/...` returns rows, but not the ones you expect

RLS is filtering. As an admin, in Studio's SQL editor, run:

```sql
set role authenticated;
set request.jwt.claims = '{"sub":"<user-uuid>","role":"authenticated"}';
select * from your_table;
```

…to simulate the user's view. Compare with `set role service_role` to see what's actually in the table. The diff is what RLS is hiding.

### OAuth redirect 404 / `redirect_uri_mismatch`

Provider rejects the redirect URI you registered. Check:

1. Did you register exactly `${API_EXTERNAL_URL}/auth/v1/callback` (case-sensitive, trailing slash?) on the provider's dashboard?
2. Settings → GitHub/Google → **Test credentials** — the validator distinguishes "wrong creds" from "creds OK but redirect_uri not registered".

## SMTP

### "Send Test" succeeds but real signups don't email

Check `GOTRUE_MAILER_AUTOCONFIRM` — if it's `true`, GoTrue auto-confirms new users without emailing. Set to `false` once SMTP is configured.

### "Send Test" returns "STARTTLS required"

Port 587 servers MUST advertise STARTTLS. Some misconfigured relays don't. Try port 465 (implicit TLS) instead.

### "Send Test" returns "auth failed"

Wrong username/password. Double-check `GOTRUE_SMTP_USER` and `GOTRUE_SMTP_PASS`. For Gmail, you need an app-specific password, not your account password.

## Backups

### Backup runs forever / hangs

The 30-minute timeout fires; admin returns 500. If the actual cause was a slow S3 endpoint, switch to an S3 closer to your DB host or use pgBackRest (incremental, smaller transfers).

### "backup not configured"

`BACKUP_S3_BUCKET` and credentials aren't set. Settings → Backup tab.

### Restore fails partway

DB is in an inconsistent state. Either:
- Retry the same restore (often safe; pg_restore is idempotent for `--clean` mode)
- Restore from an earlier backup
- Wipe the data volume and start fresh (`docker compose down -v && docker compose up -d`, then restore)

## HTTPS

### Cert never provisioned

Caddy can't reach Let's Encrypt or LE can't reach you on :80. Check:

1. `docker logs supalite-gateway-1 -f` — what's Caddy saying?
2. `dig <your-domain>` — does DNS resolve to this host?
3. `curl http://<your-domain>` from another host — does it reach Caddy?
4. `CADDY_HTTPS_BIND` empty (= 0.0.0.0)? Loopback won't reach the internet.

### Browser still shows old cert after enable

Hard refresh (Ctrl/Cmd+Shift+R). If still cached, restart gateway: `docker compose restart gateway`.

## Where to ask

- GitHub issues: <https://github.com/web-casa/supalite/issues>
- Include: SupaLite version (`git rev-parse --short HEAD`), `docker compose ps` output, relevant log excerpts.
