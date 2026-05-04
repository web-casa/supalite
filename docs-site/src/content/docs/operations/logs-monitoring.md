---
title: Logs & Monitoring
description: Live tail any container; service status at a glance.
---

## Live log tail

`/admin/logs/` streams stdout + stderr from any container in real time using Server-Sent Events (SSE).

- Pick a service from the dropdown — `db`, `rest`, `gotrue`, `gateway`, `admin`.
- Pick how many initial lines to backfill (50 / 100 / 500).
- The "live" indicator turns green when the stream is connected.
- Click **Pause** to freeze; **Resume** to reconnect (closes + reopens the SSE; lines emitted while paused are missed).

The SSE connection runs through Caddy unbuffered and through the admin's docker-API client (which itself opens a follow=true log tail on the Docker daemon). When you close the tab or click Pause, the upstream Docker connection closes too — no orphan readers.

Capped at 2000 lines client-side; older lines drop off the top so the DOM stays responsive.

## Service status

Two surfaces show service-status:

1. **Sidebar dots** — every page has a "Services" section in the sidebar with one colored dot per running container. Updates live via SSE on `/api/status/stream` (no polling).
2. **Dashboard** — full table at `/admin/`.

Colors:

- **Green** — `running`
- **Red** — `exited`
- **Yellow** — anything in between (`restarting`, `created`, `dead`)

If a service shows red, the most useful next step is checking its logs from the Logs page.

## What's NOT in the admin panel

- **Postgres metrics** (TPS, lag, query timings) — use Studio's Reports page or hit `pg_stat_*` views directly via SQL.
- **Caddy access logs** — `docker logs supalite-gateway-1` shows them. The Logs page can also tail gateway.
- **Alerting** — none built in. Wire up an external uptime check (e.g. `curl /health` from UptimeRobot) for production deployments.

## Diagnosing a sick stack

1. Sidebar dots — what's not green?
2. Open Logs → that service → scroll back ~100 lines for the panic / error.
3. If db is unhealthy, `docker inspect supalite-db-1 --format '{{.State.Health.Log}}'` shows the last healthcheck output.
4. If you suspect resource exhaustion, `docker stats` on the host shows CPU/RAM per container.

## Restarting services

Settings → **Restart Services** restarts `gotrue` + `gateway` (the most commonly-restarted pair after config edits). For other services, `docker compose restart <service>` from the host is the explicit lever.

The admin doesn't auto-restart on its own .env changes — you stay in control of when downtime happens.
