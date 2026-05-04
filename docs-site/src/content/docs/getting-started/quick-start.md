---
title: Quick Start
description: From `git clone` to a working Postgres backend in three minutes.
---

You need a host with **Docker** + **Docker Compose** installed. SupaLite expects Linux; macOS / Windows via Docker Desktop or OrbStack works for development.

## 1. Clone and run setup

```bash
git clone https://github.com/web-casa/supalite.git
cd supalite
./setup.sh
```

`setup.sh` does three things:

1. Generates all secrets — `POSTGRES_PASSWORD`, `JWT_SECRET`, `ADMIN_TOKEN`, `COOKIE_SIGNING_KEY`, `PG_META_CRYPTO_KEY` — and writes them to `.env` (mode 0600).
2. Mints `ANON_KEY` and `SERVICE_ROLE_KEY` JWTs signed with `JWT_SECRET`.
3. `docker compose pull` then `docker compose up -d`, then health-checks every public endpoint.

It prints the admin token at the end. **Save it** — it's also stored in `.env` if you lose it.

:::tip[OS notes]
Native Linux is the primary target. macOS / Windows via Docker Desktop or OrbStack work for development; OrbStack handles volume permissions more cleanly than Docker Desktop on Apple Silicon.
:::

## 2. Open the admin panel

```
http://localhost:8000/admin/
```

Log in with the `ADMIN_TOKEN` printed by `setup.sh`. The first-run wizard walks you through:

- Setting `SITE_URL` and `API_EXTERNAL_URL`
- Configuring SMTP (with a live "Send Test" button)
- Showing your `ANON_KEY` to copy into your frontend

## 3. Open Studio

```
http://localhost:8000/studio/
```

Studio is the upstream Supabase UI — table editor, SQL editor, auth users, function browser. Use it for application development. Use `/admin/` for ops.

## 4. Talk to the API from your frontend

```js
import { createClient } from '@supabase/supabase-js';

const supabase = createClient(
  'http://localhost:8000',     // API_EXTERNAL_URL
  '<ANON_KEY from /admin/>',
);

const { data } = await supabase.from('your_table').select('*');
```

For a complete working example, see [Next.js Todo](/supalite/examples/nextjs-todo/).

## What's running

```
$ docker compose ps
NAME                STATUS              PORTS
supalite-db-1       Up (healthy)        127.0.0.1:5432->5432/tcp
supalite-rest-1     Up
supalite-gotrue-1   Up
supalite-meta-1     Up (healthy)
supalite-studio-1   Up (healthy)
supalite-admin-1    Up
supalite-gateway-1  Up                  0.0.0.0:8000->8000/tcp
```

Only `gateway` is exposed externally (`:8000`). Postgres binds to loopback by default; flip `POSTGRES_BIND_ADDR=0.0.0.0` in `.env` only if you need direct DB access from outside.

## Next steps

- [Architecture](/supalite/getting-started/architecture/) — how the pieces fit together
- [Environment Reference](/supalite/configuration/environment-reference/) — every `.env` field annotated
- [Backups](/supalite/operations/backups/) — set up automatic backups before you need them
- [HTTPS](/supalite/configuration/https-tls/) — enable Let's Encrypt for production
