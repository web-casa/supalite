# PostgreSQL / Supabase 兼容性笔记

> 开发 Phase 1 前的核心依据文档
> 基础来源：`supabase/postgres` 镜像源码（~/otheruse/supabase-postgres）、Supabase 官方 self-host（~/otheruse/supabase-official/docker）、Pigsty（~/otheruse/pigsty/conf/supabase.yml）
> 日期：2026-04-17
> **本文档用于指导 supabase-lite Phase 1 的实施，不是用户文档**

---

## 0. 核心结论速查

| 问题 | 答案 |
|------|------|
| 我们用什么 PG 镜像？ | **`supabase/postgres:15.8.1.085`**（跟官方 self-host 对齐最新版）|
| 镜像会不会自动建 Supabase 规范角色？ | **会建一部分**（见下表），剩余由我们的 init SQL 补 |
| 镜像会不会自动建 schema？ | **会建 auth / storage / extensions**，**不会建** realtime / graphql_public / supabase_functions / _realtime / _analytics |
| 镜像会不会自动启用扩展？ | **uuid-ossp / pgcrypto / pg_stat_statements** 自动启；其他扩展**可用但未启用**，需要 CREATE EXTENSION |
| `auth.uid()` / `auth.role()` / `auth.email()` 谁建？ | 镜像的 init script（`00000000000001-auth-schema.sql`）自动建 |
| authenticator 能不能切到 supabase_admin？ | **Supabase 官方允许**（ `GRANT supabase_admin TO authenticator`）。我们曾经为安全移除此 grant，但这破坏了兼容性，**必须恢复** |
| pg_hba.conf 官方怎么写？ | 见 §7，本质是 "localhost trust + 其他网段 scram-sha-256" |

---

## 1. 使用的基础镜像

### 镜像 & 版本

```yaml
# docker-compose.yml
db:
  image: supabase/postgres:15.8.1.085   # 对齐官方 self-host 2026-04 版本
```

**镜像关键特性**（源自 Dockerfile-15 分析）：
- 基础 OS：Alpine 3.23
- PostgreSQL：15.x（Nix 构建）
- 默认用户：`POSTGRES_USER=supabase_admin`（不是 `postgres`）
- 默认数据库：`POSTGRES_DB=postgres`
- 预装扩展：见 §4
- `supautils` 扩展强制启用（保留角色保护）

### 镜像初始化顺序

容器首次启动时，依次执行 `/docker-entrypoint-initdb.d/` 下的文件：

```
1. init-scripts/00000000000000-initial-schema.sql  ← 建基础角色 + extensions schema
2. init-scripts/00000000000001-auth-schema.sql     ← 建 auth schema + auth.users 等
3. init-scripts/00000000000002-storage-schema.sql  ← 建 storage schema + supabase_storage_admin
4. init-scripts/00000000000003-post-setup.sql      ← 建 dashboard_user + pg_cron/pg_net 事件触发器
5. init-scripts/00-schema.sql (pgbouncer schema)
6. migrations/00-extension.sql                      ← CREATE EXTENSION pg_stat_statements
7. migrations/ 下 49 个 SQL 文件                    ← 历次修正和增强
```

**重要**：第 1 步就已经建了很多东西，**我们自己的 `00-roles.sh` 必须避免冲突**。现在的实现会和官方冲突，需要重写（见 §9）。

### 关键文件
- `Dockerfile-15` — 镜像构建
- `migrations/db/init-scripts/` — 首次初始化 SQL（4 个文件）
- `migrations/db/migrations/` — 历次增量 SQL（49 个）
- `ansible/files/postgresql_config/supautils.conf.j2` — supautils 保留角色配置

### supautils 的角色保留机制

镜像启用了 `supautils` 扩展，它**在数据库层面强制禁止修改以下角色**：

```
保留角色（不可删除 / 不可改密）:
  supabase_admin, supabase_auth_admin, supabase_storage_admin,
  supabase_read_only_user, supabase_realtime_admin,
  supabase_replication_admin, supabase_etl_admin,
  dashboard_user, pgbouncer, service_role*, authenticator*,
  authenticated*, anon*

保留角色成员关系（不可 GRANT 给其他角色 / 不可 REVOKE 这些角色）:
  pg_read_server_files, pg_write_server_files, pg_execute_server_program,
  supabase_admin, supabase_auth_admin, supabase_storage_admin,
  supabase_read_only_user, supabase_realtime_admin,
  supabase_replication_admin, supabase_etl_admin,
  dashboard_user, pgbouncer, authenticator
```

**影响我们**：SystemAccount "密码旋转"对这些保留角色受限。具体方式：
- 以 `supabase_admin` 身份连接
- 使用 `ALTER USER xxx WITH PASSWORD '...'`
- 这是 supautils 允许的操作

---

## 2. Supabase 规范角色（13 个，不是之前以为的 12）

### 2.1 完整角色清单（镜像预建 vs 我们需补齐）

