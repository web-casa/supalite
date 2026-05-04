# 可参考功能全景目录

> 基于 Dockge / Portainer / KubeBlocks / Pigsty 四家调研
> 按 supabase-lite 定位（轻量 + 一键 + 开发者友好 + Postgres 运维）标注契合度
> 日期：2026-04-17

## 图例

| 标签 | 含义 |
|------|------|
| ✅ | 契合定位，建议考虑 |
| ⚠️ | 边界地带，需要权衡 |
| ❌ | 偏离定位，谨慎 |
| 🟢 | 实现简单（<1 天） |
| 🟡 | 中等（2-5 天） |
| 🔴 | 较重（1 周以上） |
| 已有 | supabase-lite 已经实现 |

---

## 一、部署与安装

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 1.1 | 一键安装脚本 | — | setup.sh 自动生成密钥、.env、拉镜像 | ✅ | 🟢 | **已有** |
| 1.2 | Docker Compose profiles（minimal / full） | Pigsty | 用 profile 切换启用哪些服务 | ✅ | 🟢 | — |
| 1.3 | 安装向导引导 OAuth / SMTP | KubeBlocks / Pigsty | Setup Wizard 首次引导 | ✅ | 🟢 | **已有** |
| 1.4 | 离线安装支持（本地镜像仓库） | Pigsty | 预下载所有镜像到本地 tar | ⚠️ | 🟡 | — |
| 1.5 | 系统环境检查（Docker / openssl / curl） | — | 依赖检测 + 友好提示 | ✅ | 🟢 | **已有** |
| 1.6 | 自定义安装路径 / 数据目录 | Pigsty | 支持非默认路径 | ✅ | 🟢 | — |
| 1.7 | 升级脚本（在线热升级） | Pigsty | `setup.sh --upgrade` 自动迁移 | ✅ | 🟡 | — |
| 1.8 | 卸载脚本（清理数据卷） | Pigsty (`*-rm.yml`) | `teardown.sh` | ✅ | 🟢 | — |
| 1.9 | 多 arch 镜像（amd64/arm64） | Pigsty | 支持 Apple Silicon / ARM VPS | ✅ | 🟡 | — |
| 1.10 | 预构建二进制（admin server） | — | 无需 `go build`，下载即用 | ✅ | 🟡 | — |

---

## 二、鉴权与安全

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 2.1 | Bearer token 鉴权 | 所有 | 简单 API key | ✅ | — | **已有** |
| 2.2 | Cookie auth（支持 SSE/WS） | Portainer | HttpOnly Cookie，SSE 可用 | ✅ | 🟢 | — |
| 2.3 | CSRF 保护 | Portainer | gorilla/csrf，Cookie 场景必须 | ✅ | 🟡 | — |
| 2.4 | Auth 端点限流 | Portainer / Dockge | 20 次/分钟 防暴力破解 | ✅ | 🟢 | — |
| 2.5 | 全局 API 限流 | Dockge | 60 次/分钟 | ⚠️ | 🟢 | — |
| 2.6 | Admin token 定期轮换 | — | 一键生成新 token | ✅ | 🟢 | — |
| 2.7 | 多用户 / bcrypt 密码 | Dockge / Portainer | 用户名 + 密码登录 | ❌（定位单管理员）| 🟡 | — |
| 2.8 | JWT + password-hash proof | Dockge | 改密码自动失效旧 JWT | ❌ | 🟡 | — |
| 2.9 | API Key 多套并存 | Portainer | 脚本用 API Key，UI 用 Cookie | ⚠️ | 🟡 | — |
| 2.10 | RBAC / Teams / Resource Control | Portainer | 企业级权限 | ❌ | 🔴 | — |
| 2.11 | 2FA | Dockge | TOTP | ❌ | 🟡 | — |
| 2.12 | OAuth / LDAP 登录 admin | Portainer | 接入企业身份源 | ❌ | 🔴 | — |
| 2.13 | 签名鉴权到 Agent | Portainer | 非对称签名 | ❌（无 agent） | 🔴 | — |
| 2.14 | 严格 CORS origin 校验 | Portainer | 生产严格模式 | ✅ | 🟢 | **已有** |
| 2.15 | 安全 Headers（CSP / XFO / nosniff） | 自研 | 防 XSS / 点击劫持 | ✅ | — | **已有** |
| 2.16 | 常数时间 token 比较 | 自研 | crypto/subtle | ✅ | — | **已有** |
| 2.17 | 敏感字段屏蔽（密码 / Secret） | 自研 | API 不返回明文 | ✅ | — | **已有** |

