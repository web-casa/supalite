# Portainer 参考分析

> 参考项目：https://github.com/portainer/portainer
> 本地路径：`~/otheruse/portainer`
> 分析日期：2026-04-17

## 1. 项目定位

Portainer 是一个**企业级**容器管理平台，管理 Docker / Docker Swarm / Kubernetes / ACI / Nomad 等多种编排环境。与 Dockge 主要区别：

| 维度 | Dockge | Portainer |
|------|--------|-----------|
| 定位 | 个人 / 小团队 Compose 管理 | 企业级多编排平台 |
| Orchestrators | Compose | Docker + Swarm + K8s + ACI + Nomad |
| 多主机 | Agent | Agent + Edge Agent + 反向隧道 |
| RBAC | 基础用户 | Teams / Roles / Resource Controls |
| 规模 | ~5k 行 TS | ~22k 行 Go + 大量 React |

**与 supabase-lite 的关系**：Portainer 的 90% 能力（多编排、多主机、RBAC、Git、LDAP、OAuth）对 supabase-lite 都过剩。但它的**工程模式**（中间件链、依赖注入、事务抽象、WebSocket 劫持）是企业级 Go Web 项目的好范本。

## 2. 技术栈

### 后端 (Go)
| 组件 | 选型 |
|------|------|
| HTTP 路由 | `gorilla/mux` + 自定义中间件 |
| Docker 交互 | 官方 `docker/docker` SDK |
| DB | BoltDB（KV 存储）+ 事务抽象 |
| 日志 | `rs/zerolog` 结构化日志 |
| WebSocket | `gorilla/websocket` |
| 反向隧道 | `jpillora/chisel`（Edge Agent） |
| CSRF | `gorilla/csrf` |
| 鉴权 | JWT + API Key + Session Cookie 三种并存 |

### 前端 (React)
| 组件 | 选型 |
|------|------|
| 数据层 | TanStack React Query v4 |
| 状态 | Zustand |
| HTTP 客户端 | Axios + interceptor |
| UI | 自研组件 + Radix UI + Lucide |
| 路由 | UI Router（从 AngularJS 迁移中） |

## 3. 核心架构

### 3.1 Docker 客户端工厂（多环境抽象）

**文件**：`api/docker/client/client.go`

```go
type ClientFactory struct {
    signatureService     portainer.DigitalSignatureService
    reverseTunnelService portainer.ReverseTunnelService
}

func (factory *ClientFactory) CreateClient(endpoint *portainer.Endpoint, nodeName string, timeout *time.Duration) (*client.Client, error)
```

四种 endpoint 类型统一抽象：
1. **Local** — Unix socket / npipe
2. **TCP** — 远程 Docker daemon + TLS
3. **Agent** — 经 Portainer agent + 签名头
4. **EdgeAgent** — 经反向隧道 + agent

**关键模式**：请求时通过 `X-Portainer-Agent-Target` header 路由到具体 Swarm node。

### 3.2 WebSocket + HTTP 劫持（Docker exec/attach 实时交互）

**文件**：`api/http/handler/websocket/exec.go`、`api/ws/hijack.go`

这是 Portainer 最精彩的部分——如何让浏览器实时双向交互容器终端：

```go
func HijackRequest(wsConn *websocket.Conn, conn net.Conn, request *http.Request) error {
    resp, err := sendHTTPRequest(conn, request) // 发 HTTP 101 Upgrade
    if resp.StatusCode != http.StatusSwitchingProtocols {
        return fmt.Errorf("unexpected: %d", resp.StatusCode)
    }
    // 双向流
    go StreamFromWebsocketToWriter(wsConn, conn, errChan)
    go WriteReaderToWebSocket(wsConn, &mu, conn, errChan)
}

func WriteReaderToWebSocket(ws *websocket.Conn, mu *sync.Mutex, reader io.Reader, errChan chan error) {
    pingTicker := time.NewTicker(PingPeriod) // 50s keep-alive
    go func() {
        for {
            n, _ := reader.Read(out)
            input <- string(out[:n])
        }
    }()
    for {
        select {
        case msg := <-input:
            wsWrite(ws, mu, msg)       // 加 mutex 防止 WS 并发写
        case <-pingTicker.C:
            wsPing(ws, mu)             // 保活
        }
    }
}
```

**关键点**：
- HTTP 101 Switching Protocols 劫持底层 TCP 连接
- Docker daemon 的 exec/attach API 本身支持 hijack 双向流
- WebSocket 写有互斥锁（WS 规范要求同时只有一个写者）
- 周期性 ping 保活（穿透 proxy 默认 60s idle timeout）

### 3.3 Bouncer 中间件（鉴权 + 授权链）

**文件**：`api/http/security/bouncer.go`

