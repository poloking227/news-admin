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