| # | 角色 | 镜像自动建？ | 建在哪 | 用途 |
|---|------|:------------:|--------|------|
| 1 | **`supabase_admin`** | ✅ 内置（镜像的主 POSTGRES_USER） | — | SUPERUSER，拥有所有 schema |
| 2 | **`postgres`** | ✅ PG 原生 | — | 常规 admin，**非** SUPERUSER（supautils 会 revoke） |
| 3 | **`anon`** | ✅ initial-schema.sql | 镜像 init | NOLOGIN NOINHERIT，匿名访问 |
| 4 | **`authenticated`** | ✅ initial-schema.sql | 镜像 init | NOLOGIN NOINHERIT，已登录用户 |
| 5 | **`service_role`** | ✅ initial-schema.sql | 镜像 init | NOLOGIN NOINHERIT **BYPASSRLS**，后端服务 |
| 6 | **`authenticator`** | ✅ initial-schema.sql | 镜像 init | LOGIN NOINHERIT，PostgREST 连接主体 |
| 7 | **`dashboard_user`** | ✅ post-setup.sql | 镜像 init | Dashboard/Studio 使用，CREATEDB CREATEROLE REPLICATION |
| 8 | **`supabase_auth_admin`** | ✅ auth-schema.sql | 镜像 init | GoTrue 连接用，owns auth schema |
| 9 | **`supabase_storage_admin`** | ✅ storage-schema.sql | 镜像 init | Storage API 连接用 |
| 10 | **`supabase_functions_admin`** | ✅ post-setup.sql / 官方 webhooks.sql | 镜像 init 或补 | Edge Functions webhook |
| 11 | **`supabase_replication_admin`** | ✅ initial-schema.sql | 镜像 init | 复制专用 |
| 12 | **`supabase_etl_admin`** | ✅ initial-schema.sql | 镜像 init | ETL，有 pg_read_all_data + BYPASSRLS |
| 13 | **`supabase_read_only_user`** | ✅ initial-schema.sql | 镜像 init | 只读用户，BYPASSRLS |
| 14 | **`pgbouncer`** | ✅ pgbouncer_auth_schema.sql | 镜像 init | 连接池，支持 auth query |
| 15 | **`supabase_realtime_admin`** | ❌（supautils 保留但不自动建） | 需手工 | Realtime 服务用（我们不装 Realtime，**可跳过**） |

**核心发现**：**除了 `supabase_realtime_admin`，其余 14 个角色镜像都会自动建**。

**对 supabase-lite 的意义**：
- ❌ 我们当前 `00-roles.sh` 里 `CREATE ROLE supabase_admin ... CREATE ROLE anon ...` **全部重复**，会和镜像冲突
- ✅ **正确做法**：删除我们的 `00-roles.sh` 里所有 `CREATE ROLE`，改为使用 `IF NOT EXISTS` 或直接删除，**信任镜像**
- ✅ 只保留我们特有的：`auth.uid()`、`auth.role()`、`auth.email()`（其实镜像也建了，我们删掉即可）

### 2.2 关键关系图

```
supabase_admin (SUPERUSER)
  │
  ├─ 拥有所有 schema
  │
authenticator (NOINHERIT, LOGIN)
  │  ↓ PostgREST 连接这个角色
  ├─ GRANT anon           TO authenticator
  ├─ GRANT authenticated  TO authenticator
  ├─ GRANT service_role   TO authenticator
  └─ GRANT supabase_admin TO authenticator  ← **官方如此** (我们之前被 Codex 误导删了)
```

### 2.3 ⚠️ Codex 第一轮建议的"privilege escalation"其实是误判

**背景**：第一轮 Codex review 指出 `GRANT supabase_admin TO authenticator` 是安全漏洞，允许任何带 `role=supabase_admin` 的 JWT 获得超级用户权限。我们据此移除了这个 grant。

**真相**：
1. **Supabase 官方就是这么设计的**（见 `initial-schema.sql:37`）
2. **NOINHERIT** 约束：authenticator **不会自动继承** supabase_admin 权限
3. 攻击者要获得 supabase_admin 身份必须伪造一个 `role=supabase_admin` 的 JWT，这需要 `JWT_SECRET`
4. 有 `JWT_SECRET` 的人**已经拥有 SERVICE_ROLE_KEY**（绕过 RLS 的完整访问），再加上 supabase_admin 的 SUPERUSER 权限只是"锦上添花"
5. 唯一增量风险：`supabase_admin` 能执行 `COPY FROM PROGRAM`、创建 C 扩展等文件系统级操作
6. 如果担心，用 PostgREST 的 `db-allow-role` 白名单，**而不是**切断 grant 链

**结论**：**Phase 1 要恢复** `GRANT supabase_admin TO authenticator`，以保持 Supabase 规范兼容。

### 2.4 关键默认配置

```sql
-- 镜像的 initial-schema.sql 最后几行
ALTER ROLE anon          SET statement_timeout = '3s';
ALTER ROLE authenticated SET statement_timeout = '8s';
ALTER ROLE authenticator SET session_preload_libraries = 'safeupdate';  -- 后续迁移加的
ALTER ROLE authenticator SET lock_timeout = ...ms;                       -- 后续迁移加的
ALTER USER supabase_admin SET search_path TO "\$user",public,auth,extensions;
ALTER ROLE postgres      SET search_path TO "\$user",public,extensions;
```

---

## 3. Supabase 规范 Schema

### 3.1 完整 schema 清单