```go
type BouncerService interface {
    PublicAccess(http.Handler) http.Handler       // 无需鉴权
    AdminAccess(http.Handler) http.Handler        // 仅管理员
    RestrictedAccess(http.Handler) http.Handler   // 按资源权限
    AuthenticatedAccess(http.Handler) http.Handler
    AuthorizedEndpointOperation(*http.Request, *portainer.Endpoint) error
    CookieAuthLookup(*http.Request) (*portainer.TokenData, error)
    JWTAuthLookup(*http.Request) (*portainer.TokenData, error)
}
```

**三种 token 并存**：
- **JWT** (`Authorization: Bearer ...`) — API 客户端
- **API Key** (`X-API-KEY: ...`) — 脚本 / CI
- **Session Cookie** — Web UI（SSE/WS 友好）

**使用模式**：
```go
router.Use(bouncer.AuthenticatedAccess,
          middlewares.CheckEndpointAuthorization(bouncer))
router.Use(middlewares.WithEndpoint(dataStore.Endpoint(), "id"))
```

### 3.4 Endpoint Context 注入中间件

**文件**：`api/http/middlewares/endpoint.go`

```go
func WithEndpoint(svc EndpointService, param string) mux.MiddlewareFunc {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
            endpointID := request.GetParam(param)
            endpoint, _ := svc.Endpoint(portainer.EndpointID(endpointID))
            ctx := context.WithValue(r.Context(), contextEndpoint, endpoint)
            next.ServeHTTP(rw, r.WithContext(ctx))
        })
    }
}

func FetchEndpoint(r *http.Request) (*portainer.Endpoint, error) {
    return r.Context().Value(contextEndpoint).(*portainer.Endpoint), nil
}
```

**模式**：URL 参数 → 中间件查 DB → 注入 context → handler 通过 `FetchEndpoint(r)` 获取。handler 代码零样板。

### 3.5 数据库事务抽象

**文件**：`api/connection.go`、`api/dataservices/endpoint/endpoint.go`

```go
type Connection interface {
    ViewTx(fn func(Transaction) error) error      // 只读事务
    UpdateTx(fn func(Transaction) error) error    // 写事务
}

func (s *Service) Endpoint(ID portainer.EndpointID) (*portainer.Endpoint, error) {
    err := s.connection.ViewTx(func(tx portainer.Transaction) error {
        endpoint, err = s.Tx(tx).Endpoint(ID)
        return err
    })
    return endpoint, err
}
```

**关键点**：
- `ViewTx` / `UpdateTx` 区分读写意图，底层 BoltDB 自然分读写锁
- 所有数据访问都经事务 → 一致性保证
- Service 层只关心业务逻辑，事务边界由调用者控制

### 3.6 依赖注入（构造器注入）

**文件**：`api/http/handler/docker/containers/handler.go`

```go
type Handler struct {
    *mux.Router
    dockerClientFactory *dockerclient.ClientFactory
    dataStore           dataservices.DataStore
    containerService    *docker.ContainerService
    bouncer             security.BouncerService
}

func NewHandler(routePrefix string, bouncer security.BouncerService,
                dataStore dataservices.DataStore,
                dockerClientFactory *dockerclient.ClientFactory,
                containerService *docker.ContainerService) *Handler {
    h := &Handler{ /* ... */ }
    router := h.PathPrefix(routePrefix).Subrouter()
    router.Use(bouncer.AuthenticatedAccess, middlewares.CheckEndpointAuthorization(bouncer))
    return h
}
```

**模式**：
- 每个 handler 是一个包，`NewHandler(deps...)` 构造
- `main.go` / wiring 层统一装配
- 没有全局单例
- 测试时可 mock 每个依赖

### 3.7 前端服务 + React Query

**文件**：`app/react/docker/containers/containers.service.ts`、`queries/`

```typescript
// 1. 服务层：纯 API 调用 + 错误解析
export async function startContainer(envId, id, opts) {
  try {
    await axios.post<void>(buildDockerProxyUrl(envId, 'containers', id, 'start'), {});
  } catch (e) {
    throw parseAxiosError(e, 'Failed starting container');
  }
}

// 2. Query 层：缓存 + 失效
export function useContainer(envId, containerId) {
  return useQuery({
    queryKey: queryKeys.container(envId, containerId),
    queryFn: () => getContainer(envId, containerId),
  });
}

export function useStartContainer() {
  return useMutation({
    mutationFn: startContainer,
    onSuccess: (_, { envId }) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.containers(envId) });
    },
  });
}

// 3. Key 层：层级化 key，便于批量失效
const queryKeys = {
  list: () => ['containers'],
  containers: (envId) => [...list(), envId],
  container: (envId, id) => [...containers(envId), id],
};
```

