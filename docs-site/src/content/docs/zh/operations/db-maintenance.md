---
title: 数据库维护
description: 从管理面板做 VACUUM / ANALYZE / REINDEX + 会话检查。
---

两个页面覆盖 Postgres 日常：

- **`/admin/dbops/`** — VACUUM、ANALYZE、REINDEX
- **`/admin/sessions/`** — `pg_stat_activity` 视图 + 会话终止

## DB Ops 页面

挑一个操作 + 可选 target。

| 操作 | 用途 |
|---|---|
| **VACUUM** | 回收 deleted/updated 行的空间。非阻塞。怀疑表膨胀时跑。 |
| **VACUUM FULL** | 重写表压缩存储。拿 `AccessExclusiveLock`——执行期间表不可用。 |
| **ANALYZE** | 刷新统计信息让 query planner 选更好的计划。便宜、非阻塞。 |
| **REINDEX** | 重建索引。大规模数据改动后 / 怀疑索引损坏时用。锁表。 |

### Target

- **空 target** —— 操作整个 `postgres` 数据库（如 VACUUM 所有表）。
- **`schema.table`** —— 只操作这张具体表（如 `public.users`）。

REINDEX **必须**给 `schema.table` —— schema 级 reindex 不支持（schema 名和表名一样时有歧义）。

### 安全

`target` 字段先过正则 `^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$`，然后**再**经 `pgx.Identifier.Sanitize` 才进 SQL。两层都拒绝任何注入企图。请求形状不对，handler 直接 400，不碰 DB。

每个操作 10 分钟超时。长操作（巨大 VACUUM FULL）需要面板外的维护窗口。

## Sessions 页面

列出 `pg_stat_activity`，每 5s 自动刷新。tab 隐藏时暂停轮询（避免 thundering-herd）。

| 列 | 含义 |
|---|---|
| PID | 后端进程 ID |
| User | Postgres 角色 |
| DB | `postgres`（我们就一个 DB） |
| State | `active`（正在跑查询）、`idle`、`idle in transaction`（红——挂着事务持锁） |
| App / Client | 应用名 + 客户端 IP |
| Wait | 后端在等什么（如果有的话） |
| Query | 当前/最近 query 的文本截断 |

默认只显示**客户端后端**。勾 "include system" 看 autovacuum、walwriter 等。

### 终止后端

- 点行上的 **X**。
- 在确认弹窗里输入 PID。
- 点 **Terminate**。

发送 `pg_terminate_backend(pid)`。连接看到 fatal error，进行中的事务回滚。用于杀飞奔的 query 或释放锁。

**不能**杀 admin 进程自己的后端——`pid <> pg_backend_pid()` 守卫。admin pool 的其它连接技术上可杀，杀了 pgxpool 自动重连，不便但不破坏。

## 常见工作流

### 表膨胀排查

```sql
-- 在 Studio SQL 编辑器或 psql 里
SELECT schemaname, relname, n_dead_tup, n_live_tup,
       round(n_dead_tup * 100.0 / nullif(n_live_tup, 0), 1) AS dead_pct
FROM pg_stat_user_tables
WHERE n_dead_tup > 1000
ORDER BY dead_pct DESC NULLS LAST;
```

某张表 dead_pct 高就在管理面板对它跑 **VACUUM**。VACUUM 不够才上 **VACUUM FULL**。

### 长查询排查

Sessions 页 → 找 `state=active` 且 `query_start` 老的。Query 列显示在干啥。必要时 Terminate。

### 锁表

Sessions 页 → 找 `state=idle in transaction`（红 badge）。它们持锁却不干活。终止它们释放锁；受影响的客户端看到 fatal error。

不想直接拔——有时长事务在做重要事——找出负责的应用让它 commit/rollback。
