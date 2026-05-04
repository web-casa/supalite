---
title: 密钥轮换
description: 各凭证什么时候、怎么轮换。
---

Secrets 页面（`/admin/secrets/`）覆盖 4 个有安全一键路径的密钥。POSTGRES_PASSWORD 故意不在向导里——理由见下。

## 可轮换密钥

| 密钥 | 需重启 | 影响范围 |
|---|---|---|
| `ADMIN_TOKEN` | 无 | 现存浏览器 session 持续到 cookie 过期；用 bearer 的 CLI/脚本要换新值。 |
| `COOKIE_SIGNING_KEY` | `admin` | 所有管理浏览器 session 立即失效；你用当前 ADMIN_TOKEN 重新登录。 |
| `PG_META_CRYPTO_KEY` | `meta` + `studio` | Studio 存的项目凭证变得不可读；在 Studio UI 里重新输入。 |
| `JWT_SECRET` | `rest` + `gotrue` + `admin` + `gateway` | **级联。** ANON_KEY 和 SERVICE_ROLE_KEY 自动用新 secret 重签。所有已登录用户被踢。所有客户端 app 要换新 ANON_KEY。 |

## 为什么 POSTGRES_PASSWORD 不可轮换

12 个 supabase 角色 + 所有连接服务都用它。轮换需要：

1. 对活着的 DB 跑 `ALTER USER <每个角色> WITH PASSWORD '<新>'`
2. 改 `.env`
3. 重启所有连 PG 的服务

第 1 步部分失败模式多——一半 ALTER USER 成功一半失败的话，cluster 就坏了一半。值得做"小心的手工流程 + 事前备份"，但不值得做一键按钮。

要真要轮换：

```sql
-- 在 psql 里以 supabase_admin（或任意超级用户）跑
ALTER USER anon WITH PASSWORD 'new-password';
ALTER USER authenticated WITH PASSWORD 'new-password';
ALTER USER service_role WITH PASSWORD 'new-password';
ALTER USER authenticator WITH PASSWORD 'new-password';
ALTER USER supabase_admin WITH PASSWORD 'new-password';
ALTER USER supabase_auth_admin WITH PASSWORD 'new-password';
ALTER USER supabase_storage_admin WITH PASSWORD 'new-password';
ALTER USER supabase_replication_admin WITH PASSWORD 'new-password';
ALTER USER supabase_read_only_user WITH PASSWORD 'new-password';
ALTER USER supabase_functions_admin WITH PASSWORD 'new-password';
ALTER USER dashboard_user WITH PASSWORD 'new-password';
ALTER USER pgbouncer WITH PASSWORD 'new-password';
```

然后改 `.env` 里的 `POSTGRES_PASSWORD`，最后 `docker compose up -d` 重启所有。

## 一次轮换的过程

1. 打开 `/admin/secrets/`。
2. 在你想轮的卡片点 **Rotate**。
3. 在确认弹窗里输入密钥名（如 `JWT_SECRET`）。
4. 点 **Rotate**。
5. 页面显示绿色横幅：
   - 改了哪些 env key
   - 要重启哪些 service
   - 一行可复制的 `docker compose up -d --no-deps ...`

向导**不**自动重启服务——你自己跑重启命令，掌控宕机时刻。

## JWT 轮换后

立刻三件事：

1. **重启列出来的服务** —— `rest`、`gotrue`、`admin`、`gateway`。老用户 JWT 在下次请求开始报错。
2. **从 Dashboard 复制新 ANON_KEY**。更新每个客户端 app（移动、Web、脚本）。
3. **通知用户重新登录** —— 所有现存 session 都已失效。

Dashboard 的 keys 查询轮换后会自动 invalidate，刷新 Dashboard tab 立刻看到新 ANON_KEY。

:::caution[重启时机]
向导**不**自动重启服务。你会看到要重启哪些服务，自己跑命令——多服务轮换不会被一波不可控的重启拍下整个栈。
:::

## 什么时候轮换

- **`ADMIN_TOKEN`** —— 在聊天/日志/截图里泄露过之后。卫生习惯季度轮换。
- **`COOKIE_SIGNING_KEY`** —— 年度轮换；怀疑 session 被盗时紧急轮换。
- **`PG_META_CRYPTO_KEY`** —— 年度；只在 Studio 加密项目列表可能外泄时关键。
- **`JWT_SECRET`** —— 顶多年度一次（强制所有用户重登）。JWT_SECRET 泄露过则紧急轮换。

目前没有轮换历史审计日志——`.env` 的 git log（如果你提交它的话——别）或者你的 secret manager 历史是唯一真相源。
