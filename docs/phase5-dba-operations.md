# Phase 5: DBA 运维操作调研

> 目标：VACUUM / REINDEX / ANALYZE / 连接 kill / 慢查询 / 锁分析等 DBA 按钮
> 日期：2026-04-17

---

## 1. 核心 DBA 操作总览

| 操作 | 用途 | 需要权限 | 危险性 |
|------|------|---------|:------:|
| `VACUUM` | 清理死元组 | 表所有者 or SUPERUSER | 🟢 |
| `VACUUM FULL` | 重新组织表（锁表） | 同上 | 🔴 锁表 |
| `VACUUM ANALYZE` | 清理+更新统计 | 同上 | 🟢 |
| `ANALYZE` | 更新统计信息 | 同上 | 🟢 |
| `REINDEX` | 重建索引 | 索引所有者 or SUPERUSER | 🟡 短暂锁 |
| `REINDEX CONCURRENTLY` | 在线重建 | 同上 | 🟢 不锁 |
| `pg_cancel_backend(pid)` | 取消正在执行的查询 | `pg_signal_backend` 角色 | 🟢 |
| `pg_terminate_backend(pid)` | 强制断开 session | 同上 | 🟡 杀连接 |
| `pg_reload_conf()` | 重载配置 | SUPERUSER | 🟢 |
| `pg_stat_reset()` | 重置统计 | SUPERUSER | 🟢 |

## 2. 权限模型

我们的 admin 后端以 `supabase_admin` 身份连数据库（SUPERUSER）。所有 DBA 操作都能执行。

---

## 3. VACUUM / REINDEX / ANALYZE

### 3.1 命令与适用场景

```sql
-- 轻量清理（最常用）
VACUUM (ANALYZE) public.users;

-- 重新组织（会锁表，通常不自动跑）
VACUUM FULL public.users;

-- 只更新统计信息（查询变慢时先跑）
ANALYZE public.users;

-- 在线重建索引（推荐）
REINDEX INDEX CONCURRENTLY public.users_email_idx;

-- 重建整表所有索引（阻塞写）
REINDEX TABLE public.users;

-- 重建整个数据库的索引（耗时，紧急情况用）
REINDEX DATABASE postgres;
```

### 3.2 UI 设计

在"DB Ops"页，对每张表提供操作按钮：

```
Table: public.users    Rows: 12,345    Size: 15 MB    Dead tuples: 234
[VACUUM] [ANALYZE] [VACUUM ANALYZE] [⚠️ VACUUM FULL] [REINDEX CONCURRENTLY]
```

VACUUM FULL 点击前弹窗二次确认："这会锁住表 X 秒/分钟，继续？"

### 3.3 后端实现

```go
type MaintenanceRequest struct {
    Op     string `json:"op"`     // "vacuum" | "vacuum_full" | "analyze" | "reindex_concurrently" | "reindex"
    Schema string `json:"schema"` // "public"
    Table  string `json:"table"`  // "users"
}

func (d *Deps) HandleMaintenance(w http.ResponseWriter, r *http.Request) {
    var req MaintenanceRequest
    json.NewDecoder(r.Body).Decode(&req)
    
    // 严格校验 schema/table 名字（防 SQL 注入）
    if !isValidIdent(req.Schema) || !isValidIdent(req.Table) {
        writeError(w, 400, "invalid schema/table name")
        return
    }
    
    // 用 pgx.QuoteIdentifier 构造 SQL
    qual := pgx.Identifier{req.Schema, req.Table}.Sanitize()
    
    var sql string
    switch req.Op {
    case "vacuum":
        sql = fmt.Sprintf("VACUUM (ANALYZE, VERBOSE) %s", qual)
    case "vacuum_full":
        sql = fmt.Sprintf("VACUUM (FULL, ANALYZE, VERBOSE) %s", qual)
    case "analyze":
        sql = fmt.Sprintf("ANALYZE VERBOSE %s", qual)
    case "reindex_concurrently":
        sql = fmt.Sprintf("REINDEX TABLE CONCURRENTLY %s", qual)
    case "reindex":
        sql = fmt.Sprintf("REINDEX TABLE %s", qual)
    default:
        writeError(w, 400, "unknown op")
        return
    }
    
    // VACUUM 不能在事务内运行
    conn, err := d.DB.RW.Acquire(r.Context())
    if err != nil { /* ... */ }
    defer conn.Release()
    
    _, err = conn.Exec(r.Context(), sql)
    if err != nil {
        writeError(w, 500, err.Error())
        return
    }
    writeJSON(w, 200, map[string]string{"status": "ok"})
}

var identRe = regexp.MustCompile(`^[a-z_][a-z0-9_$]*$`)

func isValidIdent(s string) bool {
    return len(s) < 64 && identRe.MatchString(s)
}
```

