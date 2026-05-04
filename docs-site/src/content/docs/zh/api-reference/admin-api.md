---
title: Admin API 参考
description: 管理面板暴露的所有端点。
---

所有 admin 端点在 `/admin/api/` 下。鉴权两选一：

- **Cookie**：登录后设的 HMAC 签名 `sbl_auth`（SPA 自己用）。
- **Bearer**：`Authorization: Bearer <ADMIN_TOKEN>`（脚本/CLI 用）。

两个端点绕过鉴权中间件：`POST /api/auth/verify`、`POST /api/auth/logout`。

错方法请求（如 `PUT /api/keys`）返回 JSON 404（`/` catch-all 接住，不返 405；表现得像通用"找不到"）。

## Auth

### `POST /api/auth/verify`

登录。发 `Authorization: Bearer <ADMIN_TOKEN>`。成功后设 `sbl_auth` cookie。每 IP 每分钟 10 次限流。

### `POST /api/auth/logout`

清 `sbl_auth` cookie。

### `POST /api/auth/smtp-test`

Body：`{"to": "user@example.com"}`。用当前保存的 SMTP 配置发测试邮件。返回 200 / 4xx / 502 + 诊断消息。

### `POST /api/auth/oauth-test`

Body：`{"provider": "github"|"google"}`。用当前保存的 client_id/secret 探测 provider API。

## Config

### `GET /api/config` / `POST /api/config`

GET 返回 `.env` 可编辑子集 `{section: {key: value}}`；secret 值用 `__SECRET_UNCHANGED__` 替换。POST 接同样结构；回传 placeholder 不动既有值。

## Status / Logs

### `GET /api/status`

SupaLite 项目所有容器及其状态。

### `GET /api/status/stream`

容器状态变化时推 `snapshot` SSE 事件。15s 心跳。

### `GET /api/logs?service=<name>&lines=<n>`

一次性拉最近 N 行（默认 100，最大 1000）。

### `GET /api/logs/stream?service=<name>&tail=<n>`

实时推日志行的 SSE。

## DB

### `POST /api/dbops/maintenance`

Body：`{"op":"vacuum"|"analyze"|"reindex","target":"schema.table","full":bool}`。

### `GET /api/sessions?system=true`

`pg_stat_activity` 行；`system=true` 含 background worker。

### `POST /api/sessions/terminate`

Body：`{"pid": <int>}`。调 `pg_terminate_backend(pid)`。拒绝杀 admin 自己的后端。

## Restart

### `POST /api/restart`

通过 `docker compose up -d --no-deps` 重启 `gotrue` + `gateway`。已在运行返 409。

## Backups (pg_dump)

### `POST /api/backup/run`

Body（可选）：`{"name": "..."}`。pg_dump → S3 流式。

### `GET /api/backup/list`

S3 里的备份对象列表。服务端分页，返回全部条目。

### `POST /api/backup/delete`

Body：`{"name": "..."}`。

### `GET /api/backup/download?name=...`

返回 `{"url": "<presigned URL>"}`。10 分钟有效。

### `POST /api/backup/restore`

Body：`{"name": "...", "clean": true}`。S3 → `pg_restore` 流式。互斥；另一恢复运行中返 409。

## pgBackRest

### `GET /api/pgbackrest/info`

在 db 容器内跑 `pgbackrest info --output=json`，返回解析后的 JSON。

### `POST /api/pgbackrest/stanza-create`

幂等的 stanza 初始化。

### `POST /api/pgbackrest/backup`

Body：`{"type": "full"|"diff"|"incr"}`。异步；返 202。

### `GET /api/pgbackrest/status`

当前备份状态 + 最近一次结果。

### `POST /api/pgbackrest/restore`

Body：`{"set": "<backup-label>"}`。编排 stop→one-shot restore→start。异步。

### `GET /api/pgbackrest/restore/status`

当前恢复阶段 + 最近一次结果。

## Secrets

### `GET /api/secrets`

返回可轮换密钥目录及元数据。

### `POST /api/secrets/rotate`

Body：`{"key": "ADMIN_TOKEN"|"COOKIE_SIGNING_KEY"|"PG_META_CRYPTO_KEY"|"JWT_SECRET"}`。生成新值，原子写 `.env`，返回 `{updated, restart, notes}`。

## Keys

### `GET /api/keys`

返回公开的 `api_url` 和 `anon_key`。可安全显示。

### `POST /api/keys/service_role`

返回 `service_role_key`。包成 POST（而非 GET）避免不小心写进浏览器历史。

## 错误格式

所有错误响应是 JSON：

```json
{"error": "<message>"}
```

HTTP status 反映失败类型（400 客户端、401 鉴权、409 冲突、500 服务端）。