---

## 三、实时交互 / UX

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 3.1 | SSE 日志流 + RingBuffer | Dockge | 实时日志，LimitQueue 缓冲 | ✅ | 🟡 | — |
| 3.2 | WebSocket 双向流 | Dockge / Portainer | 比 SSE 复杂，交互终端需要 | ⚠️ | 🟡 | — |
| 3.3 | SSE 状态推送（服务 up/down） | Dockge / Portainer | 服务端推，替代前端轮询 | ✅ | 🟢 | — |
| 3.4 | 操作进度流（operation_id） | Dockge / Portainer | 重启等长操作实时反馈 | ✅ | 🟡 | — |
| 3.5 | 多客户端订阅同一日志流 | Dockge | Terminal 多订阅模式 | ⚠️（1-2 人场景）| 🟡 | — |
| 3.6 | 同名操作互斥锁 | Dockge | 防止并发 restart | ✅ | 🟢 | — |
| 3.7 | 客户端自动重连（SSE 断了重连） | Dockge | EventSource 原生支持 | ✅ | 🟢 | — |
| 3.8 | 交互式容器终端（xterm + PTY） | Dockge | `docker exec` 网页终端 | ⚠️ | 🔴 | — |
| 3.9 | psql console（网页 Postgres shell） | — | 启发自 #3.8，SQL 版 | ⚠️ | 🔴 | — |
| 3.10 | HTTP 101 劫持模式 | Portainer | 不用 PTY 做双向流 | ⚠️ | 🔴 | — |
| 3.11 | 5-10s 后端 poll 推变更 | Dockge | cron 轮询 + 差异广播 | ✅ | 🟢 | — |
| 3.12 | 日志历史回放（新客户端连上时） | Dockge | RingBuffer 做历史 | ✅ | 🟢 | — |
| 3.13 | 暂停 / 继续日志流 | — | UI 上的按钮 | ✅ | 🟢 | — |
| 3.14 | 日志搜索 / 过滤 | — | 前端关键词过滤 | ✅ | 🟢 | — |

---

## 四、Docker / 容器管理

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 4.1 | 直接 Docker Engine API（unix socket） | Portainer | 不依赖 CLI | ✅ | — | **已有** |
| 4.2 | `docker compose up/down/restart` 封装 | Dockge / Pigsty | 通过 CLI 调 | ✅ | — | **已有** |
| 4.3 | 按 compose project 过滤容器 | Portainer / Dockge | label 识别 | ✅ | — | **已有** |
| 4.4 | 查看容器资源占用（CPU/内存） | Portainer | `docker stats` 集成 | ✅ | 🟡 | — |
| 4.5 | 容器重启策略展示 / 修改 | Portainer | — | ⚠️ | 🟡 | — |
| 4.6 | 查看容器环境变量 | Portainer | inspect 容器 | ✅ | 🟢 | — |
| 4.7 | 查看容器挂载卷 | Portainer | — | ✅ | 🟢 | — |
| 4.8 | 拉取新版本镜像 + 重建 | Dockge / Pigsty | `compose pull && up -d` | ✅ | 🟢 | — |
| 4.9 | 容器/服务单独停止 | Dockge / Portainer | 不停整个 stack | ✅ | 🟢 | — |
| 4.10 | 多 stack / compose 管理 | Dockge / Portainer | 我们只有 1 个 stack | ❌ | 🔴 | — |
| 4.11 | Compose 文件编辑器 | Dockge / Portainer | 直接改 YAML | ❌ | 🟡 | — |
| 4.12 | 多主机 agent | Portainer | 管理多台服务器 | ❌ | 🔴 | — |
| 4.13 | 多编排（K8s / Swarm） | Portainer | — | ❌ | 🔴 | — |
| 4.14 | Git 部署 | Portainer | 从 Git 拉 compose | ❌ | 🔴 | — |
| 4.15 | 镜像磁盘使用可视化 | Portainer | `docker system df` | ✅ | 🟢 | — |
| 4.16 | 镜像清理（悬空镜像） | Portainer | `docker image prune` | ✅ | 🟢 | — |

