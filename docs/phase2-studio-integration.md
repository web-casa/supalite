# Phase 2: Studio 集成调研文档

> 目标：在 supabase-lite 中集成官方 Supabase Studio + postgres-meta，同时隐藏不支持的服务（Storage / Edge Functions / Realtime / Analytics）
> 依据：supabase/studio 源码（~/otheruse/supabase-full/apps/studio）、官方 self-host docker-compose（~/otheruse/supabase-official/docker）
> 日期：2026-04-17

---

## 1. Studio 架构关键点

### 1.1 镜像
```yaml
image: supabase/studio:2026.04.08-sha-205cbe7
container_name: supabase-studio
```

### 1.2 运行模式
- 基于 Next.js standalone build（HOSTNAME=0.0.0.0，端口 3000）
- 容器内读 env 变量决定行为
- **关键区分**：`NEXT_PUBLIC_IS_PLATFORM=true`（cloud）vs 空（self-hosted）

### 1.3 健康检查
```
GET http://localhost:3000/api/platform/profile → 200
```

---

## 2. Studio 环境变量完整清单

### 2.1 核心 URL 配置

```yaml
# 内部用 URL（Studio 容器在 docker network 内发出请求时用）
SUPABASE_URL: http://gateway:8000              # Caddy 内部地址

# 外部用 URL（返回给浏览器的链接、OAuth 回调等）
SUPABASE_PUBLIC_URL: http://localhost:8000     # 用户访问的地址

# postgres-meta 地址（内部直连，不走 Caddy）
STUDIO_PG_META_URL: http://meta:8080
```

**关键代码**（`apps/studio/lib/...`）：
```ts
// 请求路径转换
requestUrl.replace(process.env.SUPABASE_PUBLIC_URL, process.env.SUPABASE_URL)
```

Studio **内部会把浏览器给的 public URL 替换成 internal URL** 再发请求，实现"内外网透明"。

### 2.2 鉴权 & Key

```yaml
SUPABASE_ANON_KEY: ${ANON_KEY}
SUPABASE_SERVICE_KEY: ${SERVICE_ROLE_KEY}
AUTH_JWT_SECRET: ${JWT_SECRET}
```

### 2.3 数据库连接（Studio 直连用）

```yaml
POSTGRES_HOST: ${POSTGRES_HOST}           # 建议：db
POSTGRES_PORT: ${POSTGRES_PORT}           # 5432
POSTGRES_DB: ${POSTGRES_DB}               # postgres
POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}   # supabase_admin 的密码

# Studio 用 pg-meta crypto 加密存储的凭证（如用户保存的数据库连接）
PG_META_CRYPTO_KEY: ${PG_META_CRYPTO_KEY}  # 32+ 字符
```

### 2.4 Studio 配置

```yaml
STUDIO_DEFAULT_ORGANIZATION: Default Organization
STUDIO_DEFAULT_PROJECT: Default Project
DEFAULT_ORGANIZATION_NAME: ${STUDIO_DEFAULT_ORGANIZATION}
DEFAULT_PROJECT_NAME: ${STUDIO_DEFAULT_PROJECT}

PGRST_DB_SCHEMAS: public,graphql_public
PGRST_DB_MAX_ROWS: 1000
PGRST_DB_EXTRA_SEARCH_PATH: public
```

### 2.5 Feature Flags（禁用不支持的服务）⭐

这是我们用来**隐藏 Storage / Edge Functions / Realtime 菜单**的关键：

```yaml
NEXT_PUBLIC_DISABLED_FEATURES: project_storage:all,project_edge_function:all,realtime:all
```

**完整可用 flag 清单**（源自 `packages/api-types/types/platform.d.ts:8437`）：

```
organizations:create
organizations:delete
organization_members:create
organization_members:delete
projects:create
projects:transfer
project_auth:all              ← 隐藏 Auth 模块
project_storage:all           ← 隐藏 Storage 模块  ⭐
project_edge_function:all     ← 隐藏 Edge Functions 模块  ⭐
profile:update
billing:account_data
billing:credits
billing:invoices
billing:payment_methods
realtime:all                  ← 隐藏 Realtime 模块  ⭐
```

