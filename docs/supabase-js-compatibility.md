# supabase-js 客户端兼容性

> 目标：明确 supabase-lite 对 supabase-js 的兼容范围，作为 Phase 1 完成后的验收标准
> 依据：~/otheruse/supabase-js/packages/core/supabase-js/src/
> 日期：2026-04-17

---

## 1. SupabaseClient 初始化流程

```ts
import { createClient } from '@supabase/supabase-js'

const supabase = createClient(
  'http://localhost:8000',         // baseUrl
  '<ANON_KEY>'                      // supabaseKey
)
```

内部会构造这些 URL（`SupabaseClient.ts:287-291`）：

```ts
this.realtimeUrl = new URL('realtime/v1', baseUrl)
this.realtimeUrl.protocol = this.realtimeUrl.protocol.replace('http', 'ws')
this.authUrl = new URL('auth/v1', baseUrl)
this.storageUrl = new URL('storage/v1', baseUrl)
this.functionsUrl = new URL('functions/v1', baseUrl)
this.rest = new PostgrestClient(new URL('rest/v1', baseUrl).href, ...)
```

## 2. 每个请求自动带的 Headers

**源码**：`packages/core/supabase-js/src/lib/fetch.ts`

```ts
export const fetchWithAuth = (supabaseKey, getAccessToken, customFetch) => {
  return async (input, init) => {
    const accessToken = (await getAccessToken()) ?? supabaseKey
    let headers = new Headers(init?.headers)

    if (!headers.has('apikey')) {
      headers.set('apikey', supabaseKey)
    }
    if (!headers.has('Authorization')) {
      headers.set('Authorization', `Bearer ${accessToken}`)
    }

    return fetch(input, { ...init, headers })
  }
}
```

**关键行为**：
- **每个请求都带 `apikey: <anon_key>`**
- **每个请求都带 `Authorization: Bearer <token>`**
  - 未登录：`<token>` = `<anon_key>`
  - 已登录：`<token>` = 用户的 access_token（JWT）
  - access_token 过期会自动刷新

**含义**：我们 Caddy 的 `require_apikey` 规则就是对的——只校验 apikey 头。Authorization 走到 PostgREST 里自己 JWT 验证。

---

## 3. 支持的模块矩阵

### ✅ supabase.from(...).select() — 完全支持

**走路径**：`POST /rest/v1/<table>?select=...`

```ts
const { data, error } = await supabase
  .from('users')
  .select('id, email, created_at')
  .eq('id', userId)
  .single()
```

**我们的后端**：PostgREST。100% 兼容。

### ✅ supabase.from(...).insert/update/upsert/delete — 支持

```ts
await supabase.from('users').insert({ email, password })
await supabase.from('users').update({ name }).eq('id', id)
await supabase.from('users').delete().eq('id', id)
await supabase.from('users').upsert({ id, name })
```

100% 兼容。

### ✅ supabase.rpc(...) — 支持

```ts
const { data } = await supabase.rpc('my_function', { arg: 'value' })
```

走 `POST /rest/v1/rpc/my_function`，100% 兼容。

### ✅ supabase.schema(...).from(...) — 支持

```ts
const result = await supabase
  .schema('graphql_public')
  .rpc('resolve', { query, variables })
```

走 `POST /rest/v1/...` + `Content-Profile: graphql_public` header。**前提**：PG 已启用 `pg_graphql` 扩展 + `PGRST_DB_SCHEMAS=public,graphql_public`（Phase 1 会做）。

### ✅ supabase.auth.*  — 支持

```ts
// 注册
await supabase.auth.signUp({ email, password })

// 登录
await supabase.auth.signInWithPassword({ email, password })
await supabase.auth.signInWithOAuth({ provider: 'github' })
await supabase.auth.signInAnonymously()      // 需要 GoTrue 启用 anonymous

// 会话管理
await supabase.auth.getSession()
await supabase.auth.getUser()
await supabase.auth.refreshSession()
await supabase.auth.signOut()

// 密码管理
await supabase.auth.resetPasswordForEmail(email)
await supabase.auth.updateUser({ password })

// MFA
await supabase.auth.mfa.enroll({ factorType: 'totp' })
await supabase.auth.mfa.challenge(...)
await supabase.auth.mfa.verify(...)

// Session
supabase.auth.onAuthStateChange((event, session) => {...})
```

走 `/auth/v1/...`，100% 兼容（GoTrue 实现）。

### ❌ supabase.storage.* — **不支持**

```ts
// 这些调用会失败
await supabase.storage.from('avatars').upload(...)
await supabase.storage.from('avatars').download(...)
await supabase.storage.from('avatars').list(...)
```

