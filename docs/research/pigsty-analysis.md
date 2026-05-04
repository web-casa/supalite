# Pigsty 参考分析

> 参考项目：https://github.com/pgsty/pigsty
> 本地路径：`~/otheruse/pigsty`
> 分析日期：2026-04-17

## 1. 项目定位

Pigsty 是一个**基于 Ansible 的企业级 PostgreSQL 发行版**。"Self-Host Postgres like a Pro"：

- 464 个 PostgreSQL 扩展内置
- HA（Patroni + Etcd）+ PITR（pgBackRest）+ IaC（Ansible）
- 59 个生产级 Grafana 面板 + VictoriaMetrics 监控栈
- 开箱即用的 HAProxy + PgBouncer + VIP 服务接入
- 12 种 PG 内核分支支持（Citus / Babelfish / IvorySQL / PolarDB / Supabase ...）
- **自带 Supabase 模块**（app/supabase/）

**与 supabase-lite 的关系**：这是四家参考产品里**最相关的**一家。

| 维度 | Dockge | Portainer | KubeBlocks | **Pigsty** |
|------|--------|-----------|-----------|------------|
| 视角 | 容器 | 容器 | 数据库（K8s） | **PG 发行版（裸机）** |
| 与 Supabase 关系 | 无 | 无 | 无 | **官方 Supabase 模块** |
| 对 supabase-lite 价值 | 实时性启发 | 工程模式启发 | 数据库抽象启发 | **Supabase 最佳实践直接参考** |

Pigsty 的 Ansible 交付机制不适用（我们用 docker-compose），但它**定义了什么是"生产级自托管 Supabase"**——哪些角色、哪些扩展、哪些 schema、哪些服务、哪些面板——几乎是可以直接抄的清单。

## 2. Pigsty 对 Supabase 的完整方案

**关键发现**：Pigsty 的 Supabase 模块（`app/supabase/docker-compose.yml`）揭示了**完整 Supabase Stack** 实际上比我们现在实现的多很多。

### 2.1 完整 Supabase 服务清单

| 服务 | 版本 | 作用 | supabase-lite 现状 |
|------|------|------|---------------------|
| **PostgreSQL** | 自行管理 | 数据库 | ✅ 已有 |
| **GoTrue** | v2.186.0 | 鉴权 | ✅ 已有 |
| **PostgREST** | v14.5 | 自动 REST API | ✅ 已有 |
| **Kong** | v2.8.1 | API 网关 | ⚠️ 用 Caddy 替代 |
| **Realtime** | v2.76.5 | WebSocket 实时订阅 | ❌ |
| **Storage API** | v1.37.8 | S3 兼容对象存储 | ❌ |
| **Studio** | 2026.02.16 | 官方管理 UI | ❌（用自研 admin） |
| **postgres-meta** | v0.95.2 | Studio 后端 | ❌ |
| **Edge Functions** | v1.70.3 | Deno 边缘函数 | ❌ |
| **Analytics (Logflare)** | v1.31.2 | 日志聚合 | ❌ |
| **Vector** | v0.53.0 | 日志采集 | ❌ |
| **ImgProxy** | v3.30.1 | 图片处理 | ❌ |
| **MinIO** | - | Storage 后端 | ❌ |

**重要判断**：从"minimal Supabase"角度，我们现在的 **Postgres + PostgREST + GoTrue + Caddy + 自研 admin** 已经覆盖了 Supabase 的核心（数据 + API + 鉴权）。其他服务是**可选增值**：
- **Realtime** — 很多项目需要
- **Storage** — 多项目需要
- **Studio** — 自研 admin 可替代
- **Edge Functions / Analytics / Vector / ImgProxy** — 少数项目需要

### 2.2 Supabase 需要的 Postgres 角色（12 个）

**文件**：`conf/supabase.yml:66-79`

```yaml
pg_users:
  - { name: anon              } # NOLOGIN
  - { name: authenticated     } # NOLOGIN
  - { name: service_role, bypassrls: true }  # NOLOGIN, BYPASSRLS
  - { name: dashboard_user, replication: true, createdb: true, createrole: true }
  - { name: supabase_admin, superuser: true, pgbouncer: true }  # 全能
  - { name: authenticator, roles: [dbrole_admin, authenticated, anon, service_role] }
  - { name: supabase_auth_admin, pgbouncer: true, createrole: true }
  - { name: supabase_storage_admin, pgbouncer: true, createrole: true }
  - { name: supabase_functions_admin, pgbouncer: true, createrole: true }
  - { name: supabase_replication_admin, replication: true }
  - { name: supabase_etl_admin, roles: [pg_read_all_data] }
  - { name: supabase_read_only_user, bypassrls: true, roles: [pg_read_all_data] }
```