| Schema | 镜像自动建？ | 何时建 | 用途 |
|--------|:-----------:|--------|------|
| `public` | ✅ PG 原生 | — | 业务表 |
| `extensions` | ✅ initial-schema.sql | 镜像首次启动 | 扩展寄居地（uuid-ossp / pgcrypto 等） |
| `auth` | ✅ auth-schema.sql | 镜像首次启动 | GoTrue 用户 / 会话 |
| `storage` | ✅ storage-schema.sql | 镜像首次启动 | Storage API 元数据 |
| `realtime` | ✅ 后续迁移 `20211118015519_create-realtime-schema.sql` | 镜像首次启动 | Realtime metadata |
| `_realtime` | ❌ | 需 realtime.sql 启动时 run | Realtime 内部 |
| `_analytics` | ❌ | Logflare 首次启动 migrate 建 | Analytics 表（我们不装 Logflare，**不需要**）|
| `supabase_functions` | ❌ | webhooks.sql 首次启动 run | Edge Functions webhooks（我们不装 Edge Functions，**不需要**）|
| `graphql_public` | ❌ | pg_graphql 启用时自动建 | pg_graphql 暴露点 |
| `vault` | ❌ | supabase_vault 启用时建 | Vault 密钥管理 |
| `net` | ❌ | pg_net 启用时建 | pg_net HTTP 客户端 |
| `cron` | ❌ | pg_cron 启用时建 | pg_cron 任务表 |
| `pgsodium` | ❌ | pgsodium 启用时建 | pgsodium |
| `pgsodium_masks` | ❌ | pgsodium 启用时建 | pgsodium 视图 |
| `pgbouncer` | ✅ 0-schema.sql | 镜像 init | pgbouncer auth query |
| `_supavisor` | ❌（在 _supabase 数据库中） | 官方 pooler.sql | Supavisor metadata（我们不装 Supavisor，**不需要**）|

**对 supabase-lite Phase 1 的意义**：
- ✅ **auth / storage / extensions / realtime / pgbouncer 镜像自动创建**
- ❌ **_realtime / supabase_functions / graphql_public 我们按需补**
  - `_realtime` — 如果以后启用 Realtime，需要 `CREATE SCHEMA _realtime AUTHORIZATION supabase_admin`
  - `supabase_functions` — 如果以后启用 Edge Functions
  - `graphql_public` — `CREATE EXTENSION pg_graphql` 时会自动建
- 我们不装 Realtime / Storage / Edge Functions / Analytics，但**预留 schema 还是要建**（13.3 里说的 6 个），以保证官方镜像的兼容性声明成立

### 3.2 Phase 1 的 schema 任务

```sql
-- 我们的 init SQL 需要补的（镜像没自动建的）
CREATE SCHEMA IF NOT EXISTS _realtime AUTHORIZATION supabase_admin;
CREATE SCHEMA IF NOT EXISTS supabase_functions AUTHORIZATION supabase_admin;
-- graphql_public 由 CREATE EXTENSION pg_graphql 自动建，不用手动
-- _analytics 我们不需要（不装 Logflare）
```

---

## 4. Supabase 扩展（15+）

### 4.1 镜像预装扩展清单

镜像已经**打包**（可用但未启用）以下扩展（来自 supautils.conf 的 privileged_extensions 列表）：

```
自动启用（CREATE EXTENSION 在 init 时已跑）:
  - uuid-ossp
  - pgcrypto
  - pg_stat_statements

可用但默认未启用:
  - pg_graphql          ← 我们要启用
  - pg_jsonschema       ← 我们要启用
  - pg_net              ← 我们要启用
  - pgjwt               ← 我们要启用（其实 PostgREST v12+ 不再需要）
  - pgsodium            ← supabase_vault 依赖
  - supabase_vault      ← 我们要启用
  - pg_cron             ← 我们要启用
  - pg_tle              ← 我们要启用
  - vector (pgvector)   ← 我们要启用
  - pgmq                ← 我们要启用
  - timescaledb         ← 我们要启用
  - http                ← 我们要启用
  - wrappers            ← 我们要启用

其他可选:
  - postgis, postgis_raster, postgis_topology (GIS)
  - pgaudit, pg_repack, pg_partman, pg_buffercache (运维)
  - plpgsql_check, plv8, plcoffee, pljava (语言)
  - pgroonga, pgrouting, rum (搜索/路由)
  - fuzzystrmatch, citext, hstore, tablefunc, intarray, btree_gin (工具)
  - ...共 50+
```

### 4.2 Phase 1 需要启用的扩展（8 个）

```sql
-- 按依赖顺序
CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA extensions;  -- 已启
CREATE EXTENSION IF NOT EXISTS pgcrypto    WITH SCHEMA extensions;  -- 已启

CREATE EXTENSION IF NOT EXISTS pgjwt           WITH SCHEMA extensions;
CREATE EXTENSION IF NOT EXISTS http            WITH SCHEMA extensions;
CREATE EXTENSION IF NOT EXISTS pg_net;                                -- 固定 schema: net
CREATE EXTENSION IF NOT EXISTS pg_jsonschema   WITH SCHEMA extensions;
CREATE EXTENSION IF NOT EXISTS pg_graphql;                            -- 固定 schema: graphql
CREATE EXTENSION IF NOT EXISTS vector          WITH SCHEMA extensions; -- pgvector

-- 依赖顺序：pgsodium 必须在 supabase_vault 之前
CREATE EXTENSION IF NOT EXISTS pgsodium;                              -- 固定 schema: pgsodium
CREATE EXTENSION IF NOT EXISTS supabase_vault;                        -- 固定 schema: vault

CREATE EXTENSION IF NOT EXISTS pg_cron;                               -- 固定 schema: cron (由 supautils 强制)
```

