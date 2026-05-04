# Architecture Decision Records (ADR)

重大的架构 / 安全 / 兼容性决策记录在这里。每个文件是一条决策：为什么做 / 怎么做 / 哪些地方受影响。

## 目录

| # | 标题 | 状态 | 日期 |
|---|------|------|------|
| 001 | [撤回对 GRANT supabase_admin TO authenticator 的移除](./001-revert-supabase-admin-grant.md) | 已决策 | 2026-04-17 |

## 什么时候写 ADR

- 推翻之前的决策 / 修复（像 001 这种）
- 偏离上游官方做法且有理由
- 影响多个 Phase 的技术选型
- 安全相关的权衡
- 用户未来可能追问"为什么这么做"的地方

## 什么时候不写

- 纯实现细节（放代码注释）
- 产品功能决策（放 product-strategy.md）
- 调研结果（放 research/）