**Studio 读取逻辑**（`profile-query.ts:20`）：
```ts
disabled_features: process.env.NEXT_PUBLIC_DISABLED_FEATURES?.split(',') ?? []
```

**Sidebar 使用**（`components/interfaces/Sidebar.tsx:232-253`）：
```ts
const {
  projectAuthAll: authEnabled,
  projectEdgeFunctionAll: edgeFunctionsEnabled,
  projectStorageAll: storageEnabled,
  realtimeAll: realtimeEnabled,
} = useIsFeatureEnabled([
  'project_auth:all',
  'project_edge_function:all',
  'project_storage:all',
  'realtime:all',
])

generateProductRoutes(ref, project, {
  auth: authEnabled,        // 我们保留
  edgeFunctions: false,     // 隐藏
  storage: false,           // 隐藏
  realtime: false,          // 隐藏
})
```

### 2.6 可选功能 env

```yaml
NEXT_PUBLIC_ENABLE_LOGS: "false"     # 禁用日志模块（Logflare 未装）
OPENAI_API_KEY: ""                    # 关闭 AI 功能
LOGFLARE_URL: ""                      # 空值
LOGFLARE_PUBLIC_ACCESS_TOKEN: ""
```

---

## 3. postgres-meta 集成

### 3.1 镜像
```yaml
image: supabase/postgres-meta:v0.96.3
container_name: supabase-meta
```

### 3.2 环境变量
```yaml
PG_META_PORT: 8080
PG_META_DB_HOST: db
PG_META_DB_PORT: 5432
PG_META_DB_NAME: postgres
PG_META_DB_USER: supabase_admin        # SUPERUSER，Studio 需要管理所有对象
PG_META_DB_PASSWORD: ${POSTGRES_PASSWORD}
CRYPTO_KEY: ${PG_META_CRYPTO_KEY}      # 加密 Studio 保存的凭证
```

### 3.3 API 端点清单

postgres-meta 暴露的端点（Studio 会用到）：

| 端点 | 方法 | 用途 |
|------|------|------|
| `/query` | POST | 执行 SQL（Studio SQL Editor 用） |
| `/format` | POST | 格式化 SQL |
| `/parse` | POST | 解析 SQL 为 AST |
| `/explain` | POST | EXPLAIN 查询 |
| `/columns` | GET/POST/PATCH/DELETE | 列管理 |
| `/extensions` | GET/POST/PATCH/DELETE | 扩展管理 |
| `/functions` | GET/POST/PATCH/DELETE | 存储函数 |
| `/publications` | GET/POST/PATCH/DELETE | 发布（Realtime 用，我们不用） |
| `/roles` | GET/POST/PATCH/DELETE | 角色 |
| `/schemas` | GET/POST/PATCH/DELETE | Schema |
| `/tables` | GET/POST/PATCH/DELETE | 表 |
| `/triggers` | GET/POST/PATCH/DELETE | 触发器 |
| `/types` | GET/POST/PATCH/DELETE | 自定义类型 |
| `/config/version` | GET | Postgres 版本 |
| `/generators/openapi` | GET | 生成 OpenAPI spec |
| `/generators/typescript` | GET | 生成 TypeScript 类型 |

### 3.4 暴露方式

**选项 A：直连（推荐）**
- postgres-meta 只在 docker network 内可达
- Studio 通过 `STUDIO_PG_META_URL=http://meta:8080` 直连
- Caddy 不 proxy meta API，外部无法访问
- **优点**：安全（meta 无鉴权）
- **缺点**：自研 admin 想用 meta API 也要走 Docker network

