---
title: Changelog
description: Release history.
---

The authoritative changelog lives at [`CHANGELOG.md`](https://github.com/web-casa/supalite/blob/main/CHANGELOG.md) in the repo root, formatted per [Keep a Changelog](https://keepachangelog.com).

## Highlights so far

### v0.3.x — operational depth
- Secret rotation wizard (4 secrets, cascading JWT)
- React Query throughout the admin UI

### v0.2.x — adoption-ready
- Multi-frontend CORS + GoTrue allow list
- Scheduled pg_dump + retention
- Go unit tests for security-critical paths
- Setup wizard with live SMTP test
- `examples/nextjs-todo/` end-to-end demo

### v0.1.0 — first OSS release
- Core stack: Postgres + REST + Auth + Studio + admin panel
- pg_dump backups to S3 + restore
- pgBackRest physical incremental backups + restore
- Auto-HTTPS via Caddy + Let's Encrypt (opt-in)
- SMTP and OAuth credential test endpoints
- MIT license, CI, multi-arch image releases

For per-commit detail, the GitHub releases page links each tag to its compare diff.
