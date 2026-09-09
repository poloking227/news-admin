import axios, { AxiosError, type AxiosInstance } from 'axios'
import type { ApiErrorBody, ErrorEnvelope } from '@/shared/types/api'

/** 契约统一基路径；本地由 Vite 代理 /api → 后端 :8080 */
export const API_BASE_URL = '/api/v1'

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
        ...(Object.keys(this.details).length > 0 ? { details: this.details } : {}),
      },
    }
  }
}

/** access token 的内存持有；持久化与刷新逻辑随会话模块（C20）落地 */
let accessToken: string | null = null

export function setAccessToken(token: string | null): void {
  accessToken = token
}

export function getAccessToken(): string | null {
  return accessToken
}

/**
 * 401 刷新占位：真实实现（单飞并发去重、双提交 CSRF、旋转）随会话模块落地。
 * 现阶段恒返回 null，保证拦截器链路完整可测。
 */
async function refreshAccessToken(_reason: string): Promise<string | null> {
  return null
}

const http: AxiosInstance = axios.create({
  baseURL: API_BASE_URL,
  timeout: 15000,
})

// 请求拦截：注入 Authorization: Bearer
http.interceptors.request.use((config) => {
  if (accessToken) {
    config.headers.Authorization = `Bearer ${accessToken}`
  }
  return config
})

// 响应拦截：401 尝试刷新（占位）并重放；其余错误折叠为契约 ApiError
http.interceptors.response.use(
  (response) => response,
  async (error: unknown) => {
    const axiosError = error as AxiosError<unknown>
    const { config, response } = axiosError
    const status = response?.status

    const isAuthEndpoint = (url: string | undefined): boolean =>
      Boolean(url && url.startsWith('/auth/'))

    if (status === 401 && config && !isAuthEndpoint(config.url)) {
      const token = await refreshAccessToken('access token expired')
      if (token) {
        setAccessToken(token)
        config.headers.Authorization = `Bearer ${token}`
        return http.request(config)
      }
    }

    if (response) {
      throw new ApiError(parseErrorEnvelope(response.data), status)
    }
    throw new ApiError(
      { code: 'NETWORK_ERROR', message: '网络异常，请稍后重试' },
      undefined,
    )
  },
)

export { http }