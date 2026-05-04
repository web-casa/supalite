---
title: What is SupaLite?
description: A self-hosted Postgres backend, assembled from upstream Supabase images minus the parts most small deployments don't need.
---

SupaLite is a **self-hosted Postgres backend** that bundles:

- **PostgreSQL 15** (via `supabase/postgres`) with 12 Supabase roles and 13 extensions pre-installed
- **PostgREST** for an auto-generated REST API derived from your schema
- **GoTrue** for email + OAuth authentication and JWT minting
- **Supabase Studio** (optional) for a UI table editor / SQL editor
- **A Go-powered admin panel** for day-2 operations: backups, secret rotation, log tailing, DB maintenance

It's **what's left after we cut the Supabase pieces most small projects don't need**: Storage, Edge Functions, and Realtime. Those services have real value at scale but force operational complexity that doesn't pay for itself when you're a solo developer or a small team.

## Who is this for?

- **Indie developers** building a side project who want a real Postgres backend without paying for hosted Supabase
- **Small startups** wanting to keep the Supabase-style developer experience (auto REST API, RLS, JWT auth) on infrastructure they control
- **Self-hosting enthusiasts** who already run Docker on a VPS and want a turnkey database stack

## Who is it NOT for?

- Teams that need horizontal scaling, multi-region, or HA Postgres — go run real Postgres operators like CloudNativePG
- Teams that critically need Storage / Edge Functions / Realtime — run upstream Supabase
- Multi-tenant SaaS with hard data-isolation requirements — run separate SupaLite instances per tenant

## How is it different from upstream Supabase self-hosted?

| | Upstream Supabase | SupaLite |
|---|---|---|
| Services | 10+ containers | 4 core + 2 optional |
| RAM footprint | ~3-4 GB | ~1.5 GB |
| Day-2 ops | docker compose + raw SQL | Admin panel for the common cases |
| Setup | Manual env editing | `./setup.sh` |
| Backups | Roll your own | pg_dump + pgBackRest built in |

If you're hitting limits SupaLite imposes (no Storage, no Realtime), the upgrade path is straightforward: switch to upstream Supabase using the same Postgres data volume.
