/**
 * API 契约类型（源自 docs/openapi.yaml，契约 v4）。
 * 字段命名遵循契约的 JSON camelCase；与后端 DB snake_case 的映射由后端负责。
 */

export type Role = 'admin' | 'editor' | 'reviewer' | 'operator'

export type UserStatus = 'active' | 'disabled'

export type ArticleStatus = 'draft' | 'pending_review' | 'published' | 'unpublished'

/** 统一错误信封：响应体为 { error: { code, message, details } } */
export interface ApiErrorBody {
  /** 稳定错误码，如 VALIDATION_FAILED/UNAUTHENTICATED/FORBIDDEN/NOT_FOUND/CONFLICT/UNPROCESSABLE/RATE_LIMITED/INTERNAL */
  code: string
  message: string
  details?: Record<string, unknown>
}

export interface ErrorEnvelope {
  error: ApiErrorBody
}

/** 分页响应：page ≥ 1，pageSize 1-100 默认 10 */
export interface PageResult<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
}

export interface User {
  id: string
  username: string
  displayName: string
  role: Role
  status: UserStatus
  createdAt: string
  updatedAt: string
}

/** 当前用户：额外携带权限点与首登强制改密标记（M0） */
export interface CurrentUser extends User {
  permissions: string[]
  mustChangePassword: boolean
}

export interface LoginRequest {
  username: string
  password: string
}

export interface LoginResponse {
  accessToken: string
  expiresIn: number
  user: CurrentUser
}

/** 刷新端点与登录端点返回同一结构 */
export type RefreshResponse = LoginResponse

export interface ChangePasswordRequest {
  oldPassword: string
  newPassword: string
}

export interface Category {
  id: string
  name: string
  slug: string
  description: string | null
  sortOrder: number
  /** 文章数：公共侧为已发布数，管理端为全部非删数 */
  articleCount: number
  createdAt: string
  updatedAt: string
}

/** 创建分类：契约要求 name/slug 必填 */
export interface CategoryCreateRequest {
  name: string
  slug: string
  description?: string | null
  sortOrder?: number
}

/** 更新分类：字段均可选（部分更新语义） */
export interface CategoryUpdateRequest {
  name?: string
  slug?: string
  description?: string | null
  sortOrder?: number
}

/**
 * 文章权限点（契约未穷举枚举，对齐 RBAC：editor 生产+提交；reviewer/admin 审核/下架/置顶）。
 * 与后端 permissions 返回串联调时对齐。
 */
export const ARTICLE_PERMISSIONS = {
  update: 'articles:update',
  submit: 'articles:submit',
  approve: 'articles:approve',
  reject: 'articles:reject',
  unpublish: 'articles:unpublish',
  pin: 'articles:pin'
} as const

export interface Article {
  id: string
  title: string
  summary: string
  /** 白名单消毒后的富文本 HTML */
  bodyHtml: string
  /** 纯文本（供搜索）；不随公共端返回 */
  bodyText?: string
  categoryId: string
  categoryName: string
  coverUrl: string | null
  status: ArticleStatus
  /** 驳回 = draft + rejectReason/rejectedAt 非空（无独立 rejected 枚举） */
  rejectReason: string | null
  rejectedAt: string | null
  pinned: boolean
  pinnedAt: string | null
  submittedAt: string | null
  publishedAt: string | null
  unpublishedAt: string | null
  createdBy: string
  createdByName: string
  updatedBy: string | null
  /** 乐观锁版本：更新时以 If-Match 头回传 */
  version: number
  createdAt: string
  updatedAt: string
}

/** 创建草稿：契约要求 title/summary/bodyHtml/categoryId 必填，创建即置 draft */
export interface ArticleCreateRequest {
  title: string
  summary: string
  /** 原始输入，服务端消毒后存储；大小上限 2MB */
  bodyHtml: string
  categoryId: string
  coverUrl?: string | null
}

/** 更新文章：仅 draft（含已驳回）可编辑 */
export interface ArticleUpdateRequest {
  title?: string
  summary?: string
  bodyHtml?: string
  categoryId?: string
  coverUrl?: string | null
}

/** 驳回理由：必填 ≤500 */
export interface RejectRequest {
  reason: string
}