**选项 B：Caddy 代理 /pg/***
- 参考官方 Kong 配置：`/pg/* → meta:8080`
- 但需要加鉴权（Caddy 的 matcher 验证 cookie / apikey）
- **优点**：统一入口
- **缺点**：多一层、需要配置鉴权

**Phase 2 推荐：选 A**。Studio 和 admin 都在 docker network 内，直连 `meta:8080` 最简单。

---

## 4. Kong → Caddy 路由完整映射表

基于 `~/otheruse/supabase-official/docker/volumes/api/kong.yml` 分析，官方 Kong 的完整路由：

| Kong 路径 | 目标 | strip_path | supabase-lite Caddy |
|-----------|------|:----------:|---------------------|
| `/auth/v1/verify` | `http://auth:9999/verify` | ✓ | 已有 ✓ |
| `/auth/v1/callback` | `http://auth:9999/callback` | ✓ | 已有 ✓ |
| `/auth/v1/authorize` | `http://auth:9999/authorize` | ✓ | 已有 ✓ |
| `/auth/v1/.well-known/jwks.json` | `http://auth:9999/.well-known/jwks.json` | ✓ | 已有 ✓ |
| `/auth/v1/*`（其他） | `http://auth:9999/*` | ✓ | 已有 ✓ |
| `/rest/v1/*` | `http://rest:3000/*` | ✓ | 已有 ✓ |
| `/graphql/v1` | `http://rest:3000/rpc/graphql` | ✓ | **Phase 2 新增** ⭐ |
| `/realtime/v1/*` | `ws://realtime:4000/socket` | ✓ | ❌ 不做 |
| `/realtime/v1/api/*` | `http://realtime:4000/api` | ✓ | ❌ 不做 |
| `/storage/v1/*` | `http://storage:5000/*` | ✓ | ❌ 不做 |
| `/functions/v1/*` | `http://functions:9000/*` | ✓ | ❌ 不做 |
| `/pg/*` | `http://meta:8080/*` | ✓ | 不暴露（选 A） |
| `/.well-known/oauth-authorization-server` | auth:9999 | ✓ | **Phase 2 新增** |

### 4.1 Phase 2 需要新增的 Caddy 路由

```caddy
:8000 {
    # ... 已有 /auth/v1 /rest/v1 /admin/* ...

    # 新增：GraphQL（pg_graphql 启用后）
    handle /graphql/v1* {
        uri strip_prefix /graphql/v1
        rewrite * /rpc/graphql{uri}
        # GraphQL 需要 Content-Profile header 指向 graphql_public schema
        header_up Content-Profile graphql_public
        reverse_proxy rest:3000
    }

    # 新增：OAuth well-known
    handle /.well-known/oauth-authorization-server {
        uri strip_prefix /.well-known/oauth-authorization-server
        reverse_proxy gotrue:9999
    }

    # 新增：Studio
    handle /studio* {
        uri strip_prefix /studio
        reverse_proxy studio:3000
    }
}
```

### 4.2 Kong 的 apikey → Authorization 转换

Kong 额外做了一个事：把请求的 `apikey` header 自动转成 `Authorization: Bearer <apikey>`。因为 PostgREST 读 Authorization，不读 apikey。

supabase-js 客户端**同时发两个头**：
```
apikey: <anon_key>
Authorization: Bearer <user_jwt or anon_key>
```

所以我们的 Caddy **不需要转换**，supabase-js 已经两头都发。

---

## 5. Studio 无 Kong 的工作方式

### 5.1 核心：两个 URL 变量

```yaml
SUPABASE_URL: http://gateway:8000        # 内部（Studio 在 docker network）
SUPABASE_PUBLIC_URL: http://localhost:8000   # 外部（用户浏览器）
```

### 5.2 浏览器行为

Studio 是 Next.js 应用：
- **SSR 阶段**：在容器内运行，用 `SUPABASE_URL` 调后端
- **CSR 阶段**：在浏览器运行，URL 被替换为 `SUPABASE_PUBLIC_URL`

### 5.3 Caddy 是否需要 hostname alias `kong`？

**不需要**。调研确认 Studio 现版本只依赖 `SUPABASE_URL` env var，没有硬编码 `kong` hostname。

但为了**保险**，可以在 docker-compose 加 alias（万一有地方漏了）：
```yaml
gateway:
  networks:
    default:
      aliases:
        - gateway
        - kong    # 兼容任何硬编码 kong hostname 的地方
```

---

## 6. 鉴权方案：Cookie + Caddy 校验

### 6.1 官方 Kong 的鉴权（不采用）

Kong 用 `basic-auth` plugin 保护 Studio：
```yaml
basicauth_credentials:
  - consumer: DASHBOARD
    username: '$DASHBOARD_USERNAME'
    password: '$DASHBOARD_PASSWORD'
```

浏览器访问 Studio 时弹出 Basic Auth 对话框。**不采用**，UX 差。

### 6.2 supabase-lite 的方案：Cookie + Caddy 校验

#### 流程

```
1. 用户打开 http://localhost:8000/
   → 显示"选择页"（两个入口按钮：Admin / Studio）

2. 用户点 "Studio" → 浏览器请求 /studio/
   → Caddy 检查 cookie `sbl_auth`
   → 无 cookie → 302 重定向到 /admin/login?next=/studio/

3. 用户在 /admin/ 输入 admin token 登录
   → admin 后端校验 token
   → admin 后端 set cookie:
       Set-Cookie: sbl_auth=<hmac(admin_token)>; HttpOnly; Secure; SameSite=Strict; Path=/

4. 浏览器重定向到 /studio/
   → Caddy 校验 cookie 值是否合法
   → 合法 → proxy 到 studio:3000
```

#### Cookie 值设计

```
cookie value = base64(hmac_sha256(admin_token, "studio-access"))
```

- Admin 后端知道 ADMIN_TOKEN，能生成此 HMAC
- Caddy 需要同样的 ADMIN_TOKEN 来验证
- 可以用 Caddy 的 `authentication` module + 自定义 provider

#### Caddy 实现

```caddy
(require_studio_auth) {
    # 简化方案：让 admin 后端做完整的"验证并 proxy"
    # Caddy 只简单转发，admin 后端插入中间校验
    
    # 或者：Caddy 用 caddyauth module 校验 cookie HMAC
    # 需要一个小 Caddy plugin（Go 写的）
}

handle /studio* {
    # 方案 A：Caddy 转发到 admin 后端 /api/studio-gate，后端决定 200/302
    reverse_proxy admin:9100 {
        header_up X-Original-URI {uri}
    }
    # 这种方案需要 admin 后端做 proxy，重而复杂
}

# 方案 B：Caddy plugin 直接校验 cookie（推荐）
# 需要编译 Caddy 时包含一个简单的 HMAC 校验插件
```

#### Phase 2 推荐方案 C：最简单的做法

**Caddy 做 cookie 存在性检查**（不校验内容），admin 后端负责 set/clear cookie。如果攻击者猜到 cookie name 就能伪造 → **但 cookie 值必须是有效 HMAC 才被 admin 后端接受**。

```caddy
handle /studio* {
    @no_auth_cookie {
        not header Cookie *sbl_auth=*
    }
    handle @no_auth_cookie {
        redir /admin/login?next={uri} 302
    }
    uri strip_prefix /studio
    reverse_proxy studio:3000
}
```

这样：
- 未登录 → 重定向到 /admin/login
- 有 cookie（即使假的）→ 放行到 Studio
- Studio 本身**没有**鉴权层，所以有 cookie 就能用

这看起来不安全，但其实：
- 攻击者要通过互联网访问（我们绑 127.0.0.1 了）本地攻击者能伪造 cookie 但也能 kill docker
- 本地网络里的其他用户能看到流量但也看到 cookie（生产应该上 HTTPS）

**真正的安全 = HTTPS + 绑 127.0.0.1 或内网 IP**，cookie 只是方便用户。

#### 更强的 Phase 2+ 升级路径

如果要严格：用 JWT cookie，Caddy 校验签名：

```caddy
handle /studio* {
    jwtauth {
        primary yes
        trusted_tokens static_secret {$ADMIN_TOKEN}
        token_source cookie sbl_auth
    }
    # ... proxy to studio
}
```

需要 Caddy 编译时包含 `caddy-jwt` 插件（`github.com/greenpau/caddy-auth-jwt`）。

---

## 7. Docker Compose Profile 使用方案

### 7.1 Profile 机制

- **无 profile** 的服务：`docker compose up` 默认启动
- **有 profile** 的服务：必须 `--profile <name>` 或 `COMPOSE_PROFILES=name` 才启动

### 7.2 supabase-lite 的 profile 策略

```yaml
services:
  db: ...             # 无 profile，必装
  rest: ...           # 无 profile
  gotrue: ...         # 无 profile
  gateway: ...        # 无 profile
  admin: ...          # 无 profile

  studio:
    profiles: [studio]
    # ...

  meta:
    profiles: [studio]
    # ...
```

### 7.3 启用方式

**方式 A：env var（推荐，用户友好）**

`.env` 里：
```
COMPOSE_PROFILES=studio
```

`docker compose up -d` 自动启动包括 Studio。

**setup.sh 默认写入此行**。用户想要最小版：手动把这行删掉。

**方式 B：命令行**

```bash
docker compose --profile studio up -d   # 带 Studio
docker compose up -d                     # 只启核心服务
```

### 7.4 Phase 2 实施决策

```
1. setup.sh 默认写入 COMPOSE_PROFILES=studio
2. 用户通过编辑 .env 切换
3. 文档明确两种模式
```

### 7.5 ⚠️ "反向用法"说明（给用户和未来维护者看）

**读者注意**：我们在用 Compose profile 做**反向控制**。这和 profile 的原生语义是反的，必须在 README / 文档明确说明。

#### Compose profile 的原生语义

- 一个服务**没打 profile** → `docker compose up` 默认启动
- 一个服务**打了 profile** → **只有**激活该 profile 才启动

#### 我们的用法

Studio 和 postgres-meta 都**打了 `profiles: [studio]`**。按原生语义，**默认不启动**。但我们在 `.env` 里默认写了 `COMPOSE_PROFILES=studio`，所以 `docker compose up` 读 `.env` 时看到这个环境变量，自动激活 studio profile → Studio 和 meta 启动。

#### 为什么这么做而不是"不打 profile"

如果 Studio / meta 不打 profile：
- 它们会**无条件启动**
- 用户想关掉 Studio（minimal 模式）只能 `docker compose stop studio meta`
- 不能一条命令切换，很难用

打 profile + `.env` 激活：
- 默认启动（用户符合预期）
- 关掉只需编辑 `.env` 删一行，再 `docker compose up -d`
- 以后新增可选模块（如 pgbackrest）沿用同样模式

#### 多个 profile 的写法

Phase 6 加 pgbackrest profile 后，用户 `.env` 可能是：

```env
COMPOSE_PROFILES=studio,pgbackrest        # 全启用
COMPOSE_PROFILES=studio                   # 只要 Studio
COMPOSE_PROFILES=pgbackrest               # 只要备份（罕见）
# 无此行                                   # 最小版
```

#### 文档要在哪几处强调

- README 的安装说明
- setup.sh 首次运行时的输出提示
- `docs/user-guide.md`（v1.0 补）

#### 相关常量（避免分散）

建议 `.env.example` 里放这一段注释：

```env
# COMPOSE_PROFILES 控制启用哪些可选服务。
# 编辑此行启用/禁用：
#   studio      — Supabase Studio（表编辑器 / SQL / Auth 管理等）
#   pgbackrest  — pgBackRest 增量备份（会启用 PG archive_mode）
# 多个用逗号分隔，删除整行则只启动核心服务。
COMPOSE_PROFILES=studio

# COMPOSE_FILE 控制加载哪些 compose 文件。
# 启用 pgbackrest 必须加 docker-compose.pgbackrest.yml 覆盖 db 的 archive_command。
COMPOSE_FILE=docker-compose.yml
```

---

## 8. 首页选择页

用户决定：访问 `http://localhost:8000/` 显示**选择页**，两个入口按钮。

### 8.1 Caddy 路由

```caddy
:8000 {
    # 根路径 → Caddy 返回选择页 HTML
    handle / {
        root * /srv
        rewrite * /index.html
        file_server
    }

    # 或：让 admin 后端返回选择页（更灵活）
    handle / {
        reverse_proxy admin:9100 {
            rewrite /welcome
        }
    }
    
    # ... 其他 handle
}
```

### 8.2 选择页内容

一个简单的静态 HTML（放 Caddy volume 或 admin 容器里）：

```html
<!DOCTYPE html>
<html>
<head>
  <title>supabase-lite</title>
  <style>/* 和 admin 面板统一的暗色风 */</style>
