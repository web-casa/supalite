---
title: 什么是 SupaLite？
description: 一套自托管 Postgres 后端，从上游 Supabase 镜像组装而来，去掉了大多数小型部署用不上的部分。
---

SupaLite 是一套**自托管 Postgres 后端**，集成了：

- **PostgreSQL 15**（基于 `supabase/postgres` 镜像）带 12 个 Supabase 角色和 13 个扩展
- **PostgREST** 根据 schema 自动生成 REST API
- **GoTrue** 处理邮箱 + OAuth 认证、签发 JWT
- **Supabase Studio**（可选）提供 UI 表编辑器 / SQL 编辑器
- **Go 写的管理面板** 处理日常运维：备份、密钥轮换、实时日志、数据库维护

它是**砍掉 Supabase 中大多数小项目用不上部分后剩下的核心**：Storage、Edge Functions、Realtime。这些服务在大规模场景有价值，但对独立开发者或小团队来说，强制带来的运维复杂度回报不抵成本。

## 适合谁

- 想要真正 Postgres 后端但又不愿为托管 Supabase 付费的**独立开发者**
- 想保留 Supabase 风格开发体验（自动 REST API、RLS、JWT 鉴权）但希望基础设施自己掌控的**小型创业团队**
- 已经在 VPS 上跑 Docker、想要一套开箱即用数据库栈的**自托管爱好者**

## 不适合谁

- 需要水平扩展、多区域、Postgres HA 的团队 → 用 CloudNativePG 这类真正的 Postgres operator
- 严重依赖 Storage / Edge Functions / Realtime 的团队 → 直接用上游 Supabase
- 对租户间数据强隔离有刚性合规要求的多租户 SaaS → 每个租户跑一个独立 SupaLite 实例

## 和上游 Supabase 自托管的区别

| | 上游 Supabase | SupaLite |
|---|---|---|
| 服务数 | 10+ 个容器 | 4 个核心 + 2 个可选 |
| 内存占用 | ~3-4 GB | ~1.5 GB |
| 日常运维 | docker compose + 手撕 SQL | 常见操作有面板 |
| 安装 | 手动改 env | `./setup.sh` |
| 备份 | 自己折腾 | pg_dump + pgBackRest 内置 |

如果你撞到 SupaLite 不支持的功能（如 Storage、Realtime），升级路径很直接：切到上游 Supabase，复用同一个 Postgres 数据卷即可。