### 4.3 扩展的 schema 约定

| 扩展 | 默认 schema | 可否改 |
|------|-------------|-------|
| uuid-ossp | extensions | 可 |
| pgcrypto | extensions | 可 |
| pgjwt | extensions | 可 |
| pg_net | **net**（固定） | 否 |
| pg_cron | **cron**（supautils 强制） | 否 |
| pg_graphql | **graphql**（固定） | 否 |
| vector | extensions | 可 |
| pgsodium | **pgsodium**（固定） | 否 |
| supabase_vault | **vault**（固定） | 否 |
| pg_jsonschema | extensions | 可 |
| http | extensions | 可 |
| pg_stat_statements | extensions（镜像已 init） | — |

### 4.4 启用扩展的特殊注意

**pg_net** 启用后镜像的 event trigger（post-setup.sql 定义）会自动 GRANT 权限给各角色。

**supabase_vault** 启用时：
```sql
-- 镜像的 stat_extension.sql 会自动执行：
grant usage on schema vault to postgres with grant option;
grant select, delete, truncate, references on vault.secrets, vault.decrypted_secrets to postgres with grant option;
grant execute on function vault.create_secret, vault.update_secret, vault._crypto_aead_det_decrypt to postgres with grant option;
grant usage on schema vault to service_role;
-- ...
```

**pg_cron** 启用：
- 镜像有 event trigger `issue_pg_cron_access`，建 `cron` schema 时自动 GRANT postgres
- cron 任务默认写在 `postgres` 数据库里（不是 `_supabase`）

### 4.5 启用扩展的推荐位置

我们的做法（修改 `volumes/db/init/00-roles.sh`）：

```sh
# 在镜像完成自己的 init 后，追加我们的 SQL
# init-scripts 目录的文件按字母序执行，我们用 01 开头确保在镜像 init 之后

# 文件命名：01-supabase-lite-extensions.sh
```

或者更简单：使用 migrations 目录：

```
volumes/db/migrations/supabase-lite-<timestamp>_enable_extensions.sql
```

镜像的 `dbmate` 工具会扫描该目录。但这增加了对镜像内部工具的依赖。

**推荐方案**：用我们自己的 `docker-entrypoint-initdb.d/99-*.sh`，在镜像 init 完成后追加执行。

---

## 5. auth.uid() / auth.role() / auth.email()

### 5.1 镜像建的版本（auth-schema.sql:94-106）

```sql
create or replace function auth.uid() returns uuid as $$
  select nullif(current_setting('request.jwt.claim.sub', true), '')::uuid;
$$ language sql stable;

create or replace function auth.role() returns text as $$
  select nullif(current_setting('request.jwt.claim.role', true), '')::text;
$$ language sql stable;

create or replace function auth.email() returns text as $$
  select nullif(current_setting('request.jwt.claim.email', true), '')::text;
$$ language sql stable;
```

### 5.2 我们当前的版本（00-roles.sh 中）

```sql
CREATE OR REPLACE FUNCTION auth.uid()
RETURNS uuid LANGUAGE sql STABLE
AS $func$
  SELECT NULLIF(current_setting('request.jwt.claims', true)::json->>'sub', '')::uuid
$func$;
-- ...
```

### 5.3 差异分析 ⚠️

两种版本读的 GUC 名字**不同**：
- 镜像：`request.jwt.claim.sub` / `request.jwt.claim.role` / `request.jwt.claim.email`（单数 `claim`，每个字段单独 setting）
- 我们：`request.jwt.claims`（复数，整个 JWT 作为 JSON object）→ `::json->>'sub'`

**这是 PostgREST 的两种模式**：
- **旧模式**（< v10）：`request.jwt.claim.xxx` 每个字段一个 GUC
- **新模式**（>= v10）：`request.jwt.claims` 整个 JSON 一个 GUC，由 `PGRST_DB_USE_LEGACY_GUCS` 控制

**docker-compose.yml** 里我们已经设置了 `PGRST_DB_USE_LEGACY_GUCS: "false"`，用的是新模式。

**所以我们的版本才是正确的**（对新版 PostgREST）。镜像里的版本是旧模式、兼容性更差。

### 5.4 Phase 1 的决策

**保留我们的版本**（新模式），覆盖镜像的版本：

```sql
-- 放在 init-scripts/99-supabase-lite.sh 里
CREATE OR REPLACE FUNCTION auth.uid() ...  -- 用 request.jwt.claims
```

`CREATE OR REPLACE` 会覆盖镜像初始化的 auth.uid() 定义。

### 5.5 函数所有者

镜像 `20211124212715_update-auth-owner.sql` 把这三个函数 owner 改为了 `supabase_auth_admin`。我们覆盖时如果直接 `CREATE OR REPLACE`，owner 保持不变（好）。

---

## 6. PostgREST 约定（JWT & role 切换）

### 6.1 连接主体

