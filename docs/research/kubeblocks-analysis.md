# KubeBlocks 参考分析

> 参考项目：https://github.com/apecloud/kubeblocks
> 本地路径：`~/otheruse/kubeblocks`
> 分析日期：2026-04-17

## 1. 项目定位

KubeBlocks 是 **Kubernetes 原生的数据库 operator**，用一套统一 CRD 管理 35+ 种数据库（Postgres / MySQL / Redis / MongoDB / Kafka / etcd / ClickHouse ...）。

| 维度 | Dockge | Portainer | **KubeBlocks** |
|------|--------|-----------|----------------|
| 定位 | Compose 管理 | 多编排容器管理 | **数据库生命周期管理** |
| 平台 | Docker | Docker + Swarm + K8s | **仅 K8s** |
| 关心什么 | 容器 | 容器 + 编排 | **数据库本身（主从 / 备份 / PITR / 参数 / 角色）** |
| 规模 | 个人 | 企业 | **生产级数据库平台** |

**与 supabase-lite 的关系**：KubeBlocks 的 K8s operator 架构（CRD + Reconcile + DCS + 多副本拓扑）**完全不适用**——我们是 docker-compose 单实例。但它是一个**数据库视角**而非**容器视角**的产品，这是 Portainer / Dockge 都缺的视角。从它借鉴的不是**架构**，而是**数据库管理要做哪些事**。

## 2. 核心抽象（概念层面）

KubeBlocks 把数据库管理抽象成一套声明式 CRD：

```
Cluster                   ← 用户声明"我要一个 PG 集群"
 └─ ClusterDefinition     ← 定义拓扑（如 primary + N secondary）
     └─ Component         ← 一个具体实例（1 主 2 从中的某一个）
         └─ ComponentDefinition  ← 可复用的蓝图，含：
             - Pod 模板
             - 配置模板（ConfigMap）
             - SystemAccount 定义（用户 / 权限 / 初始化 SQL）
             - LifecycleActions（RoleProbe / Switchover / Reconfigure / MemberJoin / MemberLeave / AccountProvision）
             - 端口 / 服务 / 存储
             - Exporter / 监控
             - TLS / ServiceAccount
```

**关键文件**：
- `apis/apps/v1/cluster_types.go`
- `apis/apps/v1/componentdefinition_types.go`（~90k 行配置选项）
- `apis/dataprotection/v1alpha1/backup_types.go`
- `apis/operations/v1alpha1/`

## 3. 数据库生命周期关键模式

这一节是 KubeBlocks 真正独特的部分。

### 3.1 Lifecycle Actions（数据库行为契约）

每个数据库行为定义成**可插拔的脚本动作**，存在 ComponentDefinition 里：

| Action | 作用 | 举例（Postgres） |
|--------|------|------------------|
| **RoleProbe** | 探测当前实例的角色 | 调 Patroni API 或 `pg_is_in_recovery()`，Pod 打上 `role=primary\|secondary` label |
| **Switchover** | 主从切换 | 执行 `patronictl switchover --candidate <name>` |
| **Reconfigure** | 重新加载配置 | `pg_ctl reload` 或 `SELECT pg_reload_conf()` |
| **AccountProvision** | 初始化账号 | 执行 CreateAccountStatement SQL |
| **MemberJoin** | 新副本加入 | Patroni member 注册 |
| **MemberLeave** | 副本移除 | 清理元数据 |

**核心模式**：把"数据库能做什么"抽象成一组有名字的 action，每个 action 是一段脚本，调度和时序由 operator 负责。

### 3.2 SystemAccount（声明式账号管理）

```yaml
systemAccounts:
  - name: postgres
    initAccount: true
    statement:
      create:   "CREATE USER postgres SUPERUSER"
      delete:   "DROP USER IF EXISTS postgres"
      update:   "ALTER USER postgres PASSWORD '$1'"
  - name: replicator
    statement: { create: "CREATE USER replicator REPLICATION" }
  - name: kbadmin
    statement: { create: "..." }
```