**我们当前的实现**（`volumes/db/init/00-roles.sh`）：
- ✅ anon, authenticated, service_role, authenticator, supabase_admin
- ❌ dashboard_user, supabase_auth_admin, supabase_storage_admin, supabase_functions_admin, supabase_replication_admin, supabase_etl_admin, supabase_read_only_user

**差距**：我们缺了 7 个"按服务/按场景区分"的角色。没有 Storage/Realtime 服务时不需要，但**一旦要支持这些服务，必须补齐**。

### 2.3 Supabase 需要的 PostgreSQL 扩展（15 个）

**文件**：`conf/supabase.yml:86-102`

```yaml
pg_databases:
  - name: postgres
    extensions:
      - { name: pgcrypto,          schema: extensions }
      - { name: pg_net,            schema: extensions }
      - { name: pgjwt,             schema: extensions }
      - { name: uuid-ossp,         schema: extensions }
      - { name: pgsodium }             # vault 依赖
      - { name: supabase_vault }       # 密钥管理
      - { name: pg_graphql }           # GraphQL API
      - { name: pg_jsonschema }        # JSON Schema 校验
      - { name: wrappers }             # 外部数据源
      - { name: http }                 # HTTP client in SQL
      - { name: pg_cron }              # 定时任务
      - { name: timescaledb }          # 时序（可选）
      - { name: pg_tle }               # Trusted Language Extensions
      - { name: vector }               # pgvector - AI/embeddings
      - { name: pgmq }                 # 消息队列
```

**我们当前启用的扩展**（`00-roles.sh`）：
- ✅ uuid-ossp, pgcrypto, pgjwt
- ❌ pg_net, pgsodium, supabase_vault, pg_graphql, pg_jsonschema, wrappers, http, pg_cron, timescaledb, pg_tle, vector, pgmq

**差距**：我们只启用了最基础的 3 个。若要兼容 supabase-js 高级功能（GraphQL / vault / AI embeddings / 定时任务），需要补齐。

### 2.4 Supabase 需要的 Schema

```
extensions       - Supabase 扩展
auth             - GoTrue 用户 / 会话
realtime         - Realtime 元数据
storage          - Storage API 元数据
graphql_public   - pg_graphql 暴露
supabase_functions - Edge Function 元数据
_analytics       - Logflare 日志
_realtime        - Realtime 内部
```

**我们当前创建**：auth, extensions（仅两个）。

### 2.5 Supabase 环境变量清单（docker-compose.yml 关键片段）

从 `app/supabase/.env` 和 `app/supabase/docker-compose.yml` 可见：

```bash
POSTGRES_HOST=10.10.10.10         # 外部 PG 集群
POSTGRES_PORT=5432                # 或走 pgBouncer 端口
POSTGRES_DB=postgres              # 或 'supa'
POSTGRES_PASSWORD=...             # supabase_admin 密码
JWT_SECRET=...                    # 至少 32 字符
JWT_EXPIRY=3600
S3_ENDPOINT=https://sss.pigsty:9000  # MinIO
S3_BUCKET=supa
S3_REGION=stub
SUPABASE_PUBLIC_URL=http://supa.pigsty
ANON_KEY=...                      # 等同我们的 ANON_KEY
SERVICE_ROLE_KEY=...              # 等同我们的 SERVICE_ROLE_KEY
```

**用法关键**：所有容器用 `extra_hosts` 把 `POSTGRES_DOMAIN` 解析到外部 PG 集群 IP；Storage 服务需要挂载 Pigsty CA 证书 `/etc/pki/ca.crt`。

## 3. 其他值得借鉴的模式

### 3.1 基于模板的 pg_hba.conf

**文件**：`roles/pgsql/templates/pg_hba.conf`

Pigsty 的 HBA 规则用**地址别名 + 用户别名 + 优先级排序**，非常清晰：

