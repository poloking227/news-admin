# news-admin

新闻管理系统：管理端支撑内容生产、审核与发布治理，浏览端面向访客只读展示。前后端分离，响应式 Web。

## 技术栈

- 后端：Go + Gin + GORM，PostgreSQL 16，goose 迁移
- 前端：Vue 3 + Vite + TypeScript + Pinia + Tailwind CSS
- 质量：后端 `go test` + golangci-lint；前端 Vitest + ESLint + Prettier；Playwright 端到端

## 快速开始

前置：Docker、Go 1.23+、Node 20+（含 pnpm）。

```bash
cp .env.example .env     # 按需修改本地值
make up                  # 启动 PostgreSQL 16
make migrate-up          # 应用数据库迁移（后端迁移命令落地后可用）
make dev-back            # 启动后端开发服务器（air 热载，:8080）
make dev-front           # 启动前端开发服务器（Vite，:5173，/api 代理到后端）
```

本地浏览端与管理端入口均为 `http://localhost:5173`。

## 常用命令

| 目标 | 说明 |
| --- | --- |
| `make up` / `make down` | 启停本地 PostgreSQL |
| `make migrate-up` / `make migrate-down` | 应用 / 回滚数据库迁移 |
| `make dev-back` / `make dev-front` | 启动前后端开发服务器 |
| `make test-back` / `make test-front` | 运行前后端测试 |
| `make lint` | 前后端静态检查 |
| `make build` | 构建前后端产物 |

## 目录结构

```
backend/            后端服务（Go）
frontend/           前端应用（Vue 3）
docker-compose.yml  本地 PostgreSQL 编排
Makefile            常用任务
.env.example        环境变量模板
```

## 说明

- 本地环境配置见 `.env.example`，复制为 `.env` 后按需修改；文件内所有密钥与口令均为占位符，生产环境通过环境变量注入。
- 架构说明与接口契约将随实现逐步补充于 `docs/`。