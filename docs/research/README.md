# 参考产品调研

这里存放对类似产品的架构/功能调研，用于指导 supabase-lite admin 面板的功能取舍。

## 已调研（4 家）

| 产品 | 文档 | 定位 | 对 supabase-lite 贡献 |
|------|------|------|----------------------|
| **Dockge** | [dockge-analysis.md](./dockge-analysis.md) | Compose 可视化管理（个人/小团队） | **实时 UX 模式**（WS + PTY + LimitQueue） |
| **Portainer** | [portainer-analysis.md](./portainer-analysis.md) | 企业级多编排容器管理 | **Go 工程模式**（Bouncer / 中间件链 / Context / React Query） |
| **KubeBlocks** | [kubeblocks-analysis.md](./kubeblocks-analysis.md) | K8s 数据库 operator（35+ 数据库） | **数据库抽象**（SystemAccount / Parameter schema / 备份分层） |
| **Pigsty** | [pigsty-analysis.md](./pigsty-analysis.md) | 企业级 PostgreSQL 发行版（Ansible） | **Supabase 最佳实践清单**（角色 / 扩展 / schema / Grafana 面板） |

### 视角互补示意

```
       ┌──────────────────────────────────────────────────────┐
       │                supabase-lite 需要的能力                │
       └──────────────────────────────────────────────────────┘
           ↑                ↑              ↑              ↑
      实时 UX 启示    工程模式启示    数据库抽象启示    Supabase 规范启示
           │                │              │              │
        Dockge         Portainer       KubeBlocks       Pigsty
      （实时日志 /    （鉴权 / 路由 /  （Action / 备份 /   （角色 / 扩展 /
       操作进度 /      中间件 /        参数 schema /       schema / 面板 /
       状态推送）      DI / Query）    phase 状态）        PITR / 调优）
```

---

## 最终功能清单（4 家综合）

按优先级和来源分组：

### 🔴 P0 必做

#### A. 实时交互（来自 Dockge + Portainer）

| # | 功能 | 来源 |
|---|------|------|
| A1 | **SSE 日志流 + RingBuffer** | Dockge LimitQueue |
| A2 | **operation_id + SSE 重启进度流** | Dockge + Portainer |
| A3 | **SSE 状态推送**（替代前端 30s 轮询） | Dockge + Portainer |
| A4 | **Cookie auth**（支持 SSE/WS header 限制） | Portainer |
| A5 | **Auth 端点 rate limit** | Dockge + Portainer |

#### B. 数据库管理（来自 KubeBlocks + Pigsty）

| # | 功能 | 来源 |
|---|------|------|
| B1 | **postgres-exporter sidecar + Pigsty 14 个 Grafana 面板** | Pigsty（面板可直接用） |
| B2 | **定时备份**（pg_dump + cron + 保留；进阶可换 pgBackRest） | KubeBlocks + Pigsty |
| B3 | **Parameter 动/静态标记** | KubeBlocks |
| B4 | **SystemAccount 统一视图 + 密码旋转** | KubeBlocks |

#### C. Supabase 规范（Pigsty 独家）

| # | 功能 | 来源 |
|---|------|------|
| C1 | **补齐 7 个 Supabase PG 角色**（dashboard_user / supabase_auth_admin / supabase_storage_admin / supabase_functions_admin / supabase_replication_admin / supabase_etl_admin / supabase_read_only_user） | Pigsty |
| C2 | **补齐 8 个 Supabase PG 扩展**（pg_graphql / pgvector / pg_cron / supabase_vault / pgsodium / http / pg_net / pg_jsonschema） | Pigsty |
| C3 | **补齐 6 个预留 schema**（realtime / storage / graphql_public / supabase_functions / _analytics / _realtime） | Pigsty |
| C4 | **收紧 pg_hba.conf**（地址别名 + 用户别名 + 优先级排序） | Pigsty |

### 🟡 P1 重要

| # | 功能 | 来源 |
|---|------|------|
| D1 | VACUUM / REINDEX / ANALYZE 一键操作 | KubeBlocks |
| D2 | CSRF 保护（随 cookie 引入） | Portainer |
| D3 | 数据库 phase 状态机（不只是 running/exited） | KubeBlocks |
| D4 | 慢查询日志 / pg_stat_statements 面板 | KubeBlocks + Pigsty |
| D5 | Auto-tuning 参数（按内存/CPU 计算） | Pigsty |
| D6 | dbrole_readonly / readwrite / admin 分层角色 | Pigsty |

### 🟢 P2 增值（确认采纳）

