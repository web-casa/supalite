# supabase-lite 综合产品建议

> 基于 Dockge / Portainer / KubeBlocks / Pigsty 四家调研的综合分析
> 日期：2026-04-17

## 一、现状与市场格局

### 1.1 supabase-lite 现状

我们已经有：
- **核心栈**：Postgres + PostgREST + GoTrue + Caddy 网关
- **自研 Admin 面板**（Go + Next.js static export）覆盖：SQL 编辑器、数据表浏览器、Auth 用户管理、服务日志、设置向导、OAuth/SMTP 配置
- **工程质量**：经过多轮 Codex review，核心功能稳固

### 1.2 竞品定位图

```
                               【运维复杂度】
                                    ↑
                                    │ Pigsty（Ansible / 裸机 / HA / 464 扩展）
                                    │     ↑
                                    │     │  Supabase 官方 self-host
                                    │     │  （13 服务 docker-compose）
                                    │
                                    │                  KubeBlocks
                                    │                  （K8s operator）
                                    │
                                    │  Portainer
                                    │  （企业级容器管理）
                                    │
                                    │     Dockge（compose UI）
                                    │
                   ★ supabase-lite ──┼──────────────────────→【功能丰富度】
                  （最轻 / 最简）
```

**关键观察**：
- **"功能最全 + 运维重"** 的位置，Pigsty 已经占牢了（**它连 Supabase 模块都自带**）
- **"K8s + 生产级"** 的位置，KubeBlocks 占了
- **"轻量 + 开发者友好"** 的位置，**目前是空的**——这就是 supabase-lite 的机会

---

## 二、定位建议

### 2.1 推荐定位：**"Developer-first Minimal Supabase"**

> 为个人项目、学习者、原型验证、本地开发、简单 SaaS 提供的"足够用"的自托管 Supabase。

**核心价值主张**：
- **10 分钟一键启动**：`bash setup.sh`，零学习曲线
- **资源占用小**：< 500 MB 内存，小 VPS 可跑
- **开箱即用的 Admin**：不用装 PgAdmin、不用装 Studio、不用学 psql
- **标准兼容**：与 supabase-js 客户端无缝对接，后面想迁移到 Pigsty 或官方 self-host 零成本

**不是什么**（反向定义）：
- ❌ 不是"完整 Supabase 克隆"（那是 Pigsty / 官方 self-host 的工作）
- ❌ 不是"生产级高可用数据库平台"（那是 KubeBlocks 的工作）
- ❌ 不是"通用 Docker 管理面板"（那是 Portainer 的工作）
- ❌ 不是"多租户 BaaS 平台"（那是 hosted Supabase 的工作）

### 2.2 目标用户画像

| 场景 | 典型用户 | 诉求 |
|------|---------|------|
| **侧项目 / Side project** | 独立开发者 | 快速有一个带 auth + DB 的后端 |
| **学习 / 教程** | 初学 Supabase | 不想注册云服务，本地跑通 |
| **Hackathon** | 团队开发者 | 一键起一个 Supabase 环境 |
| **本地开发 / CI 测试** | 中小团队 | 不依赖云，本地能跑所有集成测试 |
| **简单 SaaS 原型** | 独立创业者 | 0→1 阶段的轻量后端 |
| **内部工具** | 公司内部系统 | 不需要 Realtime / Storage 的简单 CRUD |

**明确不是我们的用户**：
- 需要 HA / 主从 / 灾备的生产系统（→ Pigsty / 云厂商 RDS）
- 需要 Realtime 订阅的 IM / 协作类产品（→ 官方 Supabase / Pigsty）
- 需要 Storage 存文件/图片/视频（→ 官方 Supabase / Pigsty + MinIO）
- 大型团队 / 多租户 / RBAC（→ Portainer / KubeBlocks）

---

## 三、功能取舍（四家调研合流后的最终决策）

