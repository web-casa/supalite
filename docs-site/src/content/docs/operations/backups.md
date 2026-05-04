---
title: Backups
description: Two paths — pg_dump (logical) and pgBackRest (physical incremental).
---

SupaLite ships two independent backup paths that target the **same** S3-compatible storage.

## Choosing a path

| | pg_dump (logical) | pgBackRest (physical) |
|---|---|---|
| What it captures | SQL of schema + data | Raw Postgres files + WAL |
| Granularity | Full each time | Full + incremental + differential |
| Restore speed | Slow (replays SQL) | Fast (file-level) |
| Cross-version | Yes (PG → newer PG) | No (same major version only) |
| Setup | None — works out of box | Custom image + Postgres restart with `archive_mode=on` |
| Storage cost | High (full each time) | Low (incrementals are tiny) |
| Selective restore | Yes (`pg_restore -t table`) | No (restore is whole-cluster) |
| Minimum admin path | Run Backup button | Initialize Stanza → Run Full → Run Diff/Incr |

**Recommendation for indie projects**: start with pg_dump. Add pgBackRest only when nightly full dumps become too slow or storage-expensive.

## pg_dump path

### Configure storage

In `.env` (or admin Settings → Backup tab):

```bash
BACKUP_S3_ENDPOINT=https://<your-s3-endpoint>   # leave empty for AWS
BACKUP_S3_BUCKET=my-supalite-backups
BACKUP_S3_REGION=us-east-1                      # 'auto' for Cloudflare R2
BACKUP_S3_ACCESS_KEY=...
BACKUP_S3_SECRET_KEY=...
BACKUP_S3_PATH_STYLE=false                       # true for MinIO/Ceph
BACKUP_S3_PREFIX=backup/
```

Restart admin so the new env takes effect:
```bash
docker compose up -d admin
```

### Run a backup

Admin panel → **Backups** → **Run Backup**. Streams `pg_dump -Fc --no-owner --no-acl` directly into S3 — no temp files, works for any database size. 30-minute timeout (very large DBs may need pgBackRest instead).

Backups are named `pgdump-YYYYMMDD-HHMMSS.dump` and listed on the same page.

### Scheduled backups + retention

Add to `.env`:

```bash
BACKUP_SCHEDULE_HOURS=24       # daily
BACKUP_RETENTION_COUNT=7       # keep 7 newest
```

Restart admin. The scheduler runs in-process; backups are named `scheduled-...` to distinguish from manual ones. Retention **only deletes scheduled backups** — your manual backups are never touched.

### Download / delete

Each row has Download (presigned 10-minute URL) and Delete (typed-name confirm) buttons.

## pgBackRest path

See [Concepts → JWT](/supalite/concepts/jwt/) and [Architecture](/supalite/getting-started/architecture/) first to understand the WAL-archiving prerequisites.

### Enable

Three things have to happen:

1. **Reuse the same S3 config** as pg_dump — pgBackRest stores under `BACKUP_S3_PGBACKREST_PATH` (default `/pgbackrest`) inside the same bucket.

2. **Turn on WAL archiving** — `.env`:
   ```bash
   PGBACKREST_ARCHIVE_MODE=on
   PGBACKREST_ARCHIVE_COMMAND=pgbackrest --stanza=main archive-push %p
   PGBACKREST_WAL_LEVEL=replica
   PGBACKREST_MAX_WAL_SENDERS=3
   ```

3. **Restart Postgres** — `archive_mode` is a postmaster-level GUC, not reloadable:
   ```bash
   docker compose restart db
   ```

### Initialize the stanza

Admin panel → **Backups** → pgBackRest section → **Initialize Stanza**. Idempotent; safe to re-run.

### Run a backup

The pgBackRest card has Full / Diff / Incr buttons:

- **Full** — complete copy of all data files. Slow + large.
- **Diff** — changes since the last full. Smaller.
- **Incr** — changes since the last backup of any kind. Smallest.

Backups run **asynchronously** — the UI shows "running" with auto-refresh. Each one can take hours on large databases; the UI polls status every 3s.

Recommended cadence: weekly Full, daily Diff, hourly Incr.

### Restore

See [Restore](/supalite/operations/restore/) — destructive operation, gets its own page.

## Where data lives

- **pg_dump objects**: `s3://${BACKUP_S3_BUCKET}/${BACKUP_S3_PREFIX}<filename>`
- **pgBackRest repo**: `s3://${BACKUP_S3_BUCKET}${BACKUP_S3_PGBACKREST_PATH}/`

These are deliberately separate prefixes — restore tools won't confuse one for the other, and you can prune one without affecting the other.

## Off-host disaster recovery

If your VPS dies and S3 survives:

1. Spin up a new SupaLite on a fresh host.
2. Configure `BACKUP_S3_*` to match the old host.
3. **pg_dump path**: download a `.dump` file via the admin panel and restore it (see [Restore](/supalite/operations/restore/)).
4. **pgBackRest path**: more involved — pgBackRest restore reads the WAL archive directly. You'll need to manually initialize on the new host first.

Test your restore path **before** you need it.
