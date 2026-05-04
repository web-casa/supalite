---
title: JWT 鉴权
description: JWT 如何从登录流到 PostgREST 再到 RLS 策略。
---

JSON Web Token 是前端、GoTrue、PostgREST、Postgres 之间的连接组织。每个 token 带 claim（`sub`、`email`、`role`……），标识调用者，告诉 Postgres 该允许什么。

## 三种带秘密的 key

| Token | 由什么签 | `role` claim | 在哪 |
|---|---|---|---|
| `ANON_KEY` | `JWT_SECRET` | `anon` | 前端代码（可公开） |
| `SERVICE_ROLE_KEY` | `JWT_SECRET` | `service_role` | 服务端（绝不前端） |
| 用户 session JWT | `JWT_SECRET` | `authenticated`（带 `sub`、`email`）| GoTrue 每次登录签发 |

三种都用**同一个** `JWT_SECRET` 做 HS256。PostgREST 收到任意一种都校验。

## 流程

```
  ┌──────────┐   邮箱+密码         ┌──────────┐
  │ 前端     │ ───────────────────► │  GoTrue  │
  │          │ ◄─────────────────── │          │
  └──────────┘   用户 session JWT   └──────────┘
       │                                  │
       │  fetch /rest/v1/todos            │
       │  apikey: ANON_KEY                │
       │  Authorization: Bearer <用户 JWT>│
       ▼                                  │
  ┌──────────┐                            │
  │  Caddy   │  require_apikey 检查       │
  └──────────┘                            │
       │                                  │
       ▼                                  │
  ┌──────────┐  设置 request.jwt.claims  │
  │PostgREST │ ─────────────────────────► │   Postgres
  └──────────┘                            │   auth.uid() 读 claims
                                          ▼
                                       (RLS 策略)
```

1. 用户登录。GoTrue 返回 JWT。
2. 前端调 `/rest/v1/...` 带**两个** header：
   - `apikey: <ANON_KEY>`（Caddy 用它把闸路径）
   - `Authorization: Bearer <用户 JWT>`（PostgREST 用它识别用户）
3. PostgREST 用 `JWT_SECRET` 验签。
4. PostgREST 在 Postgres session 里设置 `request.jwt.claims` GUC。
5. RLS 策略读 `auth.uid()`（自己读 `request.jwt.claims->>sub`）。

## 为什么两个 header

`apikey` 是**路径授权**——Caddy 拒绝任何没合法 `apikey` 的 `/rest/*` 或 `/graphql/*` 请求。挡住乱扫的 internet bot。

`Authorization: Bearer` 是**用户身份**——PostgREST 解码后把 per-user claim 喂给 Postgres 用于 RLS。

匿名（未登录）用户的请求只有 `apikey: <ANON_KEY>`，没有 `Authorization`。PostgREST 当作 `anon` 角色处理；适用 `anon` 的 RLS 策略生效。

## 用户 JWT 里有什么

GoTrue 签（HS256，默认 1 小时过期）：

```json
{
  "sub": "uuid-of-the-user",
  "email": "user@example.com",
  "role": "authenticated",
  "aud": "authenticated",
  "exp": 1700003600,
  "iat": 1700000000,
  "app_metadata": { ... },
  "user_metadata": { ... }
}
```

自定义 claim 走 `app_metadata`（admin 设置，client 不可改）和 `user_metadata`（用户可编辑）。多租户的 tenant_id 放 `app_metadata`。

## Token 刷新

GoTrue 在 access JWT 旁边发 refresh token。`supabase-js` 透明处理 refresh——access token 快过期时，把 refresh token POST 到 `/auth/v1/token?grant_type=refresh_token` 拿新一对。

`JWT_SECRET` 轮换（Secrets 页）后，**旧** secret 签的 refresh token 不能在**新** secret 下签 access JWT。用户被登出，需要重新登录。

## 为什么用 HS256（不是 RS256）

对称签名 `JWT_SECRET` 让 key 管理简单——一个 secret，四个服务共用。RS256 让 PostgREST 只用公钥验签，但自托管单实例部署没有第三方验证者，非对称带来的好处用不上。

真要 RS256（如和 SaaS 服务共享 JWT 验证），GoTrue 和 PostgREST 都支持——设 `GOTRUE_JWT_ALGORITHM=RS256` 并提供密钥对。

## JWT 问题排查

1. **解码 token** —— `jwt.io` 或对中段 `jq -R`。检查 `exp`（过期？）、`role`（对吗？）、`sub`（UUID？）。
2. **签名错**：PostgREST 返 `401 invalid_token`。意味着 JWT 被另一个 `JWT_SECRET` 签的。
3. **缺 `apikey`**：Caddy 返 `401 Missing or invalid API key`。补 `apikey: <ANON_KEY>` 头。
4. **RLS 拦了合法请求**：用 `service_role`（绕过 RLS）跑同样的 query 确认数据在；问题在策略。
