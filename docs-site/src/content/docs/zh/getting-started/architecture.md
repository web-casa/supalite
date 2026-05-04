---
title: 架构总览
description: SupaLite 各容器如何协作。
---

SupaLite 是六个（启用可选服务后是八个）容器，前面挂一个 Caddy 网关。下图展示请求流向：

```
┌─────────────────────────────────────────────────────────────────┐
│                        :8000  (Caddy 网关)                       │
│                                                                   │
│  /rest/v1/*    ───►  rest    (PostgREST)         ───►  ┌─────┐   │
│  /auth/v1/*    ───►  gotrue  (auth + OAuth)       ───►  │     │   │
│  /studio/*     ───►  studio  (表编辑器 UI)         ───►  │     │   │
│                       └──── meta (postgres-meta)  ───►  │  db │   │
│  /admin/*      ───►  admin   (Go + Next.js SPA)   ───►  │     │   │
│                       └──── docker.sock (兄弟容器操作) ►  └─────┘   │
│  /              ───►  静态着陆页                                   │
└─────────────────────────────────────────────────────────────────┘
                                ▲
                       80 / 443 (可选 HTTPS)
```

## 七个服务

| 服务     | 镜像                                  | 职责 |
|----------|--------------------------------------|------|
| `db`     | 自定义（`supabase/postgres` + pgbackrest）| Postgres 15，含所有 Supabase 角色、schema、扩展，加 pgbackrest 二进制用于增量备份 |
| `rest`   | `postgrest/postgrest`                | 自动 REST API + pg_graphql，以 `authenticator` 角色连接 |
| `gotrue` | `supabase/gotrue`                    | 邮箱 + OAuth 认证、JWT 签发、密码重置，以 `supabase_auth_admin` 角色连接 |
| `meta`   | `supabase/postgres-meta`             | 元数据 REST API，仅供 Studio 使用。Profile: `studio` |
| `studio` | `supabase/studio`                    | 上游 Supabase UI。Profile: `studio` |
| `admin`  | 自定义 Go + Next.js                   | 本项目的管理面板，挂 docker.sock 做兄弟容器操作（重启、exec 进 db） |
| `gateway`| `caddy:2-alpine`                     | 反向代理、CORS、可选自动 HTTPS、`/studio/*` 的 cookie 闸门 |

## 网络

单个 Docker 网络（`supalite_default`）。容器之间通过服务名互相访问（`db`、`rest` 等）。只有 gateway 对外暴露。

## 鉴权模型

两层独立鉴权：

1. **应用层鉴权**（你的最终用户）：Caddy 的 `require_apikey` snippet 校验 `apikey` header 必须是 `ANON_KEY` 或 `SERVICE_ROLE_KEY`，对应 `/rest/v1/*` 和 `/graphql/v1/*`。GoTrue 签发用户 JWT，PostgREST 通过 RLS 策略里的 `auth.uid()` 识别身份。

2. **管理面板鉴权**（你这个运维者）：HMAC 签名的 session cookie 守卫 `/admin/*`，`SameSite=Strict` + `HttpOnly`。登录一次用 `ADMIN_TOKEN`，cookie 持续到过期。Studio 的 `/studio/*` 仅判断 cookie 是否存在（cookie 内容不在 gateway 层验签——Studio 自己的 API 调用打到 PostgREST/GoTrue/postgres-meta，那里独立校验密钥）。

`ANON_KEY` 泄露**不会**触及管理 API。`ADMIN_TOKEN` 泄露**不会**绕过应用数据的 RLS——除非运维者在管理面板里显式拿出 `SERVICE_ROLE_KEY`。

## 存储

一个 Docker named volume：

- `supalite_db-data` — Postgres 数据目录（`/var/lib/postgresql/data`）

启用 HTTPS 后多两个：

- `caddy-data` — Let's Encrypt 证书和 ACME 账户密钥
- `caddy-config` — Caddy 运行时配置

## 初始化流水线

`db` 在空卷上首次启动时：

1. supabase/postgres 镜像跑自己的 `migrate.sh`：创建 12 个 Supabase 角色、基础 schema（auth、storage、extensions……）、安装 `uuid-ossp`、`pgcrypto`、`pg_stat_statements`。
2. SupaLite 的 `volumes/db/init/zz-supalite.sh` 紧接着跑（`zz-` 前缀让它在 `/docker-entrypoint-initdb.d/` 里排最后）：
   - 启用另外 9 个扩展（`pg_graphql`、`pgvector`、`pg_net`、`pg_cron`、`pgsodium`、`supabase_vault`、`pgjwt`、`http`、`pg_jsonschema`）
   - 创建 `_realtime` 和 `supabase_functions` schema
   - 把 12 个角色密码统一为 `POSTGRES_PASSWORD`
   - 覆盖 `auth.uid()` / `auth.role()` / `auth.email()` 用新 GUC 模式（`request.jwt.claims` 取代旧的逐 claim GUC）
   - 在数据库级别设置 `app.settings.jwt_secret`

后续重启会跳过 init 脚本（Postgres 只在空数据目录上跑）。

## 另见

- [多前端配置](/supalite/zh/configuration/multi-frontend/) — 一个 SupaLite 服务多个应用
- [Compose profiles](/supalite/zh/configuration/compose-profiles/) — 最小栈 vs 完整栈
- [端口与网络](/supalite/zh/concepts/ports-networking/) — 谁绑定哪里、为什么
