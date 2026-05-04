---
title: 多前端支持
description: 一个 SupaLite 实例服务 web + 移动端 + 营销站。
---

典型独立项目通常有多个前端共用同一套鉴权后端：Next.js 网页、React Native 或移动端、可能还有营销站。SupaLite 内置了支持，靠两个 env 变量。

## 两个开关

在 `.env`（或管理面板 Settings → General tab）里：

```bash
# 主前端 — 邮件模板、OAuth 默认值都用这个
SITE_URL=https://app.example.com

# 允许的所有 CORS origin 的正则。留空 = 只允许 SITE_URL。
# 点号要转义；用 `|` 串联。
CORS_ALLOWED_ORIGINS_REGEX=https://app\.example\.com|https://admin\.example\.com|https://marketing\.example\.com

# 逗号分隔的 OAuth 额外 redirect URI（SITE_URL 自动允许）。
# 用于移动端 deep link 和任何非主前端的 web。
GOTRUE_URI_ALLOW_LIST=https://admin.example.com/auth/callback,myapp://auth
```

让相关服务读新 env：

```bash
docker compose up -d gateway gotrue
```

## CORS 匹配机制

Caddyfile 用 `header_regexp Origin "^({$CORS_ALLOWED_ORIGINS_REGEX})$"`。匹配命中后，把请求实际 `Origin` 回写到 `Access-Control-Allow-Origin`（带 cookie 时不能用 `*`，必须 per-origin echo）。

未匹配的 origin **拿不到任何 CORS 头**——预检返回 204 无头，浏览器拦截实际请求。

## OAuth redirect 机制

前端调 `signInWithOAuth({ redirectTo: "https://admin.example.com/auth/callback" })` 时，GoTrue 校验 `redirectTo`：

1. `SITE_URL`（永远允许）
2. `GOTRUE_URI_ALLOW_LIST` 里的每一条（子串匹配——`myapp://` 能命中所有以这个 scheme 开头的 URL）

都不匹配则 OAuth 请求被 400 拒。

## 为什么不直接用 `*`？

带凭证（cookie、`Authorization` 头）的 CORS 明确禁止 `Access-Control-Allow-Origin: *`。浏览器拒绝。所以必须 echo 实际 origin，意味着必须有白名单。

选择正则而不是逗号列表，是因为它对常见场景（单 origin、`https://([a-z]+)\.example\.com` 这种一组子域）更自然，扩展也简单。

## `SITE_URL` 除了 CORS 还控制什么

- **邮件确认链接** — 只用 `SITE_URL`/path。挑主前端；移动端 deep link 不会出现在邮件里。
- **OAuth 默认 `redirectTo`** — 前端没指定时
- GoTrue 容器 env 里的 `GOTRUE_SITE_URL`

## 多前端 vs 多实例

多**前端**（本页）：一个 SupaLite，多个前端共享同一套用户账号。

多**实例**（不同概念）：想在一台主机跑完全独立的多个项目？跑多个 compose project。详见 [Compose Profiles](/supalite/zh/configuration/compose-profiles/) 里的 `docker compose -p` 模式。