---

## 五、数据库 Schema 管理

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 5.1 | 表列表浏览 | KubeBlocks | information_schema | ✅ | — | **已有** |
| 5.2 | 表字段 / 约束 / 索引查看 | KubeBlocks | | ✅ | — | **部分（列已有）** |
| 5.3 | RLS 策略展示 | — | `pg_policies` | ✅ | 🟢 | — |
| 5.4 | RLS 策略模板生成器 | — | "用户只能读写自己的行" 一键生成 | ✅ | 🟡 | — |
| 5.5 | 索引可视化 + 使用率 | Pigsty Grafana | pg_stat_user_indexes | ✅ | 🟢 | — |
| 5.6 | 表膨胀 / 无效行检测 | Pigsty Grafana | pgstattuple | ✅ | 🟡 | — |
| 5.7 | 预置 Supabase schemas | Pigsty | auth / storage / realtime / graphql_public / _analytics / _realtime / supabase_functions | ✅ | 🟢 | **部分（auth 已有）** |
| 5.8 | Schema 多租户视图（切换 schema） | — | dropdown 切换当前 schema | ✅ | 🟢 | — |
| 5.9 | 外键关系图 | — | 生成 ERD | ⚠️ | 🔴 | — |
| 5.10 | Schema diff / migration 生成 | — | 对比两个版本 | ⚠️ | 🔴 | — |

---

## 六、数据库运维 / 操作

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 6.1 | SQL Editor（只读 / 读写） | — | 跑 SQL | ✅ | — | **已有** |
| 6.2 | Ctrl+Enter 快捷键 | — | 快速执行 | ✅ | — | **已有** |
| 6.3 | SQL 查询结果 CSV 导出 | — | 一键下载 | ✅ | 🟢 | — |
| 6.4 | SQL 查询历史 | — | 本地 storage 保存最近 100 条 | ✅ | 🟢 | — |
| 6.5 | SQL 收藏 / 命名 | — | "我常跑的 SQL" | ✅ | 🟢 | — |
| 6.6 | VACUUM 一键按钮 | KubeBlocks | 按表 VACUUM | ✅ | 🟢 | — |
| 6.7 | REINDEX 一键按钮 | KubeBlocks | 重建索引 | ✅ | 🟢 | — |
| 6.8 | ANALYZE 一键按钮 | KubeBlocks | 更新统计信息 | ✅ | 🟢 | — |
| 6.9 | pg_stat_reset() 一键 | — | 重置统计 | ✅ | 🟢 | — |
| 6.10 | 强制终止 session（pg_terminate_backend） | Pigsty | 杀连接 | ✅ | 🟢 | — |
| 6.11 | 取消查询（pg_cancel_backend） | Pigsty | | ✅ | 🟢 | — |
| 6.12 | 锁分析（谁阻塞谁） | Pigsty pgcat | pg_locks 分析 | ✅ | 🟡 | — |
| 6.13 | 长事务告警 | — | > N 秒的事务列出 | ✅ | 🟢 | — |
| 6.14 | pg_reload_conf | KubeBlocks | 热加载配置 | ✅ | 🟢 | — |
| 6.15 | 数据库大小 / 表大小 Top N | Pigsty | pg_database_size | ✅ | 🟢 | — |
| 6.16 | Generic Lifecycle Action 框架 | KubeBlocks | 可插拔的脚本动作 | ⚠️（过度抽象）| 🔴 | — |
| 6.17 | 参数热更新 vs 重启分类 | KubeBlocks | Static / Dynamic 标记 | ✅ | 🟢 | — |

