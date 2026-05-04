---
title: Compose Profiles
description: Optional services and multi-instance deployment.
---

## What profiles control

Docker Compose [profiles](https://docs.docker.com/compose/profiles/) let you mark services as opt-in. SupaLite uses one profile:

| Profile | Services it activates | Default? |
|---|---|---|
| `studio` | `meta`, `studio` | **Yes** (set via `COMPOSE_PROFILES=studio` in `.env`) |

Set `COMPOSE_PROFILES=` (empty) to run the **minimal stack** — just db, rest, gotrue, admin, gateway. Saves ~800 MB of images and ~500 MB RAM. Useful for headless deployments where you only access via API.

## Other compose files

| File | Purpose |
|---|---|
| `docker-compose.yml` | Default stack |
| `docker-compose.https.yml` | Override that adds `:80`/`:443` host port bindings for auto-HTTPS |

Combine with `-f`:

```bash
docker compose -f docker-compose.yml -f docker-compose.https.yml up -d
```

## Multiple unrelated projects on one host

Different products, shared hardware: run one compose project per app. Each gets its own Postgres data volume, admin token, and ports.

```bash
# Project A
cp .env .env.a
$EDITOR .env.a   # unique POSTGRES_PASSWORD, ADMIN_TOKEN, GATEWAY_HTTP_BIND port etc.
docker compose -p app-a --env-file .env.a up -d

# Project B
cp .env .env.b
$EDITOR .env.b   # change gateway port too if reusing port 8000
docker compose -p app-b --env-file .env.b up -d
```

Each project's containers are namespaced (`app-a-db-1`, `app-b-db-1`). Volumes are isolated (`app-a_db-data`, `app-b_db-data`). Route external traffic with a top-level Caddy/Traefik by domain.

This is the right pattern when projects:
- Have **no shared user base**
- Need blast-radius isolation (A compromised must not leak B)
- Have different upgrade cadences

For one product across multiple frontends, use a single instance + multi-frontend config — see [Multi-frontend](/supalite/configuration/multi-frontend/).
