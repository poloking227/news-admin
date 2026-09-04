# ADR-0002 会话与认证方案

- 状态：已接受
- 日期：2026-09-01

## 背景

管理端需防跨站请求伪造与令牌泄露，同时支持登出吊销与首登强制改密；浏览端匿名只读，不参与认证。

## 决策

- 密码以 bcrypt(12) 存储；登录限流并写入审计。
- access token（JWT，1h）由前端内存持有，请求头 `Authorization: Bearer`。
- refresh token（7d）写入 HttpOnly cookie（SameSite=Strict、Path 限 /auth）；刷新端点要求双提交 CSRF 校验；旋转 + 复用检测（旧令牌再次使用即吊销整个族）。
- 首登强制改密（M0）：`must_change_password=true` 期间仅 `/auth/me`、`/auth/change-password`、`/auth/logout` 放行；改密成功写 `password_changed_at` 并吊销现有会话。

## 影响

- 访问令牌不落持久化存储，降低 XSS 面；cookie 严格同源，配合双提交防 CSRF。
- 登出与改密均可即时吊销会话族，满足「关键操作可追溯」的审计要求。