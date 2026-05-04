# Phase 4: SSE 实现调研

> 目标：用 Server-Sent Events 实现实时日志流、状态推送、操作进度
> 依据：Go 标准库 net/http、Caddy reverse_proxy 文档
> 日期：2026-04-17

---

## 1. 为什么选 SSE 而不是 WebSocket

| 维度 | SSE | WebSocket |
|------|-----|-----------|
| 方向 | 服务端 → 客户端（单向） | 双向 |
| 协议 | 纯 HTTP（easy proxy） | HTTP Upgrade 到 WS |
| 浏览器 API | EventSource（内置） | WebSocket（内置） |
| 自动重连 | ✅ 浏览器自动重连 | ❌ 需自己实现 |
| 鉴权 header | ❌ EventSource 不能带自定义 header | ✅ 可带 |
| Go 标准库支持 | ✅ net/http + Flusher | ❌ 需 gorilla/websocket 等库 |
| Caddy 代理 | ✅ 自动识别 text/event-stream 不缓冲 | ✅ 原生支持 upgrade |
| 实现复杂度 | 🟢 极低 | 🟡 中 |

**supabase-lite 场景**：日志 / 状态 / 进度都是服务端推送，不需要客户端 → 服务端的交互通道。**SSE 是最佳选择**。

---

## 2. Caddy 透传 SSE 的关键

### 2.1 自动识别机制

Caddy `reverse_proxy` **自动识别 `Content-Type: text/event-stream`**，并：
- 立即 flush 响应到客户端（不缓冲）
- 即使客户端早早断开，也不取消后端请求

> "Caddy automatically detects and handles SSE responses. When a response has the header `Content-Type: text/event-stream`, Caddy flushes immediately to the client and does not cancel the request to the backend even if the client disconnects early."

### 2.2 保险起见的 flush_interval

如果不相信自动识别，可以显式禁用缓冲：

```caddy
handle /api/logs/stream {
    reverse_proxy admin:9100 {
        flush_interval -1   # -1 = 每次写完立即 flush（low-latency 模式）
    }
}
```

**Phase 4 策略**：依赖自动识别为主，Caddyfile 里不加 `flush_interval`。**除非**实测有缓冲问题再加。

### 2.3 HTTP/2 行为

Caddy 默认支持 HTTP/2。HTTP/2 流式响应会**跳过缓冲，立即传输**，进一步改善延迟。

> "HTTP/2 streams receive automatic flushing treatment: responses with unknown `Content-Length` on HTTP/2 connections skip buffering."

---

## 3. Go SSE server 实现

### 3.1 最简骨架

```go
func (h *Handler) HandleLogStream(w http.ResponseWriter, r *http.Request) {
    // SSE 必需 headers
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("X-Accel-Buffering", "no")  // 告诉 nginx/caddy 别缓冲
    
    // 确保能 flush
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
        return
    }
    
    // 客户端断连通知
    ctx := r.Context()
    
    // 订阅日志源
    logs := h.LogHub.Subscribe(r.URL.Query().Get("service"))
    defer h.LogHub.Unsubscribe(logs)
    
    // 先发 replay buffer
    for _, line := range h.LogHub.Replay(r.URL.Query().Get("service"), 500) {
        fmt.Fprintf(w, "data: %s\n\n", line)
    }
    flusher.Flush()
    
    // 循环发实时日志
    for {
        select {
        case <-ctx.Done():
            return  // 客户端断开
        case line := <-logs:
            fmt.Fprintf(w, "data: %s\n\n", line)
            flusher.Flush()
        case <-time.After(30 * time.Second):
            // 心跳，防止中间代理超时
            fmt.Fprintf(w, ": keepalive\n\n")
            flusher.Flush()
        }
    }
}
```

### 3.2 消息格式

SSE 格式很简单：

```
data: {"type":"log","service":"gotrue","line":"INFO starting server"}\n
\n
```

注意：**每条消息后两个 `\n`**（一个结束当前行，一个结束消息）。

进阶格式（带事件类型和 ID）：

