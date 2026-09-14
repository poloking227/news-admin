import axios, { AxiosError, type AxiosInstance } from 'axios'
import type { ApiErrorBody, ErrorEnvelope, LoginResponse } from '@/shared/types/api'

/** 契约统一基路径；本地由 Vite 代理 /api → 后端 :8080 */
export const API_BASE_URL = '/api/v1'

/**
 * 双提交 CSRF cookie 名（非 HttpOnly，前端读取后经 X-CSRF-Token 回传）。
 * 登录/刷新响应以 Set-Cookie 下发；与后端会话任务（契约 v4）对齐。
 */
export const CSRF_COOKIE_NAME = 'csrf_token'

/** 解析错误信封：遵循契约 { error: { code, message, details } }，对畸形载荷兜底 */
export function parseErrorEnvelope(payload: unknown): ApiErrorBody {
  if (
    payload !== null &&
    typeof payload === 'object' &&
    'error' in payload &&
    payload.error !== null &&
    typeof payload.error === 'object' &&
    'code' in payload.error &&
    'message' in payload.error &&
    typeof payload.error.code === 'string' &&
    typeof payload.error.message === 'string'
  ) {
    const error = payload.error as ApiErrorBody
    return { code: error.code, message: error.message, details: error.details }
  }
  return { code: 'INTERNAL_ERROR', message: '响应不符合错误信封契约：' + JSON.stringify(payload) }
}

/** 契约错误类型：携带稳定 code / message / details / HTTP status */
export class ApiError extends Error {
  readonly code: string
  readonly details: Record<string, unknown>
  readonly status?: number

  constructor(body: ApiErrorBody, status?: number) {
    super(body.message)
    this.name = 'ApiError'
    this.code = body.code
    this.details = body.details ?? {}
    this.status = status
  }

  get envelope(): ErrorEnvelope {
    return {
      error: {
        code: this.code,
        message: this.message,
        ...(Object.keys(this.details).length > 0 ? { details: this.details } : {})
      }
    }
  }
}

/** access token 按契约仅内存持有（1h），不落持久化存储 */
let accessToken: string | null = null

export function setAccessToken(token: string | null): void {
  accessToken = token
}

export function getAccessToken(): string | null {
  return accessToken
}

/** 双提交 CSRF 值（内存副本），随请求头 X-CSRF-Token 回传 */
let csrfToken: string | null = null

export function getCsrfToken(): string | null {
  return csrfToken
}

/** 从 document.cookie 同步 CSRF 值；登录/刷新响应回写 cookie 后调用 */
export function syncCsrfFromCookie(cookieName: string = CSRF_COOKIE_NAME): string | null {
  if (typeof document === 'undefined') return null
  const part = document.cookie
    .split(';')
    .map((c) => c.trim())
    .find((c) => c.startsWith(`${cookieName}=`))
  csrfToken = part ? decodeURIComponent(part.slice(cookieName.length + 1)) : null
  return csrfToken
}

/** 会话失效回调：刷新失败时由应用层注册（清理会话并跳转登录）；http 层不依赖 store 避免循环引用 */
type AuthFailureHandler = () => void
let authFailureHandler: AuthFailureHandler | null = null

export function setAuthFailureHandler(handler: AuthFailureHandler | null): void {
  authFailureHandler = handler
}

const http: AxiosInstance = axios.create({
  baseURL: API_BASE_URL,
  timeout: 15000,
  withCredentials: true
})

// 请求拦截：注入 Authorization: Bearer 与双提交 CSRF 头
http.interceptors.request.use((config) => {
  if (accessToken) {
    config.headers.set('Authorization', `Bearer ${accessToken}`)
  }
  if (csrfToken) {
    config.headers.set('X-CSRF-Token', csrfToken)
  }
  return config
})

/** 单飞刷新：并发 401 只触发一次 /auth/refresh，成功后共享新 token */
let refreshPromise: Promise<string | null> | null = null

export function refreshAccessToken(): Promise<string | null> {
  if (!refreshPromise) {
    refreshPromise = (async () => {
      try {
        const { data } = await http.post<LoginResponse>('/auth/refresh', null, {
          headers: { 'X-CSRF-Token': csrfToken ?? '' }
        })
        setAccessToken(data.accessToken)
        // 刷新会轮换 refresh cookie，并可能回写新的 CSRF cookie
        syncCsrfFromCookie()
        return data.accessToken
      } catch {
        setAccessToken(null)
        return null
      } finally {
        refreshPromise = null
      }
    })()
  }
  return refreshPromise
}

// 响应拦截：401 尝试单飞刷新并重放；其余错误折叠为契约 ApiError
http.interceptors.response.use(
  (response) => response,
  async (error: unknown) => {
    const axiosError = error as AxiosError<unknown>
    const { config, response } = axiosError
    const status = response?.status

    const isAuthEndpoint = (url: string | undefined): boolean =>
      Boolean(url && url.startsWith('/auth/'))

    if (status === 401 && config && !isAuthEndpoint(config.url)) {
      const token = await refreshAccessToken()
      if (token) {
        config.headers.set('Authorization', `Bearer ${token}`)
        return http.request(config)
      }
      authFailureHandler?.()
      throw new ApiError({ code: 'UNAUTHENTICATED', message: '登录已过期，请重新登录' }, 401)
    }

    if (response) {
      throw new ApiError(parseErrorEnvelope(response.data), status)
    }
    throw new ApiError({ code: 'NETWORK_ERROR', message: '网络异常，请稍后重试' })
  }
)

export { http }
