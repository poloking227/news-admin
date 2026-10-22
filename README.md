# news-admin

新闻管理系统：管理端支撑内容生产、审核与发布治理，浏览端面向访客只读展示。前后端分离，响应式 Web，单站点部署。

## 技术栈

- 后端：Go 1.23 + Gin + GORM（pgx 驱动），PostgreSQL 16，goose 数据库迁移，JWT/bcrypt 会话，bluemonday 富文本白名单清洗
- 前端：Vue 3.5 + Vite + TypeScript（`strict`）+ Pinia + Tailwind CSS 4；管理端 Element Plus + Tiptap 编辑器
- 质量：Go 单测（含 PostgreSQL 16 实跑的检索与历史数据回归）、Vitest、Playwright 端到端、ESLint 9 / Prettier

## 前置依赖

| 依赖 | 版本 | 用途 |
| --- | --- | --- |
| PostgreSQL | 16 | 唯一数据库 |
| Go | 1.23+ | 后端编译与测试 |
| Node.js | 20+ | 前端工具链 |
| pnpm | 近期稳定版 | 前端包管理 |
| Docker | — | 本地 PostgreSQL 编排（非必需，自备实例亦可） |
| Chromium | 由 Playwright 管理 | 端到端测试浏览器（首次需 `pnpm exec playwright install chromium`） |

## 快速开始

```bash
cp .env.example .env                       # 按需修改本地值
make up                                    # 启动本地 PostgreSQL 16（docker compose）
make migrate-up                            # 应用数据库迁移
cd backend && ADMIN_INITIAL_PASSWORD=<强密码> go run ./cmd/seed  # 写入管理员首登口令
make dev-back                              # 后端开发服务器（air 热载，:8080）
make dev-front                             # 前端开发服务器（Vite，:5173，/api 代理到后端）
```

管理端与浏览端入口均为 `http://localhost:5173`。

数据库初始化在本地执行一次即可；goose 保证迁移可重复应用。

### 不依赖 air 的本地启动

```bash
# 终端 1：后端
cd backend && go run ./cmd/server

# 终端 2：前端
cd frontend && pnpm dev
```

## 测试

### 后端（Go）

`go test ./...` 运行全部后端测试：单元测试 + 检索与历史数据回归（含 JSONB 迁移场景）实跑，需要可达的 PostgreSQL 16 实例；`DATABASE_URL` 未设置或不可达时回归包自动跳过，其余单测照常运行。

```bash
cd backend && go test ./...
```

### 前端单测（Vitest）

```bash
cd frontend && pnpm test
```

### 端到端测试（Playwright 浏览器全链路）

前置：本地 PostgreSQL 16（默认 `newsadmin/newsadmin_dev@localhost:5432`）、Node 20+、可构建的后端（Go）、Chromium。首次运行会创建独立库 `news_admin_e2e`，数据库角色需具备建库权限；如无，以超级用户执行一次 `ALTER ROLE newsadmin CREATEDB;`，或通过 `E2E_DATABASE_URL` 指向已建好的库。

```bash
make test-e2e      # 等价于 cd frontend && pnpm test:e2e
```

运行时自动依次执行：重建独立库（`backend/cmd/e2e prep`）→ 应用迁移（`go run ./cmd/migrate up`）→ 写入已知首登口令的测试账号（`backend/cmd/e2e seed`，首登强制改密标记保持开启）→ 启动后端 `:8080` → 启动前端 `:5173`。

- webServer 编排与时序见 `frontend/playwright.config.ts`；命令实现见 `backend/cmd/e2e`。
- 完整说明见 `frontend/README.md`「端到端测试」节。

## 生产部署

### 构建产物

```bash
cd backend && go build -o news-admin-server ./cmd/server
cd frontend && pnpm build      # 含类型检查；产物在 frontend/dist/
```

前端产物为纯静态资源（内置 `/api/v1` 相对基址），交由静态服务器托管并将 `/api` 反向代理至后端。

### 环境变量

| 变量 | 必填 | 说明 |
| --- | --- | --- |
| `DATABASE_URL` | 是 | PostgreSQL DSN，如 `postgres://user:pass@host:5432/news_admin?sslmode=disable` |
| `JWT_SECRET` | 是 | access token 签名密钥，须为强随机长串，仅通过环境注入 |
| `CORS_ORIGIN` | 是 | 允许的浏览器来源（逗号分隔）；生产为实际站点 origin |
| `APP_ENV` | 否 | `development` / `production`；`production` 时认证 cookie 加 `Secure` |
| `APP_PORT` | 否 | 监听端口，默认 `8080` |
| `LOG_LEVEL` | 否 | `debug` / `info` / `warn` / `error`，默认 `info` |

`APP_ENV=production` 时 refresh / CSRF cookie 均带 `Secure` 属性，站点必须以 HTTPS 提供。

### 首次初始化的执行时序

1. **迁移**：`cd backend && go run ./cmd/migrate up`——先于服务启动；后续每次发布新版本同样先迁移、再切流量，goose 幂等可重复执行。
2. **管理员首登口令**：`ADMIN_INITIAL_PASSWORD=<强密码> go run ./cmd/seed`——仅覆盖迁移内置的占位哈希；管理员账号保持 `must_change_password=true`，首次登录强制改密。
3. **启动服务**：`./news-admin-server`，`DATABASE_URL` / `JWT_SECRET` / `CORS_ORIGIN` 等经环境注入（应用支持读取进程环境，本地开发时也可通过 `.env` 注入）。

其他环境变量见 `.env.example` 模板。

## 常用命令

| 目标 | 说明 |
| --- | --- |
| `make up` / `make down` | 启停本地 PostgreSQL |
| `make migrate-up` / `make migrate-down` | 应用 / 回滚数据库迁移 |
| `make dev-back` / `make dev-front` | 启动前后端开发服务器 |
| `make test-back` / `make test-front` / `make test-e2e` | 后端 / 前端单测 / 端到端测试 |
| `make lint` / `make fmt` / `make build` | 前后端静态检查 / 格式化 / 构建 |
| `pnpm lint` / `pnpm format:check` / `pnpm type-check` | 前端专项检查（见 `frontend/package.json`） |

## 目录结构

```
backend/            后端服务（Go）
  cmd/              server（API 服务）、migrate（goose）、seed（管理员初始口令）、e2e（E2E 库准备）
  internal/         config / api / service / domain / repository / auth / audit
frontend/           前端应用（Vue 3）
  src/              admin（管理端）、public（浏览端）、shared（客户端与共享组件）
  e2e/              Playwright 用例
docs/               架构说明、ADR、OpenAPI 契约、发布校验清单
docker-compose.yml  本地 PostgreSQL 编排
Makefile            常用任务
.env.example        环境变量模板
```

## 文档

- 架构与模块边界：[docs/architecture.md](docs/architecture.md)；关键选型：[docs/adr/](docs/adr/)
- 接口契约：[docs/openapi.yaml](docs/openapi.yaml)
- 发布前本地校验清单：[docs/release-checklist.md](docs/release-checklist.md)