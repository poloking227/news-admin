import MockAdapter from 'axios-mock-adapter'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ApiError,
  getAccessToken,
  http,
  parseErrorEnvelope,
  refreshAccessToken,
  setAccessToken,
  setAuthFailureHandler,
  syncCsrfFromCookie
} from '@/shared/api/http'
import type { CurrentUser } from '@/shared/types/api'

const user: CurrentUser = {
  id: '7f9b1f35-76a5-4d63-8581-9e6d3a9f0c54',
  username: 'admin',
  displayName: '系统管理员',
  role: 'admin',
  status: 'active',
  permissions: ['articles:submit', 'articles:approve'],
  mustChangePassword: false,
  createdAt: '2026-09-01T03:00:00Z',
  updatedAt: '2026-09-01T03:00:00Z'
}

/** axios-mock-adapter 中 config.headers 类型为 AxiosHeaders | 扁平对象 联合，统一按属性读取 */
function headerOf(config: { headers?: unknown }, name: string): unknown {
  if (!config.headers || typeof config.headers !== 'object') return undefined
  const headers = config.headers as Record<string, unknown>
  if (typeof headers.get === 'function') return headers.get(name)
  return headers[name] ?? headers[name.toLowerCase()]
}

describe('parseErrorEnvelope', () => {
  it('解析契约格式的错误信封', () => {
    const body = parseErrorEnvelope({
      error: { code: 'VALIDATION_FAILED', message: '字段非法', details: { username: '必填' } }
    })
    expect(body).toEqual({
      code: 'VALIDATION_FAILED',
      message: '字段非法',
      details: { username: '必填' }
    })
  })

  it('details 缺省时保持 undefined', () => {
    const body = parseErrorEnvelope({ error: { code: 'NOT_FOUND', message: '不存在' } })
    expect(body.details).toBeUndefined()
  })

  it('对不符合信封结构的载荷兜底为 INTERNAL_ERROR', () => {
    const body = parseErrorEnvelope({ message: 'no wrapper' })
    expect(body.code).toBe('INTERNAL_ERROR')
    expect(body.message).toContain('no wrapper')
  })

  it('对 null / 非对象载荷兜底', () => {
    expect(parseErrorEnvelope(null).code).toBe('INTERNAL_ERROR')
    expect(parseErrorEnvelope('oops').code).toBe('INTERNAL_ERROR')
  })
})

describe('ApiError', () => {
  it('携带稳定 code、message、status 与信封还原', () => {
    const err = new ApiError(
      { code: 'RATE_LIMITED', message: '请求过于频繁', details: { retryAfter: 60 } },
      429
    )
    expect(err).toBeInstanceOf(Error)
    expect(err.code).toBe('RATE_LIMITED')
    expect(err.message).toBe('请求过于频繁')
    expect(err.status).toBe(429)
    expect(err.envelope).toEqual({
      error: { code: 'RATE_LIMITED', message: '请求过于频繁', details: { retryAfter: 60 } }
    })
  })

  it('无 details 时信封不含 details 字段', () => {
    const err = new ApiError({ code: 'FORBIDDEN', message: '权限不足' }, 403)
    expect(err.envelope).toEqual({ error: { code: 'FORBIDDEN', message: '权限不足' } })
  })
})