/** 下架原因：可选 ≤500 */
export interface UnpublishRequest {
  reason?: string
}

/** 置顶开关 */
export interface PinRequest {
  pinned: boolean
}

/** 文章列表查询：status/categoryId/keyword/pinned 过滤 + 分页（pageSize 1-100 默认 10） */
export interface ArticleListQuery {
  status?: ArticleStatus
  categoryId?: string
  keyword?: string
  pinned?: boolean
  page?: number
  pageSize?: number
}

export type ArticlePage = PageResult<Article>

/** 浏览端公共文章：仅 published；契约 v4 未含作者字段，authorDisplayName 预留供契约补充后展示 */
export interface PublicArticle {
  id: string
  title: string
  summary: string
  /** 服务端白名单清洗后的富文本 HTML；渲染前经 DOMPurify 客户端再清洗 */
  bodyHtml: string
  categoryId: string
  categoryName: string
  coverUrl: string | null
  publishedAt: string
  pinned: boolean
  /** 契约 v4 未定义；后端补充 createdBy/displayName 后转为必填 */
  authorDisplayName?: string
}

/** 浏览端列表查询：分类筛选/关键字 + 分页 */
export interface PublicListQuery {
  categoryId?: string
  keyword?: string
  page?: number
  pageSize?: number
}

/** 浏览端搜索结果查询：q 必填（minLength 1，空关键词不请求） */
export interface PublicSearchQuery {
  q: string
  page?: number
  pageSize?: number
}

export type PublicArticlePage = PageResult<PublicArticle>

/**
 * 用户管理权限点（契约 v4：users:manage → admin）。
 * 与后端 permissions 返回串联调时对齐。
 */
export const USER_PERMISSIONS = {
  manage: 'users:manage'
} as const

/** 审计查询权限点（契约 v4：audit:read → admin） */
export const AUDIT_PERMISSIONS = {
  read: 'audit:read'
} as const

/** 创建用户：临时口令开通，mustChangePassword=true（首登强制改密，同规则） */
export interface UserCreateRequest {
  username: string
  password: string
  displayName: string
  role: Role
}

/** 更新用户：修改展示名/角色；禁止停用或降级自己 */
export interface UserUpdateRequest {
  displayName?: string
  role?: Role
}

/** 启用/停用用户：禁止停用自己；停用后其会话即时失效 */
export interface UserStatusUpdateRequest {
  status: UserStatus
}

/** 重置密码响应：一次性临时口令，仅此响应返回；被重置用户 mustChangePassword=true */
export interface ResetPasswordResponse {
  temporaryPassword: string
}

/** 用户列表查询：role/status/keyword 过滤 + 分页（pageSize 1-100 默认 10） */
export interface UserListQuery {
  role?: Role
  status?: UserStatus
  keyword?: string
  page?: number
  pageSize?: number
}

export type UserPage = PageResult<User>

/** 审计动作（AuditAction 枚举，与 openapi.yaml 对齐） */
export type AuditAction =
  | 'login'
  | 'failed_login'
  | 'logout'
  | 'article_create'
  | 'article_update'
  | 'article_soft_delete'
  | 'article_submit'
  | 'article_approve'
  | 'article_reject'
  | 'article_unpublish'
  | 'article_pin'
  | 'user_create'
  | 'user_update'
  | 'user_disable'
  | 'user_reset_password'
  | 'user_password_change'
  | 'category_create'
  | 'category_update'
  | 'category_soft_delete'

/** 审计记录：before/after 为变更前后摘要快照（可能为 null） */
export interface AuditLog {
  id: number
  actorId: string
  actorName: string
  action: AuditAction
  resourceType: string
  resourceId: string
  before: Record<string, unknown> | null
  after: Record<string, unknown> | null
  ip: string
  createdAt: string
}

/** 审计查询：actor/action/resourceType/resourceId/时间窗 + 分页 */
export interface AuditLogListQuery {
  actorId?: string
  action?: AuditAction
  resourceType?: string
  resourceId?: string
  from?: string
  to?: string
  page?: number
  pageSize?: number
}

export type AuditLogPage = PageResult<AuditLog>