```
event: log
id: 123
data: {"service":"gotrue","line":"..."}

event: status
id: 124
data: {"service":"rest","state":"running"}
```

客户端通过 `event.type` 区分，`event.lastEventId` 用于重连回溯。

### 3.3 Ring Buffer（日志缓冲）

```go
package logstream

import "sync"

type RingBuffer struct {
    mu       sync.RWMutex
    items    []string
    head     int
    size     int
    capacity int
}

func NewRingBuffer(capacity int) *RingBuffer {
    return &RingBuffer{
        items:    make([]string, capacity),
        capacity: capacity,
    }
}

func (rb *RingBuffer) Push(item string) {
    rb.mu.Lock()
    defer rb.mu.Unlock()
    rb.items[rb.head] = item
    rb.head = (rb.head + 1) % rb.capacity
    if rb.size < rb.capacity {
        rb.size++
    }
}

func (rb *RingBuffer) Snapshot() []string {
    rb.mu.RLock()
    defer rb.mu.RUnlock()
    out := make([]string, 0, rb.size)
    start := (rb.head - rb.size + rb.capacity) % rb.capacity
    for i := 0; i < rb.size; i++ {
        out = append(out, rb.items[(start+i)%rb.capacity])
    }
    return out
}
```

### 3.4 LogHub（订阅广播）

```go
type LogHub struct {
    mu          sync.Mutex
    buffers     map[string]*RingBuffer           // service → buffer
    subscribers map[string]map[chan string]bool  // service → subscribers
    streams     map[string]context.CancelFunc    // 管理 docker log stream 生命周期
    docker      *docker.Client
}

func (h *LogHub) Subscribe(service string) chan string {
    h.mu.Lock()
    defer h.mu.Unlock()
    
    ch := make(chan string, 100)
    
    if h.subscribers[service] == nil {
        h.subscribers[service] = make(map[chan string]bool)
        h.buffers[service] = NewRingBuffer(500)
        h.startDockerLogStream(service)  // 第一个订阅者启动 docker logs -f
    }
    h.subscribers[service][ch] = true
    return ch
}

func (h *LogHub) Unsubscribe(ch chan string) {
    h.mu.Lock()
    defer h.mu.Unlock()
    
    for svc, subs := range h.subscribers {
        if _, ok := subs[ch]; ok {
            delete(subs, ch)
            close(ch)
            
            if len(subs) == 0 {
                // 最后一个订阅者退出，停止 docker log stream
                h.streams[svc]()  // cancel context
                delete(h.subscribers, svc)
                delete(h.buffers, svc)
                delete(h.streams, svc)
            }
            return
        }
    }
}

func (h *LogHub) Publish(service, line string) {
    h.mu.Lock()
    defer h.mu.Unlock()
    
    h.buffers[service].Push(line)
    for ch := range h.subscribers[service] {
        select {
        case ch <- line:
        default:
            // 订阅者太慢，丢弃消息（不阻塞）
        }
    }
}

func (h *LogHub) Replay(service string, maxLines int) []string {
    h.mu.Lock()
    defer h.mu.Unlock()
    if rb, ok := h.buffers[service]; ok {
        return rb.Snapshot()
    }
    return nil
}

func (h *LogHub) startDockerLogStream(service string) {
    ctx, cancel := context.WithCancel(context.Background())
    h.streams[service] = cancel
    
    go func() {
        id, _ := h.docker.FindContainerID(service)
        reader, err := h.docker.StreamLogs(ctx, id)
        if err != nil { return }
        defer reader.Close()
        
        scanner := bufio.NewScanner(reader)
        for scanner.Scan() {
            line := scanner.Text()
            h.Publish(service, line)
        }
    }()
}
```

### 3.5 Docker Engine API: follow logs

扩展现有的 `internal/docker/docker.go`：