---

## 七、数据库监控

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 7.1 | postgres-exporter sidecar | Pigsty / KubeBlocks | Prometheus 格式 metrics | ✅ | 🟢 | — |
| 7.2 | node-exporter（主机指标） | Pigsty | CPU/内存/磁盘 | ✅ | 🟢 | — |
| 7.3 | pgbouncer-exporter | Pigsty | 连接池指标 | ⚠️（我们无 pgbouncer）| 🟢 | — |
| 7.4 | Prometheus / VictoriaMetrics | Pigsty | 存储指标 | ⚠️（+1 服务）| 🟡 | — |
| 7.5 | Grafana | Pigsty | 看板 | ⚠️（+1 服务）| 🟡 | — |
| 7.6 | Pigsty pgsql-instance 面板 | Pigsty | 核心实例面板 | ✅ | 🟢（拷贝 JSON）| — |
| 7.7 | Pigsty pgsql-query 面板 | Pigsty | pg_stat_statements 面板 | ✅ | 🟢 | — |
| 7.8 | Pigsty pgsql-tables 面板 | Pigsty | 表膨胀 / 索引 | ✅ | 🟢 | — |
| 7.9 | Pigsty pgsql-session 面板 | Pigsty | 会话 / 锁 | ✅ | 🟢 | — |
| 7.10 | Pigsty pgsql-activity 面板 | Pigsty | 连接活动 | ✅ | 🟢 | — |
| 7.11 | Pigsty pgsql-pitr 面板 | Pigsty | 备份历史 | ✅ | 🟢 | — |
| 7.12 | admin 面板内嵌 Grafana iframe | — | 直接嵌入而非跳转 | ✅ | 🟢 | — |
| 7.13 | 自研 mini-dashboard（不引入 Grafana） | — | admin 前端自己画简单图 | ✅（更轻）| 🟡 | — |
| 7.14 | Alerts / 告警规则 | Pigsty | Prometheus Alerting | ⚠️ | 🟡 | — |
| 7.15 | 健康检查 endpoint（ready / live） | Portainer | K8s 风格 | ✅ | 🟢 | **部分** |

---

## 八、数据库备份与恢复

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 8.1 | 一键 pg_dump 备份下载 | — | 手动备份 | ✅ | 🟢 | — |
| 8.2 | 上传 .sql 恢复 | — | 手动恢复 | ✅ | 🟢 | — |
| 8.3 | 定时 pg_dump（cron） | KubeBlocks BackupSchedule | 每天自动 | ✅ | 🟡 | — |
| 8.4 | 保留策略（最近 N 份 / M 天） | KubeBlocks | 自动清理旧备份 | ✅ | 🟢 | — |
| 8.5 | 备份加密（AES） | Pigsty | 备份文件加密 | ✅ | 🟢 | — |
| 8.6 | 备份到 S3 / MinIO | Pigsty | 云存储 | ⚠️ | 🟡 | — |
| 8.7 | 备份到本地卷 | KubeBlocks | 默认 | ✅ | 🟢 | — |
| 8.8 | pgBackRest 集成（增量） | Pigsty | 真增量 + PITR | ⚠️（增加依赖）| 🔴 | — |
| 8.9 | WAL 归档 / PITR | Pigsty / KubeBlocks | 恢复到时间点 | ⚠️（进阶可选）| 🔴 | — |
| 8.10 | 备份恢复时含角色密码 | KubeBlocks | pg_dumpall --globals | ✅ | 🟢 | — |
| 8.11 | 备份元数据 / 索引 | KubeBlocks | 每次备份的 size/path/time | ✅ | 🟢 | — |
| 8.12 | 多种备份方式开关（pg_dump / pg_basebackup / wal-g） | KubeBlocks BackupPolicy | 可插拔 | ⚠️ | 🟡 | — |
| 8.13 | 备份验证（恢复到临时 DB 检验） | — | 确保备份可用 | ⚠️ | 🔴 | — |
| 8.14 | 恢复前自动快照现状 | Pigsty | 防覆盖 | ✅ | 🟡 | — |