| # | 功能 | 状态 | 归入版本 |
|---|------|------|---------|
| E3 | **postgres-meta 服务** | ✅ 采纳 | v0.5 (Phase 2) |
| E4 | **Studio 集成**（官方 Supabase Studio UI）| ✅ 采纳 | v0.5 (Phase 2) |
| E5 | **pgBackRest 增量**（无 PITR / WAL）| ✅ 采纳 | v0.7 (Phase 6) |
| E6 | React Query 三层重构 | ✅ 采纳 | v1.0 (Phase 8) |

### ⚫ 明确跳过

- 监控模块（postgres-exporter + Prometheus/Grafana / 告警规则 / Pigsty 面板）—— **本期不做**
- Realtime 服务（Pigsty Supabase 模块）—— 用户需要建议上 Pigsty
- Storage API + MinIO / 自建 S3 —— 只支持外部 S3
- Edge Functions (Deno)
- Analytics / Logflare / Vector
- WAL 归档 / PITR
- 官方 Kong 网关（保留 Caddy）
- 多用户 / RBAC / Teams（Dockge / Portainer）
- 多主机 / Agent / 反向隧道
- 多编排（Swarm / K8s）
- Compose 文件编辑
- 交互终端（PTY 或 HTTP 劫持）
- Git deploy / Stack 版本
- HA / Patroni / Etcd / HAProxy
- 464 扩展全量
- 多 PG 内核分支（Citus / Babelfish / OrioleDB 等）
- Ansible / IaC 交付
- 交互式 CLI
- Auto-tuning 参数（Pigsty）—— 未列入本期

---

## 架构决策汇总

| 问题 | 决策 | 来源 |
|------|------|------|
| 实时推送用什么？ | **SSE**（Go 标准库，单向足够） | Dockge / Portainer |
| Auth 怎么支持 SSE？ | **Cookie + Bearer 双支持**（HMAC cookie）| Portainer |
| Cookie auth 什么时候做？ | **Phase 2**（和 Studio 一起做，一步到位） | 产品决策 |
| 日志怎么缓冲？ | **内存 RingBuffer + fan-out** | Dockge LimitQueue |
| 备份存哪？ | **本地卷 + 外部 S3**（无自建 MinIO） | 产品决策 |
| S3 客户端选什么？ | **minio-go v7**（国内云兼容好）| 调研 |
| 备份加密？ | **AES-256-GCM + 独立 SECRET_ENCRYPTION_KEY** | 产品决策 |
| 增量备份？ | **pgBackRest（可选 profile）**，**不做 PITR / WAL 归档** | 产品决策 |
| pgBackRest 启用方式？ | **独立 override compose 文件**，避免默认 archive_mode=on 堆积 | 冲突修订 |
| 监控指标？ | **本期放弃**（未来单独开一期） | 产品决策 |
| 配置参数 UI？ | **Schema-driven 表单，标注 dynamic/static**（基于 pg_settings.context） | KubeBlocks Parameter |
| 账号管理 UI？ | **SystemAccount 清单页 + 一键密码旋转** | KubeBlocks SystemAccount |
| PG 角色标准？ | **信任 supabase/postgres 镜像的 14 个角色，自己不重复建** | 调研 + ADR-001 |
| PG 扩展标准？ | **启用 8 个核心扩展**（pg_graphql / pgvector / pg_cron / supabase_vault / pgsodium / pg_net / http / pg_jsonschema） | Pigsty |
| HBA 怎么写？ | **保留镜像默认 + `POSTGRES_BIND_ADDR=127.0.0.1`**（不覆盖 HBA 模板）| 调研 |
| 网关用什么？ | **Caddy**（轻，SSE 自动识别）| 自研 |
| Admin UI 策略？ | **Studio + 自研 admin 双栈并存**，自研聚焦运维视角 | 产品决策 |
| Studio 禁用哪些菜单？ | `NEXT_PUBLIC_DISABLED_FEATURES=project_storage:all,project_edge_function:all,realtime:all` | 调研 |
| Compose profile？ | **反向用法**：Studio/pgbackrest 打 profile，`.env` 默认写 `COMPOSE_PROFILES=...` 激活 | 调研 |

---

## 执行计划（路线图见 `docs/product-strategy.md` §五）

- **v0.5**：Phase 1（Supabase 规范对齐）+ Phase 2（Studio 集成 + Cookie auth）
- **v0.6**：Phase 3（自研 admin 瘦身）+ Phase 4（SSE 实时）
- **v0.7**：Phase 5（DBA 驾驶舱）+ Phase 6（备份 + 外部 S3）
- **v1.0**：Phase 7（DX）+ Phase 8（工程化）

---

## 下一步

准备好执行 **v0.5 / Phase 1**。改动清单在 `docs/pg-supabase-compatibility.md` §9。

开工前必读：
- `docs/decisions/001-revert-supabase-admin-grant.md`（Codex 第一轮修复的撤回）
- `docs/pg-supabase-compatibility.md` §9（Phase 1 动手清单）