```go
func (c *Client) StreamLogs(ctx context.Context, containerID string) (io.ReadCloser, error) {
    url := fmt.Sprintf("http://docker/containers/%s/logs?follow=true&stdout=true&stderr=true&tail=0", containerID)
    req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
    resp, err := c.http.Do(req)
    if err != nil {
        return nil, err
    }
    if resp.StatusCode >= 300 {
        body, _ := io.ReadAll(resp.Body)
        resp.Body.Close()
        return nil, fmt.Errorf("logs failed (%d): %s", resp.StatusCode, body)
    }
    // 返回 reader，调用方需要自己 strip 8-byte headers
    return &dockerLogReader{r: resp.Body}, nil
}

// dockerLogReader 在读取时逐帧剥离 Docker multiplex header
type dockerLogReader struct {
    r       io.ReadCloser
    remain  int     // 当前帧剩余字节
    header  [8]byte
}

func (d *dockerLogReader) Read(p []byte) (int, error) {
    for {
        if d.remain > 0 {
            toRead := len(p)
            if toRead > d.remain {
                toRead = d.remain
            }
            n, err := d.r.Read(p[:toRead])
            d.remain -= n
            return n, err
        }
        // 读下一个 8-byte header
        if _, err := io.ReadFull(d.r, d.header[:]); err != nil {
            return 0, err
        }
        d.remain = int(binary.BigEndian.Uint32(d.header[4:8]))
    }
}

func (d *dockerLogReader) Close() error { return d.r.Close() }
```

---

## 4. EventSource 断线重连

### 4.1 浏览器默认行为

```js
const es = new EventSource('/admin/api/logs/stream?service=gotrue')

es.onmessage = (ev) => {
    const data = JSON.parse(ev.data)
    // 显示日志
}

es.onerror = (err) => {
    // 浏览器会自动重连（默认 3 秒）
}

// 关闭
es.close()
```

**默认重连间隔**：3 秒。

### 4.2 服务端控制重连间隔

服务端可以在任何时候发一条特殊消息告诉客户端重连间隔：

```go
fmt.Fprintf(w, "retry: 5000\n\n")  // 5 秒
flusher.Flush()
```

### 4.3 Last-Event-ID 回溯

每条消息可以带 id，断线后客户端重连时会发 `Last-Event-ID` header：

```go
// 发送带 ID 的消息
fmt.Fprintf(w, "id: %d\ndata: %s\n\n", msgID, line)

// 重连时读取 header
lastID := r.Header.Get("Last-Event-ID")
// 从 RingBuffer 找大于 lastID 的消息 replay
```

**Phase 4 策略**：**先不实现 Last-Event-ID**，因为日志场景允许少量丢失（重启后 RingBuffer 也会丢）。未来需要时再加。

---

## 5. Cookie 鉴权（EventSource 限制的解法）

### 5.1 问题

`EventSource` 不能设置自定义 header，所以不能发 `Authorization: Bearer <token>`。

### 5.2 方案：Cookie

Phase 2 的 Cookie 方案天然适用：
- 用户登录 admin 后，cookie `sbl_auth` 已设置
- EventSource 请求自动带 cookie（同源）
- Go 后端从 cookie 取 token 校验

```go
func (s *Server) authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 优先 Bearer header（API 调用）
        auth := r.Header.Get("Authorization")
        if strings.HasPrefix(auth, "Bearer ") {
            token := strings.TrimPrefix(auth, "Bearer ")
            if subtle.ConstantTimeCompare([]byte(token), []byte(s.adminToken)) == 1 {
                next.ServeHTTP(w, r)
                return
            }
        }
        
        // 回退 cookie（SSE / WS）
        if c, err := r.Cookie("sbl_auth"); err == nil {
            if subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.adminToken)) == 1 {
                next.ServeHTTP(w, r)
                return
            }
        }
        
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
    })
}
```

**安全考虑**：cookie 必须是 `HttpOnly + Secure + SameSite=Strict`。

---

## 6. 前端 API 封装

```tsx
// admin/web/src/lib/sse.ts
export function subscribeLogs(
    service: string,
    onLine: (line: string) => void,
    onError?: (err: Event) => void
): () => void {
    const es = new EventSource(`/admin/api/logs/stream?service=${service}`)
    
    es.onmessage = (ev) => {
        onLine(ev.data)
    }
    
    if (onError) {
        es.onerror = onError
    }
    
    return () => es.close()
}
```