- 声明账号及其建/删/改 SQL
- 密码自动生成，存 K8s Secret
- 备份时加密封存（`kubeblocks.io/encrypted-system-accounts` annotation），恢复时重新注入

### 3.3 Parameter Schema（动态 vs 静态）

**文件**：`pkg/parameters/config_util.go`

每个参数在 schema 里标明：
- **Dynamic**：热更新，调 Reconfigure action 即可（如 `max_connections`、`work_mem`）
- **Static**：必须重启容器（如 `shared_buffers`、`max_wal_size`）

改参数的流程：
1. 用户修改 → Reconfigure OpsRequest
2. KubeBlocks 验证新值（范围 / 类型）+ merge 到 config 文件
3. 根据 schema 决定：调 Reconfigure action（热） vs 滚动重启

### 3.4 Backup / Restore / PITR（备份分层）

**文件**：`apis/dataprotection/v1alpha1/`

```
BackupRepo          ← 存储目标（S3 / Minio / NAS）
BackupPolicy        ← "对这种 DB 我有哪些备份方式"
  └─ Methods
     - pg-basebackup   (全量)
     - postgres-wal-g  (增量 WAL)
     - postgresql-pitr (连续 WAL 归档)
BackupSchedule      ← cron + 保留策略
  - method: pg-basebackup
    cron:   "0 18 * * *"
    retention: 7d
  - method: archive-wal
    cron:   "*/5 * * * *"
    retention: 8d
Backup              ← 单次备份记录（status / 路径 / 大小 / 加密）
Restore             ← 从备份恢复（支持 PITR 到指定时间点）
```

**亮点**：
- 备份方法**可插拔**：每种数据库声明自己支持的方法
- PITR 用持续 WAL 归档实现，恢复时回放到指定时间
- 每次备份附带 SystemAccount 的加密密码 → 恢复后账号仍可用

### 3.5 Cluster Phase 状态机

```
Creating  →  Running  →  Updating  →  Stopping  →  Stopped  →  Deleting
```

每个阶段有明确语义。UI / CLI 按 phase 展示不同状态 + 可执行操作。

### 3.6 Sidecar Exporter 做 Metrics

- `disableExporter: false` → 自动注入 `postgres-exporter` sidecar
- 暴露 `:9187/metrics`（Prometheus 格式）
- PodMonitor CRD 自动被 Prometheus 抓取
- 预置 Grafana 面板

## 4. Postgres 专项（实际能用的细节）

从 `examples/postgresql/` 和 addon 定义看，KubeBlocks 管 Postgres 的典型做法：

| 方面 | 做法 |
|------|------|
| 基础镜像 | Zalando **Spilo**（内置 Patroni + pg_basebackup + wal-g） |
| HA | Patroni 用 K8s API 作为 DCS |
| 角色检测 | RoleProbe 调 Patroni REST API |
| 角色 | `postgres`(super) / `replicator` / `kbadmin` / `kbprobe` / `kbmonitoring` |
| 备份方式 | pg_basebackup（全量） + wal-g（增量） + pg_archivecleanup（清理） |
| Metrics | postgres-exporter sidecar |
| 扩展 | 启动脚本里 `CREATE EXTENSION IF NOT EXISTS ...`（pg_stat_statements、pgvector 等） |
| 参数调优 | 通过 Parameter schema，区分 dynamic/static |

## 5. 值得借鉴的模式

### ✅ 适合借鉴（抽象层面）

