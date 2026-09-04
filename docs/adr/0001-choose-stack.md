# ADR-0001 技术栈选择

- 状态：已接受
- 日期：2026-09-01

## 背景

需要为前后端分离的新闻管理系统选定技术栈，要求：工程可维护、测试与静态检查成熟、适合小团队快速交付、无强约束的跨端模板一致性要求。

## 决策

- 后端：Go 1.23 + Gin；ORM 用 GORM v2（pgx 驱动）；数据库迁移用 goose（版本化 SQL）。
- 数据库：PostgreSQL 16（UUID 主键、citext、CHECK 约束、部分唯一索引、pg_trgm 中文搜索）。
- 前端：Vue 3（Composition API）+ Vite + TypeScript(strict) + Vue Router + Pinia；管理端组件用 Element Plus，浏览端轻量样式用 Tailwind CSS。
- 测试：后端 Go test/testify + golangci-lint；前端 Vitest + ESLint/Prettier + Playwright。

## 影响

- 单二进制部署、并发模型简单、静态检查友好；前后端各自遵循自身语言与工具链约定。
- GORM 内置软删（DeletedAt）与 scope 机制，配合数据库层约束兜底。