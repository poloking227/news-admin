# 架构说明

本页记录新闻管理系统的架构基线；接口的最终权威为 [openapi.yaml](./openapi.yaml)（OpenAPI 3.1）。关键选型与取舍见 [adr](./adr/)。

## 1. 系统形态

前后端分离：管理端支撑内容生产、审核与发布治理；浏览端面向访客只读展示（响应式 Web）。单站点部署。

## 2. 模块边界

```
backend/                      Go 服务
  cmd/server                  入口（装配配置、路由、中间件）
  internal/config             环境配置（密钥仅由环境注入）
  internal/api                路由 + handler + 中间件（鉴权/RBAC/审计/限流/错误信封）
  internal/service            用例与状态机（流转表为权威）
  internal/domain             实体与状态机定义、仓储接口
  internal/repository         GORM 实现（软删与可见性 scope）
  internal/auth               JWT/bcrypt/refresh 族
  internal/audit              审计写入（与业务同事务）

frontend/                     Vue 3 应用
  src/admin/                  管理端（认证、文章、分类、用户、审计台）
  src/public/                 浏览端（列表/搜索/详情）
  src/shared/                 api client、stores、组件库
```

管理端与浏览端共享领域词汇与接口契约，但各自遵循自身语言与工具链约定。

## 3. 数据模型

| 表 | 关键字段与约束 |
| --- | --- |
| users | username(citext, 唯一)、password_hash、display_name、role(admin/editor/reviewer/operator)、status(active/disabled)、must_change_password、password_changed_at、软删 |
| articles | title(≤200)、summary(≤500)、body_html、body_text(供搜索)、category_id(必填外键)、cover_url(可空)、status(draft/pending_review/published/unpublished)、reject_reason(≤500)、rejected_at、pinned/pinned_at、submitted_at/published_at/unpublished_at、created_by/updated_by、version(乐观锁)、软删 |
| categories | name/slug 唯一、description、sort_order、软删 |
| audit_logs | actor、action(仅关键操作)、resource_type/id、before/after(jsonb)、ip、created_at；与业务变更同事务写入 |
| refresh_sessions | user_id、jti、expires_at、revoked_at、family_id(旋转复用检测) |

共性：UUID 主键、timestamptz 时间；软删以 `deleted_at` 表达，已删除数据不进入任何接口（管理端列表亦过滤）。

内容状态机仅 5 条合法迁移：

```text
draft → pending_review → published（审核通过即发布）
pending_review → draft（驳回，reason 必填 ≤500，写 rejected_at）
published → unpublished（下架，reason 可选）
unpublished → pending_review（重新提交审核，无直接重发）
```

状态枚举不含 rejected：驳回由 `draft + reject_reason/rejected_at 非空` 表达，前端显示为「已驳回待修改」。非法迁移由 API 层与数据库约束双重拒绝（409）。

## 4. 安全模型

- **认证**：密码 bcrypt(12)；access token（JWT，1h）放请求头，refresh token（7d）为 HttpOnly cookie（SameSite=Strict，Path 限 /auth），旋转并支持复用检测吊销族；刷新端点要求双提交 CSRF 校验。登录限流（5 次/分钟/IP + 10 次/15 分钟/账号）且失败登录写审计。
- **首登强制改密（M0）**：以初始/临时口令开通的账号（预置管理员、新建用户、管理员重置密码）`must_change_password=true`；期间仅 `/auth/me`、`/auth/change-password`、`/auth/logout` 放行，其余管理端点 403；改密成功写 password_changed_at 并吊销 refresh 族；动作落审计。
- **RBAC**：路由中间件（权限点）与业务层双重校验；roles：admin（全权限）、editor（生产+提交，不可审核发布、软删仅自有草稿）、reviewer（审核通过即发布/驳回/下架，不可编辑正文，M0 可置顶）。
- **可见性硬约束**：公共侧全部读路径强制 `status='published' AND deleted_at IS NULL`；详情对隐藏内容一律 404 防枚举；管理端按角色过滤（editor 可见自有全部非删除内容 + 全部已发布，保证提交后可跟踪）。
- **输入校验**：请求体严格模式（拒绝未知字段）+ 字段长度/大小限制；富文本白名单消毒（后端权威，剥离脚本/事件属性）；日志脱敏，不打印令牌与口令。
- **审计范围**：仅关键操作（登录/登出、发布、驳回、下架、软删、置顶、用户与角色变更、改密、分类变更），与业务在同一事务写入，操作者取自令牌而非客户端。

## 5. 部署与运行

本地开发：`docker compose` 提供 PostgreSQL 16；后端 air 热载（:8080），前端 Vite（:5173，/api 代理）；常用任务见根目录 Makefile 与 README。