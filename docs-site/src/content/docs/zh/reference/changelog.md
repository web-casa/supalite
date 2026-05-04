---
title: 变更日志
description: 发布历史。
---

权威 changelog 在仓库根的 [`CHANGELOG.md`](https://github.com/web-casa/supalite/blob/main/CHANGELOG.md)，遵循 [Keep a Changelog](https://keepachangelog.com)。

## 截至目前的亮点

### v0.3.x — 运维深度
- 密钥轮换向导（4 个 secret，含级联 JWT）
- 管理 UI 全面迁 React Query

### v0.2.x — 准备好被采用
- 多前端 CORS + GoTrue allow list
- 定时 pg_dump + retention
- 安全关键路径的 Go 单测
- Setup 向导含实时 SMTP 测试
- `examples/nextjs-todo/` 端到端示例

### v0.1.0 — 首个 OSS 版本
- 核心栈：Postgres + REST + Auth + Studio + 管理面板
- pg_dump 备份到 S3 + 恢复
- pgBackRest 物理增量备份 + 恢复
- Caddy + Let's Encrypt 自动 HTTPS（opt-in）
- SMTP 和 OAuth 凭证测试端点
- MIT 许可，CI，多架构镜像发布

每次 commit 详情，GitHub releases 页每个 tag 都链接到对应 compare diff。
