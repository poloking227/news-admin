# news-admin frontend

Vue 3 管理端 + 浏览端（前后端分离，契约见仓库根 `docs/openapi.yaml`，基址 `/api/v1`）。

## 技术栈

- Vue 3（Composition API）+ Vite + TypeScript `strict`
- Vue Router 4（admin/public 双壳）+ Pinia
- Axios（统一错误信封解析、Bearer 注入、401 刷新占位）
- Tailwind CSS v4（CSS-first 配置，见 `src/style/main.css`）
- Vitest + ESLint 9 + Prettier

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
  admin/    管理端壳与视图（认证、文章、分类、用户、审计）
  public/   浏览端壳与视图（列表/详情）
  shared/   api client、契约类型、stores、跨端组件
```

路由 `meta` 携带 `requiresAuth` / `roles` 角色声明（占位），真实 RBAC 守卫随会话模块（登录、refresh 旋转、首登强制改密门控）落地。