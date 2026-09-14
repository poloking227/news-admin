# news-admin frontend

Vue 3 管理端 + 浏览端（前后端分离，契约见仓库根 `docs/openapi.yaml`，基址 `/api/v1`）。

## 技术栈

- Vue 3（Composition API）+ Vite + TypeScript `strict`
- Vue Router 4（admin/public 双壳）+ Pinia
- Axios（统一错误信封解析、Bearer 注入、401 单飞刷新旋转、双提交 CSRF）
- Element Plus（仅管理端）+ Tailwind CSS v4（CSS-first 配置，见 `src/style/main.css`）
- Vitest + ESLint 9 + Prettier

## 会话与权限

- access token（JWT）仅内存持有（1h），refresh token 为 HttpOnly cookie（7d，旋转）。
- 401 自动单飞刷新（`/auth/refresh`）并重放原请求；刷新失败回登录页。
- 双提交 CSRF：登录/刷新响应回写 `csrf_token` cookie，请求头 `X-CSRF-Token` 回传。
- 路由守卫基于 `/auth/me` 的 role 与权限点校验 `requiresAuth` / `roles` meta；
  M0 首登强制改密（`mustChangePassword`）期间仅放行改密页，其余管理路由跳 `/admin/change-password`。

## 命令

```bash
pnpm install       # 安装依赖（Node 20+）
pnpm dev           # 开发服务器 http://localhost:5173（/api 代理到 :8080）
pnpm test          # 单测（Vitest）
pnpm lint          # ESLint 静态检查
pnpm type-check    # vue-tsc 类型检查
pnpm build         # 类型检查 + 生产构建
```

## 目录约定（docs/architecture.md 模块边界）

```
src/
  admin/    管理端壳与视图（登录、改密、文章、分类、用户、审计）
  public/   浏览端壳与视图（列表/详情）
  shared/   api client、契约类型、stores、跨端组件
```
