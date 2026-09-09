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

export interface ChangePasswordRequest {
  oldPassword: string
  newPassword: string
}