---

## 九、数据库性能

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 9.1 | pg_stat_statements 启用 | Pigsty | 跟踪所有 SQL | ✅ | 🟢 | — |
| 9.2 | 慢查询 Top N | KubeBlocks / Pigsty | 按 total_time / calls 排序 | ✅ | 🟡 | — |
| 9.3 | EXPLAIN / EXPLAIN ANALYZE 按钮 | — | SQL Editor 里一键 | ✅ | 🟢 | — |
| 9.4 | 查询计划可视化 | — | 图形化 plan tree | ⚠️ | 🔴 | — |
| 9.5 | 连接池状态（pgbouncer） | Pigsty | — | ⚠️ | 🟡 | — |
| 9.6 | 自动参数调优（按 mem/CPU） | Pigsty oltp.yml | shared_buffers / work_mem 计算 | ✅ | 🟡 | — |
| 9.7 | Buffer cache hit ratio | Pigsty | 缓存命中率 | ✅ | 🟢 | — |
| 9.8 | Checkpoint 频率 / WAL 生成速率 | Pigsty | | ✅ | 🟢 | — |
| 9.9 | IO 等待分析 | Pigsty | pg_stat_io | ✅ | 🟢 | — |

---

## 十、数据库参数管理

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 10.1 | postgresql.conf 可视化编辑 | KubeBlocks | 按分类展示参数 | ✅ | 🟡 | — |
| 10.2 | 参数动态 / 静态标记 | KubeBlocks | 要不要重启 | ✅ | 🟢 | — |
| 10.3 | 参数默认值 / 推荐值提示 | KubeBlocks | 根据硬件推荐 | ✅ | 🟡 | — |
| 10.4 | 参数 schema 校验（范围/类型） | KubeBlocks | 防设错 | ✅ | 🟡 | — |
| 10.5 | 参数变更历史 | — | 审计谁什么时候改了什么 | ⚠️ | 🟡 | — |
| 10.6 | 参数 diff 对比 | Pigsty | 当前 vs 默认 | ✅ | 🟢 | — |
| 10.7 | pg_settings 一键查询 | — | SQL Runner 封装 | ✅ | 🟢 | — |
| 10.8 | 预设模板（OLTP / OLAP / Tiny） | Pigsty | 一键切换风格 | ⚠️ | 🟡 | — |
| 10.9 | pg_hba.conf 模板化管理 | Pigsty | 地址 + 用户别名 | ✅ | 🟡 | — |

---

## 十一、用户 / 角色 / 权限管理

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 11.1 | GoTrue auth.users 列表 | — | 业务用户管理 | ✅ | — | **已有** |
| 11.2 | 删除用户 | — | | ✅ | — | **已有** |
| 11.3 | 重置用户密码 | — | admin 发重置邮件或直接改 | ✅ | 🟢 | — |
| 11.4 | 用户搜索 / 分页 | — | 大量用户时需要 | ✅ | 🟢 | — |
| 11.5 | 用户元数据 / app_metadata 编辑 | — | Supabase 常用 | ✅ | 🟡 | — |
| 11.6 | 手动创建用户 | — | admin 直接加 | ✅ | 🟢 | — |
| 11.7 | 邀请用户（发邮件） | — | | ✅ | 🟡 | — |
| 11.8 | 封禁 / 解封用户 | — | | ✅ | 🟢 | — |
| 11.9 | PG 角色 SystemAccount 统一视图 | KubeBlocks | 所有 DB role 一页展示 | ✅ | 🟡 | — |
| 11.10 | PG 角色密码旋转 | KubeBlocks | 一键生成新密码 | ✅ | 🟡 | — |
| 11.11 | dbrole_readonly / readwrite / admin 分层 | Pigsty | 角色继承 | ✅ | 🟢 | — |
| 11.12 | 补齐 12 个 Supabase PG 角色 | Pigsty | 对齐标准 | ✅ | 🟢 | **部分（5/12）** |
| 11.13 | 按数据库 / schema 授权 UI | — | GRANT/REVOKE 可视化 | ⚠️ | 🟡 | — |
| 11.14 | 审计用户登录记录 | Dockge / Portainer | auth.audit_log_entries | ✅ | 🟢 | — |

