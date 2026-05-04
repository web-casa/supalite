# ADR-001：撤回对 `GRANT supabase_admin TO authenticator` 的移除

> **状态**：已决策（2026-04-17）
> **关联**：Phase 1 实施；`volumes/db/init/00-roles.sh`
> **影响面**：数据库角色体系 / Supabase 规范兼容性

---

## 背景

在第一轮 Codex 自动 review（2026-04-16）中，Codex 把以下行标为 **CRITICAL 级安全漏洞**：

```sql
-- volumes/db/init/00-roles.sh
GRANT supabase_admin TO authenticator;
```

Codex 的理由是：

> `supabase_admin` 是 SUPERUSER。`authenticator` 是 PostgREST 连接角色。如果 JWT 带 `role=supabase_admin`，PostgREST 会 `SET ROLE supabase_admin`，任何调用 REST API 的人都能拿到超级用户权限。

基于此，第一轮修复**删除了这一行 grant**，并改成：

```sql
-- NOTE: supabase_admin is NOT granted to authenticator to prevent
-- privilege escalation via PostgREST JWT with role=supabase_admin
```

## 问题

**Phase 1 实施前的兼容性调研**（见 `docs/pg-supabase-compatibility.md`）发现：

1. **官方 Supabase 就是这么设计的**
   - 源码：`supabase/postgres/migrations/db/init-scripts/00000000000000-initial-schema.sql:37`
     ```sql
     GRANT anon              TO authenticator;
     GRANT authenticated     TO authenticator;
     GRANT service_role      TO authenticator;
     GRANT supabase_admin    TO authenticator;  -- ← 就在这里
     ```
   - 所有 supabase/postgres 官方镜像启动时自动执行此 grant
   - 我们上游镜像是 `supabase/postgres:15.8.1.085`，这一 grant 在**镜像 init 阶段就已存在**，我们的 `00-roles.sh` 即使不加也会被镜像建好

2. **`NOINHERIT` 约束已经阻止了"自动提权"**
   - `authenticator` 角色创建时带 `NOINHERIT`
   - NOINHERIT 意味着：authenticator **不会自动继承** supabase_admin 的权限
   - 只有在显式 `SET ROLE supabase_admin` 后才获得权限

3. **攻击者要伪造 JWT 需要 `JWT_SECRET`**
   - `role=supabase_admin` 的 JWT 必须用我们的 JWT_SECRET 签名
   - 能拿到 JWT_SECRET 的攻击者，也能拿到 SERVICE_ROLE_KEY（同一个 secret 签的）
   - `service_role` 本身带 `BYPASSRLS`，已经能读写任何数据
   - `supabase_admin` 相对 `service_role` 的增量权限是：文件系统操作、C 扩展、角色管理——**这是锦上添花的风险，不是独立的攻击面**

4. **PostgREST 层面可以白名单控制**
   - 如果严格，可配置 `PGRST_DB_ALLOW_ROLES=anon,authenticated,service_role`
   - 这是比"切断 grant 链"更正确的防御方式——在入口层阻止而不是在权限模型层

## 结论

Codex 第一轮的判断**过度保守**。真实情况：
- "可以 SET ROLE supabase_admin"的前提是 JWT_SECRET 已泄露
- JWT_SECRET 已泄露 = 系统已经完全失陷（service_role 等价）
- 切断 grant 破坏了 Supabase 官方兼容性（Studio / supabase-js 某些高级场景可能依赖）

## 决策

**Phase 1 明确恢复 `GRANT supabase_admin TO authenticator`**。

具体做法：**不在 `00-roles.sh` 里自己建角色**（镜像已建），让镜像的 `00000000000000-initial-schema.sql` 自然执行。`00-roles.sh` 改名 `99-supabase-lite-custom.sh`，只做镜像没做的补充（见 Phase 1 实施清单）。

## 追加防御（与 grant 无关的安全加固）

虽然恢复 grant，但我们保留以下独立的安全措施：

1. **JWT_SECRET 必须严格保护**
   - `.env` 权限 600（setup.sh 已做）
   - 不写入任何日志
   - 考虑后续支持 JWT_KEYS（非对称）降低对称密钥风险

2. **数据库端口默认绑 127.0.0.1**（`POSTGRES_BIND_ADDR=127.0.0.1`）
   - 已实施（见 docker-compose.yml）
   - 外网 JWT 伪造者连不到 DB

3. **考虑启用 PGRST_DB_ALLOW_ROLES 白名单**（Phase 5 可选）
   - 只允许 `anon,authenticated,service_role` 三个角色
   - 明确禁止 supabase_admin / dashboard_user 等通过 REST 被 SET ROLE
   - 这是业务层限制，不影响数据库角色模型

## 参考

- `docs/pg-supabase-compatibility.md` §2.3（Codex 误判详细分析）
- 上游：https://github.com/supabase/postgres/blob/develop/migrations/db/init-scripts/00000000000000-initial-schema.sql
- PostgreSQL docs: [Role Membership](https://www.postgresql.org/docs/current/role-membership.html)（NOINHERIT 行为）

## 历史

| 日期 | 变更 |
|------|------|
| 2026-04-16 | Codex 第一轮 review 后删除 grant |
| 2026-04-17 | Phase 1 调研发现官方做法，决定撤回，保留官方行为 |
