---
title: Roadmap
description: What's next.
---

## In flight

- **Audit log** — record destructive admin actions (restart, restore, terminate, secret rotation) for after-the-fact accountability.
- **Apple OAuth credential validation** — current `Test credentials` button skips Apple because Apple's client_secret is a short-lived JWT, not a static string.

## Considered

- **Prometheus `/metrics` endpoint** — basic gauges (services up/down, last-backup-age, db connections).
- **OpenAPI spec for admin API** — enables `openapi-generator` clients in any language.
- **Scheduled pgBackRest backups** — currently only pg_dump path has a scheduler.
- **Per-secret rotation history** — track last-rotated timestamp + who triggered.
- **WAL archive consistency banner** — when pgBackRest is enabled but `archive_mode` is still off, surface in admin sidebar.

## Deliberately out of scope

These were considered and rejected:

- **Postgres HA / replication** — at this complexity, run real Postgres operators (CloudNativePG, Crunchy).
- **Multi-tenant data isolation** — schema-per-tenant or DB-per-tenant. Run multiple SupaLite instances instead.
- **Storage / Edge Functions / Realtime** — the upstream Supabase services we cut. If you need them, run upstream.
- **Studio embedded config panel** — Studio is opinionated about its own setup; exposing its config in our admin adds maintenance burden for low information value.
- **POSTGRES_PASSWORD rotation wizard** — too many partial-failure modes for one-click. Manual procedure documented in [Secret Rotation](/supalite/operations/secret-rotation/).

## Help wanted

If you want to contribute one of the "Considered" items, file an issue first to align on scope. PRs that add scope without prior discussion may be asked to split.
