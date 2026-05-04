---
title: Compose Profiles
description: 可选服务与多实例部署。
---

## profiles 控制什么

Docker Compose [profiles](https://docs.docker.com/compose/profiles/) 让你把服务标记为 opt-in。SupaLite 用了一个 profile：

| Profile | 激活的服务 | 默认开？ |
|---|---|---|
| `studio` | `meta`、`studio` | **是**（`.env` 里 `COMPOSE_PROFILES=studio`）|

设 `COMPOSE_PROFILES=`（空）跑**最小栈**——只有 db、rest、gotrue、admin、gateway。省 ~800 MB 镜像 + ~500 MB 内存。适合只通过 API 访问的无头部署。

## 其它 compose 文件

| 文件 | 用途 |
|---|---|
| `docker-compose.yml` | 默认栈 |
| `docker-compose.https.yml` | Override，加 `:80`/`:443` 宿主端口绑定用于自动 HTTPS |

`-f` 组合使用：

```bash
docker compose -f docker-compose.yml -f docker-compose.https.yml up -d
```

## 一台主机跑多个无关项目

不同产品共享硬件：每个 app 一个 compose project，各自独立 Postgres 数据卷、管理令牌、端口。

```bash
# Project A
cp .env .env.a
$EDITOR .env.a   # 改 POSTGRES_PASSWORD、ADMIN_TOKEN、GATEWAY_HTTP_BIND 端口等
docker compose -p app-a --env-file .env.a up -d

# Project B
cp .env .env.b
$EDITOR .env.b   # 复用 8000 端口的话还要改 gateway 端口
docker compose -p app-b --env-file .env.b up -d
```

各 project 容器自动加命名空间（`app-a-db-1`、`app-b-db-1`）。卷互相隔离（`app-a_db-data`、`app-b_db-data`）。外部流量用上层 Caddy/Traefik 按域名分发。

这种模式适合项目满足：
- **用户体系完全无关**
- 需要爆炸半径隔离（A 被攻破不能泄露 B）
- 升级节奏不同

如果是同一产品的多个前端，用单实例 + 多前端配置——详见 [多前端支持](/supalite/zh/configuration/multi-frontend/)。
