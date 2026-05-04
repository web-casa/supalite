---
title: DB Maintenance
description: VACUUM / ANALYZE / REINDEX from the admin panel + session inspection.
---

Two pages handle Postgres day-to-day:

- **`/admin/dbops/`** — VACUUM, ANALYZE, REINDEX
- **`/admin/sessions/`** — `pg_stat_activity` view + session termination

## DB Ops page

Pick an operation and an optional target.

| Operation | Use it for |
|---|---|
| **VACUUM** | Reclaim space from deleted/updated rows. Non-blocking. Run when bloat is suspected. |
| **VACUUM FULL** | Rewrite the table to compact storage. Acquires an `AccessExclusiveLock` — table is unavailable for the duration. |
| **ANALYZE** | Refresh statistics so the query planner makes better choices. Cheap, non-blocking. |
| **REINDEX** | Rebuild indexes. Useful after large data changes or if an index is suspected to be corrupted. Locks the table. |

### Target

- **Empty target** — operates on the whole `postgres` database (e.g., `VACUUM` on every table).
- **`schema.table`** — operates on that specific relation only (e.g., `public.users`).

REINDEX **requires** a `schema.table` target — schema-level reindex isn't supported (would be ambiguous when a schema is named like a table).

### Safety

The `target` field is regex-validated to `^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$` AND wrapped through `pgx.Identifier.Sanitize` before going into SQL. Both layers reject anything that could be a SQL injection attempt. If the request shape doesn't match, the handler 400s without ever touching the DB.

10-minute timeout per operation. Long ops (huge VACUUM FULL) may need a maintenance window outside the admin panel.

## Sessions page

Lists `pg_stat_activity` rows with auto-refresh every 5s. Polling pauses when the tab is hidden (no thundering-herd refetch).

| Column | Meaning |
|---|---|
| PID | Backend process ID |
| User | Postgres role |
| DB | `postgres` (we have one DB) |
| State | `active` (running query), `idle`, `idle in transaction` (red — open txn holding locks) |
| App / Client | App name + client IP |
| Wait | What the backend is blocked on, if anything |
| Query | Trimmed text of the current/last query |

By default only **client backends** are shown. Toggle "include system" to see autovacuum, walwriter, etc.

### Terminating a backend

- Click the **X** on a row.
- Type the PID in the confirm dialog.
- Click **Terminate**.

Sends `pg_terminate_backend(pid)`. The connection sees a fatal error, in-flight transactions roll back. Use this for runaway queries or to free locks.

You **cannot** terminate the admin process's own backend(s) — a guard checks `pid <> pg_backend_pid()`. Other admin pool connections are technically targetable; killing one just causes pgxpool to reconnect. Inconvenient but not destructive.

## Common workflows

### Bloat investigation

```sql
-- In Studio's SQL editor or psql
SELECT schemaname, relname, n_dead_tup, n_live_tup,
       round(n_dead_tup * 100.0 / nullif(n_live_tup, 0), 1) AS dead_pct
FROM pg_stat_user_tables
WHERE n_dead_tup > 1000
ORDER BY dead_pct DESC NULLS LAST;
```

If a table has high dead_pct, run **VACUUM** on it from the admin panel. Run **VACUUM FULL** only if VACUUM doesn't free enough space.

### Long-running query investigation

Sessions page → look for `state=active` with old `query_start`. The Query column shows what they're doing. Terminate if necessary.

### Locked tables

Sessions page → look for `state=idle in transaction` (red badge). These hold locks without doing useful work. Terminating them releases the locks; the affected client sees a fatal error.

If you'd rather not yank — sometimes the long transaction is doing important work — find the responsible application and have it commit/rollback.