---

## 十二、扩展管理

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 12.1 | 已启用扩展列表 | Pigsty | pg_extension | ✅ | 🟢 | — |
| 12.2 | 可用但未启用扩展列表 | Pigsty | pg_available_extensions | ✅ | 🟢 | — |
| 12.3 | CREATE EXTENSION 一键按钮 | Pigsty | UI 安装扩展 | ✅ | 🟢 | — |
| 12.4 | DROP EXTENSION 一键按钮 | Pigsty | | ✅ | 🟢 | — |
| 12.5 | 扩展说明 / 文档链接 | Pigsty | 每个扩展注释 | ✅ | 🟢 | — |
| 12.6 | 预装 15 个 Supabase 扩展 | Pigsty | pg_graphql / pgvector / pg_cron / supabase_vault... | ✅ | 🟡 | **部分（3/15）** |
| 12.7 | 扩展分类（FDW / GIS / AI / 时序） | Pigsty | 按场景分组 | ⚠️ | 🟢 | — |
| 12.8 | 自定义 PG 镜像（预装所需扩展） | Pigsty | Dockerfile FROM postgres + 扩展 | ✅ | 🟡 | — |

---

## 十三、Supabase 规范对齐

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 13.1 | 补齐 7 个 Supabase PG 角色 | Pigsty | dashboard_user / supabase_*_admin | ✅ | 🟢 | — |
| 13.2 | 补齐 8 个核心扩展 | Pigsty | pg_graphql / pgvector / pg_cron... | ✅ | 🟡 | — |
| 13.3 | 补齐 6 个预留 schema | Pigsty | realtime / storage / graphql_public... | ✅ | 🟢 | — |
| 13.4 | 收紧 pg_hba.conf | Pigsty | 模板化，默认不 trust | ✅ | 🟡 | — |
| 13.5 | Realtime 服务（WebSocket） | Pigsty Supabase 模块 | | ⚠️（违反 minimal）| 🔴 | — |
| 13.6 | Storage API + MinIO | Pigsty Supabase 模块 | | ⚠️ | 🔴 | — |
| 13.7 | postgres-meta 服务 | Pigsty Supabase 模块 | 为 Studio 铺路 | ⚠️ | 🟡 | — |
| 13.8 | Edge Functions (Deno) | Pigsty Supabase 模块 | | ❌ | 🔴 | — |
| 13.9 | Analytics (Logflare) | Pigsty Supabase 模块 | | ❌ | 🔴 | — |
| 13.10 | Vector（日志采集） | Pigsty Supabase 模块 | | ❌ | 🔴 | — |
| 13.11 | ImgProxy | Pigsty Supabase 模块 | | ❌ | 🔴 | — |
| 13.12 | 官方 Supabase Studio UI | Pigsty Supabase 模块 | 替代自研 admin | ❌（自研是差异化）| 🔴 | — |
| 13.13 | Kong API 网关 | Pigsty Supabase 模块 | 我们用 Caddy | ❌ | 🔴 | — |

---

