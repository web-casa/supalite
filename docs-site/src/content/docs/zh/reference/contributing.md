---
title: 贡献指南
description: 如何提 issue 和 PR。
---

## 提 bug

提之前先 `docker compose ps` 并查相关容器日志（`docker logs supalite-<service>-1`）。[常见问题](/supalite/zh/troubleshooting/common-errors/) 页覆盖了大多数已报告问题。

提 issue 时请包含：

- SupaLite 版本：`git rev-parse --short HEAD`
- 系统 + Docker 版本：`docker version`
- `docker compose ps` 输出
- 相关日志片段（**脱敏**——不要带 API key / token / 密码）

## 代码贡献

### 仓库结构

```
admin/          # Go 后端 + Next.js 管理面板
  internal/     # Go 包
  web/          # Next.js SPA
volumes/        # 容器初始化脚本与配置
  api/          # Caddyfile、着陆页
  db/           # 自定义 Dockerfile、pgbackrest 配置、初始化脚本
examples/       # 独立示例项目
docs-site/      # 本文档站
```

### 本地开发

**后端 (Go)**：
```bash
cd admin
go vet ./...
go build ./...
go test ./... -count=1
```

**前端**：
```bash
cd admin/web
npm install
npm run build
```

**文档站**：
```bash
cd docs-site
npm install
npm run dev      # 本地预览 :4321
npm run build
```

### 测试改动

多数改动需要真实运行栈。仓库现成的 `setup.sh` 产生确定性本地实例。改完后：

```bash
docker compose build admin db
docker compose up -d
```

打开 `http://localhost:8000/admin/` 走一遍受影响路径。

### CI

`.github/workflows/ci.yml` 在 PR 上跑：

- `go vet`、`go build`、`go test ./... -count=1`
- `next build`
- `docker compose config`（默认 + HTTPS override）

PR 必须 CI 通过才进 review。

### Pull Request

- 小、聚焦的 PR > 大杂烩。一个 PR 一个主题。
- 新公共包 / 端点要带测试。
- 更新相关文档页（`docs-site/src/content/docs/` 和 `docs-site/src/content/docs/zh/` **两边都要**）。
- 更新 `CHANGELOG.md`。

## 翻译

文档双语（英 + 中）。一边加 / 改页面时另一边也要更新——哪怕只是先放一份"和英文一致的占位、等人翻译"。

新增语种步骤：

1. `astro.config.mjs` 的 `locales` 加一行。
2. 在 `src/content/docs/<locale>/` 镜像目录结构。
3. 给 `astro.config.mjs` 里每个 sidebar item 加 `translations` 条目。

## 许可

参与贡献即同意你的贡献以项目的 [MIT 许可](https://github.com/web-casa/supalite/blob/main/LICENSE) 发布。