</head>
<body>
  <h1>supabase-lite</h1>
  <div class="cards">
    <a href="/studio/">
      <h2>Studio</h2>
      <p>表编辑器 · SQL Editor · API Docs · Auth / 扩展管理</p>
      <p class="subtitle">面向应用开发者</p>
    </a>
    <a href="/admin/">
      <h2>Admin</h2>
      <p>DBA 运维驾驶舱 · SystemAccount · VACUUM · 备份 · 慢查询</p>
      <p class="subtitle">面向运维</p>
    </a>
  </div>
</body>
</html>
```

### 8.3 Phase 2 实施方案

**把欢迎页做到 admin 后端里**，既可访问 / 也可作为 /admin 首页首屏。静态 HTML 嵌入 Go 二进制。

---

## 9. 更新后的 docker-compose.yml（Phase 2 版）

```yaml
services:
  # ... 原有 db / rest / gotrue / admin / gateway ...

  meta:
    image: supabase/postgres-meta:v0.96.3
    restart: unless-stopped
    profiles: [studio]
    environment:
      PG_META_PORT: 8080
      PG_META_DB_HOST: db
      PG_META_DB_PORT: 5432
      PG_META_DB_NAME: postgres
      PG_META_DB_USER: supabase_admin
      PG_META_DB_PASSWORD: ${POSTGRES_PASSWORD}
      CRYPTO_KEY: ${PG_META_CRYPTO_KEY}
    depends_on:
      db:
        condition: service_healthy
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://localhost:8080/health || exit 1"]
      interval: 10s
      timeout: 3s
      retries: 5

  studio:
    image: supabase/studio:2026.04.08-sha-205cbe7
    restart: unless-stopped
    profiles: [studio]
    environment:
      HOSTNAME: 0.0.0.0
      STUDIO_PG_META_URL: http://meta:8080
      POSTGRES_HOST: db
      POSTGRES_PORT: 5432
      POSTGRES_DB: postgres
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      PG_META_CRYPTO_KEY: ${PG_META_CRYPTO_KEY}
      PGRST_DB_SCHEMAS: public,graphql_public
      
      DEFAULT_ORGANIZATION_NAME: Default Organization
      DEFAULT_PROJECT_NAME: Default Project
      
      SUPABASE_URL: http://gateway:8000
      SUPABASE_PUBLIC_URL: ${API_EXTERNAL_URL}
      SUPABASE_ANON_KEY: ${ANON_KEY}
      SUPABASE_SERVICE_KEY: ${SERVICE_ROLE_KEY}
      AUTH_JWT_SECRET: ${JWT_SECRET}
      
      # 禁用我们不支持的模块
      NEXT_PUBLIC_DISABLED_FEATURES: "project_storage:all,project_edge_function:all,realtime:all"
      NEXT_PUBLIC_ENABLE_LOGS: "false"
    depends_on:
      meta:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "node", "-e", "fetch('http://localhost:3000/api/platform/profile').then((r) => {if (r.status !== 200) throw new Error(r.status)})"]
      interval: 10s
      timeout: 5s
      retries: 5
