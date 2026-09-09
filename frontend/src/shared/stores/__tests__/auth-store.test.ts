import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { getAccessToken } from '@/shared/api/http'
import { useAuthStore } from '@/shared/stores/auth'
import type { CurrentUser } from '@/shared/types/api'

const user: CurrentUser = {
  id: '7f9b1f35-76a5-4d63-8581-9e6d3a9f0c54',
  username: 'admin',
  displayName: '系统管理员',
  role: 'admin',
  status: 'active',
  permissions: ['articles:submit', 'articles:approve'],
  mustChangePassword: true,
  createdAt: '2026-09-01T03:00:00Z',
  updatedAt: '2026-09-01T03:00:00Z',
}

describe('auth store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

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
})