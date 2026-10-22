# 发布前本地校验清单

合并发布前在干净环境按序执行。分三部分：环境准备 → 自动化全量检查 → 关键用户流程人工复核；全部通过即可按根目录 README 的「生产部署」节发布。

## 0. 环境准备

```bash
cp .env.example .env  # 校验机首次
make up && make migrate-up
cd backend && ADMIN_INITIAL_PASSWORD=Verify1! go run ./cmd/seed
```

## 1. 自动化全量检查

| 层 | 命令 | 期望结果 |
| --- | --- | --- |
| 后端编译 | `cd backend && go build ./cmd/server` | 编译通过 |
| 后端静态检查 | `cd backend && go vet ./...` | 无告警 |
| 后端全量测试 | `cd backend && go test ./...` | 全部 PASS（含 PG16 实跑的检索与历史数据回归，依赖本地 PostgreSQL 16） |
| 前端 lint | `cd frontend && pnpm lint` | ESLint 0 警告（`--max-warnings 0`） |
| 前端格式 | `cd frontend && pnpm format:check` | Prettier 无差异 |
| 前端类型检查 | `cd frontend && pnpm type-check` | vue-tsc 与应用/ e2e 两套 tsconfig 均通过 |
| 前端构建 | `cd frontend && pnpm build` | 产出 `frontend/dist/` |
| 前端单测 | `cd frontend && pnpm test` | Vitest 全绿 |
| 端到端 | `make test-e2e` | Playwright 全绿（首次先 `pnpm exec playwright install chromium`） |

## 2. 关键用户流程手测

在已 seed 的环境通过浏览器完成（入口 `http://localhost:5173`）：

1. **首登强制改密（M0）**：admin 首次登录 → 仅放行改密页 → 设置新密码 → 进入管理端；旧口令此后不可登录。
2. **内容生产**：editor 登录 → 创建文章（分类必选，正文/摘要齐全）→ 保存草稿 → 提交审核。
3. **审核**：reviewer 通过 → 文章立即发布（写 published_at，无独立发布动作）；驳回（须填原因）→ editor 侧文章显示「已驳回待修改」，可编辑后重新提交。
4. **浏览端可见性**：已发布文章在列表 / 搜索 / 详情可见；草稿、已下架、已删除文章任意直链一律 404；标题 / 摘要 / 正文关键字搜索可命中；置顶文章排最前。
5. **下架与重审**：下架已发布文章 → 浏览端不可见 → 重新提交审核 → 再次发布后恢复可见（无直接重发通道）。
6. **权限门控（RBAC）**：editor 访问审核、用户管理、审计等端点返回 403；他人草稿不可编辑。
7. **会话与 CSRF**：写请求携带 `X-CSRF-Token` 请求头（双提交 cookie 值一致）；access 过期后自动单飞刷新并重放原请求；登出后 refresh 族吊销。
8. **分类管理**：创建 / 编辑 / 软删；删除仍存在已发布文章的分类返回 409，需先迁移文章。
9. **用户管理（admin）**：新建用户、停用 / 启用、重置密码——新建与重置后的账号首次登录均强制改密。
10. **审计**：审核通过 / 驳回、发布 / 下架、改密、用户管理等关键操作在审计台有记录，动作与资源可追溯。

## 3. 收尾

- `git status` 干净；本地无未忽略的测试产物（`test-results/`、`playwright-report/` 已在 `.gitignore`）。
- 按 README「生产部署」节核对环境变量（`DATABASE_URL` / `JWT_SECRET` / `CORS_ORIGIN` / `APP_ENV`）与迁移执行时序后发布。