### ✅ 核心保留（不可妥协）

| 模块 | 为什么不能砍 |
|------|-------------|
| Postgres | 一切的基础 |
| PostgREST | Supabase 的核心价值是"自动 REST API" |
| GoTrue | Auth 是 BaaS 的另一核心 |
| Caddy 网关 | API key 校验 / CORS / 统一入口 |
| 自研 Admin | **这是差异化的核心武器**（见下节） |

### 🔥 必加（P0，四家共识或 Pigsty 规范对齐）

**A. 对齐 Supabase 规范**（Pigsty）—— 让用户的 supabase-js 代码能直接跑：

1. **补齐 7 个 Supabase PG 角色** —— 即使不用也要有，兼容性问题
2. **补齐 8 个核心扩展**：pg_graphql / pgvector / pg_cron / supabase_vault / pgsodium / http / pg_net / pg_jsonschema
3. **补齐 6 个预留 schema**
4. **收紧 pg_hba.conf**（默认 trust 不能上生产）

**B. 实时 UX 基建**（Dockge + Portainer）—— 一次铺好底座：

5. Cookie auth（为 SSE/WS 铺路）
6. SSE 日志流 + RingBuffer
7. SSE 状态推送
8. operation_id + SSE 重启进度流
9. Auth 端点 rate limit

**C. 数据库指标**（Pigsty 免费午餐）：

10. **postgres-exporter sidecar + Pigsty 14 个 Grafana 面板**（JSON 直接拷贝，几小时落地）

### 💎 强烈推荐（P1）

**D. 数据库专属管理**（KubeBlocks）—— 差异化优势：

11. **SystemAccount 统一视图** —— 所有 DB 角色/密钥/密码一页展示，支持一键旋转
12. **定时备份**（pg_dump + cron + 保留策略）
13. **Parameter 动/静态标记** —— Settings 字段注明"改这个要重启"
14. **Maintenance 一键按钮**（VACUUM / REINDEX / ANALYZE）
15. 慢查询面板（pg_stat_statements）

**E. 工程规范化**（Portainer）—— 代码质量：

16. 中间件链重构（Bouncer 模式）
17. CSRF 保护（随 cookie 引入）
18. React Query 三层重构（新功能用）

### 🌟 增值（P2，可选）

19. Auto-tuning 参数（按容器 mem/cpu）— Pigsty
20. pgBackRest + PITR（进阶用户开关）— Pigsty
21. dbrole_* 分层角色体系 — Pigsty

### ⛔ 明确放弃（决策已定）

| 功能 | 放弃理由 | 替代建议 |
|------|---------|---------|
| Realtime 服务 | 超出"简单后端"定位，复杂度陡增 | 用户需要时推荐 Pigsty / 官方 self-host |
| Storage API + MinIO | 对象存储加两个服务 | 同上 |
| Edge Functions | 小众需求 | 用户自己跑 Deno / Cloudflare Workers |
| 官方 Studio | 我们有自研 admin | — |
| Analytics / Logflare | 日志查询用 admin 面板足够 | — |
| HA / Patroni / Etcd | 超出"minimal"定位 | 推荐 Pigsty |
| 多主机 / Agent | 单机场景 | 推荐 Portainer |
| Compose 文件编辑 | 我们是固定 stack | — |
| 交互终端（PTY / xterm） | 用 SQL Runner 足够 | 推荐 Portainer |
| 多用户 / Teams / RBAC | 单 admin 场景 | — |

---

## 四、差异化武器：自研 Admin 的定位

自研 Admin 面板在加入 Studio 后，**聚焦成"DBA 运维驾驶舱"**——不再和 Studio 竞争开发者日常功能，专做 Studio 没有的运维面：