| 模式 | 价值 | supabase-lite 怎么用 |
|------|------|---------------------|
| **SystemAccount 声明式账号** | 所有 DB 角色统一管理、自动生成密码、绑定 Secret | 把 `anon / authenticated / service_role / supabase_admin / authenticator` 定义成声明式清单，admin 面板统一展示和旋转 |
| **Parameter schema（动态/静态）** | 改参数时告知用户是否需要重启 | Settings 页面标注 `GOTRUE_MAILER_AUTOCONFIRM` 这类"重启生效"，避免盲目重启 |
| **Cluster phase 状态机** | 运维视角统一 | supabase-lite 也可以暴露 `Starting / Running / Degraded / Stopped / Migrating` 状态 |
| **pg_dump + WAL 归档的备份分层** | 全量 + 增量 + PITR 组合 | 备份功能里提供 pg_dump（日）+ 可选 WAL 归档（PITR，进阶） |
| **postgres-exporter sidecar** | 零侵入 metrics 采集 | docker-compose 加一个 exporter 容器 |
| **Lifecycle Action 抽象思路** | 每种 DB 操作是有名字的脚本 | supabase-lite 的 "重启 / 重建索引 / VACUUM / 密码旋转" 都可以做成声明式 action |
| **备份加密 + 账号封存** | 恢复后账号可直接用 | pg_dump 时把 role 密码也打包（或用 pg_dumpall --globals-only） |

### ⚠️ 部分借鉴（简化后用）

| 模式 | 为什么简化 |
|------|-----------|
| **BackupPolicy + BackupSchedule** | CRD 不需要，但**"方法声明 + 调度"的分层**可以落成 JSON 配置 |
| **RoleProbe** | 单实例不需要探主从，但可以借**"周期性探测 DB 就绪 + 写入状态"** 的模式 |
| **PITR** | 需要外部对象存储，进阶用户可选启用 |

### ❌ 不适合借鉴（K8s 原生能力）

| 模式 | 原因 |
|------|------|
| CRD + Reconcile Loop | 没有 K8s |
| ComponentDefinition / ClusterDefinition 完整 CRD | 我们是固定单机 stack |
| Patroni / DCS / 分布式共识 | 单实例无需 HA |
| 分片 / 多副本拓扑 | 场景不适用 |
| ServiceAccount + RBAC | K8s 专有 |
| Kubernetes Secret / TLS 管理 | 用 docker secrets 或 env 即可 |
| kbcli（交互式 CLI） | 我们走 Web UI 优先 |

## 6. 对 supabase-lite 的具体建议

### 高 ROI（真正补齐数据库视角的短板）

| # | 借鉴点 | 当前缺失 | ROI | 改动量 |
|---|--------|----------|-----|--------|
| **K1** | **定时自动备份（pg_dump + 保留策略）** | admin 只有手动备份按钮（规划中），无调度 | ⭐⭐⭐⭐⭐ | 中 |
| **K2** | **Postgres 监控 sidecar（postgres-exporter + 面板）** | 没有任何 DB 指标 | ⭐⭐⭐⭐ | 低 |
| **K3** | **Parameter 静态/动态标记** | Settings 里所有字段一视同仁，用户不知道哪些要重启 | ⭐⭐⭐⭐ | 低 |
| **K4** | **SystemAccount 清单视图 + 密码旋转** | `service_role_key` / `anon_key` / DB 角色密码散落，无统一视图 | ⭐⭐⭐ | 中 |
| **K5** | **数据库 "phase" 状态机（不只是容器状态）** | 现在只看容器 running/exited，看不到 "迁移中 / 备份中 / 就绪未验证"  | ⭐⭐⭐ | 低 |

### 中 ROI（高级功能）

| # | 借鉴点 | 当前缺失 | ROI | 改动量 |
|---|--------|----------|-----|--------|
| **K6** | **Lifecycle Action 抽象** ("VACUUM / REINDEX / ANALYZE / 密码旋转" 等一键操作) | 都要用户自己进 SQL | ⭐⭐⭐ | 中 |
| **K7** | **慢查询日志查看** | 有 logs 页但没针对 DB 优化 | ⭐⭐⭐ | 中 |
| **K8** | **pg_stat_statements 启用 + 查询 TopN** | 没有 | ⭐⭐ | 中 |
| **K9** | **WAL 归档 / PITR（可选）** | 没有 | ⭐⭐ | 高 |

### 跳过

