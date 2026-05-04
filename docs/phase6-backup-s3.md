# Phase 6: 备份 + S3 集成调研文档

> 目标：定时 pg_dump + pgBackRest 增量备份 + 外部 S3 存储 + 加密 Secret Store
> 日期：2026-04-17

---

## 1. 备份方案总览

| 维度 | pg_dump | pgBackRest 增量 |
|------|---------|-----------------|
| 类型 | 逻辑备份（SQL 文本） | 物理备份（文件块） |
| 体积 | 全量每次，大 | 首次全量 + 后续增量，小 |
| 速度 | 慢 | 快（只传变化块） |
| 恢复粒度 | 整库 / 单表 / 导入到不同版本 PG | 整库（只能同版本） |
| 复杂度 | 简单 | 中等（需改 postgresql.conf） |
| 依赖 | postgresql-client | pgBackRest 二进制 + archive_command |
| 适合 | 侧项目 / 开发 | 有一定数据量 / 生产 |

**supabase-lite 策略**：**两者都支持**，用户按需选择。

---

## 2. pg_dump 方案

### 2.1 基本命令

```bash
# 全量备份（包括数据 + schema）
pg_dump -h db -p 5432 -U supabase_admin -F c -f backup.dump postgres

# 逻辑 SQL 备份（可读文本）
pg_dump -h db -p 5432 -U supabase_admin -f backup.sql postgres

# 只结构（不带数据）
pg_dump --schema-only -f schema.sql postgres

# 只数据（不带结构）
pg_dump --data-only -f data.sql postgres

# 特定 schema
pg_dump --schema=public -f public.sql postgres
```

### 2.2 Supabase 特定的排除项 ⚠️

GoTrue 的 `auth.refresh_tokens` 是用户活跃会话。**备份应该跳过这张表**：
- 恢复时旧 tokens 已经失效
- 包含敏感信息
- 频繁变动导致备份体积大

```bash
pg_dump -h db -U supabase_admin \
  --exclude-table-data=auth.refresh_tokens \
  --exclude-table-data=auth.sessions \
  --exclude-table-data=auth.mfa_amr_claims \
  --exclude-table-data=auth.mfa_challenges \
  --exclude-table-data=auth.flow_state \
  --exclude-table-data=auth.one_time_tokens \
  --exclude-table-data=auth.audit_log_entries \
  -F c -f backup.dump postgres
```

保留 `auth.users` 表数据（包含 `encrypted_password`）—— 用户仍需登录。

### 2.3 角色密码备份

```bash
# 导出 role + 密码 hash（需要 SUPERUSER 连接）
pg_dumpall -h db -U supabase_admin --globals-only --roles-only -f globals.sql
```

恢复时：
```bash
psql -h db -U supabase_admin -f globals.sql   # 先恢复角色
pg_restore -h db -U supabase_admin -d postgres backup.dump   # 再恢复数据
```

**注意**：supautils 保留角色的密码不会出现在 `pg_dumpall --globals-only` 里（出于安全）。我们知道所有角色共用 `POSTGRES_PASSWORD`，恢复时用 init 脚本的 `ALTER USER` 统一重设。

### 2.4 实施位置

**方案 A：admin 容器内执行**（推荐）
- admin 容器已经有 `postgresql-client` 工具（Dockerfile 里加）
- admin Go 后端 `exec.Command("pg_dump", ...)` 启动进程
- 管道输出到 S3 streaming upload

**方案 B：独立 backup 容器**
- 运行 cron，定时执行
- 更干净，但多一个容器

**Phase 6 推荐 A**。admin 已经是运维中心，备份功能放这里自然。

### 2.5 定时调度

admin Go 后端启一个 goroutine，读 `.env` 里的 cron 表达式：

```go
// 默认每天凌晨 2:00 UTC
BACKUP_CRON_PGDUMP="0 2 * * *"
BACKUP_RETENTION_DAYS=7
```

用 `github.com/robfig/cron/v3` 解析和调度。

### 2.6 保留策略

```go
// 每次备份完，列出远端所有 backup.*.dump，按日期排序，删掉超过 7 天的
```

---

## 3. pgBackRest 方案

### 3.1 前置要求（postgresql.conf）

pgBackRest 需要 Postgres 配置：

```
# postgresql.conf
archive_mode = on
archive_command = 'pgbackrest --stanza=main archive-push %p'
wal_level = replica   # 默认值，确认即可
max_wal_senders = 3   # 至少 1
```

