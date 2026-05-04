---
title: 恢复
description: 从备份恢复。破坏性操作——先读完再做。
---

:::danger
**恢复是破坏性的。** 会覆盖或替换现有数据。无 undo。生产前先在非生产实例上演练。
:::

## pg_dump 恢复（逻辑）

### 从管理面板

1. 管理面板 → **Backups** tab → 找到备份行。
2. 点行上的 **rotate** 图标。
3. 弹窗说明要做什么：
   - Postgres 连接保持
   - `pg_restore --clean --if-exists` 跑在活着的 DB 上
   - 备份覆盖到的所有对象被 drop + recreate
4. 输入备份文件名确认。
5. 点 **Restore**。

handler 把 S3 对象流式喂给 `pg_restore` stdin（不落盘）。默认 30 分钟超时。失败 = 部分恢复，需要排查。

### 命令行

```bash
# 在 db 容器内自连
docker compose exec db pg_restore \
    --dbname postgres \
    --clean --if-exists --no-owner --no-acl \
    /path/to/backup.dump
```

S3 上的备份先下到本地，admin 面板下载或 `aws s3 cp` 都行。

## pgBackRest 恢复（物理）

:::caution
物理恢复期间**停 db 容器**。downtime 从几分钟（小库）到几小时（多 TB）不等。
:::

### 从管理面板

1. 管理面板 → **Backups** tab → pgBackRest 区。
2. 找到备份 label。
3. 点该行的 rotate 图标。
4. 弹窗说明做什么：
   - 停 db
   - 启一个共享 db-data 卷的 one-shot 容器跑 `pgbackrest --stanza=main --set=<label> --delta restore`
   - 启 db
5. 输入 backup label 确认。
6. 点 **Start Restore**。

UI 显示 phase 变化：*Stopping db* → *Restoring from backup* → *Starting db* → *Done*（或 *Error*）。

### 失败语义

恢复失败，**db 保持 stopped**。管理面板显示错误。恢复选择：

1. **排查** — `docker logs <孤儿恢复容器ID>` 看 pgBackRest 报了什么。如果清理也失败，UI 会告诉你容器 ID。
2. **重试恢复** — 通常安全；pgBackRest 自己能处理半完成态。
3. **手动启 db 用当前状态** — **只有**你决定要保留磁盘上现在的状态时：`docker compose up -d db`。

:::danger
失败的 `--delta` 恢复后**别盲目** `docker compose up -d db` —— data dir 可能被部分覆盖。Postgres 大概率拒绝启动，万一启起来了，数据处于未定义状态。
:::

### 孤儿容器检测

admin 在恢复中崩溃，one-shot 容器可能继续跑。下次恢复尝试时，admin 检查标签 `com.supalite.pgbackrest.op=restore` 的容器，若存在则拒绝再起一次直到清理：

```bash
docker rm -f <孤儿ID>
```

## 跨环境恢复

常见场景：生产 → 预发，复现真实 bug。

pg_dump 路径：
1. 从生产下备份：管理面板 → Download → 存 `.dump` 文件。
2. 拷到预发的 db 容器：
   ```bash
   docker cp backup.dump supalite-db-1:/tmp/
   docker compose exec db pg_restore --dbname postgres --clean --if-exists /tmp/backup.dump
   ```
3. 视需要脱敏（`UPDATE auth.users SET email = ...`）。

pgBackRest 跨环境要求 **stanza 名 + S3 路径** 一致。最简单是把相关 pgBackRest 对象拷到独立 S3 前缀，让预发的 `BACKUP_S3_PGBACKREST_PATH` 指过去。

## 备份 → 恢复演练

每月演练是对备份腐烂最便宜的保险：

1. 起一个临时 SupaLite（`docker compose -p drill up -d`）。
2. 把最近的备份恢复进去。
3. 跑几条查询确认数据对得上。
4. `docker compose -p drill down -v` 清理。

第 2 或第 3 步挂掉，就在你真要用前 30 天发现了。
