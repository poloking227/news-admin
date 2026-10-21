.PHONY: up down logs migrate-up migrate-down dev-back dev-front test-back test-front test-e2e lint fmt build

up: ## 启动本地 PostgreSQL
	docker compose up -d

down: ## 停止本地 PostgreSQL
	docker compose down

logs: ## 查看数据库日志
	docker compose logs -f db

migrate-up: ## 应用数据库迁移（后端迁移命令落地后可用）
	cd backend && go run ./cmd/migrate up

migrate-down: ## 回滚最近一次数据库迁移
	cd backend && go run ./cmd/migrate down

dev-back: ## 启动后端开发服务器（需安装 air）
	cd backend && air

dev-front: ## 启动前端开发服务器
	cd frontend && pnpm dev

test-back: ## 运行后端测试
	cd backend && go test ./...

test-front: ## 运行前端单元测试
	cd frontend && pnpm test

test-e2e: ## 运行浏览器端到端测试（Playwright；需要本地 PostgreSQL 16 与 Chromium）
	cd frontend && pnpm test:e2e

lint: ## 前后端静态检查
	cd backend && golangci-lint run ./...
	cd frontend && pnpm lint

fmt: ## 格式化前后端代码
	cd backend && gofmt -w .
	cd frontend && pnpm format

build: ## 构建前后端产物
	cd backend && go build ./cmd/server
	cd frontend && pnpm build