**核心冲突**：
- pgBackRest 是**可选功能**（profile 激活才启用）
- 但 `archive_mode = on` 一旦设置，Postgres 就会**每产生一个 WAL 段就调用 archive_command**
- 如果 archive_command 指向 pgBackRest 但 pgBackRest 容器没启动 → **archive 失败堆积** → 占满磁盘

所以**不能无脑 `archive_mode=on + archive_command=pgbackrest ...`**。

### 3.2 解决方案：Compose Override 文件

**核心思路**：默认 `archive_mode=off`。只有启用 pgbackrest profile 时，用 override compose 覆盖 db 的 `command` 字段。

#### 默认 docker-compose.yml（Phase 1 开始就这样）

```yaml
services:
  db:
    image: supabase/postgres:15.8.1.085
    command:
      - postgres
      - -c
      - max_connections=200
      - -c
      - shared_preload_libraries=pg_stat_statements,supautils
      # NO archive_mode here
    volumes:
      - db-data:/var/lib/postgresql/data
```

#### docker-compose.pgbackrest.yml（Phase 6 新增）

```yaml
# 只有 --profile pgbackrest 或 COMPOSE_PROFILES 含 pgbackrest 时加载
services:
  db:
    # 覆盖 command，追加 archive 相关参数
    command:
      - postgres
      - -c
      - max_connections=200
      - -c
      - shared_preload_libraries=pg_stat_statements,supautils
      - -c
      - archive_mode=on
      - -c
      - archive_command=pgbackrest --stanza=main --config=/etc/pgbackrest/pgbackrest.conf archive-push %p
      - -c
      - max_wal_senders=3
    volumes:
      - db-data:/var/lib/postgresql/data
      - pgbackrest-conf:/etc/pgbackrest
      - pgbackrest-log:/var/log/pgbackrest

  pgbackrest:
    image: woblerr/pgbackrest:2.54.2   # 或自己 build
    profiles: [pgbackrest]
    volumes:
      - db-data:/var/lib/postgresql/data:ro
      - pgbackrest-conf:/etc/pgbackrest
      - pgbackrest-log:/var/log/pgbackrest
    environment:
      - PGBACKREST_STANZA=main
      - PGBACKREST_REPO1_TYPE=s3
      - PGBACKREST_REPO1_S3_ENDPOINT=${S3_ENDPOINT}
      - PGBACKREST_REPO1_S3_BUCKET=${S3_BUCKET}
      - PGBACKREST_REPO1_S3_REGION=${S3_REGION}
      - PGBACKREST_REPO1_S3_KEY=${S3_ACCESS_KEY}
      - PGBACKREST_REPO1_S3_KEY_SECRET=${S3_SECRET_KEY}
      - PGBACKREST_REPO1_PATH=/backup/pgbackrest

volumes:
  pgbackrest-conf:
  pgbackrest-log:
```

#### 启用方式（两选一）

**方式 A：命令行显式**

```bash
# 启用 pgBackRest
docker compose -f docker-compose.yml -f docker-compose.pgbackrest.yml up -d

# 或者短写法（等效）
docker compose --profile pgbackrest up -d
```

**方式 B：setup.sh 安装时询问 + .env 控制**

```bash
# setup.sh 交互：
# "是否启用 pgBackRest 备份？ [y/N]: "
# 选 y → .env 追加:
#   COMPOSE_PROFILES=studio,pgbackrest
#   COMPOSE_FILE=docker-compose.yml:docker-compose.pgbackrest.yml

docker compose up -d   # 自动读 COMPOSE_FILE，加载两份 compose
```

**Phase 6 推荐方式 B**：用户安装时决定，无需记复杂命令。后续要禁用：编辑 `.env` 改回 `COMPOSE_FILE=docker-compose.yml`。

#### 切换注意事项

**从"无 pgBackRest"切到"启用"**：
1. 编辑 `.env` 加 `COMPOSE_FILE=docker-compose.yml:docker-compose.pgbackrest.yml` + `COMPOSE_PROFILES=studio,pgbackrest`
2. `docker compose up -d` — db 容器会被 recreate（因为 command 变了）
3. Postgres 重启，archive_mode 生效
4. 首次运行 `pgbackrest stanza-create` 初始化

**从"启用"切回"禁用"**：
1. 编辑 `.env` 恢复 `COMPOSE_FILE=docker-compose.yml`（或删除该行）
2. `docker compose up -d` — db 重启，archive_mode 关闭
3. WAL 不再被 archive（老的归档文件保留在 S3）

