---
title: Contributing
description: How to file issues and PRs.
---

## Reporting bugs

Before filing, check `docker compose ps` and the relevant container's logs (`docker logs supalite-<service>-1`). The [Common Errors](/supalite/troubleshooting/common-errors/) page covers most reported issues.

When you do file an issue, include:

- SupaLite version: `git rev-parse --short HEAD`
- OS + Docker version: `docker version`
- `docker compose ps` output
- Relevant log excerpts (sanitized — no API keys / tokens / passwords)

## Code contributions

### Repo layout

```
admin/          # Go backend + Next.js admin panel
  internal/     # Go packages
  web/          # Next.js SPA
volumes/        # Container init scripts and config
  api/          # Caddyfile, landing page
  db/           # Custom Dockerfile, pgbackrest config, init script
examples/       # Standalone example projects
docs-site/      # This documentation site
```

### Local development

**Backend (Go)**:
```bash
cd admin
go vet ./...
go build ./...
go test ./... -count=1
```

**Frontend**:
```bash
cd admin/web
npm install
npm run build
```

**Docs site**:
```bash
cd docs-site
npm install
npm run dev      # local preview at :4321
npm run build
```

### Testing changes

Most changes need a real running stack. The repo's existing `setup.sh` produces a deterministic local instance. After your changes:

```bash
docker compose build admin db
docker compose up -d
```

Hit `http://localhost:8000/admin/` and exercise the affected paths.

### CI

`.github/workflows/ci.yml` runs on PRs:

- `go vet`, `go build`, `go test ./... -count=1`
- `next build`
- `docker compose config` (default + HTTPS override)

PRs must pass CI before review.

### Pull requests

- Small focused PRs > big ones. One topic per PR.
- Tests for new public packages / endpoints.
- Update the relevant docs page (in `docs-site/src/content/docs/` AND `docs-site/src/content/docs/zh/`).
- Update `CHANGELOG.md`.

## Translation

Docs are bilingual (English + Chinese). When adding or changing a page in one language, update the other side too — even if the change is "applied identical English copy as a placeholder until someone translates".

To add a new locale:

1. Add to `astro.config.mjs` `locales`.
2. Mirror the directory structure under `src/content/docs/<locale>/`.
3. Add `translations` entries on every sidebar item in `astro.config.mjs`.

## License

By contributing, you agree your contributions are licensed under the project's [MIT License](https://github.com/web-casa/supalite/blob/main/LICENSE).