describe('http client 拦截器', () => {
  let mock: MockAdapter

  beforeEach(() => {
    mock = new MockAdapter(http)
    setAccessToken(null)
    document.cookie = 'csrf_token=; Path=/; Max-Age=0'
    syncCsrfFromCookie()
  })

  afterEach(() => {
    setAuthFailureHandler(null)
    mock.reset()
  })

  it('请求注入 Authorization 与双提交 X-CSRF-Token 头', async () => {
    setAccessToken('token-1')
    document.cookie = 'csrf_token=csrf-1; Path=/'
    syncCsrfFromCookie()

    let seenAuth: unknown
    let seenCsrf: unknown
    mock.onGet(/\/protected$/).reply((config) => {
      seenAuth = headerOf(config, 'Authorization')
      seenCsrf = headerOf(config, 'X-CSRF-Token')
      return [200, { ok: true }]
    })

    const { data } = await http.get('/protected')
    expect(seenAuth).toBe('Bearer token-1')
    expect(seenCsrf).toBe('csrf-1')
    expect(data).toEqual({ ok: true })
  })

  it('并发 401 单飞刷新：仅一次 /auth/refresh，重放成功后共享新 token', async () => {
    setAccessToken('expired-token')
    document.cookie = 'csrf_token=csrf-1; Path=/'
    syncCsrfFromCookie()

    let refreshCalls = 0
    mock.onPost(/\/auth\/refresh$/).reply(() => {
      refreshCalls += 1
      return [200, { accessToken: 'fresh-token', expiresIn: 3600, user }]
    })
    mock.onGet(/\/protected$/).reply((config) => {
      return headerOf(config, 'Authorization') === 'Bearer fresh-token'
        ? [200, { ok: true }]
        : [401, { error: { code: 'UNAUTHENTICATED', message: 'expired' } }]
    })

    const [first, second] = await Promise.all([http.get('/protected'), http.get('/protected')])

    expect(first.data).toEqual({ ok: true })
    expect(second.data).toEqual({ ok: true })
    expect(refreshCalls).toBe(1)
    expect(getAccessToken()).toBe('fresh-token')
  })

  it('刷新失败：触发会话失效回调并折叠为 UNAUTHENTICATED', async () => {
    setAccessToken('expired-token')
    document.cookie = 'csrf_token=csrf-1; Path=/'
    syncCsrfFromCookie()

    const onFailure = vi.fn()
    setAuthFailureHandler(onFailure)

    mock.onPost(/\/auth\/refresh$/).reply(401, {
      error: { code: 'UNAUTHENTICATED', message: '刷新令牌失效' }
    })
    mock.onGet(/\/protected$/).reply(401, {
      error: { code: 'UNAUTHENTICATED', message: 'token expired' }
    })

    await expect(http.get('/protected')).rejects.toMatchObject({
      code: 'UNAUTHENTICATED',
      message: '登录已过期，请重新登录'
    })
    expect(onFailure).toHaveBeenCalledTimes(1)
    expect(getAccessToken()).toBeNull()
  })

  it('auth 端点自身的 401 不触发刷新循环', async () => {
    setAccessToken('expired-token')
    mock.onGet(/\/auth\/me$/).reply(401, {
      error: { code: 'UNAUTHENTICATED', message: '会话无效' }
    })

    await expect(http.get('/auth/me')).rejects.toMatchObject({ code: 'UNAUTHENTICATED' })
    expect(mock.history.post.filter((r) => /\/auth\/refresh$/.test(r.url ?? ''))).toHaveLength(0)
  })
})

describe('refreshAccessToken', () => {
  let mock: MockAdapter

  beforeEach(() => {
    mock = new MockAdapter(http)
    setAccessToken(null)
    document.cookie = 'csrf_token=; Path=/; Max-Age=0'
    syncCsrfFromCookie()
  })

  afterEach(() => {
    mock.reset()
  })

  it('刷新成功：回写新 token 并从 cookie 同步 CSRF', async () => {
    document.cookie = 'csrf_token=csrf-old; Path=/'
    syncCsrfFromCookie()
    mock.onPost(/\/auth\/refresh$/).reply(() => {
      document.cookie = 'csrf_token=csrf-new; Path=/'
      return [200, { accessToken: 'rotated-token', expiresIn: 3600, user }]
    })

    const token = await refreshAccessToken()

    expect(token).toBe('rotated-token')
    expect(getAccessToken()).toBe('rotated-token')
    expect(syncCsrfFromCookie()).toBe('csrf-new')
  })

  it('刷新失败：返回 null 并清空 token', async () => {
    document.cookie = 'csrf_token=csrf-1; Path=/'
    syncCsrfFromCookie()
    mock.onPost(/\/auth\/refresh$/).reply(500, {
      error: { code: 'INTERNAL', message: 'upstream error' }
    })

    const token = await refreshAccessToken()

    expect(token).toBeNull()
    expect(getAccessToken()).toBeNull()
  })
})