## 十四、前端 / 工程模式

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 14.1 | shadcn/ui + lucide + Tailwind | — | | ✅ | — | **已有** |
| 14.2 | Next.js static export | — | 打包进 Go 二进制 | ✅ | — | **已有** |
| 14.3 | React Query 三层（service/query/keys） | Portainer | | ✅ | 🟡 | — |
| 14.4 | Zustand store | Portainer | | ✅ | — | **已有** |
| 14.5 | xterm.js 终端 | Dockge / Portainer | | ⚠️ | 🟡 | — |
| 14.6 | Monaco SQL Editor | — | 语法高亮 / 自动补全 | ✅ | 🟡 | — |
| 14.7 | 暗黑模式 | — | | ✅ | 🟢 | **已有** |
| 14.8 | 主题切换（暗 / 亮） | — | | ✅ | 🟢 | — |
| 14.9 | 中英文 i18n | Pigsty | | ⚠️ | 🟡 | — |
| 14.10 | Go 中间件链 / Bouncer | Portainer | 鉴权统一 | ✅ | 🟡 | — |
| 14.11 | Context 注入中间件 | Portainer | handler 零样板 | ✅ | 🟢 | — |
| 14.12 | 构造器依赖注入 | Portainer | | ✅ | 🟢 | — |
| 14.13 | Service / Handler 分层 | Portainer | | ✅ | 🟡 | — |
| 14.14 | DB 事务抽象（ViewTx / UpdateTx） | Portainer | | ⚠️（已有 pgx Tx） | 🟢 | — |
| 14.15 | 结构化日志（zerolog） | Portainer | JSON 日志 | ✅ | 🟢 | — |
| 14.16 | OpenAPI / Swagger | Portainer | 自动生成 API 文档 | ⚠️ | 🟡 | — |
| 14.17 | 单元测试覆盖率门槛 | — | CI 强制 | ✅ | 🟡 | — |

---

## 十五、日志管理

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 15.1 | 单服务日志查看 | Dockge / Portainer | docker logs | ✅ | — | **已有** |
| 15.2 | 服务切换下拉 | Dockge | | ✅ | — | **已有** |
| 15.3 | 实时日志流（SSE） | Dockge | | ✅ | 🟡 | — |
| 15.4 | 日志关键词搜索 / 过滤 | — | 前端 | ✅ | 🟢 | — |
| 15.5 | 日志级别高亮（ERROR 红 / WARN 黄） | — | | ✅ | 🟢 | — |
| 15.6 | 日志下载（.log 文件） | — | | ✅ | 🟢 | — |
| 15.7 | 聚合日志（所有服务合并） | — | 看全局 | ⚠️ | 🟡 | — |
| 15.8 | Postgres 日志解析（按查询聚合） | Pigsty | | ⚠️ | 🔴 | — |
| 15.9 | 审计日志 /api/audit | Dockge | 记录所有 admin 操作 | ✅ | 🟢 | — |

---

## 十六、操作历史 / 审计

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 16.1 | Admin 操作历史（谁什么时候做了什么） | Portainer | | ✅ | 🟡 | — |
| 16.2 | 配置变更历史 | KubeBlocks | .env 每次改动留记录 | ✅ | 🟢 | — |
| 16.3 | 备份操作历史 | KubeBlocks | 每次备份的状态 | ✅ | 🟢 | — |
| 16.4 | Auth 事件历史 | — | auth.audit_log_entries | ✅ | 🟢 | — |
| 16.5 | OpsRequest 风格（运维操作统一抽象） | KubeBlocks | 每个操作一个 ID + status | ⚠️（过度抽象）| 🔴 | — |

---

## 十七、开发者体验

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 17.1 | Quick Start 代码片段（JS/Python/curl） | — | Dashboard 直接复制 | ✅ | — | **已有** |
| 17.2 | supabase-js 连接示例 | — | | ✅ | — | **已有** |
| 17.3 | REST API 端点浏览器 | — | PostgREST 自动生成 | ✅ | 🟡 | — |
| 17.4 | GraphQL Explorer | Pigsty | 如果启用了 pg_graphql | ⚠️ | 🟡 | — |
| 17.5 | ENV 变量查看（.env 只读视图） | — | | ✅ | 🟢 | — |
| 17.6 | 命令行 CLI（kbcli 风格） | KubeBlocks / Pigsty | `supabase-lite sql "..."` | ⚠️ | 🟡 | — |
| 17.7 | 一键导出 docker-compose.yml（给其他场景用） | — | | ⚠️ | 🟢 | — |
| 17.8 | 示例项目模板 | — | 生成 Next.js + supabase-js 项目 | ⚠️ | 🟡 | — |
| 17.9 | 安装时生成的 setup 报告（.md） | — | 记录本次安装的所有参数 | ✅ | 🟢 | — |
| 17.10 | 在线 playground / 尝鲜模式 | — | 体验产品 | ❌ | 🔴 | — |