使用：

```tsx
function LogsPage() {
    const [lines, setLines] = useState<string[]>([])
    const [service, setService] = useState('gotrue')
    
    useEffect(() => {
        const unsubscribe = subscribeLogs(service, (line) => {
            setLines(prev => [...prev.slice(-1000), line])
        })
        return unsubscribe
    }, [service])
    
    return <LogViewer lines={lines} />
}
```

---

## 7. 状态推送 SSE（同样模式）

```go
func (h *Handler) HandleStatusStream(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    
    flusher := w.(http.Flusher)
    ctx := r.Context()
    ticker := time.NewTicker(3 * time.Second)
    defer ticker.Stop()
    
    var lastHash string
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            containers, _ := h.docker.ListContainers()
            data, _ := json.Marshal(containers)
            hash := fmt.Sprintf("%x", sha256.Sum256(data))
            
            if hash != lastHash {
                fmt.Fprintf(w, "data: %s\n\n", data)
                flusher.Flush()
                lastHash = hash
            }
        }
    }
}
```

前端：

```tsx
useEffect(() => {
    const es = new EventSource('/admin/api/status/stream')
    es.onmessage = (ev) => {
        setServices(JSON.parse(ev.data))
    }
    return () => es.close()
}, [])
```

---

## 8. 操作进度 SSE（重启等长操作）

### 8.1 Operation ID 模式

```go
type Operation struct {
    ID      string
    Type    string
    Status  string  // "pending" | "running" | "completed" | "failed"
    Output  *RingBuffer
    Done    chan struct{}
    Err     error
    Started time.Time
}

type OperationManager struct {
    mu  sync.Mutex
    ops map[string]*Operation
}

func (m *OperationManager) Start(opType string, fn func(op *Operation)) string {
    opID := generateID()
    op := &Operation{
        ID:      opID,
        Type:    opType,
        Status:  "running",
        Output:  NewRingBuffer(1000),
        Done:    make(chan struct{}),
        Started: time.Now(),
    }
    
    m.mu.Lock()
    m.ops[opID] = op
    m.mu.Unlock()
    
    go func() {
        defer close(op.Done)
        fn(op)
    }()
    
    // 5 分钟后清理
    time.AfterFunc(5*time.Minute, func() {
        m.mu.Lock()
        delete(m.ops, opID)
        m.mu.Unlock()
    })
    
    return opID
}
```

### 8.2 Restart handler

```go
func (h *Handler) HandleRestart(w http.ResponseWriter, r *http.Request) {
    opID := h.OpMgr.Start("restart", func(op *Operation) {
        cmd := exec.Command("docker", "compose", ...)
        stdout, _ := cmd.StdoutPipe()
        go func() {
            scanner := bufio.NewScanner(stdout)
            for scanner.Scan() {
                op.Output.Push(scanner.Text())
            }
        }()
        
        if err := cmd.Run(); err != nil {
            op.Status = "failed"
            op.Err = err
        } else {
            op.Status = "completed"
        }
    })
    
    writeJSON(w, 202, map[string]string{"operation_id": opID})
}
```

### 8.3 Stream operation progress

```go
func (h *Handler) HandleOperationStream(w http.ResponseWriter, r *http.Request) {
    opID := r.PathValue("id")
    op := h.OpMgr.Get(opID)
    if op == nil {
        http.Error(w, "not found", 404)
        return
    }
    
    w.Header().Set("Content-Type", "text/event-stream")
    flusher := w.(http.Flusher)
    
    // 发已有输出
    for _, line := range op.Output.Snapshot() {
        fmt.Fprintf(w, "event: output\ndata: %s\n\n", line)
    }
    flusher.Flush()
    
    // 订阅新输出...（或简单轮询 Output，看它是否变长）
    lastLen := len(op.Output.Snapshot())
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()
    
    for {
        select {
        case <-r.Context().Done():
            return
        case <-op.Done:
            // 发最终状态
            fmt.Fprintf(w, "event: done\ndata: %s\n\n", op.Status)
            flusher.Flush()
            return
        case <-ticker.C:
            snap := op.Output.Snapshot()
            if len(snap) > lastLen {
                for _, line := range snap[lastLen:] {
                    fmt.Fprintf(w, "event: output\ndata: %s\n\n", line)
                }
                flusher.Flush()
                lastLen = len(snap)
            }
        }
    }
}
```

