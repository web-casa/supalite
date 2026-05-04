# Dockge 参考分析

> 参考项目：https://github.com/louislam/dockge
> 本地路径：`~/otheruse/dockge`
> 分析日期：2026-04-17

## 1. 项目定位

Dockge 是一个**以 stack（compose 文件）为中心**的自托管 Docker Compose 管理面板。它不追求替代 Portainer 那种"全能 Docker 管理"，而是专注：

- 可视化编辑 / 部署 / 停止 compose stack
- 实时日志流
- 每个容器的交互式 shell（xterm.js）
- 多主机（agent）支持
- 不"绑架"用户的 compose 文件（directly edit `compose.yaml` on disk）

**与 supabase-lite 的关系**：supabase-lite 本身就是一个固定的 compose stack，不需要 stack 发现/创建，但对"日志流、重启操作反馈、状态实时性"有同样的需求。

## 2. 技术栈

| 层 | 选型 |
|---|------|
| 后端 | Node.js + Express + Socket.io |
| 前端 | Vue 3 + xterm.js |
| DB | SQLite（存用户/配置） |
| 命令执行 | `node-pty` PTY spawn |
| 通信 | WebSocket (socket.io) |
| YAML | `yaml` npm 包 |

## 3. 核心架构

### 3.1 命令执行：PTY + Terminal 抽象

**文件**：`backend/terminal.ts`、`backend/stack.ts:408-419`

- 所有 `docker compose` 操作（up/down/restart/logs -f/exec）都通过 `node-pty` spawn 一个 PTY 进程
- 每个 PTY 对应一个 `Terminal` 实例，有唯一 name
- `Terminal` 内部维护 `LimitQueue`（环形缓冲，默认 100 条）保留历史输出
- 多个 socket 可订阅同一个 Terminal → 广播 PTY 输出
- 相同 name 不能并发运行 → 天然的"操作互斥锁"

```typescript
// stack.ts
async deploy(socket) {
  const terminalName = getComposeTerminalName(socket.endpoint, this.name);
  return await Terminal.exec(
    this.server, socket, terminalName,
    "docker", this.getComposeOptions("up", "-d", "--remove-orphans"),
    this.path
  );
}
```

**关键细节**：参数走数组形式（`["compose", "up", "-d"]`），无 shell 注入风险。

### 3.2 日志流：WebSocket 而非 HTTP

**流程**：
1. 后端 spawn `docker compose logs -f --tail 100`（PTY 进程）
2. PTY stdout → `LimitQueue` 缓冲 → 实时 emit 到所有订阅 socket
3. 前端 `agentSocket.on("terminalWrite")` → xterm.js 渲染
4. 新 socket 连接时先回放 buffer，再接上实时流

**优势**：
- 单个 PTY 进程服务 N 个客户端
- 客户端断连自动清理
- 保留短期历史，新连接不用从头建

### 3.3 状态检测：10s Cron 轮询

**文件**：`dockge-server.ts:399-...`

- 每 10s 执行 `docker compose ls --all --format json`
- 解析 status 字符串（如 `"exited(1), running(1)"`）
- 和上次快照比对，**变化时** 广播给所有客户端

**状态模型**（`common/util-common.ts`）：
```
UNKNOWN = 0
CREATED_FILE = 1    # compose.yaml 存在但未部署
CREATED_STACK = 2   # compose 创建了但没 running
RUNNING = 3         # ≥1 服务 running
EXITED = 4          # 部署了但全部 exited
```

### 3.4 鉴权：bcrypt + JWT（带 password-hash proof）

- 用户名 + 密码（bcrypt）
- JWT payload 含 `{username, h: shake256(passwordHash, 16)}`
- 重连时校验：重新 hash 当前密码 → 比对 → 如果密码已改，旧 JWT 失效

### 3.5 其他安全措施

- 登录限流 20/min，API 限流 60/min，2FA 限流 30/min
- 生产环境严格 CORS origin 校验（开发放开）
- docker.sock 必须挂载，所有已登录用户 = 全权限（无 per-user ACL）

## 4. 值得借鉴的模式

### ✅ 适合借鉴

| 模式 | 价值 |
|------|------|
| **流式命令输出** | 取代"发起 → 盲等 → 刷新"的 UX |
| **服务端 Ring Buffer 日志** | 单一数据源给 N 个订阅者，内存可控 |
| **后端状态轮询 + 推送** | 变化驱动，取代前端轮询 |
| **同名操作互斥** | 防止"双击重启"、"并发 up" |
| **参数数组形式调 docker** | 避免 shell 注入（我们已经这样做了 ✓） |
| **engine API label 过滤容器** | 按 compose project 识别自己管理的服务（我们已实现 ✓） |