### 3.4 注意：VACUUM 不能在事务内

`VACUUM` 和 `REINDEX CONCURRENTLY` 不能包装在 BEGIN/COMMIT 里。我们的 SQL runner 用了事务模式。所以 DBA handler 要**独立于 SQL runner**，用 `conn.Exec` 直接跑（自动提交模式）。

---

## 4. 连接管理

### 4.1 列出所有 session

```sql
SELECT 
    pid,
    usename,
    application_name,
    client_addr,
    state,
    query_start,
    now() - query_start AS duration,
    wait_event_type,
    wait_event,
    LEFT(query, 200) AS query
FROM pg_stat_activity
WHERE state IS NOT NULL
  AND pid != pg_backend_pid()
ORDER BY query_start NULLS LAST;
```

### 4.2 取消查询（温和）

```sql
SELECT pg_cancel_backend(pid);
```
- 发送 SIGINT 给 backend
- 查询会 rollback 返回错误
- Session 保留

### 4.3 强制断开 session（强硬）

```sql
SELECT pg_terminate_backend(pid);
```
- 发送 SIGTERM
- Session 直接断开
- 客户端下次查询需要重连

### 4.4 权限

- 超级用户：可以 kill 任何连接
- `pg_signal_backend` 角色：可以 kill 非超级用户的连接
- 用户自己：可以 kill 自己的连接

我们以 `supabase_admin`（SUPERUSER）身份连，无限制。

### 4.5 UI

```
Active Sessions (12)
┌────┬──────────────┬─────────────┬────────┬───────────┬─────────────────────┐
│PID │ User         │ Client      │ State  │ Duration  │ Query               │
├────┼──────────────┼─────────────┼────────┼───────────┼─────────────────────┤
│1234│authenticator │172.17.0.5   │active  │00:00:02   │SELECT * FROM users…│[Cancel][Kill]│
│1235│supabase_auth_│172.17.0.6   │idle    │00:05:00   │-                    │     [Kill]   │
│... │              │             │        │           │                     │              │
└────┴──────────────┴─────────────┴────────┴───────────┴─────────────────────┘
```

刷新按钮 / 自动 5 秒刷新。

### 4.6 安全：防止 admin 杀自己

```go
// 不允许 kill pg_backend_pid（我们自己的连接）
if targetPID == ourOwnPID { return error }
```

---

## 5. 锁分析

### 5.1 阻塞关系查询

```sql
SELECT 
    blocked_locks.pid     AS blocked_pid,
    blocked_activity.usename  AS blocked_user,
    blocking_locks.pid     AS blocking_pid,
    blocking_activity.usename AS blocking_user,
    blocked_activity.query    AS blocked_query,
    blocking_activity.query   AS blocking_query,
    blocked_locks.mode     AS blocked_mode,
    blocking_locks.mode    AS blocking_mode
FROM  pg_catalog.pg_locks         blocked_locks
JOIN pg_catalog.pg_stat_activity blocked_activity  ON blocked_activity.pid = blocked_locks.pid
JOIN pg_catalog.pg_locks         blocking_locks 
    ON blocking_locks.locktype = blocked_locks.locktype
    AND blocking_locks.database IS NOT DISTINCT FROM blocked_locks.database
    AND blocking_locks.relation IS NOT DISTINCT FROM blocked_locks.relation
    AND blocking_locks.page IS NOT DISTINCT FROM blocked_locks.page
    AND blocking_locks.tuple IS NOT DISTINCT FROM blocked_locks.tuple
    AND blocking_locks.virtualxid IS NOT DISTINCT FROM blocked_locks.virtualxid
    AND blocking_locks.transactionid IS NOT DISTINCT FROM blocked_locks.transactionid
    AND blocking_locks.classid IS NOT DISTINCT FROM blocked_locks.classid
    AND blocking_locks.objid IS NOT DISTINCT FROM blocked_locks.objid
    AND blocking_locks.objsubid IS NOT DISTINCT FROM blocked_locks.objsubid
    AND blocking_locks.pid != blocked_locks.pid
JOIN pg_catalog.pg_stat_activity blocking_activity ON blocking_activity.pid = blocking_locks.pid
WHERE NOT blocked_locks.granted;
```

或更简洁用 PG 14+ 的 `pg_blocking_pids()`:

```sql
SELECT 
    activity.pid,
    activity.usename,
    activity.query,
    blocking.pid     AS blocking_pid,
    blocking.query   AS blocking_query
FROM pg_stat_activity activity
CROSS JOIN LATERAL unnest(pg_blocking_pids(activity.pid)) AS blocking_pid
JOIN pg_stat_activity blocking ON blocking.pid = blocking_pid
WHERE activity.state = 'active';
```