走 `/storage/v1/...`，**我们不装 Storage API**。调用会返回 404。

### ❌ supabase.functions.invoke(...) — **不支持**

```ts
await supabase.functions.invoke('my-function', { body: {...} })
```

走 `/functions/v1/...`，**我们不装 Edge Functions**。调用会返回 404。

### ❌ supabase.realtime.channel(...).subscribe(...) — **不支持**

```ts
// 这些调用会失败
const channel = supabase.channel('my-channel')
channel.on('postgres_changes', ...).subscribe()
channel.send({ type: 'broadcast', event: 'cursor', payload })
```

走 `ws://.../realtime/v1/socket`，**我们不装 Realtime**。WebSocket 连接失败。

---

## 4. 错误处理模式

supabase-js 错误格式：

```ts
const { data, error } = await supabase.from('x').select('*')
if (error) {
  // error.code  - PostgreSQL error code
  // error.message - 错误描述
  // error.details - 详情
  // error.hint   - 建议
}
```

PostgREST 返回的 JSON 错误格式：
```json
{
  "code": "42P01",
  "details": null,
  "hint": null,
  "message": "relation \"x\" does not exist"
}
```

**supabase-lite 必须保证**：PostgREST 的错误格式不被 Caddy 篡改。已满足（Caddy 只做 proxy）。

---

## 5. RLS 与 JWT 关键点

### 5.1 supabase-js 会自动附带 JWT

已登录用户的 access_token 自动放到 `Authorization` 头：
```
Authorization: Bearer eyJhbG...（user JWT）
```

### 5.2 PostgREST 读 JWT claims

配置了 `PGRST_DB_USE_LEGACY_GUCS: "false"`，PostgREST 把整个 JWT 写入 session 的 `request.jwt.claims` GUC：

```sql
SELECT current_setting('request.jwt.claims', true)::json->>'sub';  -- 用户 ID
SELECT current_setting('request.jwt.claims', true)::json->>'role'; -- authenticated / anon / service_role
```

这就是我们 `auth.uid()` 函数读取的位置。

### 5.3 role 切换

PostgREST 根据 JWT 的 `role` claim 执行 `SET ROLE`：
- `role: "anon"` → SET ROLE anon
- `role: "authenticated"` → SET ROLE authenticated
- `role: "service_role"` → SET ROLE service_role

然后 RLS 策略生效。

### 5.4 service_role_key 绕过 RLS

客户端用 `supabase.service_role_key` 创建的 client：
```ts
const adminClient = createClient(url, SERVICE_ROLE_KEY, {
  auth: { persistSession: false }
})
// JWT 的 role = service_role，BYPASSRLS 生效
```

Phase 1 的 init SQL 要确保 service_role 有 `BYPASSRLS` 属性（镜像默认已设置）。

---

## 6. Phase 1 完成后的验收测试

用 supabase-js 客户端跑的最小测试集：

### 6.1 匿名访问 RLS 表

```ts
// 1. 建表 + RLS
await supabase.rpc('sql', { q: `
  CREATE TABLE items (id uuid DEFAULT gen_random_uuid() PRIMARY KEY, public boolean, user_id uuid);
  ALTER TABLE items ENABLE ROW LEVEL SECURITY;
  CREATE POLICY "public items" ON items FOR SELECT USING (public = true);
` })

// 2. 以 service_role 插入
const admin = createClient(URL, SERVICE_ROLE_KEY)
await admin.from('items').insert([
  { public: true },
  { public: false }
])

// 3. 匿名查询只能看到 public=true 的
const anon = createClient(URL, ANON_KEY)
const { data } = await anon.from('items').select('*')
expect(data.length).toBe(1)
expect(data[0].public).toBe(true)
```

### 6.2 Auth 登录流程

```ts
// 注册
const { user, error } = await supabase.auth.signUp({
  email: 'test@example.com',
  password: 'test123456',
})
expect(error).toBeNull()

// 登录
const { data } = await supabase.auth.signInWithPassword({
  email: 'test@example.com',
  password: 'test123456',
})
expect(data.session).toBeDefined()

// 带着登录态查 RLS 表（auth.uid() 应该返回 user.id）
await supabase.rpc('sql', { q: 'SELECT auth.uid()' })
```

### 6.3 存储过程 + 参数

```ts
await supabase.rpc('sql', { q: `
  CREATE FUNCTION add(a int, b int) RETURNS int AS $$ SELECT a + b $$ LANGUAGE sql;
` })

const { data } = await supabase.rpc('add', { a: 1, b: 2 })
expect(data).toBe(3)
```

### 6.4 pg_graphql