```
PGRST_DB_URI: postgres://authenticator:${POSTGRES_PASSWORD}@db:5432/postgres
PGRST_DB_ANON_ROLE: anon
PGRST_JWT_SECRET: ${JWT_SECRET}
PGRST_DB_USE_LEGACY_GUCS: "false"
```

### 6.2 请求生命周期

```
1. 客户端请求 → Caddy → PostgREST
2. PostgREST 以 authenticator 身份连 DB
3. 检查 Authorization header 的 JWT（用 JWT_SECRET 验签）
4. 从 JWT 取 role claim：
   - 没有 JWT / 没有 role → SET ROLE anon
   - role=authenticated → SET ROLE authenticated
   - role=service_role → SET ROLE service_role
   - role=supabase_admin → SET ROLE supabase_admin（⚠️ 见 §2.3）
5. 把 JWT claims 写入 request.jwt.claims GUC
6. 执行用户 SQL
7. 返回结果后 RESET ROLE
```

### 6.3 `PGRST_DB_USE_LEGACY_GUCS=false` 的含义

- **true**（旧）：每个 claim 都写成 `request.jwt.claim.sub=xxx` / `request.jwt.claim.role=xxx` / ...
- **false**（新）：整个 JWT 写成 `request.jwt.claims='{"sub":"xxx","role":"authenticated",...}'`

新模式支持嵌套 claims（`request.jwt.claims::json->'app_metadata'->>'org_id'`），旧模式不行。

**我们用 false**，所以 auth.uid() 必须读 `request.jwt.claims::json->>'sub'`。

### 6.4 CORS 和 rate limit 在哪

PostgREST **本身不做 CORS 和 rate limit**。全部在 Caddy 网关层做。

### 6.5 `PGRST_DB_SCHEMAS`

我们现在只 expose `public`。官方自托管 expose `public,graphql_public,storage`。

**Phase 1 更新**：
```yaml
PGRST_DB_SCHEMAS: public,graphql_public
```

（不装 storage 就不暴露 storage schema）

---

## 7. pg_hba.conf 最佳实践

### 7.1 Supabase 官方镜像默认 HBA（pg_hba.conf.j2）

```
local all  supabase_admin     scram-sha-256
local all  all                peer map=supabase_map
host  all  all  127.0.0.1/32  trust                ← ⚠️ 注意这里
host  all  all  ::1/128       trust                ← 和这里
host  all  all  10.0.0.0/8     scram-sha-256
host  all  all  172.16.0.0/12  scram-sha-256
host  all  all  192.168.0.0/16 scram-sha-256
host  all  all  0.0.0.0/0      scram-sha-256       ← 允许任意 IP（有密码）
host  all  all  ::0/0          scram-sha-256
```

### 7.2 对 supabase-lite 的风险评估

在 docker-compose 场景下：
- **容器内 127.0.0.1 trust** = 不危险（容器自己）
- **主机 127.0.0.1 trust** = 有风险（多租户主机上的其他用户可能免密连）
- **0.0.0.0/0 scram-sha-256** = 允许任意外网，靠密码保护。我们已经把 ports 绑到 127.0.0.1:5432，外网无法直连，所以此规则实际不生效

### 7.3 Phase 1 收紧方案

**策略**：保留官方默认，但明确限制 `postgres` 端口的绑定（已在 docker-compose.yml 做到：`127.0.0.1:5432`）。

可选进一步：挂载我们自己的 pg_hba.conf 覆盖默认的，去掉 `0.0.0.0/0` 这条（反正绑 127.0.0.1 也没人能用到）：

```
# volumes/db/config/pg_hba.conf.custom
local all  supabase_admin    scram-sha-256
local all  all               peer map=supabase_map
host  all  all  127.0.0.1/32  scram-sha-256       ← 改 trust → scram-sha-256，防主机共享
host  all  all  ::1/128       scram-sha-256
host  all  all  172.16.0.0/12 scram-sha-256        ← docker 默认网段
# 其他外网规则删除
```

但这会**影响官方镜像自己的内部 bootstrap**（它假设 127.0.0.1 trust 能快速 init）。

**推荐方案**：**不改 HBA，改为严格控制端口绑定** + 加 `POSTGRES_BIND_ADDR=127.0.0.1`（我们已做）。HBA 层维持官方默认，保证兼容性。

---

## 8. 官方 self-host docker-compose 关键配置

### 8.1 服务列表（13 个）

```
supabase-studio        v2026.04.08     Studio UI
supabase-kong          3.9.1           API 网关（我们用 Caddy 替代）
supabase-auth          v2.186.0        GoTrue
supabase-rest          v14.8           PostgREST
supabase-realtime      v2.76.5         （不用）
supabase-storage       v1.48.26        （不用）
supabase-imgproxy      v3.30.1         （不用）
supabase-meta          v0.96.3         postgres-meta（Studio 依赖）
supabase-edge-functions v1.71.2        （不用）
supabase-analytics     1.36.1 (Logflare) （不用）
supabase-db            15.8.1.085      PostgreSQL
supabase-vector        0.53.0          日志采集（不用）
supabase-pooler        2.7.4 (Supavisor) 连接池（不用）
```

### 8.2 supabase-lite 对应关系

