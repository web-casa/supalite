---
title: 环境变量参考
description: 每个 .env 字段的解释。
---

所有配置都在仓库根的 `.env`。大多数字段也能从管理面板 Settings 页编辑（写回 `.env` 是原子的）。

## 密钥

| 变量 | 由谁生成 | 备注 |
|---|---|---|
| `POSTGRES_PASSWORD` | setup.sh | 12 个 supabase 角色共用。**不能**通过管理向导轮换。 |
| `JWT_SECRET` | setup.sh | 签发 ANON_KEY、SERVICE_ROLE_KEY、所有用户 JWT。可在 Secrets 页轮换（级联）。 |
| `ADMIN_TOKEN` | setup.sh | 管理面板登录令牌。可轮换。 |
| `COOKIE_SIGNING_KEY` | setup.sh | 管理 session cookie 的 HMAC 密钥。可轮换。 |
| `PG_META_CRYPTO_KEY` | setup.sh | Studio 存储 DB 凭证的加密密钥。可轮换。 |
| `ANON_KEY` | setup.sh（签好的 JWT）| 前端可见；绑定到 `anon` 角色。JWT_SECRET 轮换时自动重签。 |
| `SERVICE_ROLE_KEY` | setup.sh（签好的 JWT）| 绕过 RLS；切勿暴露到前端。JWT_SECRET 轮换时自动重签。 |

## 网络

| 变量 | 默认值 | 用途 |
|---|---|---|
| `SITE_URL` | `http://localhost:3000` | 你**前端应用**的公网地址。邮件模板、CORS 主 origin 都用它。 |
| `API_EXTERNAL_URL` | `http://localhost:8000` | 本栈网关地址。OAuth `redirect_uri` 注册到 provider 时用它。 |
| `CORS_ALLOWED_ORIGINS_REGEX` | （空 → 回落 SITE_URL）| 允许的前端 origin 的正则。详见 [多前端](/supalite/zh/configuration/multi-frontend/)。 |
| `GOTRUE_URI_ALLOW_LIST` | （空）| 逗号分隔的 OAuth 额外 redirect URI。SITE_URL 隐式允许。 |
| `POSTGRES_BIND_ADDR` | `127.0.0.1` | `:5432` 映射的宿主 IP。`0.0.0.0` 把 Postgres 暴露到外网。 |
| `GATEWAY_HTTP_BIND` | （空 → 0.0.0.0）| `:8000` 映射的宿主 IP。 |
| `CADDY_SITE_ADDR` | `:8000` | Caddyfile 的站点地址。改成域名启用自动 HTTPS。 |
| `CADDY_HTTPS_BIND` | （空 → 0.0.0.0）| `:80` `:443` 的宿主 IP（仅与 HTTPS override 文件配合）。 |

## 鉴权（GoTrue）

| 变量 | 默认值 | 备注 |
|---|---|---|
| `JWT_EXP` | `3600` | 用户 JWT 寿命（秒）。 |
| `GOTRUE_DISABLE_SIGNUP` | `false` | `true` = 邀请制。 |
| `GOTRUE_EXTERNAL_ANONYMOUS_USERS_ENABLED` | `true` | 允许匿名登录。 |
| `GOTRUE_MAILER_AUTOCONFIRM` | `true` | 跳过邮件确认。配好 SMTP 后改 `false`。 |

## SMTP

`GOTRUE_SMTP_HOST`、`GOTRUE_SMTP_PORT`、`GOTRUE_SMTP_USER`、`GOTRUE_SMTP_PASS`、`GOTRUE_SMTP_ADMIN_EMAIL`。在管理面板 Settings → SMTP 页用 **Send Test** 按钮验证。

## OAuth

GitHub / Google / Apple 每家各有：

- `GOTRUE_EXTERNAL_<PROVIDER>_ENABLED` — `true` / `false`
- `GOTRUE_EXTERNAL_<PROVIDER>_CLIENT_ID`
- `GOTRUE_EXTERNAL_<PROVIDER>_SECRET`
- `GOTRUE_EXTERNAL_<PROVIDER>_REDIRECT_URI` — 通常是 `{API_EXTERNAL_URL}/auth/v1/callback`

GitHub/Google 凭证可在 Settings → 对应 tab 用 **Test credentials** 按钮验证。Apple 用的是 JWT 形式 secret，不在测试端点覆盖范围。

## 备份

S3 兼容存储（AWS / Cloudflare R2 / MinIO / Backblaze B2 / …）：

| 变量 | 备注 |
|---|---|
| `BACKUP_S3_ENDPOINT` | 自定义 endpoint URL（AWS 留空）。 |
| `BACKUP_S3_BUCKET` | 必填。 |
| `BACKUP_S3_REGION` | 默认 `us-east-1`。R2 用 `auto`。 |
| `BACKUP_S3_ACCESS_KEY` / `BACKUP_S3_SECRET_KEY` | 必填。 |
| `BACKUP_S3_PATH_STYLE` | MinIO/Ceph 设 `true`。 |
| `BACKUP_S3_PREFIX` | pg_dump 对象前缀（默认 `backup/`）。 |
| `BACKUP_S3_PGBACKREST_PATH` | pgBackRest 对象前缀（默认 `/pgbackrest`）。 |
| `BACKUP_SCHEDULE_HOURS` | 设了就每 N 小时跑一次定时 pg_dump。 |
| `BACKUP_RETENTION_COUNT` | 保留最新的 N 份定时备份；更老的自动删除。 |

## pgBackRest（opt-in 物理备份）

| 变量 | 默认值 | 备注 |
|---|---|---|
| `PGBACKREST_ARCHIVE_MODE` | `off` | 设 `on` 启用 WAL 归档。需重启 Postgres。 |
| `PGBACKREST_ARCHIVE_COMMAND` | `pgbackrest --stanza=main archive-push %p` | Postgres 每个 WAL 段调用什么命令。 |
| `PGBACKREST_WAL_LEVEL` | `replica` | 归档必需。 |
| `PGBACKREST_MAX_WAL_SENDERS` | `3` | 复制 / pgbackrest 的连接槽。 |

## Compose / 镜像

| 变量 | 默认值 | 备注 |
|---|---|---|
| `COMPOSE_PROFILES` | `studio` | 逗号分隔。留空跳过 Studio + meta。 |
| `ADMIN_IMAGE` | `ghcr.io/web-casa/supalite-admin:latest` | fork 到私有 registry 时覆盖。 |
| `DB_IMAGE` | `ghcr.io/web-casa/supalite-db:latest` | 同上。 |
| `DB_DATA_VOLUME` | `supalite_db-data` | 持有 Postgres 数据的 Docker named volume。 |
| `DB_DATA_VOLUME_EXTERNAL` | `false` | 从老项目名迁移时设 `true`（复用已有 external volume）。 |
| `HOST_PROJECT_DIR` | （自动）| 本仓库的宿主路径，setup.sh 填好。 |
| `SETUP_COMPLETE` | `false` | 首次跑 setup wizard 后被标 `true`。 |

## 在哪里被消费

每个变量 `grep -rn "VAR_NAME" docker-compose*.yml volumes/` 能看到精确使用位置。管理面板 Settings 把高频字段封装成 UI；高级用户可直接手撕 `.env`。
