import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getAccessToken, refreshAccessToken } from '@/shared/api/http'
import { useAuthStore } from '@/shared/stores/auth'
import type { CurrentUser } from '@/shared/types/api'

vi.mock('@/shared/api/auth', () => ({
  login: vi.fn(),
  refreshSession: vi.fn(),
  fetchCurrentUser: vi.fn(),
  changePassword: vi.fn(),
  logout: vi.fn()
}))

vi.mock('@/shared/api/http', async (importOriginal) => {
  const original = await importOriginal<typeof import('@/shared/api/http')>()
  return {
    ...original,
    refreshAccessToken: vi.fn()
  }
})

import * as authApi from '@/shared/api/auth'

const user: CurrentUser = {
  id: '7f9b1f35-76a5-4d63-8581-9e6d3a9f0c54',
  username: 'admin',
  displayName: '系统管理员',
  role: 'admin',
  status: 'active',
  permissions: ['articles:submit', 'articles:approve'],
  mustChangePassword: true,
  createdAt: '2026-09-01T03:00:00Z',
  updatedAt: '2026-09-01T03:00:00Z'
}

const normalUser: CurrentUser = { ...user, mustChangePassword: false }

const mockedLogin = vi.mocked(authApi.login)
const mockedLogout = vi.mocked(authApi.logout)
const mockedChangePassword = vi.mocked(authApi.changePassword)
const mockedFetchCurrentUser = vi.mocked(authApi.fetchCurrentUser)
const mockedRefresh = vi.mocked(refreshAccessToken)

describe('auth store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    // 清理双提交 CSRF cookie（Path=/，与测试内写入路径一致，避免 jsdom path 失配）
    document.cookie = 'csrf_token=; Path=/; Max-Age=0'
    storeCleanup()
  })

  function storeCleanup() {
    const store = useAuthStore()
    store.clear()
  }

  it('setSession 写入 token 与用户，且同步 http client 的 token 持有', () => {
    const store = useAuthStore()
    store.setSession('access-token-1', user)
    expect(store.accessToken).toBe('access-token-1')
    expect(store.user?.role).toBe('admin')
    expect(store.isAuthenticated).toBe(true)
    expect(getAccessToken()).toBe('access-token-1')
  })

  it('clear 清空会话并同步 http client', () => {
    const store = useAuthStore()
    store.setSession('access-token-1', user)
    store.clear()
    expect(store.isAuthenticated).toBe(false)
    expect(store.user).toBeNull()
    expect(getAccessToken()).toBeNull()
  })

  it('setUser 仅更新用户信息（保持 token）', () => {
    const store = useAuthStore()
    store.setSession('access-token-1', user)
    store.setUser({ ...user, mustChangePassword: false })
    expect(store.user?.mustChangePassword).toBe(false)
    expect(store.isAuthenticated).toBe(true)
  })

  it('login 成功：持有 token 与用户，且能从 cookie 同步双提交 CSRF 值', async () => {
    mockedLogin.mockResolvedValue({ accessToken: 'token-1', expiresIn: 3600, user: normalUser })
    document.cookie = 'csrf_token=csrf-abc; Path=/'

    const store = useAuthStore()
    await store.login({ username: 'editor1', password: 'password-1' })

    expect(mockedLogin).toHaveBeenCalledWith({ username: 'editor1', password: 'password-1' })
    expect(store.isAuthenticated).toBe(true)
    expect(store.role).toBe('admin')
    expect(store.mustChangePassword).toBe(false)
  })

  it('login 失败：ApiError 向上抛出且不建立会话', async () => {
    const failure = new (await import('@/shared/api/http')).ApiError(
      { code: 'UNAUTHENTICATED', message: '账号或密码错误' },
      401
    )
    mockedLogin.mockRejectedValue(failure)

    const store = useAuthStore()
    await expect(store.login({ username: 'admin', password: 'wrong' })).rejects.toMatchObject({
      code: 'UNAUTHENTICATED'
    })
    expect(store.isAuthenticated).toBe(false)
  })

  it('logout：调用吊销接口并清空会话', async () => {
    mockedLogout.mockResolvedValue()
    const store = useAuthStore()
    store.setSession('token-1', normalUser)

    await store.logout()

    expect(mockedLogout).toHaveBeenCalledTimes(1)
    expect(store.isAuthenticated).toBe(false)
    expect(getAccessToken()).toBeNull()
  })

  it('logout 网络异常时仍清理本地会话', async () => {
    mockedLogout.mockRejectedValue(new Error('network down'))
    const store = useAuthStore()
    store.setSession('token-1', normalUser)

    await store.logout()

    expect(store.isAuthenticated).toBe(false)
  })

  it('changePassword 成功：调用接口并清空会话（提示重新登录）', async () => {
    mockedChangePassword.mockResolvedValue()
    const store = useAuthStore()
    store.setSession('token-1', user)

    await store.changePassword('old-pass', 'new-pass-123')

    expect(mockedChangePassword).toHaveBeenCalledWith({
      oldPassword: 'old-pass',
      newPassword: 'new-pass-123'
    })
    expect(store.isAuthenticated).toBe(false)
    expect(store.user).toBeNull()
  })

  it('restoreSession：凭 refresh cookie 静默恢复，并获取当前用户', async () => {
    document.cookie = 'csrf_token=csrf-abc; Path=/'
    mockedRefresh.mockResolvedValue('fresh-token')
    mockedFetchCurrentUser.mockResolvedValue(normalUser)

    const store = useAuthStore()
    const restored = await store.restoreSession()

    expect(restored).toBe(true)
    expect(store.isAuthenticated).toBe(true)
    expect(store.accessToken).toBe('fresh-token')
    expect(store.hasPermission('articles:approve')).toBe(true)
  })

  it('restoreSession：无 CSRF 双提交值（从未登录）时不发起刷新', async () => {
    const store = useAuthStore()
    const restored = await store.restoreSession()

    expect(restored).toBe(false)
    expect(mockedRefresh).not.toHaveBeenCalled()
  })

  it('restoreSession：刷新失败时返回 false 且不建立会话', async () => {
    document.cookie = 'csrf_token=csrf-abc; Path=/'
    mockedRefresh.mockResolvedValue(null)

    const store = useAuthStore()
    const restored = await store.restoreSession()

    expect(restored).toBe(false)
    expect(store.isAuthenticated).toBe(false)
  })
})