| 官方服务 | 版本 | supabase-lite |
|---------|------|---------------|
| studio | v2026.04.08 | ✅ 直接用（Phase 2） |
| kong | 3.9.1 | ❌ 用 Caddy 替代 |
| auth (GoTrue) | v2.186.0 | ✅ 已用 v2.164.0，可升级到 v2.186.0 |
| rest (PostgREST) | v14.8 | ⚠️ 已用 v12.2.3，**建议升级到 v14.8** |
| meta (postgres-meta) | v0.96.3 | ✅ 加入（Phase 2） |
| db | 15.8.1.085 | ⚠️ 已用 15.8.1.060，**建议升级** |
| realtime / storage / imgproxy / edge / analytics / vector / pooler | — | ❌ 不做 |

### 8.3 PostgREST 升级：v12 → v14 需注意

- v13 增加了 `PGRST_DB_AGGREGATES_ENABLED` 等新特性
- v14 改进了 OpenAPI 生成和 schema 加载
- **兼容性**：v14 向后兼容 v12 的基本 REST API，但 GUC 处理可能有微调
- **风险**：我们的 auth 函数依赖 `request.jwt.claims` GUC，需确认 v14 仍保留

官方自托管用 v14.8，我们跟进。

### 8.4 GoTrue 环境变量关键项

官方用（我们对齐）：
```yaml
GOTRUE_API_HOST: 0.0.0.0
GOTRUE_API_PORT: 9999
GOTRUE_DB_DRIVER: postgres
GOTRUE_DB_DATABASE_URL: postgres://supabase_auth_admin:${POSTGRES_PASSWORD}@db:5432/postgres
# ⚠️ 连的是 supabase_auth_admin，不是 supabase_admin！
GOTRUE_JWT_SECRET: ${JWT_SECRET}
GOTRUE_JWT_ADMIN_ROLES: service_role
GOTRUE_JWT_AUD: authenticated
GOTRUE_JWT_DEFAULT_GROUP_NAME: authenticated
# ... 其他
```

**重要差异**：我们当前用 `supabase_admin` 连 GoTrue，官方用 `supabase_auth_admin`。后者权限更小（只管 auth schema）。

**Phase 1 修改**：`docker-compose.yml` 里把 GoTrue 的 DATABASE_URL 改用 `supabase_auth_admin`。

### 8.5 Studio 环境变量

```yaml
STUDIO_PG_META_URL: http://meta:8080    # postgres-meta 地址
POSTGRES_HOST: db
POSTGRES_PORT: 5432
POSTGRES_DB: postgres
POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}  # 用于连接
PG_META_CRYPTO_KEY: ${PG_META_CRYPTO_KEY}  # 加密存储的项目凭证
PGRST_DB_SCHEMAS: ${PGRST_DB_SCHEMAS}
DEFAULT_ORGANIZATION_NAME: ...
DEFAULT_PROJECT_NAME: ...

SUPABASE_URL: http://kong:8000           # 内部 API URL → 改成 http://gateway:8000
SUPABASE_PUBLIC_URL: ${SUPABASE_PUBLIC_URL}
SUPABASE_ANON_KEY: ${ANON_KEY}
SUPABASE_SERVICE_KEY: ${SERVICE_ROLE_KEY}
AUTH_JWT_SECRET: ${JWT_SECRET}
```

### 8.6 postgres-meta 环境变量

```yaml
PG_META_PORT: 8080
PG_META_DB_HOST: db
PG_META_DB_PORT: 5432
PG_META_DB_NAME: postgres
PG_META_DB_USER: supabase_admin         # ⚠️ 用 supabase_admin（有 SUPERUSER 权限）
PG_META_DB_PASSWORD: ${POSTGRES_PASSWORD}
CRYPTO_KEY: ${PG_META_CRYPTO_KEY}       # 用于加密存储的连接凭证
```

**注意**：postgres-meta 用 `supabase_admin` 连 DB，这是为了让 Studio 能管所有 schema 的所有对象。

---

## 9. Phase 1 动手清单

基于以上所有调研，Phase 1 需要做的具体变更：

### 9.1 docker-compose.yml

```yaml
services:
  db:
    image: supabase/postgres:15.8.1.085   # 从 15.8.1.060 升级
    environment:
      POSTGRES_USER: supabase_admin       # 对齐官方
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: postgres
      POSTGRES_HOST: /var/run/postgresql
      JWT_SECRET: ${JWT_SECRET}
      JWT_EXP: 3600
    # 移除我们自己之前加的 GOTRUE_DB_PASSWORD / AUTHENTICATOR_PASSWORD
    # （角色由镜像建，密码通过 ALTER USER 统一设为 POSTGRES_PASSWORD）

  rest:
    image: postgrest/postgrest:v14.8      # 从 v12.2.3 升级
    environment:
      PGRST_DB_URI: postgres://authenticator:${POSTGRES_PASSWORD}@db:5432/postgres
      PGRST_DB_SCHEMAS: public,graphql_public   # 新增 graphql_public
      PGRST_DB_ANON_ROLE: anon
      PGRST_JWT_SECRET: ${JWT_SECRET}
      PGRST_DB_USE_LEGACY_GUCS: "false"
      PGRST_APP_SETTINGS_JWT_SECRET: ${JWT_SECRET}

  gotrue:
    image: supabase/gotrue:v2.186.0       # 从 v2.164.0 升级
    environment:
      GOTRUE_DB_DATABASE_URL: postgres://supabase_auth_admin:${POSTGRES_PASSWORD}@db:5432/postgres
      # ⚠️ 改为 supabase_auth_admin，不再用 supabase_admin
      GOTRUE_JWT_ADMIN_ROLES: service_role       # 新增
      GOTRUE_JWT_AUD: authenticated              # 新增
      # ... 其他保持不变
```