**关键点**：三层分离（service / query / keys），mutation 自动 invalidate 相关 query。

### 3.8 CSRF 保护

**文件**：`api/http/csrf/csrf.go`

```go
handler = gcsrf.Protect(
    token,
    gcsrf.Path("/"),
    gcsrf.TrustedOrigins(trustedOrigins),
)(handler)

// Skip CSRF for API keys (they're not from browsers)
func ShouldSkipCSRFCheck(r *http.Request, isDockerDesktopExtension bool) (bool, error) {
    if r.Header.Get("X-API-KEY") != "" { return true, nil }
    // ...
}
```

**关键**：Session Cookie auth 下必须有 CSRF token；Bearer/API Key auth 可跳过（它们不会被浏览器自动附带）。

### 3.9 Rate Limiting

```go
h.Handle("/auth",
    rateLimiter.LimitAccess(bouncer.PublicAccess(...))
).Methods(POST)
```

登录端点单独限流，防止暴力破解。

### 3.10 签名鉴权到 Agent

**文件**：`api/docker/client/client.go:54`

```go
signature, _ := signatureService.CreateSignature(portainer.PortainerAgentSignatureMessage)
headers := map[string]string{
    portainer.PortainerAgentPublicKeyHeader: signatureService.EncodedPublicKey(),
    portainer.PortainerAgentSignatureHeader: signature,
    portainer.PortainerAgentTargetHeader:    nodeName,
}
```

Portainer server → agent 用非对称签名：server 持私钥签名一条固定 message，agent 用预置公钥验签。比对称 token 更安全（公钥不怕泄露）。

## 4. 值得借鉴的模式

### ✅ 适合借鉴

| 模式 | 价值 | 何时用 |
|------|------|--------|
| **Bouncer / Middleware 链** | 鉴权逻辑集中 | supabase-lite 加 cookie auth 时顺便重构 |
| **Context 注入中间件** | handler 零样板 | 比如查 DB 得到 user，注入 context |
| **ViewTx/UpdateTx 抽象** | DB 操作显式读写边界 | SQL Runner 已经用 pgx 事务，可泛化 |
| **构造器依赖注入** | 清晰可测 | 未来多模块时采用 |
| **HTTP 101 劫持实现 WS-Docker 双向流** | 交互终端必备 | 如果要加 "psql console" 或 "container shell" |
| **CSRF skip by auth type** | 不同 token 类型不同 CSRF 策略 | 如果引入 cookie auth |
| **React Query 三层（service/query/keys）** | 前端数据层清晰 | 功能变复杂时 |
| **Rate limit on auth endpoint** | 防暴力破解 | 应该立刻加 |

### ❌ 不适合借鉴

| 模式 | 原因 |
|------|------|
| **多 endpoint 抽象 / 反向隧道** | supabase-lite 只管单机本地 |
| **Agent 签名鉴权** | 没有 agent |
| **Resource Controls / Teams / RBAC** | 单租户单 admin |
| **OAuth / LDAP 接入** | 场景不需要 |
| **BoltDB 事务抽象** | supabase-lite 自己就是 Postgres，不需要额外 KV |
| **Kubernetes / Swarm 编排抽象** | 不支持多编排 |
| **Git deploy / Stack versioning** | 单一固定 stack |

## 5. 对 supabase-lite 的具体建议

基于 Portainer 分析，**新增**以下可借鉴点（与 Dockge 建议合并考虑）：

### 高 ROI（建议采纳）

| # | 借鉴点 | 当前问题 | ROI | 改动量 |
|---|--------|----------|-----|--------|
| **P1** | **Auth 端点 rate limit** | `/api/auth/verify` 无限流，理论可暴力破解（虽然 48 字符 token 不现实） | ⭐⭐⭐⭐ | 低 |
| **P2** | **Cookie auth 并存** | 当前只有 Bearer header → SSE/WS 无法携带 → 阻碍实时特性 | ⭐⭐⭐⭐⭐ | 低 |
| **P3** | **Context 注入中间件** | handler 里散落 `envfile.Read(d.EnvFile)` 等重复代码 | ⭐⭐ | 低 |
| **P4** | **构造器 DI 收敛到 wiring.go** | 当前 main.go 手动 new Deps，未来加模块会变乱 | ⭐⭐ | 低 |

### 中 ROI（值得考虑）

| # | 借鉴点 | 当前问题 | ROI | 改动量 |
|---|--------|----------|-----|--------|
| **P5** | **HTTP 劫持实现 psql console / container shell** | 当前只能跑单条 SQL，无交互 session | ⭐⭐⭐ | 高 |
| **P6** | **CSRF 保护** | 有 cookie auth 后需要 | ⭐⭐⭐ | 中 |
| **P7** | **前端 React Query 三层** | 当前手动 fetch + useState，功能增加后维护累 | ⭐⭐⭐ | 中 |
| **P8** | **Service 分层（service.go / handler.go 分开）** | 当前 handler 里混合业务逻辑 | ⭐⭐ | 中 |