| # | 借鉴点 | 原因 |
|---|--------|------|
| K10 | HA / 主从 / Patroni | 单实例定位 |
| K11 | 多副本拓扑 / 分片 | 超范围 |
| K12 | K8s CRD / Reconcile | 不是 K8s |

## 7. 与前两份分析的关系

KubeBlocks 补充的**独特视角**：

| 视角 | Dockge | Portainer | **KubeBlocks** |
|------|--------|-----------|----------------|
| 容器是否运行 | ✅ | ✅ | ✅ |
| 容器日志 | ✅ | ✅ | ✅ |
| **数据库角色管理** | ❌ | ❌ | **✅ SystemAccount** |
| **DB 参数 schema（动/静态）** | ❌ | ❌ | **✅ Parameter schema** |
| **定时备份 + 保留策略** | ❌ | ❌ | **✅ BackupSchedule** |
| **PITR / WAL 归档** | ❌ | ❌ | **✅** |
| **DB metrics 集成（exporter）** | ❌ | ❌ | **✅** |
| **VACUUM / REINDEX 一键操作** | ❌ | ❌ | **✅ Lifecycle Action** |
| 实时流式日志 | ✅ | ✅ | ⚠️（通过 kubectl） |
| 交互终端 | ✅ | ✅ | ⚠️（通过 kubectl exec） |
| 多用户 RBAC | ✅ | ✅ | ⚠️（K8s RBAC） |

**结论**：前两家告诉我们**怎么管容器**，KubeBlocks 告诉我们**怎么管数据库**。supabase-lite 作为"Postgres-first"的产品，数据库视角的功能（备份调度、参数分类、账号管理、DB 指标）反而是差异化的核心。

## 8. 结论

- KubeBlocks **架构（K8s operator）完全不适用**，别试图克隆
- 但它的**数据库管理哲学**值得全盘吸收：**数据库不是普通容器**，需要对它的参数 / 账号 / 备份 / 指标 / 角色有专门的抽象
- 最值得立刻落地：**K2（postgres-exporter 监控）** + **K1（定时备份）** + **K3（Parameter 动/静态标记）**
- 中期可做：**K4（SystemAccount 统一视图）** + **K6（VACUUM/REINDEX 一键动作）** + **K7（慢查询查看）**
- 高级/可选：**K9（PITR）** — 需要外部存储

这三家产品合起来给出完整蓝图：
- **实时性** 学 Dockge（日志流 / 操作进度）
- **工程模式** 学 Portainer（鉴权 / 中间件 / React Query）
- **数据库管理** 学 KubeBlocks（账号 / 参数 / 备份 / 监控）

---

## 附录：关键文件索引

| 主题 | 文件 | 要点 |
|------|------|------|
| Cluster CRD | `apis/apps/v1/cluster_types.go` | 顶层抽象 + phase 状态机 |
| ComponentDefinition | `apis/apps/v1/componentdefinition_types.go` | 核心蓝图：账号 / 参数 / Action / 监控 / TLS 全包 |
| Backup / PITR | `apis/dataprotection/v1alpha1/backup_types.go` | BackupPolicy / Schedule / Repo 分层 |
| OpsRequest | `apis/operations/v1alpha1/` | 所有运维动作（备份 / 恢复 / 切换 / 重配）统一入口 |
| Reconfigure 实现 | `pkg/operations/reconfigure.go` | 参数 merge + 校验 + 动静决策 |
| Backup 实现 | `pkg/operations/backup.go` | 调度 → 执行 → 封存密码 |
| Switchover | `pkg/operations/switchover.go` | 用 Lifecycle Action 脚本完成 |
| 参数工具 | `pkg/parameters/config_util.go` | Schema 驱动的配置合并 / 校验 |
| Postgres 例子 | `examples/postgresql/README.md` | 完整生命周期演示 |
| Postgres cluster 样例 | `examples/postgresql/cluster.yaml` | Spilo + Patroni 标签样例 |
| 标签常量 | `pkg/constant/labels.go` | role / app / component 标准化 |