### 9.2 删除并重写 `volumes/db/init/00-roles.sh`

- **删除** `CREATE ROLE supabase_admin / anon / authenticated / service_role / authenticator`（镜像自己建）
- **删除** 重复的 `auth.uid() / auth.role() / auth.email()`（镜像自己建，且我们要覆盖为新 GUC 模式）
- **删除** 手工创建 `auth / extensions` schema（镜像建）
- **保留**：
  - 设置所有角色的密码（通过镜像的 post-init 机制）
  - 我们的 `auth.uid()` 等函数的覆盖版本（新 GUC 模式）

### 9.3 新建 `volumes/db/init/99-supabase-lite-setup.sh`

```bash
#!/bin/bash
set -e

psql -v ON_ERROR_STOP=1 --username supabase_admin --dbname postgres <<-EOSQL

  -- 1. 设置所有 Supabase 规范角色的密码
  ALTER USER supabase_admin        WITH PASSWORD '${POSTGRES_PASSWORD}';
  ALTER USER authenticator         WITH PASSWORD '${POSTGRES_PASSWORD}';
  ALTER USER supabase_auth_admin   WITH PASSWORD '${POSTGRES_PASSWORD}';
  ALTER USER supabase_storage_admin WITH PASSWORD '${POSTGRES_PASSWORD}';
  ALTER USER supabase_functions_admin WITH PASSWORD '${POSTGRES_PASSWORD}';
  ALTER USER dashboard_user        WITH PASSWORD '${POSTGRES_PASSWORD}';
  ALTER USER pgbouncer             WITH PASSWORD '${POSTGRES_PASSWORD}';

  -- 2. 覆盖 auth.uid() 等函数为新 GUC 模式
  CREATE OR REPLACE FUNCTION auth.uid() RETURNS uuid LANGUAGE sql STABLE AS \$func\$
    SELECT NULLIF(current_setting('request.jwt.claims', true)::json->>'sub', '')::uuid
  \$func\$;

  CREATE OR REPLACE FUNCTION auth.role() RETURNS text LANGUAGE sql STABLE AS \$func\$
    SELECT NULLIF(current_setting('request.jwt.claims', true)::json->>'role', '')::text
  \$func\$;

  CREATE OR REPLACE FUNCTION auth.email() RETURNS text LANGUAGE sql STABLE AS \$func\$
    SELECT NULLIF(current_setting('request.jwt.claims', true)::json->>'email', '')::text
  \$func\$;

  -- 3. 预留的 schema（镜像没自动建）
  CREATE SCHEMA IF NOT EXISTS _realtime          AUTHORIZATION supabase_admin;
  CREATE SCHEMA IF NOT EXISTS supabase_functions AUTHORIZATION supabase_admin;

  -- 4. 启用 Supabase 核心扩展
  CREATE EXTENSION IF NOT EXISTS pgjwt         WITH SCHEMA extensions;
  CREATE EXTENSION IF NOT EXISTS http          WITH SCHEMA extensions;
  CREATE EXTENSION IF NOT EXISTS pg_net;
  CREATE EXTENSION IF NOT EXISTS pg_jsonschema WITH SCHEMA extensions;
  CREATE EXTENSION IF NOT EXISTS pg_graphql;
  CREATE EXTENSION IF NOT EXISTS vector        WITH SCHEMA extensions;

  CREATE EXTENSION IF NOT EXISTS pgsodium;
  CREATE EXTENSION IF NOT EXISTS supabase_vault;

  CREATE EXTENSION IF NOT EXISTS pg_cron;

  -- 5. JWT 相关 session settings
  ALTER DATABASE postgres SET "app.settings.jwt_secret" TO '${JWT_SECRET}';
  ALTER DATABASE postgres SET "app.settings.jwt_exp" TO '3600';

EOSQL
```

### 9.4 环境变量简化

由于角色密码统一用 `POSTGRES_PASSWORD`，之前我们拆的 `GOTRUE_DB_PASSWORD` / `AUTHENTICATOR_PASSWORD` **都可以删除**。

**setup.sh 生成的 .env**：
```env
POSTGRES_PASSWORD=<随机32字符>   # 统一密码（所有 admin 角色共用）
JWT_SECRET=<随机64字符>
ADMIN_TOKEN=<随机48字符>
ANON_KEY=<JWT>
SERVICE_ROLE_KEY=<JWT>
```

### 9.5 Phase 1 验证清单

完成后要验证：