---

## 十八、其他 / 杂项

| # | 功能 | 来源 | 描述 | 契合 | 难度 | 现状 |
|---|------|------|------|------|------|------|
| 18.1 | SMTP 测试发送按钮 | — | 配置完一键测试 | ✅ | 🟢 | — |
| 18.2 | OAuth 配置校验（ping 一下端点） | — | 配错立刻知道 | ✅ | 🟡 | — |
| 18.3 | TLS / HTTPS 自动（Caddy auto_https） | — | 生产必备 | ✅ | 🟢 | — |
| 18.4 | 多环境（dev / staging / prod）切换 | Portainer | | ⚠️ | 🔴 | — |
| 18.5 | 导出 / 导入 admin 配置 | — | 迁移用 | ✅ | 🟢 | — |
| 18.6 | 自动更新通知（新版本可用） | — | GitHub API 查 release | ✅ | 🟢 | — |
| 18.7 | 匿名使用统计（telemetry） | Portainer | 默认关，用户可选 | ⚠️ | 🟡 | — |
| 18.8 | 错误上报 / crash report | — | | ⚠️ | 🟡 | — |
| 18.9 | 文档内嵌（? 图标点开说明） | — | 降低学习成本 | ✅ | 🟢 | — |
| 18.10 | 快捷键帮助（`?` 弹出） | — | | ✅ | 🟢 | — |

---

## 统计摘要

| 契合度 | 数量 | 占比 |
|--------|------|------|
| ✅ 契合 | 约 140 | ~70% |
| ⚠️ 边界 | 约 35 | ~18% |
| ❌ 偏离 | 约 25 | ~12% |
| **总数** | **~200** | — |

| 现状 | 数量 |
|------|------|
| 已实现 | ~25 |
| 部分实现 | ~8 |
| 未实现 | ~167 |

---

## 使用说明

1. 我从四家调研里**罗列所有功能点**（不预先过滤），标注契合度和难度
2. **✅** 默认契合定位，可以考虑；**⚠️** 需要权衡（往往和 minimal 定位有冲突）；**❌** 建议跳过
3. **🟢 / 🟡 / 🔴** 是实现难度参考
4. 挑你想做的告诉我，我们可以合并成一个具体的 Sprint 计划

---

## 建议挑选思路

如果想**现在就动手**，按以下三种思路之一挑：

### 思路 A：对齐规范（"让 supabase-js 代码都能跑"）
从 **13.1 / 13.2 / 13.3 / 13.4** 开始 → **11.12 / 12.6** → **10.9**

### 思路 B：提升现有体验（"让 admin 更好用"）
从 **3.1（SSE 日志）/ 3.3（状态推送）/ 3.4（重启进度）/ 2.4（rate limit）** 开始 → **15.4 / 15.5 / 15.6** → **6.3 / 6.4 / 6.5**

### 思路 C：加差异化功能（"做运维驾驶舱"）
从 **11.9（SystemAccount 视图）** 开始 → **6.6 / 6.7 / 6.8（Maintenance）** → **9.1 / 9.2（慢查询）** → **8.3 / 8.4（定时备份）** → **7.1 / 7.6 / 7.7（监控面板）**

---

最后，整个目录里有不少**反面案例**（标 ❌ 的），保留它们是为了让你看到"我们应该放弃什么"也是一种决策。