```ts
// 启用后，通过 GraphQL 查询
const result = await fetch('http://localhost:8000/graphql/v1', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'apikey': ANON_KEY,
    'Authorization': `Bearer ${ANON_KEY}`,
  },
  body: JSON.stringify({
    query: `query { itemsCollection { edges { node { id public } } } }`
  })
})
```

要求：
- 启用 `pg_graphql` 扩展（Phase 1 已做）
- `PGRST_DB_SCHEMAS=public,graphql_public`（Phase 1 已做）
- Caddy 路由 `/graphql/v1` → `/rpc/graphql`（Phase 2 做）

### 6.5 预期失败测试

```ts
// Storage 应该 404
const { error } = await supabase.storage.from('x').upload('a.txt', 'data')
expect(error?.message).toMatch(/404|not found/i)

// Edge Functions 应该 404
const result = await supabase.functions.invoke('hello')
expect(result.error).toBeDefined()

// Realtime 应该连接失败
const channel = supabase.channel('test')
channel.subscribe((status) => {
  if (status === 'CHANNEL_ERROR') {
    // 预期行为
  }
})
```

---

## 7. 对"最简单的自托管 Supabase"定位的含义

| 功能 | 支持 | 价值 |
|------|:----:|------|
| Postgres + RLS + SQL | ✅ | 核心 |
| Auto REST API (PostgREST) | ✅ | 核心 |
| 鉴权 (GoTrue) | ✅ | 核心 |
| GraphQL (pg_graphql) | ✅ | 加分 |
| RPC | ✅ | 核心 |
| OAuth 三方登录 | ✅ | 常用 |
| 匿名会话 | ✅ | 常用 |
| MFA / TOTP | ✅ | 加分 |
| 对象存储 | ❌ | 推荐 Pigsty/官方 |
| Edge Functions | ❌ | 推荐别的 |
| Realtime 订阅 | ❌ | 推荐 Pigsty/官方 |

**结论**：**对于 80% 的"Supabase 入门 / 侧项目 / 简单 SaaS"场景**，我们的 Phase 1 已经足够。想要更全的场景建议用户用 Pigsty 或官方自托管。

---

## 8. 给用户的明确沟通

在 README 和文档里：

### 支持
```
✅ Database: PostgreSQL 15 + 15 Supabase 扩展
✅ Auto REST API: PostgREST (via /rest/v1/)
✅ Authentication: GoTrue (via /auth/v1/)
   - Email / Password
   - OAuth: GitHub / Google / Apple
   - Anonymous sign-in
   - Password reset, MFA
✅ GraphQL: pg_graphql (via /graphql/v1/)
✅ Row Level Security (RLS)
✅ RPC (stored procedures)
✅ supabase-js v2 客户端兼容
```

### 不支持
```
❌ Realtime (WebSocket subscriptions) — 用 Pigsty 或官方自托管
❌ Storage (Object storage) — 同上
❌ Edge Functions (Deno runtime) — 同上
❌ Analytics / Logflare — 同上
```

### 验证命令

```bash
# 1. 确认能跑 basic SELECT
curl 'http://localhost:8000/rest/v1/' \
  -H "apikey: $ANON_KEY" \
  -H "Authorization: Bearer $ANON_KEY" | head -20

# 2. 确认 auth
curl 'http://localhost:8000/auth/v1/health' | jq

# 3. 用 supabase-js 跑示例项目
npm create @supabase/app
# ... 编辑 URL/Key 指向本地
npm run dev
```

---

## 9. 开发者接入示例

### 9.1 JavaScript / Node.js

```ts
import { createClient } from '@supabase/supabase-js'

const supabase = createClient(
  'http://localhost:8000',
  '<ANON_KEY>'
)

// 只要不碰 storage / functions / realtime 就跟云上一样
const { data, error } = await supabase
  .from('users')
  .select('*')
```

### 9.2 Python

```python
from supabase import create_client, Client

supabase: Client = create_client(
    'http://localhost:8000',
    '<ANON_KEY>'
)

data = supabase.table('users').select('*').execute()
```

### 9.3 curl

```bash
curl 'http://localhost:8000/rest/v1/users?select=*' \
  -H "apikey: $ANON_KEY" \
  -H "Authorization: Bearer $ANON_KEY"
```

---

## 10. 已知限制

1. **无 migrations 工具**：我们不提供 `supabase db push`。用户用 `psql` 或 Studio SQL Editor 改 schema
2. **无 type 生成 CLI**：`supabase gen types typescript` 不能用（我们有 postgres-meta 但没 Supabase CLI 对接）。用户可直接调 `http://localhost:8000/pg/generators/typescript`（如果 Phase 2 暴露了）
3. **无 Vault UI**：`supabase_vault` 扩展的 UI 在 Studio 里（Phase 2 启用后可用）