```bash
# 1. 容器启动无错误
docker compose up -d
docker compose logs db

# 2. 连进去检查
docker compose exec db psql -U supabase_admin -c "SELECT rolname FROM pg_roles WHERE rolname LIKE 'supabase%' OR rolname IN ('anon','authenticated','service_role','authenticator','dashboard_user') ORDER BY rolname;"
# 期望：看到所有规范角色

docker compose exec db psql -U supabase_admin -c "SELECT nspname FROM pg_namespace WHERE nspname NOT LIKE 'pg_%' AND nspname <> 'information_schema' ORDER BY nspname;"
# 期望：auth / extensions / realtime / storage / _realtime / supabase_functions / graphql / vault / net / cron / pgsodium / public

docker compose exec db psql -U supabase_admin -c "SELECT extname, extnamespace::regnamespace FROM pg_extension ORDER BY extname;"
# 期望：uuid-ossp, pgcrypto, pgjwt, pg_net, pg_jsonschema, pg_graphql, vector, pgsodium, supabase_vault, pg_cron, http

# 3. PostgREST 能连上并认出 anon
curl -H "apikey: <ANON_KEY>" -H "Authorization: Bearer <ANON_KEY>" http://localhost:8000/rest/v1/
# 期望：200 OK，返回 OpenAPI schema

# 4. GoTrue 能连上（用 supabase_auth_admin）
curl http://localhost:8000/auth/v1/health
# 期望：200 OK
```

---

## 10. Phase 2 之前要补充调研的

Phase 2（Studio 集成）开工前还需要搞清的：

1. **Studio 的 feature flag 环境变量完整清单**：调研 supabase/studio 最新版本源码，找出哪些模块可以禁用
2. **Studio 的 cookie 鉴权方式**：Studio 自己处理 cookie 还是依赖 Kong？我们用 Caddy 怎么替代？
3. **postgres-meta 的 API 路径**：哪些端点需要暴露给 Studio
4. **Studio 访问 `/rest/v1/` 时走哪条路径**：通过 Kong/Caddy 还是直连？我们要同时支持哪些入口？
5. **DASHBOARD_USERNAME / DASHBOARD_PASSWORD 机制**：官方自托管用 Kong 做 Basic Auth 保护 Studio，我们要怎么用 Caddy 实现

这些在 Phase 1 完成后、Phase 2 开工前单独调研。

---

## 11. 常见坑与注意事项

| 坑 | 描述 | 解决 |
|----|------|------|
| supautils 保留角色限制 | 无法 DROP / 改密某些保留角色 | 所有角色管理通过 `supabase_admin` 身份做 |
| auth 函数 owner | 镜像把 `auth.uid()` owner 改为 `supabase_auth_admin` | 我们覆盖时要保持 owner 不变（CREATE OR REPLACE 不改 owner） |
| 扩展 schema 固定 | pg_net 必须在 net schema，pgsodium 必须在 pgsodium | 不要加 `WITH SCHEMA extensions` |
| pg_cron 数据库 | cron 任务存在 `postgres` 库 | 官方用 `_supabase` 库放 cron，我们按官方做 |
| 镜像 init 只跑一次 | data volume 存在后就不重跑 init-scripts | 改 init SQL 要先 `docker compose down -v` |
| `PGRST_DB_USE_LEGACY_GUCS` | 切换会让 auth 函数读错 GUC | 固定为 false，auth 函数配套 |
| JWT `exp` 太短 | 默认 3600 秒，长会话需处理 refresh | 由 GoTrue 签 refresh_token |
| 镜像 POSTGRES_USER 默认 supabase_admin | 不能改成 postgres，会破坏很多假设 | **必须** 保持 supabase_admin |

---

## 12. 版本兼容矩阵（Phase 1 对齐）

| 组件 | Phase 1 版本 | 官方对应版本 | 备注 |
|------|--------------|--------------|------|
| supabase/postgres | 15.8.1.085 | 15.8.1.085 | 对齐 |
| postgrest | v14.8 | v14.8 | 升级 |
| supabase/gotrue | v2.186.0 | v2.186.0 | 升级 |
| supabase/studio | v2026.04.08 | v2026.04.08 | Phase 2 加入 |
| supabase/postgres-meta | v0.96.3 | v0.96.3 | Phase 2 加入 |
| caddy | 2-alpine | - | 替代 Kong，保持 |

---

## 附：关键源码位置索引

```
# Supabase 镜像源码
~/otheruse/supabase-postgres/
├── Dockerfile-15                         镜像构建
├── migrations/db/
│   ├── init-scripts/
│   │   ├── 00000000000000-initial-schema.sql    ← 最重要（角色 / extensions）
│   │   ├── 00000000000001-auth-schema.sql        ← auth 初始化
│   │   ├── 00000000000002-storage-schema.sql    ← storage 初始化
│   │   └── 00000000000003-post-setup.sql        ← dashboard_user / pg_cron 事件触发
│   └── migrations/ (49 个增量)
├── ansible/files/postgresql_config/
│   ├── supautils.conf.j2                 ← 保留角色清单
│   ├── pg_hba.conf.j2                    ← HBA 默认
│   └── postgresql.conf.j2                ← 参数默认

# Supabase 官方 self-host
~/otheruse/supabase-official/docker/
├── docker-compose.yml                    ← 13 服务参考配置
├── .env.example                          ← 所有 env 变量
├── volumes/
│   ├── db/
│   │   ├── roles.sql                     ← 角色密码统一设置
│   │   ├── jwt.sql                       ← app.settings.jwt_secret
│   │   ├── realtime.sql                  ← 建 _realtime schema
│   │   ├── webhooks.sql                  ← 建 supabase_functions + pg_net
│   │   └── _supabase.sql                 ← 建 _supabase 数据库
│   └── api/kong.yml                      ← Kong 路由配置（strip_path 模式）
```
