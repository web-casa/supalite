---
title: Restore
description: Recover from a backup. Destructive operations — read first.
---

:::danger
**Restore is destructive.** It overwrites or replaces existing data. There is no undo. Practice the procedure on a non-production instance first.
:::

## pg_dump restore (logical)

### From the admin panel

1. Admin panel → **Backups** tab → find the backup row.
2. Click the **rotate** icon on the row.
3. The dialog shows what will happen:
   - SUPa Postgres connection stays up.
   - `pg_restore --clean --if-exists` runs against the live DB.
   - All current objects covered by the backup get dropped + recreated.
4. Type the backup filename to confirm.
5. Click **Restore**.

The handler streams the S3 object directly into `pg_restore` stdin (no temp files). Capped at 30 minutes by default. On failure, partial restore — you'll need to investigate.

### From the command line

```bash
# Inside the db container, talking to itself
docker compose exec db pg_restore \
    --dbname postgres \
    --clean --if-exists --no-owner --no-acl \
    /path/to/backup.dump
```

For a backup that lives in S3, download it first via admin or `aws s3 cp`.

## pgBackRest restore (physical)

:::caution
Physical restore **stops the db container** for the duration of the restore. Expect downtime measured in minutes (small DB) to hours (multi-TB).
:::

### From the admin panel

1. Admin panel → **Backups** tab → pgBackRest section.
2. Find the backup label in the list.
3. Click the rotate icon on that row.
4. The dialog shows what happens:
   - Stop db
   - Run `pgbackrest --stanza=main --set=<label> --delta restore` in a one-shot container that mounts the same db-data volume
   - Start db back up
5. Type the backup label to confirm.
6. Click **Start Restore**.

The UI shows phase transitions: *Stopping db* → *Restoring from backup* → *Starting db* → *Done* (or *Error*).

### Failure semantics

If restore fails, **db stays stopped**. The admin panel surfaces the error. Recovery options:

1. **Investigate** — `docker logs <orphan-restore-container-id>` to see what pgBackRest saw. The UI tells you the container ID if cleanup also failed.
2. **Retry restore** — usually safe; pgBackRest handles partial state.
3. **Manually start db with current state** — only if you've decided you want whatever's on disk now: `docker compose up -d db`.

:::danger
Don't blindly `docker compose up -d db` after a failed `--delta` restore — the data dir might be partially overwritten. Postgres will likely refuse to start, but if it does start, the data is in an undefined state.
:::

### Orphan container detection

If admin crashes mid-restore, the one-shot container may keep running. On the next restore attempt, admin checks for any container labeled `com.supalite.pgbackrest.op=restore` and refuses to start a second one until you remove it:

```bash
docker rm -f <orphan-id>
```

## Cross-environment restore

Common scenario: production → staging restore so you can debug a real bug.

For pg_dump path:
1. Download a backup from prod: admin panel → Download → save the `.dump` file.
2. On staging, copy it into the db container:
   ```bash
   docker cp backup.dump supalite-db-1:/tmp/
   docker compose exec db pg_restore --dbname postgres --clean --if-exists /tmp/backup.dump
   ```
3. Optionally scrub user PII (`UPDATE auth.users SET email = ...`).

For pgBackRest, restoring across environments requires the **stanza name + S3 path** match. Easiest is to copy the relevant pgBackRest objects to a separate S3 prefix and point staging's `BACKUP_S3_PGBACKREST_PATH` at it.

## Backup → Restore drill

A monthly drill is the cheapest insurance against backup-rot:

1. Spin up a throwaway SupaLite (`docker compose -p drill up -d`).
2. Restore your most recent backup into it.
3. Verify a few queries return expected data.
4. `docker compose -p drill down -v` to clean up.

If step 2 or 3 fails, you've found out 30 days before you'd have needed it for real.