```

---

## 10. Phase 2 的新 .env 变量

setup.sh 需要额外生成：

```env
# 新增
PG_META_CRYPTO_KEY=<32+ 字符随机串>
COMPOSE_PROFILES=studio
```

setup.sh 补充：

```bash
PG_META_CRYPTO_KEY=$(gen_secret 32)
ok "PG_META_CRYPTO_KEY"

# ... 写 .env 时加:
# PG_META_CRYPTO_KEY=...
# COMPOSE_PROFILES=studio
```

---

## 11. Phase 2 实施清单（顺序）

1. ✅ 确认 PG 镜像版本升级到 15.8.1.085（Phase 1 已做）
2. setup.sh 生成 PG_META_CRYPTO_KEY + COMPOSE_PROFILES=studio
3. docker-compose.yml 加 studio + meta 服务（profiles: [studio]）
4. Caddyfile 加路由：
   - `/studio/*` → studio:3000
   - `/graphql/v1` → rest:3000/rpc/graphql
   - `/.well-known/oauth-authorization-server` → gotrue:9999
   - `/` → 选择页
5. Cookie 鉴权：
   - Admin 后端加 `/api/auth/verify` 成功时 set cookie
   - Caddy `/studio/*` 检查 cookie 存在
6. 自研 admin 前端加"选择页"组件 + "切换到 Studio"链接
7. 测试：
   - 最小版（无 COMPOSE_PROFILES）正常运行
   - 完整版（COMPOSE_PROFILES=studio）Studio 可访问
   - Studio 隐藏的 Storage/Edge/Realtime 菜单确实不显示
   - supabase-js 通过 Studio 的 connect 弹窗给出的参数能正常工作

---

## 11.5. 顺带做：envfile 严格校验（Phase 1 review 遗留）

**背景**：Phase 1 自审发现了 4 个 heredoc/变量插值类问题。其中 C1/C2 已修（`zz-supabase-lite.sh` 全改用 `psql -v`），D2 已修（`setup.sh` 路径校验 + 引号）。剩下 D1/D3/D4 都是 `envfile` 模块的防御深度问题，Phase 2 Settings 编辑时一起收紧。

### D1. `envfile.ValidateValue` 黑名单扩展

**现状**（`admin/internal/envfile/envfile.go:14`）：

```go
func ValidateValue(v string) error {
    if strings.ContainsAny(v, "\n\r") {
        return fmt.Errorf("value contains newline")
    }
    return nil
}
```

只挡 `\n\r`。应扩展为：

```go
func ValidateValue(v string) error {
    if strings.ContainsAny(v, "\n\r") {
        return fmt.Errorf("value contains newline")
    }
    // Null byte
    if strings.ContainsRune(v, 0) {
        return fmt.Errorf("value contains null byte")
    }
    // Reject values that would need quoting but we don't yet support
    // quoted writes. When D4 lands we can relax this.
    if strings.ContainsAny(v, "\"\\") {
        return fmt.Errorf("value contains unsupported characters (\" \\)")
    }
    return nil
}
```

Plus: **字段专用校验**（见 D3）。

### D3. URL / 关键字段白名单正则

目前 `ValidateValue` 对所有字段一视同仁。以下字段应有更严格校验：

| 字段 | 校验模式 |
|------|---------|
| `SITE_URL` | `^https?://[a-zA-Z0-9.\-_:/]+$` |
| `API_EXTERNAL_URL` | 同上 |
| `GOTRUE_SMTP_HOST` | `^[a-zA-Z0-9.\-]+$` |
| `GOTRUE_SMTP_PORT` | `^[0-9]{1,5}$` |
| `GOTRUE_SMTP_USER` | Email 或 allowed user format |
| `GOTRUE_SMTP_ADMIN_EMAIL` | Email 正则 |
| `GOTRUE_EXTERNAL_*_REDIRECT_URI` | URL 正则 |
| `GOTRUE_EXTERNAL_*_CLIENT_ID` | `^[A-Za-z0-9._\-]+$` |
| Secret 类（密码 / Secret） | `ValidateValue` 默认即可 |

实现位置：`admin/internal/handler/config.go` 的 `postConfig` 里，除调用 `envfile.ValidateValue` 外，再按字段名查一个 `map[string]*regexp.Regexp` 做专用校验。

### D4. `envfile.Read/Write` 支持引号格式

**现状**：

```go
// Read
m := lineRe.FindStringSubmatch(line)
if m != nil {
    values[m[1]] = m[2]  // 不剥离引号
}

// Write
lines[i] = m[1] + "=" + val  // 不加引号
```

**问题**：
- 用户填的值如果含空格 / `#` / `{}`，写出来 Docker Compose 解析不一致（和我们的 Go 读取器读到的不同）
- 读取如果用户手动在 .env 里加了 `KEY="value"`，Go 读到 `"value"`（含引号），Compose 读到 `value`

**目标行为**（和 Docker Compose V2 对齐）：

**Read**：
```go
// Strip surrounding quotes if balanced
if (strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`)) ||
   (strings.HasPrefix(v, `'`) && strings.HasSuffix(v, `'`)) {
    v = v[1 : len(v)-1]
    // 如果是双引号，处理 \" \\ \n 等转义
}
```

**Write**：按值内容决定是否加引号：
```go
func escapeForEnv(v string) string {
    // No special chars → raw
    if !strings.ContainsAny(v, " \t#\"'`$\\") {
        return v
    }
    // Has special chars → single-quote (literal, no escaping inside)
    if !strings.Contains(v, "'") {
        return "'" + v + "'"
    }
    // Has single quote → double-quote with escapes
    escaped := strings.ReplaceAll(v, `\`, `\\`)
    escaped = strings.ReplaceAll(escaped, `"`, `\"`)
    escaped = strings.ReplaceAll(escaped, "$", `\$`)
    return `"` + escaped + `"`
}
```

同时：
- `Caddyfile` 里的 `{$VAR}` 替换 Caddy 也是字面量注入，若值含 `{` `}` 会破 Caddy 语法 → 靠 **D1+D3 前置校验**挡掉
- Docker Compose `${VAR}` 替换也是字面量 → 同样靠前置校验

### 实施顺序

在 Phase 2 的 Settings / Cookie auth 工作里，按以下顺序集成：
1. 先扩展 `ValidateValue` + 字段专用正则（D1 + D3）—— 纯后端，不影响 UI
2. 再 Read/Write 支持引号格式（D4）—— 需改两端 + 回归测试
3. 前端 Settings 表单加对应的字段校验提示（UX 层）

### 验证

完成后跑以下测试：
```sql
-- 尝试提交以下值到 /api/config，应被拒绝：
SITE_URL = 'http://x.com } attack {'
GOTRUE_SMTP_HOST = 'evil$(rm -rf)'
GOTRUE_SMTP_PORT = 'not-a-number'

-- 以下应接受：
GOTRUE_SMTP_PASS = "password with 'quotes' and spaces"
GOTRUE_SMTP_ADMIN_EMAIL = 'user+tag@example.com'
```

---

## 12. 风险与回退计划

| 风险 | 概率 | 影响 | 缓解 |
|------|:----:|:----:|------|
| Studio feature flag 失效（新版本改了字段名） | 低 | 菜单都露出来 | 方案 C：CSS 注入兜底隐藏 |
| Studio 硬编码了 `kong` hostname | 低 | Studio 内部请求失败 | 在 gateway 加 `kong` alias |
| Cookie 鉴权机制在不同浏览器行为不一致 | 中 | 部分用户登不上 | fallback 到 query param auth |
| postgres-meta 没有鉴权，万一 Caddy 配错暴露了 | 低 | Studio SQL 任意执行 | 只绑定到 docker network，不 expose 到 host |
| Studio 升级新版本兼容性破坏 | 中 | 升级失败 | 镜像 tag 固定，升级前测试 |
| PG_META_CRYPTO_KEY 丢失 | 低 | Studio 保存的连接信息不可解密 | 文档说明密钥重要性、备份建议 |

---

## 13. Phase 2 之后可能的优化

（**不在 Phase 2 范围内，记录备忘**）

1. **CSS 注入兜底隐藏**：如果 feature flag 万一不生效，用 Caddy 在响应里 inject CSS 隐藏菜单项
2. **JWT cookie 鉴权**：升级到真正的 HMAC JWT cookie（Caddy 加 jwt 插件）
3. **统一 Admin + Studio 登录**：SSO 方式，用户只登录一次
4. **Studio 主题定制**：supabase-lite 品牌 logo / 颜色
5. **关闭 Studio 的 Sentry / Analytics 上报**：设置相关 env 为空
