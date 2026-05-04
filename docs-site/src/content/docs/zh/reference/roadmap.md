---
title: 路线图
description: 接下来要做什么。
---

## 进行中

- **审计日志** —— 记录破坏性管理操作（restart、restore、terminate、secret rotation）以便事后追溯。
- **Apple OAuth 凭证验证** —— 目前 `Test credentials` 按钮跳过 Apple，因为 Apple 的 client_secret 是短期 JWT 而非静态字符串。

## 在考虑

- **Prometheus `/metrics` 端点** —— 基础指标（服务起没起、最近备份多老、db 连接数）。
- **管理 API 的 OpenAPI 规范** —— 让任何语言能用 `openapi-generator` 生成 client。
- **pgBackRest 定时备份** —— 目前只有 pg_dump 路径有调度器。
- **每条密钥的轮换历史** —— 跟踪最近一次轮换时间 + 谁触发。
- **WAL 归档一致性横幅** —— pgBackRest 已启用但 `archive_mode` 还是 off 时，在 admin 侧栏给警告。

## 故意不在范围

考虑过被拒：

- **Postgres HA / 复制** —— 这个复杂度上请用真正的 Postgres operator（CloudNativePG、Crunchy）。
- **多租户数据隔离** —— schema-per-tenant 或 DB-per-tenant。请跑多个 SupaLite 实例代替。
- **Storage / Edge Functions / Realtime** —— 我们砍掉的上游 Supabase 服务。需要就跑上游。
- **Studio 嵌入式配置面板** —— Studio 自己管自己的配置；把它的配置塞进我们 admin 加维护负担、信息价值低。
- **POSTGRES_PASSWORD 轮换向导** —— 一键操作部分失败模式太多。手工流程在 [密钥轮换](/supalite/zh/operations/secret-rotation/)。

## 招贡献

要做 "在考虑" 里的事情，请先开 issue 对齐范围。无前置讨论扩大范围的 PR 可能被要求拆分。