```
# 地址别名：local, localhost, admin, infra, cluster, intra, world
# 用户别名：${dbsu}, ${repl}, ${monitor}, ${admin}
# 规则优先级：0-99 用户高优 / 100-999 默认 / 1000+ 用户低优

# 顺序规则：0, 100, 200, 300, ...
- { user: '${dbsu}',      db: all,         addr: local,     auth: ident,    title: 'dbsu access via local os user ident' }
- { user: '${repl}',      db: replication, addr: localhost, auth: ssl,      title: 'replicator replication from localhost' }
- { user: '${repl}',      db: replication, addr: intra,     auth: ssl,      title: 'replicator from intranet' }
- { user: '${monitor}',   db: all,         addr: localhost, auth: pwd,      title: 'monitor from localhost' }
- { user: all,            db: all,         addr: intra,     auth: pwd,      title: 'app users from intranet' }
- { user: all,            db: all,         addr: world,     auth: deny,     title: 'reject all other connections' }
```

**关键点**：别名 + 排序 + 角色区分 = 安全且可审计。我们现在的 HBA 是默认 trust，需要收紧。

### 3.2 Postgres 自动调优（按硬件）

**文件**：`roles/pgsql/templates/oltp.yml`

四种模式：`oltp.yml` / `olap.yml` / `crit.yml` / `tiny.yml`。

自动计算参数：
```
shared_buffers        = 25% × node_mem  (可配置比例)
work_mem              = shared_buffers / max_connections, 范围 64MB-1GB
effective_cache_size  = total_mem - shared_buffers
maintenance_work_mem  = shared_buffers × 25%
max_worker_processes  = CPU cores + 8, 最小 16
max_parallel_workers  = CPU cores × 50%, 最小 2
max_parallel_workers_per_gather = CPU cores × 20%, 2-8
```

**RTO 模式**：fast(30s) / norm(45s) / safe(90s) / wide(150s)—— 控制 Patroni TTL 和故障转移行为。

### 3.3 默认角色体系

**文件**：`roles/pgsql/defaults/main.yml:101-125`

```yaml
# 角色层级
dbrole_readonly  # SELECT / USAGE
dbrole_offline   # SELECT，用于报表 / 离线节点
dbrole_readwrite # readonly + INSERT/UPDATE/DELETE
dbrole_admin     # readwrite + CREATE SCHEMA / TRUNCATE / TRIGGER

# 系统用户
dbuser_dba       # SUPERUSER + pgbouncer session mode
dbuser_monitor   # pg_monitor + pgbouncer readonly

# 业务用户通过继承 dbrole_* 获得权限
```

清晰的**角色层级继承**让权限管理可扩展。supabase-lite 可以借鉴作为基础层，Supabase 特定角色叠加在上面。

### 3.4 pgBackRest 备份

**文件**：`pgsql-pitr.yml`，`conf/supabase.yml:252-293`

比 pg_dump 强：
- **增量备份** + **WAL 归档** = 真正的 PITR
- 本地仓库 + 远程 S3/MinIO 双重保留策略
- 可选 AES-256-CBC 加密
- 恢复目标：`time` / `xid` / `name` / `lsn` / `immediate`

配置示例：
```yaml
pgbackrest_method: minio
pgbackrest_repo:
  local:  { path: /pg/backup, retention_full_type: count, retention_full: 2 }
  minio:
    type: s3, s3_endpoint: sss.pigsty, s3_bucket: pgsql
    cipher_type: aes-256-cbc
    retention_full_type: time, retention_full: 14
```

**默认 cron**：`00 01 * * * /pg/bin/pg-backup full`

### 3.5 Grafana 面板库（59 个）

**文件**：`files/grafana/pgsql/`

核心 14 个（可直接抄）：
```
pgsql-overview.json       集群总览
pgsql-instance.json       单实例详情
pgsql-database.json       单库活动
pgsql-tables.json         表膨胀 / 索引效率
pgsql-query.json          慢查询（pg_stat_statements）
pgsql-session.json        活跃会话 / 锁
pgsql-activity.json       连接活跃度
pgsql-replication.json    复制延迟 / standby
pgsql-patroni.json        HA 状态
pgsql-pgbouncer.json      连接池
pgsql-pitr.json           备份历史 / WAL 归档
pgsql-exporter.json       exporter 健康
```

进阶 10 个（pg_catalog 视角）：`pgcat-instance/database/query/locks/table/schema/...`

**这些 JSON 是生产级的**，Pigsty 自己的 demo.pigsty.io 就是用这些面板。**可以直接拷贝用在 supabase-lite**（前提：有 postgres_exporter 接入 Prometheus/VM）。