### 8.4 前端

```tsx
async function triggerRestart() {
    // 1. POST 启动操作
    const { operation_id } = await fetch('/admin/api/restart', { method: 'POST' })
        .then(r => r.json())
    
    // 2. 打开 SSE
    const es = new EventSource(`/admin/api/operations/${operation_id}/stream`)
    
    es.addEventListener('output', (ev) => {
        appendLog(ev.data)
    })
    
    es.addEventListener('done', (ev) => {
        if (ev.data === 'completed') {
            toast.success('Restart successful')
        } else {
            toast.error('Restart failed')
        }
        es.close()
    })
    
    es.onerror = () => {
        // gateway 重启中可能断连，5 秒后重试
        es.close()
        setTimeout(() => triggerRestart(), 5000)
    }
}
```

---

## 9. Phase 4 实施清单

1. **internal/logstream/**:
   - `ringbuffer.go`: 环形缓冲
   - `hub.go`: 订阅广播 + Docker log stream 管理
2. **internal/docker/**:
   - 扩展 `StreamLogs(ctx, containerID) (io.ReadCloser, error)`
3. **internal/ops/**:
   - `manager.go`: Operation manager
4. **internal/server/**:
   - 中间件加 cookie 鉴权分支
5. **handler/**:
   - `logs_stream.go`: SSE /api/logs/stream
   - `status_stream.go`: SSE /api/status/stream
   - `operations.go`: GET /api/operations/{id}/stream
   - 改 `restart.go` 返回 operation_id
6. **前端**:
   - `lib/sse.ts`: EventSource 封装
   - `components/log-viewer.tsx`: 重写为 SSE 订阅
   - `components/operation-progress.tsx`: 新组件，显示操作进度
   - Dashboard 状态卡片改为 SSE 订阅
7. **Caddyfile**:
   - 无需改动（自动识别 SSE）

---

## 10. 验证测试

### 10.1 SSE 基本功能
```bash
# 应该流式打印日志
curl -N 'http://localhost:8000/admin/api/logs/stream?service=gotrue' \
  -H "Cookie: sbl_auth=<token>"
```

### 10.2 多订阅者共享
开两个浏览器标签访问 `/admin/logs`，看是否同步刷新（验证 fan-out）。

### 10.3 Caddy 不缓冲
打开 DevTools Network tab，看 `/api/logs/stream` 请求是"Pending"状态，数据陆续到达（不是一次性）。

### 10.4 断线重连
停止 admin 容器 10 秒再启动，前端应该自动重连恢复。

### 10.5 Operation 场景
点"Restart Services"，观察：
- 立即返回 operation_id
- SSE 实时推送 docker compose up 的 stdout
- 即使 gateway 重启中也能重连拿到最终状态

---

## 11. 坑与注意

| 坑 | 说明 | 解决 |
|---|------|------|
| **浏览器 tab 隐藏时 SSE 可能被 throttle** | 后台 tab 节流 | 接受这一行为 |
| **EventSource 无法设 Authorization header** | API 限制 | 用 Cookie（Phase 2 已做） |
| **Caddy 默认 read_timeout 30s** | 长空闲会被断 | 加心跳 `: keepalive\n\n` 每 30 秒发一次 |
| **客户端断开服务端需要感知** | 否则 goroutine 泄漏 | 监听 `r.Context().Done()` |
| **订阅者 channel buffer 满了丢消息** | 慢消费者 | 设计上允许丢（日志场景）+ 有 replay buffer |
| **Docker log follow 可能返回部分帧** | API 行为 | 我们的 dockerLogReader 已处理 |
| **同一容器多个订阅者启动多个 follow** | 浪费 | LogHub 统一管理，1 容器 1 follow |