**警告**：切换都需要 db 重启（~5 秒不可用）。

### 3.3 Stanza 初始化

```bash
# 一次性初始化（在 pgbackrest 容器内执行）
docker compose exec pgbackrest pgbackrest --stanza=main stanza-create

# 验证配置
docker compose exec pgbackrest pgbackrest --stanza=main check

# 手工全量备份
docker compose exec pgbackrest pgbackrest --stanza=main --type=full backup

# 增量备份
docker compose exec pgbackrest pgbackrest --stanza=main --type=incr backup
```

### 3.4 admin 触发备份

admin Go 后端通过 Docker API `exec` 调用 pgBackRest 命令：

```go
// 示例：触发增量备份
cmd := []string{"pgbackrest", "--stanza=main", "--type=incr", "backup"}
// 通过 Docker Engine API 在 pgbackrest 容器内执行
```

### 3.5 保留策略

pgBackRest 自己支持保留策略，配置在 `pgbackrest.conf`：

```
[global]
repo1-retention-full=2      # 保留 2 份全量
repo1-retention-full-type=count
repo1-retention-diff=3
```

### 3.6 恢复流程

```bash
# 1. 停 Postgres
docker compose stop db

# 2. 清空 data volume
docker volume rm supabase-lite_db-data

# 3. 用 pgBackRest 恢复
docker compose run --rm pgbackrest \
  pgbackrest --stanza=main restore

# 4. 重启
docker compose up -d db
```

### 3.7 Phase 6 不做 WAL 归档 / PITR

用户明确说过 WAL 可以不要。所以：
- ✅ 全量 + 增量备份（pgBackRest）
- ❌ 连续 WAL 归档 → PITR

虽然 pgBackRest 配置里启用了 archive_mode，但我们不做 "push-on-every-commit" 的 PITR，只用 archive_command 让 pgBackRest 能做增量。

---

## 4. S3 客户端选型

### 4.1 方案对比

| 库 | 体积 | S3 兼容性 | 国内云兼容性 |
|---|:----:|:---------:|:-----------:|
| **aws-sdk-go-v2** | 大（官方完整）| 原生 AWS，其他需配置 | 可以，但需要配置 path-style、禁用特定 header |
| **minio-go** | 中 | 通用 S3 兼容好 | 通用性更好，自动处理 path-style |
| **手写 HTTP + Sigv4** | 最小 | 可定制 | 自己处理 |

### 4.2 Phase 6 推荐：`minio-go`

理由：
1. 为 S3 兼容服务设计（不绑定 AWS）
2. 自动选择 path-style vs virtual-hosted-style
3. 国内云测试相对完善
4. 镜像体积小
5. Go 生态中最主流的 S3 兼容客户端

```go
import "github.com/minio/minio-go/v7"

client, err := minio.New(endpoint, &minio.Options{
    Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
    Secure: useSSL,
    Region: region,
})

// 上传
_, err = client.FPutObject(ctx, bucket, "backup/2026-04-17.dump", "/tmp/backup.dump", minio.PutObjectOptions{})

// 下载
err = client.FGetObject(ctx, bucket, "backup/2026-04-17.dump", "/tmp/backup.dump", minio.GetObjectOptions{})

// 列表（用于保留策略）
for obj := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: "backup/"}) {
    // obj.Key, obj.LastModified, obj.Size
}

// 删除（清理旧备份）
err = client.RemoveObject(ctx, bucket, "backup/2026-04-10.dump", minio.RemoveObjectOptions{})
```

---

## 5. 国内云 S3 兼容 preset 清单

基于调研，**明确支持**的 provider（UI preset 下拉里的选项）：

### 5.1 全球通用

| Provider | Endpoint 格式 | 签名版本 | Path Style | 备注 |
|----------|---------------|:--------:|:----------:|------|
| AWS S3 | `https://s3.<region>.amazonaws.com` | v4 | virtual | 标准 |
| Cloudflare R2 | `https://<accountId>.r2.cloudflarestorage.com` | v4 | virtual | 无出网费 ⭐ |
| Backblaze B2 | `https://s3.<region>.backblazeb2.com` | v4 | virtual | 便宜 |
| Wasabi | `https://s3.<region>.wasabisys.com` | v4 | virtual | 无出网费 |
| DigitalOcean Spaces | `https://<region>.digitaloceanspaces.com` | v4 | virtual | 常用 |
| MinIO（自建） | `https://minio.example.com` | v4 | path | 自托管 |