### 5.2 UI

```
Lock Analysis
┌────────────────────────────────────────────────────────────┐
│ Blocked PID 1234 (authenticator) is waiting for...         │
│   UPDATE users SET ...                                     │
│                                                            │
│ Blocking PID 5678 (supabase_admin):                        │
│   BEGIN; SELECT * FROM users FOR UPDATE;                   │
│   started 00:02:34 ago                                     │
│                                                            │
│   [Cancel 5678]  [Kill 5678]                               │
└────────────────────────────────────────────────────────────┘
```

## 6. 长事务告警

### 6.1 查询

```sql
SELECT 
    pid,
    usename,
    application_name,
    xact_start,
    now() - xact_start AS xact_duration,
    state,
    LEFT(query, 200) AS query
FROM pg_stat_activity
WHERE xact_start IS NOT NULL
  AND now() - xact_start > INTERVAL '5 minutes'
ORDER BY xact_start;
```

### 6.2 UI

放在 Dashboard 最顶端，作为"健康告警"。如果有长事务就红色高亮。

---

## 7. 慢查询（pg_stat_statements）

### 7.1 启用确认

Phase 1 的 init SQL 已经确保 `pg_stat_statements` 在 `extensions` schema 启用。需要 postgresql.conf 包含：

```
shared_preload_libraries = 'pg_stat_statements'
```

supabase/postgres 镜像**默认已加入**，因为 supautils 也在 shared_preload_libraries 里。

### 7.2 参数调优

```
# 以下可通过 command 参数在 docker-compose 传
pg_stat_statements.max = 10000           # 最多追踪 10000 条
pg_stat_statements.track = 'all'         # 追踪所有语句（含嵌套）
pg_stat_statements.track_utility = 'on'  # 追踪 DDL
pg_stat_statements.save = 'on'           # 重启保留
```

### 7.3 Top N 慢查询

```sql
SELECT 
    calls,
    total_exec_time,
    mean_exec_time,
    max_exec_time,
    stddev_exec_time,
    rows,
    100.0 * shared_blks_hit / NULLIF(shared_blks_hit + shared_blks_read, 0) AS hit_percent,
    LEFT(query, 500) AS query
FROM extensions.pg_stat_statements
WHERE query NOT LIKE '%pg_stat_statements%'
ORDER BY total_exec_time DESC
LIMIT 20;
```

### 7.4 UI

```
Top 20 Slowest Queries (by total time)
┌────┬─────────┬──────────┬──────────┬───────┬─────────────────────────┐
│ #  │ Calls   │ Total(ms)│ Mean(ms) │Rows   │ Query                   │
├────┼─────────┼──────────┼──────────┼───────┼─────────────────────────┤
│ 1  │ 15,234  │ 8,234.56 │ 0.54     │50,000 │SELECT * FROM users WHERE│
│ 2  │ 892     │ 5,123.40 │ 5.74     │ 8,920 │SELECT ... FROM orders   │
│... │         │          │          │       │                         │
└────┴─────────┴──────────┴──────────┴───────┴─────────────────────────┘

[Reset Stats]  按钮
```

点 query 可展开看 full SQL + `EXPLAIN ANALYZE` 按钮。

### 7.5 EXPLAIN

```sql
EXPLAIN (ANALYZE, BUFFERS, VERBOSE) <query>;
```

需要实际执行查询（ANALYZE 会跑），只读查询安全，写入要谨慎。

---

## 8. 其他 DBA 操作

### 8.1 数据库大小

```sql
SELECT 
    datname,
    pg_size_pretty(pg_database_size(datname)) AS size
FROM pg_database
ORDER BY pg_database_size(datname) DESC;
```

### 8.2 表大小 Top N

```sql
SELECT
    schemaname,
    relname AS table,
    pg_size_pretty(pg_total_relation_size(relid)) AS total_size,
    pg_size_pretty(pg_relation_size(relid)) AS table_size,
    pg_size_pretty(pg_indexes_size(relid)) AS indexes_size,
    n_live_tup,
    n_dead_tup
FROM pg_stat_user_tables
ORDER BY pg_total_relation_size(relid) DESC
LIMIT 20;
```

### 8.3 Buffer Cache Hit Ratio

```sql
SELECT
    100.0 * sum(heap_blks_hit) / nullif(sum(heap_blks_hit) + sum(heap_blks_read), 0) AS cache_hit_ratio
FROM pg_statio_user_tables;
```
通常 > 99% 说明内存够用。

### 8.4 索引使用情况

