---
title: 备份
description: 两条路径——pg_dump（逻辑）和 pgBackRest（物理增量）。
---

SupaLite 提供两条独立的备份路径，目标是**同一个** S3 兼容存储。

## 选哪一条

| | pg_dump（逻辑）| pgBackRest（物理）|
|---|---|---|
| 抓什么 | schema + data 的 SQL | 原始 Postgres 文件 + WAL |
| 粒度 | 每次都全量 | 全量 + 增量 + 差分 |
| 恢复速度 | 慢（重放 SQL）| 快（文件级）|
| 跨版本 | 可以（旧 PG → 新 PG）| 不行（同主版本号）|
| 配置 | 开箱即用 | 自定义镜像 + Postgres 重启开 `archive_mode=on` |
| 存储成本 | 高（每次都全量）| 低（增量很小）|
| 选择性恢复 | 可以（`pg_restore -t table`）| 不行（整个 cluster）|
| 最少操作 | 点 Run Backup | Initialize Stanza → Run Full → Run Diff/Incr |

**给独立项目的建议**：先用 pg_dump。当每天的全量太慢或存储贵时再加 pgBackRest。

## pg_dump 路径

### 配置存储

在 `.env`（或管理面板 Settings → Backup tab）：

```bash
BACKUP_S3_ENDPOINT=https://<your-s3-endpoint>   # AWS 留空
BACKUP_S3_BUCKET=my-supalite-backups
BACKUP_S3_REGION=us-east-1                      # Cloudflare R2 用 'auto'
BACKUP_S3_ACCESS_KEY=...
BACKUP_S3_SECRET_KEY=...
BACKUP_S3_PATH_STYLE=false                       # MinIO/Ceph 设 true
BACKUP_S3_PREFIX=backup/
```

重启 admin 让新 env 生效：
```bash
docker compose up -d admin
```

### 跑一次备份

管理面板 → **Backups** → **Run Backup**。`pg_dump -Fc --no-owner --no-acl` 流式直传 S3——不落盘、任意数据库大小适用。30 分钟超时（极大 DB 应改用 pgBackRest）。

备份命名 `pgdump-YYYYMMDD-HHMMSS.dump`，同页面列出。

### 定时备份 + retention

`.env`：

```bash
BACKUP_SCHEDULE_HOURS=24       # 每天
BACKUP_RETENTION_COUNT=7       # 保留 7 份最新
```

重启 admin。调度器在 admin 进程里跑；定时备份命名带 `scheduled-` 前缀以区分手动备份。Retention **只删定时备份**——手动备份永不被动。

### 下载 / 删除

每行有 Download（10 分钟有效期 presigned URL）和 Delete（typed-name 确认）按钮。

## pgBackRest 路径

先读 [核心概念 → JWT](/supalite/zh/concepts/jwt/) 和 [架构总览](/supalite/zh/getting-started/architecture/) 理解 WAL 归档前置条件。

### 启用

三件事：

1. **复用 pg_dump 的 S3 配置** — pgBackRest 在同一桶里、`BACKUP_S3_PGBACKREST_PATH`（默认 `/pgbackrest`）下存。

2. **打开 WAL 归档** — `.env`：
   ```bash
   PGBACKREST_ARCHIVE_MODE=on
   PGBACKREST_ARCHIVE_COMMAND=pgbackrest --stanza=main archive-push %p
   PGBACKREST_WAL_LEVEL=replica
   PGBACKREST_MAX_WAL_SENDERS=3
   ```

3. **重启 Postgres** — `archive_mode` 是 postmaster 级 GUC，不能 reload：
   ```bash
   docker compose restart db
   ```

### 初始化 stanza

管理面板 → **Backups** → pgBackRest 区 → **Initialize Stanza**。幂等，可重复点。

### 跑备份

pgBackRest 卡片有 Full / Diff / Incr 三个按钮：

- **Full** — 全量数据文件。慢 + 大。
- **Diff** — 自上次 Full 以来的变化。中等。
- **Incr** — 自上次任意备份以来的变化。最小。

备份**异步**跑——UI 显示 "running" 自动刷新。大库可能数小时；UI 每 3s 轮询状态。

推荐节奏：每周 Full、每天 Diff、每小时 Incr。

### 恢复

见 [恢复](/supalite/zh/operations/restore/)——破坏性操作，独立一页。

## 数据存哪

- **pg_dump 对象**：`s3://${BACKUP_S3_BUCKET}/${BACKUP_S3_PREFIX}<filename>`
- **pgBackRest 仓库**：`s3://${BACKUP_S3_BUCKET}${BACKUP_S3_PGBACKREST_PATH}/`

故意分两个前缀——恢复工具不会混淆，删一边不影响另一边。

## 异地灾难恢复

VPS 挂了但 S3 还在：

1. 在新主机起一个新 SupaLite
2. 把 `BACKUP_S3_*` 配成老主机的值
3. **pg_dump 路径**：管理面板下载 `.dump` 文件后恢复（见 [恢复](/supalite/zh/operations/restore/)）
4. **pgBackRest 路径**：更复杂——pgBackRest 直接从 WAL archive 恢复，需要先在新主机手动初始化

恢复流程**用前先测**。
