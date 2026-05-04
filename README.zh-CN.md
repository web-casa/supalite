# SupaLite

[English](./README.md) · [中文](./README.zh-CN.md)

[![ci](https://github.com/web-casa/supalite/actions/workflows/ci.yml/badge.svg)](https://github.com/web-casa/supalite/actions/workflows/ci.yml)
[![license](https://img.shields.io/badge/license-MIT-blue)](./LICENSE)
[![release](https://img.shields.io/github/v/release/web-casa/supalite?display_name=tag&sort=semver)](https://github.com/web-casa/supalite/releases)
[![docs](https://img.shields.io/badge/docs-web--casa.github.io-3ECF8E)](https://web-casa.github.io/supalite/zh/)

> 给独立开发者的一键自托管 Postgres + Auth + Studio。
> 基于上游 Supabase 镜像构建，砍掉了大多数小型部署用不上的部分
>（Storage、Edge Functions、Realtime）。

---

## 一键安装

```bash
git clone https://github.com/web-casa/supalite.git && cd supalite && ./setup.sh
```

`setup.sh` 会自动生成所有密钥（`POSTGRES_PASSWORD`、`JWT_SECRET`、
`ADMIN_TOKEN` 等），写入 `.env`（权限 600），拉取镜像，启动整套服务，
对所有公开端点做健康检查，最后打印管理员 token。

跑完之后打开：

- **首页**：<http://localhost:8000/>
- **管理面板**：<http://localhost:8000/admin/>（用 `ADMIN_TOKEN` 登录）
- **Studio**：<http://localhost:8000/studio/>

Token 找不到了？在 `.env` 文件里 `ADMIN_TOKEN` 字段。

> **前置依赖**：Docker + Docker Compose v2、`openssl`、`curl`。
> Linux：把你的用户加到 `docker` 组，或者命令前加 `sudo`。

---

## 包含哪些组件

| 服务       | 镜像                            | 作用                                                  |
|------------|---------------------------------|-------------------------------------------------------|
| `db`       | supabase/postgres + pgbackrest  | PG 15 + 12 个 Supabase 角色 + 12 个扩展 + pgBackRest |
| `rest`     | postgrest/postgrest             | 基于 schema 自动生成的 REST API                       |
| `gotrue`   | supabase/gotrue                 | 邮箱 + OAuth 鉴权、JWT 签发                           |
| `admin`    | （由 `./admin` 构建）            | Go 后端 + Next.js SPA —— 本项目的管理面板            |
| `meta`     | supabase/postgres-meta          | Studio 用的元数据 API（profile: `studio`）           |
| `studio`   | supabase/studio                 | 上游 Supabase Studio（profile: `studio`）            |
| `gateway`  | caddy:2-alpine                  | 反向代理、CORS、自动 HTTPS、admin cookie 网关        |

Studio + postgres-meta 在 `COMPOSE_PROFILES=studio` 下启动（默认开）。
不需要的话，从 `.env` 删掉这一行就是最小栈。

架构图：[`architecture.mmd`](./architecture.mmd)（Mermaid）。

---

## 管理面板

`http://localhost:8000/admin/` —— Cookie 鉴权，HMAC 签名，SameSite=Strict。

| 页面      | 功能                                                            |
|-----------|-----------------------------------------------------------------|
| Dashboard | 服务状态 + API keys                                              |
| Logs      | 任意容器 stdout/stderr 的实时 SSE 日志流                        |
| DB Ops    | VACUUM / VACUUM FULL / ANALYZE / REINDEX，可指定目标            |
| Sessions  | `pg_stat_activity` + `pg_terminate_backend` 操作                |
| Backups   | pg_dump → S3 **+** pgBackRest（full / diff / incr / restore）   |
| Settings  | General、SMTP、GitHub / Google / Apple OAuth、Backup S3         |

长任务（pg_dump、pg_restore、pgBackRest）异步执行，UI 轮询状态。
破坏性操作（restore、terminate、delete）需要键入名称确认。

---

## 启用 HTTPS

默认只在 `:8000` 用 http。给 Caddy 加上自动 TLS：

1. 公网 DNS（A/AAAA）解析到这台主机。
2. `.env`：`CADDY_SITE_ADDR=db.example.com`（多域名用空格分隔）。
3. `.env`：`API_EXTERNAL_URL=https://db.example.com`。
4. 保证 80 + 443 端口公网可达（ACME HTTP-01 需要 `:80`）。
5. 用 HTTPS override 文件启动：
   ```bash
   docker compose -f docker-compose.yml -f docker-compose.https.yml up -d
   ```

Caddy 自动签发并续期 Let's Encrypt 证书。证书状态持久化在
`caddy-data` 卷里。

---

## 一个实例服务多个前端

一个 SupaLite 通常服务一个独立项目 —— 同一套鉴权后端给你的
Next.js Web 应用、手机 App 和落地页用。在 `.env` 里配 CORS 和
GoTrue allow list：

```env
SITE_URL=https://app.example.com
CORS_ALLOWED_ORIGINS_REGEX=https://app\.example\.com|https://admin\.example\.com
GOTRUE_URI_ALLOW_LIST=https://admin.example.com/auth/callback,myapp://auth
```

如果是同一台主机上跑互不相关的多个项目，用独立的 compose project
（`docker compose -p app-a` / `-p app-b`）让数据卷和 admin token
互相隔离。

完整指引：[多前端文档](https://web-casa.github.io/supalite/zh/configuration/multi-frontend/)

---

## 备份

两条路径都打到同一个 S3 兼容存储（AWS / R2 / MinIO / B2）：

- **逻辑备份** —— `pg_dump -Fc` 流式上传到 S3。永远可用，
  管理面板点 "Run Backup" 即可。
- **物理备份** —— pgBackRest，持续 WAL 归档 + 增量备份。
  通过 `PGBACKREST_ARCHIVE_MODE=on` 开启。Restore 流程
  `停 db → 还原 → 启 db`，失败时保留停机状态供你查问题。

两条路径共享 `.env` 里的 `BACKUP_S3_*` 凭证。

---

## 文档

完整中英双语文档：<https://web-casa.github.io/supalite/zh/>

- [快速上手](https://web-casa.github.io/supalite/zh/getting-started/quick-start/)
- [配置](https://web-casa.github.io/supalite/zh/configuration/environment-reference/)
- [运维](https://web-casa.github.io/supalite/zh/operations/backups/)
- [核心概念：RLS、JWT、ANON vs SERVICE_ROLE](https://web-casa.github.io/supalite/zh/concepts/rls/)
- [Admin API 参考](https://web-casa.github.io/supalite/zh/api-reference/admin-api/)
- [常见问题](https://web-casa.github.io/supalite/zh/troubleshooting/common-errors/)

---

## 示例项目

[`examples/nextjs-todo/`](./examples/nextjs-todo/) —— 极简 Next.js +
`supabase-js` 应用，展示完整的 SupaLite 调用链路：邮箱注册、登录、
RLS 保护的 `todos` 表、通过自动 REST API 做 CRUD。可以直接 fork
作为起点。

---

## 开发

管理面板源码在 `./admin`：

- `internal/` —— Go 包（handler、docker 客户端、sse、备份、db…）
- `web/` —— Next.js 应用（静态导出，通过 `go:embed` 嵌入二进制）

改完后重新构建：

```bash
docker compose up -d --build admin
```

`go vet ./...` 和 `go build ./...` 必须在 `admin/` 目录下跑
（不是 `admin/web/`，那里没有 Go 代码会"静默通过"）。

---

## 许可证

MIT —— 见 [LICENSE](./LICENSE)。