### ❌ 不适合借鉴

| 模式 | 原因 |
|------|------|
| **PTY + xterm.js 交互终端** | 需要大量前端工作，supabase-lite 没有"进容器执行命令"的需求 |
| **WebSocket（socket.io）** | 只有服务端→客户端推送，SSE 更简单，Go 标准库即可 |
| **多 stack 管理 / 文件编辑** | supabase-lite 只有 1 个固定 stack |
| **多用户 + bcrypt + JWT** | 现在是单一 admin token，场景不需要多用户 |
| **`docker compose ls` 轮询** | 我们已经直接用 engine API 过 label 过滤 |

## 5. 对 supabase-lite 的具体建议

| # | 借鉴点 | 当前问题 | ROI | 改动量 |
|---|--------|----------|-----|--------|
| 1 | **SSE 日志流 + Ring Buffer** | `/api/logs` 一次性快照，需手动刷新 | ⭐⭐⭐⭐⭐ | 中 |
| 2 | **重启操作流（operation_id）** | 客户端收到 202 后无法知道结果 | ⭐⭐⭐⭐ | 中 |
| 3 | **后端推送状态（SSE）** | 前端 30s 轮询，重启后等 30s 才更新 | ⭐⭐⭐⭐ | 低 |
| 4 | **Engine API 直接重启** 替代 `docker compose up` shell-out | 省掉 HOST_PROJECT_DIR 体操、`docker-cli-compose` 依赖 | ⭐⭐⭐ | 低 |
| 5 | 合并为单一 `/api/events` SSE 通道 | 3 个连接变 1 个 | ⭐ | 中 |

### 关键技术决策（如果实施）

- **SSE 而非 WebSocket** — Go 标准库 `net/http` + `http.Flusher` 即可
- **Cookie auth** — EventSource 无法带 Authorization header，需要在 `/api/auth/verify` 时 set `HttpOnly` cookie
- **复用现有 Docker 客户端** — `stripDockerLogHeaders` 已实现帧解析，加 `follow=true` 即可

### 推荐执行顺序

1. #4（重启简化）— 最小、解耦后续
2. #1（日志流）— 日常价值最高，建立 SSE 基建
3. #3（状态推送）— 复用 #1 基建
4. #2（重启进度流）— 建立在 #1 和 #4 之上
5. 跳过 #5

预估工作量：~500-700 行 Go + ~150 行 TS，**零新依赖**。

## 6. 值得记录的实现细节

### LimitQueue（环形缓冲）
```typescript
// backend/utils/limit-queue.ts
// 简单的固定长度 FIFO，超过容量时丢最旧
```
**Go 等价**：`container/ring` 或手动实现 `type ringBuffer struct { items []string; head, size, cap int }`。

### Terminal 多订阅
```typescript
// terminal.ts
class Terminal {
  private _sockets: Set<DockgeSocket>;
  onData(data) {
    this.buffer.push(data);
    this._sockets.forEach(s => s.emit("terminalWrite", this.name, data));
  }
}
```
**Go 等价**：`sync.Map[connID]chan []byte`，PTY goroutine 写入所有 channel。

### Docker 日志多路复用解析
Docker 日志 API 返回的流有 8 字节 header：
```
[type(1) + padding(3) + size(4big-endian)] [payload] ...
```
我们的 `docker.go:stripDockerLogHeaders` 已经处理这个格式，流式场景下改成用 `bufio.Reader` 按帧读即可。

## 7. 结论

Dockge 最大的价值是**"PTY + WebSocket + Ring Buffer" 这套实时流式架构**。我们不需要 PTY（没有交互终端需求），但可以用 **SSE + Ring Buffer** 实现同等的"日志实时、状态实时、操作进度实时"效果，且不引入新依赖。

其他复杂能力（多 stack、多用户、文件编辑器、多主机 agent）都是 Dockge 的产品定位所需，不适用于 supabase-lite。

---

## 附录：关键文件索引

| 文件 | 作用 |
|------|------|
| `backend/stack.ts` | Stack 模型 + `docker compose` 操作封装 |
| `backend/terminal.ts` | PTY 封装 + 日志广播 |
| `backend/dockge-server.ts` | Express + Socket.io 入口 |
| `backend/utils/limit-queue.ts` | 环形缓冲实现 |
| `common/util-common.ts` | 状态常量 + YAML 解析工具 |
| `common/agent-socket.ts` | Socket 事件分发层 |
| `backend/socket-handlers/main-socket-handler.ts` | 登录/注册鉴权 |
| `backend/rate-limiter.ts` | 登录/API 限流 |