## 4. 对 supabase-lite 的具体建议

基于 Pigsty 对 Supabase 的完整参考方案，**直接补齐**以下"一线 Supabase"的清单：

### 🔴 P0 立即补齐（对齐 Supabase 规范）

| # | 借鉴点 | 当前缺口 | ROI | 改动 |
|---|--------|----------|-----|------|
| **PG1** | **补齐 PG 角色（7 个）**：dashboard_user, supabase_auth_admin, supabase_storage_admin, supabase_functions_admin, supabase_replication_admin, supabase_etl_admin, supabase_read_only_user | 缺 7 个 | ⭐⭐⭐⭐ 对齐标准 | 低（SQL） |
| **PG2** | **补齐 PG 扩展**：pg_graphql, pg_jsonschema, pgvector, pg_cron, http, pg_net, pgsodium, supabase_vault | 缺 8 个核心 | ⭐⭐⭐⭐⭐ 决定能力边界 | 中（需改 Dockerfile 或换镜像） |
| **PG3** | **补齐 Schema**：realtime, storage, graphql_public, supabase_functions, _analytics, _realtime | 缺 6 个 | ⭐⭐⭐⭐ 为后续服务铺路 | 低（SQL） |
| **PG4** | **收紧 pg_hba.conf**：地址 + 用户别名 + 规则排序 | 默认 trust | ⭐⭐⭐⭐ 安全 | 低（模板） |

### 🟡 P1 按需增值服务

| # | 借鉴点 | 决策 | ROI |
|---|--------|------|-----|
| **SVC1** | **加 Realtime 服务** | Supabase 核心之一，很多项目需要 | ⭐⭐⭐⭐ |
| **SVC2** | **加 Storage API + MinIO** | 对象存储刚需 | ⭐⭐⭐ |
| **SVC3** | **加 postgres-meta** | 未来集成 Studio 的基础 | ⭐⭐ |
| **SVC4** | **可选集成官方 Studio**（作为自研 admin 的替代选项） | 权衡：自研保留 DB 管理特色，Studio 功能更全 | ⭐⭐ 讨论 |
| **SVC5** | 加 Edge Functions (Deno) | 小众需求 | ⭐ |
| **SVC6** | 加 Analytics (Logflare) | 对日志要求高才需要 | ⭐ |

### 🟢 P2 运维增强（与 KubeBlocks 建议叠加）

| # | 借鉴点 | 当前缺口 | ROI |
|---|--------|----------|-----|
| **OPS1** | **pgBackRest 替代 pg_dump**（或作为进阶选项） | 只有 pg_dump 规划 | ⭐⭐⭐ PITR + 增量 |
| **OPS2** | **自动调优参数**（按容器内存/CPU 计算 shared_buffers 等） | 全默认 | ⭐⭐⭐ |
| **OPS3** | **拿来 Pigsty 的 Grafana 面板**（14 个核心） | 只有 postgres-exporter 规划 | ⭐⭐⭐⭐ 零成本获得生产级 UI |
| **OPS4** | **pg_stat_statements 启用 + 慢查询面板** | 未启用 | ⭐⭐⭐ |
| **OPS5** | **dbrole_* 分层角色体系**（在 Supabase 角色之上再叠） | 没有 | ⭐⭐ |

### ⚫ 跳过

| # | 借鉴点 | 原因 |
|---|--------|------|
| — | Ansible / Patroni / Etcd / HAProxy | 我们用 docker-compose，单实例无 HA |
| — | 464 扩展全量 | 只需 Supabase 那 15 个 |
| — | 多节点 / 多 PG 版本 / 多内核 | 超范围 |

## 5. 与前三家分析的对比

### Pigsty 提供的**独特价值**

| 能力 | Dockge | Portainer | KubeBlocks | **Pigsty** |
|------|--------|-----------|-----------|------------|
| 实时日志 | ✅ | ✅ | ⚠️ | - |
| 容器管理 | ✅ | ✅ | ✅ | - |
| 数据库抽象 | - | - | ✅ | ✅ |
| **Supabase 模块** | - | - | - | **✅ 完整参考** |
| **生产 Grafana 面板** | - | - | 一般 | **✅ 59 个** |
| **pgBackRest + PITR** | - | - | ✅ | **✅ 裸机版** |
| **自动参数调优** | - | - | ✅ | **✅** |
| **扩展管理** | - | - | 一般 | **✅ 464 个** |
| **默认角色层级** | - | - | - | **✅ dbrole_*** |
| **HBA 模板化** | - | - | - | **✅** |