```sql
SELECT
    schemaname,
    relname AS table,
    indexrelname AS index,
    idx_scan AS scans,
    pg_size_pretty(pg_relation_size(indexrelid)) AS size,
    CASE WHEN idx_scan = 0 THEN '⚠️ never used' ELSE 'ok' END AS status
FROM pg_stat_user_indexes
ORDER BY idx_scan ASC, pg_relation_size(indexrelid) DESC;
```

找未使用的索引清理。

### 8.5 表膨胀

```sql
-- 需要 pgstattuple 扩展（supabase/postgres 镜像可能没预装）
SELECT 
    schemaname,
    relname,
    pg_size_pretty(pg_table_size(c.oid)) AS table_size,
    n_dead_tup,
    100.0 * n_dead_tup / NULLIF(n_live_tup + n_dead_tup, 0) AS dead_ratio
FROM pg_stat_user_tables
JOIN pg_class c ON c.relname = relname
WHERE n_dead_tup > 1000
ORDER BY dead_ratio DESC NULLS LAST;
```

dead_ratio > 10% 提示应该 VACUUM。

### 8.6 Checkpoint / WAL

```sql
SELECT
    checkpoints_timed,
    checkpoints_req,
    checkpoint_write_time,
    checkpoint_sync_time,
    buffers_checkpoint,
    buffers_clean,
    buffers_backend
FROM pg_stat_bgwriter;
```

### 8.7 复制状态（我们单实例，但 Supabase 官方可能有）

```sql
SELECT * FROM pg_stat_replication;
```

---

## 9. 参数管理

### 9.1 读当前参数

```sql
SELECT
    name,
    setting,
    unit,
    category,
    short_desc,
    context,       -- 'internal' | 'postmaster' | 'sighup' | 'superuser' | 'user'
    vartype,
    source         -- 'default' | 'configuration file' | 'session'
FROM pg_settings
ORDER BY category, name;
```

**`context` 决定是否需要重启**：
- `internal`：不可改
- `postmaster`：需要完全重启
- `sighup`：reload 即可（`SELECT pg_reload_conf()`）
- `superuser` / `user`：会话级或事务级

### 9.2 UI 标记动/静态

```
Parameter                  Value       Category    Effect
─────────────────────────────────────────────────────────────
shared_buffers            128MB       Memory      🔴 需要重启
work_mem                  4MB         Memory      🟢 立即生效
max_connections           100         Connection  🔴 需要重启
log_min_duration_statement -1         Logging     🟢 立即生效（reload）
```

### 9.3 改参数

```sql
-- 会话级
SET work_mem = '16MB';

-- 全局（需要 SUPERUSER，persistent）
ALTER SYSTEM SET work_mem = '16MB';
SELECT pg_reload_conf();   -- 应用到所有 session（非 postmaster 参数）
```

---

## 10. Phase 5 实施清单

1. **handler/dbops.go**:
   - POST /api/dbops/vacuum (body: schema, table, mode)
   - POST /api/dbops/reindex
   - POST /api/dbops/analyze
2. **handler/sessions.go**:
   - GET /api/sessions — 活跃连接列表
   - POST /api/sessions/:pid/cancel
   - POST /api/sessions/:pid/terminate
3. **handler/locks.go**:
   - GET /api/locks — 阻塞关系
4. **handler/stats.go**:
   - GET /api/stats/slow-queries — pg_stat_statements top N
   - POST /api/stats/reset — pg_stat_reset()
   - GET /api/stats/cache-hit
   - GET /api/stats/table-sizes
   - GET /api/stats/index-usage
5. **handler/params.go**:
   - GET /api/params — 所有参数 + context 标记
   - POST /api/params — ALTER SYSTEM + reload
6. **前端新增页**:
   - `/admin/dbops` — Maintenance operations
   - `/admin/sessions` — 连接管理
   - `/admin/performance` — 慢查询 + 统计
   - `/admin/params` — 参数管理

---

## 11. 风险清单

| 风险 | 缓解 |
|------|------|
| 用户点 VACUUM FULL 锁大表 | UI 弹窗二次确认 + 提示锁时长 |
| 用户杀错 session | 展示详细信息，kill 前 confirm dialog |
| ALTER SYSTEM 设错参数导致 PG 启不来 | 列白名单，危险参数（shared_buffers 等）不允许从 UI 改 |
| pg_stat_statements 数据丢失 | reset 前 confirm dialog |
| SQL 注入（schema/table 名 from user） | 严格 regex 校验 + pgx.Identifier 转义 |
| REINDEX CONCURRENTLY 失败留下 invalid 索引 | UI 显示 invalid 索引 + 提供清理按钮 |
