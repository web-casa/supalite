// @ts-check
import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";

// SupaLite docs site.
// - Bilingual (en root, zh under /zh/), English is the default.
// - Theme switcher (light/dark/auto) is built into Starlight; default
//   behavior is `auto` which follows OS color-scheme preference.
// - Brand color comes from src/styles/brand.css overriding Starlight
//   CSS vars.
export default defineConfig({
  // For GitHub Pages under github.com/<owner>/<repo>, both `site` and
  // `base` need to be set so internal links resolve correctly under
  // the /<repo>/ path. Operators forking should override these.
  site: "https://web-casa.github.io",
  base: "/supalite",
  trailingSlash: "always",
  integrations: [
    starlight({
      title: {
        en: "SupaLite",
        zh: "SupaLite",
      },
      description:
        "One-command self-hosted Postgres + Auth + Studio for indie developers.",
      logo: {
        src: "./src/assets/logo.svg",
        replacesTitle: false,
      },
      favicon: "/favicon.svg",
      defaultLocale: "root",
      locales: {
        root: { label: "English", lang: "en" },
        zh: { label: "中文", lang: "zh" },
      },
      social: {
        github: "https://github.com/web-casa/supalite",
      },
      editLink: {
        baseUrl: "https://github.com/web-casa/supalite/edit/main/docs-site/",
      },
      lastUpdated: true,
      pagefind: true,
      customCss: ["./src/styles/brand.css"],
      // Brand color for the browser theme bar (mobile address-bar tint).
      // Starlight handles og:type / twitter:card defaults itself; we don't
      // ship a social-card image yet because SVGs aren't supported by most
      // crawlers. Add a PNG og:image once we have one.
      head: [
        { tag: "meta", attrs: { name: "theme-color", content: "#3ECF8E" } },
      ],
      // Light/dark code-block themes; default Starlight theme is dark only.
      expressiveCode: {
        themes: ["github-dark", "github-light"],
      },
      sidebar: [
        {
          label: "Getting Started",
          translations: { zh: "快速上手" },
          items: [
            {
              slug: "getting-started/what-is-supalite",
              translations: { zh: "什么是 SupaLite" },
            },
            {
              slug: "getting-started/quick-start",
              translations: { zh: "5 分钟启动" },
            },
            {
              slug: "getting-started/architecture",
              translations: { zh: "架构总览" },
            },
          ],
        },
        {
          label: "Configuration",
          translations: { zh: "配置" },
          items: [
            {
              slug: "configuration/environment-reference",
              translations: { zh: "环境变量参考" },
            },
            {
              slug: "configuration/https-tls",
              translations: { zh: "HTTPS / TLS" },
            },
            {
              slug: "configuration/multi-frontend",
              translations: { zh: "多前端支持" },
            },
            {
              slug: "configuration/compose-profiles",
              translations: { zh: "Compose Profiles" },
            },
          ],
        },
        {
          label: "Operations",
          translations: { zh: "运维" },
          items: [
            { slug: "operations/backups", translations: { zh: "备份" } },
            { slug: "operations/restore", translations: { zh: "恢复" } },
            {
              slug: "operations/secret-rotation",
              translations: { zh: "密钥轮换" },
            },
            {
              slug: "operations/db-maintenance",
              translations: { zh: "数据库维护" },
            },
            {
              slug: "operations/logs-monitoring",
              translations: { zh: "日志与监控" },
            },
          ],
        },
        {
          label: "Concepts",
          translations: { zh: "核心概念" },
          items: [
            { slug: "concepts/rls", translations: { zh: "行级安全 RLS" } },
            { slug: "concepts/jwt", translations: { zh: "JWT 鉴权" } },
            {
              slug: "concepts/anon-vs-service-role",
              translations: { zh: "ANON vs SERVICE_ROLE" },
            },
            {
              slug: "concepts/ports-networking",
              translations: { zh: "端口与网络" },
            },
          ],
        },
        {
          label: "Examples",
          translations: { zh: "示例" },
          items: [
            {
              slug: "examples/nextjs-todo",
              translations: { zh: "Next.js Todo" },
            },
          ],
        },
        {
          label: "API Reference",
          translations: { zh: "API 参考" },
          collapsed: true,
          items: [
            {
              slug: "api-reference/admin-api",
              translations: { zh: "Admin API" },
            },
          ],
        },
        {
          label: "Troubleshooting",
          translations: { zh: "故障排查" },
          collapsed: true,
          items: [
            {
              slug: "troubleshooting/common-errors",
              translations: { zh: "常见问题" },
            },
          ],
        },
        {
          label: "Reference",
          translations: { zh: "参考" },
          collapsed: true,
          items: [
            { slug: "reference/changelog", translations: { zh: "变更日志" } },
            {
              slug: "reference/contributing",
              translations: { zh: "贡献指南" },
            },
            { slug: "reference/roadmap", translations: { zh: "路线图" } },
          ],
        },
      ],
    }),
  ],
});