### 四家合流的完整视图

| 维度 | 主要贡献者 |
|------|-----------|
| 实时 UX（日志流 / 操作进度 / 状态推送） | **Dockge** |
| Go 工程模式（中间件链 / Context / React Query / cookie auth） | **Portainer** |
| 数据库抽象（账号 / 参数 / 备份 / 指标 / Action） | **KubeBlocks** |
| **Supabase 具体清单**（角色 / 扩展 / schema / 服务 / env） | **Pigsty** |
| 生产级 Grafana 面板 | **Pigsty**（可拿来直接用） |

## 6. 结论

Pigsty 是四家里**对 supabase-lite 最直接相关**的。它告诉我们：

1. **"Minimal Supabase" 的定义需要重新审视**：
   - 我们的 PostgreSQL + PostgREST + GoTrue + Caddy 是**骨架**
   - 对标"一线 Supabase"还缺：Realtime / Storage / 更多角色 / 更多扩展
   - 决定保留 minimal 还是逐步补齐，是**产品决策**

2. **即使保持 minimal，也该立刻补齐**：
   - 缺失的 7 个 PG 角色（至少文档化，用户需要时一键启用）
   - 缺失的 8 个 PG 扩展（至少给一键启用开关）
   - 6 个预留 schema
   - 收紧 pg_hba.conf

3. **Grafana 面板 14 个是免费午餐**：接入 postgres-exporter 就能直接用

4. **pgBackRest 是生产级备份事实标准**：比 pg_dump 强很多，值得作为进阶选项

## 7. 最终四家综合建议（更新）

合并四家后的 **P0 清单**（供你决策）：

```
🔴 实时性（Dockge + Portainer）
  1. SSE 日志流 + RingBuffer
  2. operation_id + SSE 重启进度流  
  3. SSE 状态推送
  4. Cookie auth
  5. Auth 端点 rate limit

🔴 数据库管理（KubeBlocks + Pigsty）
  6. postgres-exporter sidecar + 14 个 Grafana 面板（Pigsty 面板直接用）
  7. 定时 pg_dump 备份（或 pgBackRest 进阶）
  8. Parameter 动/静态标记
  9. SystemAccount 统一视图 + 密码旋转

🔴 Supabase 规范（Pigsty 独家）
  10. 补齐 7 个 Supabase PG 角色
  11. 补齐 8 个 Supabase PG 扩展（核心：pg_graphql, pgvector, pg_cron, supabase_vault）
  12. 补齐 6 个 schema
  13. 收紧 pg_hba.conf（用模板化别名 + 优先级）
```

---

## 附录：关键文件索引

| 主题 | 文件 | 要点 |
|------|------|------|
| **Supabase compose 定义** | `app/supabase/docker-compose.yml` | 13 个服务完整示例 |
| **Supabase .env 模板** | `app/supabase/.env` | 完整环境变量清单 |
| **Supabase 配置示例** | `conf/supabase.yml` | 12 角色 + 15 扩展 + pgBackRest 配置 |
| PG 角色定义 | `roles/pgsql/defaults/main.yml:101-125` | dbrole_* 分层 |
| PG 角色模板 | `roles/pgsql/templates/pg-user.sql` | SQL 模板 |
| PG 库模板 | `roles/pgsql/templates/pg-db.sql` | 含 schema / extension / pool 配置 |
| HBA 模板 | `roles/pgsql/templates/pg_hba.conf` | 地址 + 用户别名 + 排序 |
| 参数自动调优 | `roles/pgsql/templates/oltp.yml` | shared_buffers / work_mem 计算 |
| PITR 执行 | `pgsql-pitr.yml` | 恢复流程 |
| pgBackRest 配置 | `roles/pgsql/defaults/main.yml` (`pgbackrest_*`) | 本地 + S3 |
| Grafana 面板 | `files/grafana/pgsql/pgsql-*.json` | 14 核心 + 10 pg_cat |
| pg_exporter 查询定义 | `roles/pg_monitor/templates/pg_exporter.yml` | metrics 采集规则 |
| Supabase 服务版本 | `app/supabase/docker-compose.yml` 各 image tag | 可对标升级 |