### 5.2 国内云

| Provider | Endpoint 格式 | 签名版本 | 坑点 |
|----------|---------------|:--------:|------|
| **阿里云 OSS** | `https://s3.oss-cn-hangzhou.aliyuncs.com` ⚠️ | v4 | 必须用 **S3 专用 endpoint**（`s3.` 前缀）。Go 默认不使用 chunked encoding 所以 OK。 |
| **腾讯云 COS** | `https://cos.ap-guangzhou.myqcloud.com` | v4 | Bucket 名字必须 `<bucket>-<appid>` 格式（如 `mybucket-1250000000`） |
| **华为云 OBS** | `https://obs.cn-north-4.myhuaweicloud.com` | v4 | 较好兼容 |
| **七牛云 Kodo** | `https://s3-cn-east-1.qiniucs.com` | v4 | `s3-` 前缀，和普通 endpoint 不同 |
| **百度智能云 BOS** | `https://s3.bj.bcebos.com` | v4 | 基础兼容 |
| **京东云 JD OSS** | `https://s3.<region>.jdcloud-oss.com` | v4 | — |

### 5.3 国内云实测结论（调研来源：阿里云/腾讯云官方文档）

**阿里云 OSS**：
- ⚠️ 默认 OSS endpoint（`oss-cn-hangzhou.aliyuncs.com`）**不完全兼容** S3
- ✅ 必须用 **S3 API endpoint**（`s3.oss-cn-hangzhou.aliyuncs.com`）
- Go 默认配置下能正常工作（Python 需要 v2 签名，Go 默认 v4 即可）
- 文档链接：[Aliyun OSS S3 兼容](https://help.aliyun.com/zh/oss/developer-reference/use-amazon-s3-sdks-to-access-oss)

**腾讯云 COS**：
- ✅ 完全兼容 AWS S3 API
- ⚠️ Bucket 名字 `name-appid` 格式，SDK 里填 bucket 参数时要包含 appid
- 签名 v4

### 5.4 UI preset 下拉设计

```
[Provider 预设]
  ├ AWS S3           → endpoint 模板：https://s3.{region}.amazonaws.com
  ├ Cloudflare R2    → endpoint 模板：https://{accountId}.r2.cloudflarestorage.com
  ├ Backblaze B2     → endpoint 模板：https://s3.{region}.backblazeb2.com
  ├ Wasabi           → endpoint 模板：https://s3.{region}.wasabisys.com
  ├ DigitalOcean Spaces → https://{region}.digitaloceanspaces.com
  ├ 阿里云 OSS        → 模板：https://s3.oss-cn-{region}.aliyuncs.com  ⚠️ "需启用 S3 API 兼容"
  ├ 腾讯云 COS        → 模板：https://cos.ap-{region}.myqcloud.com    ⚠️ "Bucket 格式: name-appid"
  ├ 华为云 OBS        → 模板：https://obs.cn-{region}.myhuaweicloud.com
  ├ 七牛云 Kodo       → 模板：https://s3-cn-{region}.qiniucs.com       ⚠️ 注意 s3- 前缀
  ├ 百度智能云 BOS    → 模板：https://s3.{region}.bcebos.com
  ├ 京东云 OSS        → 模板：https://s3.{region}.jdcloud-oss.com
  └ Custom / MinIO   → 用户自填 endpoint
```

选中预设后：
- 自动填 endpoint 模板（用户替换 `{region}`）
- 显示对应的警告 / 注意事项
- 选 Custom 则 Path Style / 签名版本等选项全部打开

### 5.5 Test Connection 按钮

点击后跑一次完整的上传-下载-删除流程：
```go
func testS3Connection(cfg S3Config) error {
    client, err := minio.New(cfg.Endpoint, ...)
    if err != nil { return fmt.Errorf("new client: %w", err) }

    // 1. 尝试上传一个小文件
    testKey := fmt.Sprintf(".supabase-lite-test-%d", time.Now().Unix())
    _, err = client.PutObject(ctx, cfg.Bucket, testKey, 
        strings.NewReader("supabase-lite test"), -1, minio.PutObjectOptions{})
    if err != nil { return fmt.Errorf("put: %w", err) }

    // 2. 下载验证内容
    obj, err := client.GetObject(ctx, cfg.Bucket, testKey, minio.GetObjectOptions{})
    if err != nil { return fmt.Errorf("get: %w", err) }
    // ... 检查内容

    // 3. 删除测试文件
    err = client.RemoveObject(ctx, cfg.Bucket, testKey, minio.RemoveObjectOptions{})
    if err != nil { return fmt.Errorf("delete: %w", err) }

    return nil
}
```

---

## 6. 加密 Secret Store 设计

### 6.1 算法选择：AES-256-GCM

- 标准库支持（`crypto/aes` + `crypto/cipher`）
- AEAD：同时加密 + 认证
- 96-bit nonce / 128-bit auth tag
- 广泛审计，安全

### 6.2 主密钥来源

三种方案：

**方案 A：独立 SECRET_ENCRYPTION_KEY env var**
- 在 `.env` 里存一个新 key（由 setup.sh 生成）
- 优点：和 ADMIN_TOKEN 解耦
- 缺点：多一个 key

**方案 B：从 ADMIN_TOKEN 派生**
- 用 scrypt 或 argon2 派生
- 优点：少一个 key
- 缺点：ADMIN_TOKEN 变了所有加密数据就解密不了

**方案 C：独立的 master.key 文件**
- 放在 Docker volume 里，setup.sh 生成
- 优点：和 env 解耦
- 缺点：需要额外备份

**Phase 6 推荐 A**：最清晰，用户 setup.sh 时就知道有这个密钥。

```bash
# setup.sh
SECRET_ENCRYPTION_KEY=$(openssl rand -base64 32)  # 32 字节
echo "SECRET_ENCRYPTION_KEY=$SECRET_ENCRYPTION_KEY" >> .env
```

### 6.3 存储位置

**方案 A：独立文件（推荐）**

```
volumes/admin/secrets/
  s3.enc           # 加密的 S3 配置
  ...
```

**方案 B：.env 里直接加密字段**

```env
S3_CONFIG_ENCRYPTED=<base64(nonce + ciphertext)>
```

缺点：长度受 .env 解析限制。

**Phase 6 推荐 A**：独立文件存 JSON 加密块，方便扩展和迁移。

### 6.4 数据格式

```go
type encryptedBlob struct {
    Version int    `json:"v"`        // 版本号（未来兼容）
    Nonce   []byte `json:"nonce"`    // base64 encoded
    Ct      []byte `json:"ct"`       // ciphertext + tag
}
```

写入磁盘：`{v:1, nonce:"...", ct:"..."}`.

读取：
1. JSON 解码
2. 用 master key + nonce 解密 ct
3. 得到原始 JSON S3 配置

### 6.5 完整代码模式

```go
// internal/secretstore/secretstore.go
package secretstore

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "encoding/json"
    "io"
    "os"
)

type Store struct {
    key     []byte
    path    string
}

func New(path, masterKeyB64 string) (*Store, error) {
    key, err := base64.StdEncoding.DecodeString(masterKeyB64)
    if err != nil { return nil, err }
    if len(key) != 32 {
        return nil, fmt.Errorf("master key must be 32 bytes")
    }
    return &Store{key: key, path: path}, nil
}

func (s *Store) Save(v any) error {
    plaintext, err := json.Marshal(v)
    if err != nil { return err }

    block, err := aes.NewCipher(s.key)
    if err != nil { return err }
    gcm, err := cipher.NewGCM(block)
    if err != nil { return err }

    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil { return err }

    ct := gcm.Seal(nil, nonce, plaintext, nil)

    blob := map[string]string{
        "v":     "1",
        "nonce": base64.StdEncoding.EncodeToString(nonce),
        "ct":    base64.StdEncoding.EncodeToString(ct),
    }
    data, _ := json.Marshal(blob)
    return os.WriteFile(s.path, data, 0600)
}

func (s *Store) Load(v any) error {
    data, err := os.ReadFile(s.path)
    if err != nil { return err }

    var blob map[string]string
    if err := json.Unmarshal(data, &blob); err != nil { return err }

    nonce, _ := base64.StdEncoding.DecodeString(blob["nonce"])
    ct, _ := base64.StdEncoding.DecodeString(blob["ct"])

    block, _ := aes.NewCipher(s.key)
    gcm, _ := cipher.NewGCM(block)
    plaintext, err := gcm.Open(nil, nonce, ct, nil)
    if err != nil { return err }

    return json.Unmarshal(plaintext, v)
}
```

### 6.6 S3 配置的数据结构

```go
type S3Config struct {
    Provider    string `json:"provider"`      // "aws" / "cloudflare-r2" / ...
    Endpoint    string `json:"endpoint"`
    Region      string `json:"region"`
    Bucket      string `json:"bucket"`
    AccessKey   string `json:"access_key"`
    SecretKey   string `json:"secret_key"`    // 敏感
    PathStyle   bool   `json:"path_style"`
    PathPrefix  string `json:"path_prefix"`
    UseSSL      bool   `json:"use_ssl"`
}
```

### 6.7 Master Key Rotation

用户想换 master key：
1. 读取当前加密数据（旧 key 解密）
2. 用新 key 重新加密
3. 更新 `.env` 里的 `SECRET_ENCRYPTION_KEY`

admin 面板提供"轮换密钥"按钮。

---

## 7. UI 设计

### 7.1 Settings 页新增 "Backups" tab

```
┌── Backups ─────────────────────────────┐
│                                         │
│  Storage Destination                    │
│  ◯ Local only                           │
│  ● External S3                          │
│                                         │
│  ┌ S3 Configuration ─────────────────┐  │
│  │ Provider: [Aliyun OSS    ▼]       │  │
│  │ Endpoint: [https://s3.oss-cn-...]  │  │
│  │ Region:   [cn-hangzhou]            │  │
│  │ Bucket:   [my-backups]             │  │
│  │ Access Key: [...]                  │  │
│  │ Secret Key: [••••••]               │  │
│  │ Path prefix: [backups/pg1/]        │  │
│  │                                    │  │
│  │  ⚠️ 需启用 S3 API 兼容             │  │
│  │                                    │  │
│  │ [Test Connection]                  │  │
│  └────────────────────────────────────┘  │
│                                         │
│  Schedule                               │
│  pg_dump cron: [0 2 * * *]              │
│  pgBackRest cron: [0 3 * * *]           │
│  Retention days: [7]                    │
│                                         │
│  [Save]  [Backup Now]                   │
└─────────────────────────────────────────┘
```

### 7.2 Backups 列表页

```
┌── Backup History ──────────────────────┐
│  Date        Size   Type  Method  [Action]│
│  2026-04-17  142 MB full  pg_dump [Restore]│
│  2026-04-16  3.2 MB incr  pgBackRest [...] │
│  ...                                      │
└───────────────────────────────────────────┘
```

---

## 8. Phase 6 实施清单

1. **Dockerfile**: admin 镜像加 `postgresql-client` 工具
2. **依赖**: `go get github.com/minio/minio-go/v7 github.com/robfig/cron/v3`
3. **internal/secretstore/**: AES-256-GCM 加密包装
4. **internal/s3client/**: minio-go 封装 + provider preset 定义
5. **internal/backup/**:
   - `pgdump.go`: 执行 pg_dump，流式上传 S3
   - `pgbackrest.go`: Docker exec pgBackRest 命令
   - `scheduler.go`: cron 调度 + 保留策略
6. **handler/backup.go**: 
   - GET /api/backups — 列表
   - POST /api/backups/pgdump — 手动触发
   - POST /api/backups/restore — 恢复
   - POST /api/backups/s3/test — 测试连接
7. **docker-compose**: 加 `pgbackrest` sidecar（profile: pgbackrest）+ `archive_command` 配置
8. **setup.sh**: 生成 `SECRET_ENCRYPTION_KEY`
9. **前端**: Settings 新增 Backups tab + 备份历史页
10. **测试**: 
    - AWS S3 / Cloudflare R2 / 阿里云 OSS / 腾讯云 COS 各测一次
    - 恢复流程完整测试

---

## 9. 风险与缓解

| 风险 | 缓解 |
|------|------|
| pgBackRest archive_command 配置错导致 archive 堆积 | `check` 命令验证 + 文档强调 |
| Master key 丢失导致 S3 配置不可用 | 文档强调备份 .env；提供重置功能（清空 secret store 重来） |
| 国内云兼容性万一有坑 | 每个 provider 做 E2E 测试，文档列"known issues" |
| Admin 容器挂了导致备份中断 | 独立 pgbackrest 容器可被外部 cron 触发兜底 |
| 用户同时开 pg_dump + pgBackRest 占磁盘 | UI 提示、日志监控 |
| WAL 归档配置但不做 PITR，WAL 会被 pgBackRest 清理 | 默认保留 24-48 小时的 WAL（pgBackRest 增量依赖） |