| 能力 | Studio | supabase-lite Admin |
|------|:------:|:-------------------:|
| **日常开发** | | |
| 表结构浏览 / 编辑 | ✅ | ❌（移除） |
| SQL Editor | ✅ | ❌（移除，彻底放弃） |
| RLS 策略可视化 | ✅ | ❌（移除） |
| 扩展管理 | ✅ | ❌（移除） |
| auth.users 管理 | ✅ | ❌（移除） |
| API Docs 自动生成 | ✅ | ❌ |
| **运维 / DBA** | | |
| SystemAccount 统一视图 + 密码旋转 | ❌ | ✅ 独占 |
| VACUUM / REINDEX / ANALYZE 一键 | ❌ | ✅ 独占 |
| 活跃连接管理 / session kill / 锁分析 | ❌ | ✅ 独占 |
| 长事务告警 | ❌ | ✅ 独占 |
| 慢查询 Top N | ⚠️ 基础 | ✅ 专门面板 |
| 参数动/静态标记 + pg_hba 模板 | ❌ | ✅ 独占 |
| 定时备份 + S3 推送 | ❌ | ✅ 独占 |
| 服务日志实时流（SSE） | ⚠️ 基础 | ✅ 更强 |
| 服务状态推送 + 重启进度 | ❌ | ✅ 独占 |
| 设置向导 / OAuth / SMTP | ❌ | ✅ 独占 |

**口号**：Studio 管"应用开发"；自研 Admin 管"数据库运维"。两者并行，侧边栏互相切换。

---

## 五、版本路线图

> 本路线图和 `docs/research/README.md` 的 Phase 分解对齐。每个版本由一到两个 Phase 组成。

### v0.5 "对齐 Supabase 规范 + 加入 Studio"（约 3 周）

**Phase 1 — Supabase 规范对齐**（约 1 周）
- [ ] 升级 `supabase/postgres` 到 15.8.1.085
- [ ] 升级 PostgREST 到 v14.8、GoTrue 到 v2.186.0
- [ ] 重写 `00-roles.sh` 为 `99-supabase-lite-custom.sh`（不重复建角色，只补镜像没做的）
- [ ] 启用 8 个核心扩展（pg_graphql / pgvector / pg_cron / supabase_vault / pgsodium / pg_net / http / pg_jsonschema）
- [ ] 建 `_realtime` / `supabase_functions` schema（镜像没建的）
- [ ] 覆盖 auth.uid() / role() / email() 为新 GUC 模式
- [ ] GoTrue 改用 `supabase_auth_admin` 连接
- [ ] PGRST expose `public,graphql_public`
- [ ] 恢复 `GRANT supabase_admin TO authenticator`（见 ADR-001）
- [ ] 简化 `.env`：角色密码统一用 `POSTGRES_PASSWORD`

**Phase 2 — Studio 集成 + Cookie auth**（约 2 周）
- [ ] 新增 `studio` 和 `postgres-meta` 容器（profile: studio）
- [ ] Cookie auth（HMAC 完整版，一步到位）
- [ ] Caddy 路由：`/studio/*` / `/graphql/v1` / `/.well-known/oauth-authorization-server`
- [ ] Studio feature flag 禁用 Storage/Edge/Realtime 菜单
- [ ] 首页选择页（两个入口按钮）
- [ ] setup.sh 生成 `PG_META_CRYPTO_KEY`、写 `COMPOSE_PROFILES=studio`
- [ ] Auth 端点 rate limit（Phase 4 提前的小功能，和 cookie 一起做）

**里程碑**：
- `supabase-js` 测试通过（REST / Auth / RPC / GraphQL）
- Studio 可访问，Storage/Edge/Realtime 菜单隐藏
- 自研 admin 和 Studio 并存，用 cookie 统一登录

### v0.6 "自研 Admin 重定位 + 实时体验"（约 3 周）