### 低 ROI（可跳过）

| # | 借鉴点 | 原因 |
|---|--------|------|
| **P9** | RBAC / Teams | 单 admin 场景不需要 |
| **P10** | 多 endpoint 抽象 | 单机本地 |
| **P11** | 签名鉴权 | 没有 agent |
| **P12** | Git deploy | 单一固定 stack |

## 6. 与 Dockge 分析的关系

### 共同推荐（两者都指向同一方向）

| 方向 | Dockge | Portainer | 结论 |
|------|--------|-----------|------|
| 实时日志 | WS + PTY + LimitQueue | HTTP stream + WS 劫持 | **必做**：supabase-lite 选 SSE + RingBuffer（标准库即可） |
| 操作反馈 | WS 流输出 | WS 劫持双向 | **必做**：operation_id + SSE |
| 交互终端 | xterm + PTY | xterm + HTTP 劫持 | 暂缓（两家都支持，但 supabase-lite 场景没强需求） |
| 状态推送 | 10s cron broadcast | React Query + invalidate | **必做**：后端推 SSE + 前端 Query 缓存 |
| Auth | bcrypt + JWT + password-hash proof | JWT + API Key + Session | **小改**：加 cookie 支持（为 SSE），rate limit |

### Portainer 独有的价值

| 模式 | Dockge 没有 | 价值 |
|------|------------|------|
| Middleware 链 / Context 注入 | ❌ | Go 工程规范化 |
| CSRF 保护 | ❌ | Cookie auth 必须 |
| React Query 三层 | ❌（用 socket.io） | 数据层清晰可维护 |
| HTTP 劫持双向流 | ❌（用 PTY） | 更轻量，不用 PTY 依赖 |

### Dockge 独有的价值

| 模式 | Portainer 没有 | 价值 |
|------|---------------|------|
| Terminal 多订阅（多客户端共享同一 PTY） | ❌（每人独立） | supabase-lite 场景完美契合 |
| Stack 级 LimitQueue 日志缓冲 | ❌（直连流） | 实现简单 |

## 7. 结论

**Portainer 的 90% 能力不适用**，但它的**Go 工程模式（Bouncer / 中间件链 / Context 注入 / 事务抽象）** 和 **前端数据层模式（React Query 三层）** 是大型 Go+React 项目的好范本。

对 supabase-lite 最有价值的具体动作：
1. **加 cookie 鉴权** — 为 SSE/WS 铺路（从 Portainer）
2. **加 auth rate limit** — 防暴力破解（从 Portainer）
3. **HTTP 劫持做 psql console** — 如果要做交互功能（从 Portainer）
4. **后续功能用 React Query 三层结构** — 不要手动 fetch（从 Portainer）
5. **Service 分层（handler/service/repo）** — 等业务变复杂时（从 Portainer）

和 Dockge 的建议合流后，完整的推荐清单将在 `README.md` 对比矩阵中整理。

---

## 附录：关键文件索引

| 主题 | 文件 | 要点 |
|------|------|------|
| Docker 客户端工厂 | `api/docker/client/client.go` | 四种 endpoint 统一抽象 |
| WS exec 入口 | `api/http/handler/websocket/exec.go` | 路由到 hijack 或 agent proxy |
| HTTP 劫持 | `api/ws/hijack.go` | 双向流 + mutex + ping |
| 鉴权中间件 | `api/http/security/bouncer.go` | Public/Admin/Auth/Restricted |
| 认证 | `api/http/handler/auth/authenticate.go` | JWT + API Key + timing-safe |
| Endpoint 中间件 | `api/http/middlewares/endpoint.go` | Context 注入 |
| Stack 部署 | `api/exec/compose_stack.go` | 封装 `libstack` 调 CLI |
| 隧道 | `api/chisel/service.go` | Chisel 反向隧道 + keep-alive |
| Connection 事务 | `api/connection.go` | ViewTx/UpdateTx |
| Endpoint Service | `api/dataservices/endpoint/endpoint.go` | 事务 + 心跳缓存 |
| CSRF | `api/http/csrf/csrf.go` | gorilla/csrf + skip 逻辑 |
| API Key | `api/apikey/service.go` | Hash 存储 + 过期 + 审计 |
| 前端服务 | `app/react/docker/containers/containers.service.ts` | URL builder + 错误解析 |
| 前端 Query | `app/react/docker/containers/queries/` | useQuery/useMutation + invalidate |