**Phase 3 — 自研 Admin 瘦身**（约 1 周）
- [ ] **删除自研 SQL Editor 前端**（用户写 SQL 去 Studio）
- [ ] 删除表浏览器（Studio 覆盖）
- [ ] 删除 auth.users 业务用户管理（Studio 覆盖）
- [ ] 删除扩展管理 UI（Studio 覆盖）
- [ ] 删除 RLS 策略模板（Studio 覆盖）
- [ ] 重组侧边栏：Dashboard / Logs / DB Ops / DB Roles / DB Backups / DB Performance / Settings

**Phase 4 — SSE 实时基建**（约 2 周）
- [ ] SSE 日志流 + RingBuffer（LogHub fan-out）
- [ ] SSE 服务状态推送（3 秒后端 poll + 差异广播）
- [ ] operation_id + SSE 重启进度流（解决 gateway 重启时的响应中断）
- [ ] 日志增强：关键词过滤 / 级别高亮 / 下载 .log / 自动滚动
- [ ] 前端 `subscribeLogs` / `subscribeStatus` hooks

**里程碑**：
- 日志实时滚动（不用手动刷新）
- 服务状态秒级同步
- Restart 操作能看到实时输出和最终状态

### v0.7 "DBA 驾驶舱 + 备份"（约 4 周）

**Phase 5 — DBA 驾驶舱**（约 2 周）
- [ ] SystemAccount 视图（所有 Supabase 角色 + 一键密码旋转）
- [ ] VACUUM / REINDEX / ANALYZE 按表按钮
- [ ] 活跃连接列表 + cancel / terminate
- [ ] 锁分析（pg_blocking_pids）
- [ ] 长事务告警（Dashboard 顶部红条）
- [ ] 慢查询 Top N（pg_stat_statements）
- [ ] 参数管理（标注动态/静态）
- [ ] 数据库 / 表大小 Top N

**Phase 6 — 备份 + 外部 S3**（约 2 周）
- [ ] 定时 pg_dump（cron + 保留策略）
- [ ] 推送到外部 S3（minio-go + 10 家 provider preset）
- [ ] 国内云兼容实测（阿里云 OSS / 腾讯云 COS / 七牛）
- [ ] pgBackRest 增量（可选 profile，独立 override compose 文件）
- [ ] 加密 secret store（AES-256-GCM，独立 `SECRET_ENCRYPTION_KEY`）
- [ ] 恢复流程 + UI

**里程碑**：
- 用户可以一键 VACUUM 任意表、kill 问题连接、看慢查询
- 定时备份自动推到 S3，恢复可一键

### v1.0 "工程化 + DX"（约 3 周）

**Phase 7 — 开发者体验**（约 1 周）
- [ ] Setup Wizard 重设计成 6 步（加 OAuth / S3 配置）
- [ ] SMTP 测试发送按钮
- [ ] OAuth 配置 ping 校验
- [ ] Caddy auto_https
- [ ] 导出 / 导入 admin 配置
- [ ] 新版本自动提醒
- [ ] 安装报告 .md

**Phase 8 — 前后端工程化**（约 2 周）
- [ ] Go Bouncer 中间件链
- [ ] Context 注入中间件
- [ ] 构造器 DI
- [ ] React Query 三层（service / query / keys）
- [ ] CSRF 保护（配套 cookie）
- [ ] 结构化日志 zerolog
- [ ] Admin 操作历史
- [ ] 配置变更历史

**里程碑**：生产可用质量；文档 + 示例完善；欢迎外部贡献者。

---

## 六、运营建议（非技术层面）

### 6.1 命名与品牌

目前叫 `supabase-lite`，定位为"minimal Supabase"，**建议保留这个名字**——清晰传达定位，SEO 友好。

考虑增加 tagline：
> **"Self-hosted Supabase in under 500MB. For side projects, not datacenters."**

### 6.2 文档核心内容

1. **Getting Started**（5 分钟跑通，对标 Supabase CLI）
2. **对比表**：supabase-lite vs 官方 self-host vs Pigsty vs hosted Supabase —— 帮用户选择
3. **Migration guide**：从 supabase-lite 迁移到官方 self-host / Pigsty 的路径（强调"零成本升级"）
4. **Extension matrix**：我们启用了哪些扩展，官方还有哪些（用户可自行添加）

### 6.3 和其他项目的关系

- **定位为 Pigsty 的"入门版"**：官网加一行"需要生产级 HA？试试 Pigsty"
- **官方 Supabase 的"本地开发/侧项目替代"**：用户需要 Realtime/Storage 时，引导到官方
- **不直接竞争**，作为生态的补充

### 6.4 不做的事

- ❌ 不卖 SaaS（和 hosted Supabase 竞争）
- ❌ 不做企业版（和 Pigsty / KubeBlocks 竞争）
- ❌ 不做多租户（复杂度失控）
- ❌ 不追新的数据库（Redis / MongoDB 等）—— 专注 Postgres

---

## 七、优先级决策矩阵

用户经常会纠结"下一步做什么"。下面是**简单决策规则**：

```
是否对齐 Supabase 规范?  ← 如果 "否"，一切免谈（用户代码跑不起来）
  ↓ 是
  ├─ 是否对 MVP 用户（侧项目开发者）直接可见? 
  │   ├─ 是 → P0 立刻做
  │   └─ 否 → 进入下一问
  │
  是否影响未来功能的基座?
  │   ├─ 是（如 Cookie auth）→ P0
  │   └─ 否 → 进入下一问
  │
  是否差异化武器（自研 admin 的特色）?
  │   ├─ 是 → P1 做
  │   └─ 否 → 进入下一问
  │
  是否是"生产级要求"?
  │   ├─ 是（HA / PITR / 多租户）→ 明确放弃，推荐竞品
  │   └─ 否 → P2 或放弃
```

---

## 八、结论与下一步

### 三个一句话

1. **定位**："开发者友好的、极简的、自托管 Supabase"——不和 Pigsty 竞争全功能，占领"轻量 + 一键"的空缺
2. **武器**：自研 Admin 面板做成"DBA + 开发者双视角的 Postgres 驾驶舱"，不是 Studio 的克隆
3. **边界**：放弃 Realtime / Storage / Edge / HA / 多租户，明确指引用户到 Pigsty / 官方 self-host

### 立刻要做的事（按顺序）

1. **v0.5 Sprint**（2 周）：补齐 Pigsty 发现的 Supabase 规范差距 + postgres-exporter + Grafana 面板
2. **写一份 README 对比表**：明确告诉潜在用户"什么时候选我们，什么时候选 Pigsty"
3. **评估镜像大小**：看 8 个扩展加进来后 Docker 镜像有多大，必要时分 "base" / "full" 两个 tag

---

## 附：如果用户坚持要 Realtime / Storage 怎么办？

这是产品决策点。建议策略：

**策略 A（推荐）**：不做，指引到官方/Pigsty
- 优点：聚焦、简单、易维护
- 缺点：流失一部分需要这些的用户

**策略 B**：做成可选 profile（`docker-compose.override.realtime.yml`）
- 优点：保持 minimal 默认，需要的用户可加
- 缺点：维护成本翻倍

**策略 C**：全部做，追平官方 self-host
- 优点：功能全
- 缺点：和官方/Pigsty 重复造轮子，定位模糊

**我的建议**：**策略 A**。supabase-lite 的核心价值就是"轻"，为了功能全失去这个特点得不偿失。如果未来真的有很多用户需要 Realtime，可以考虑**单独开一个 `supabase-mid` 项目**作为中间层，保持 supabase-lite 的极简定位。

---

**这份建议的核心判断**：supabase-lite 不需要追赶 Pigsty / 官方 Supabase，而要在"轻量 + 一键 + 开发者友好 + Postgres 运维能力"这四个维度上做到最好。四家调研印证了：这个位置确实空，而且我们已经走在正确路上。
