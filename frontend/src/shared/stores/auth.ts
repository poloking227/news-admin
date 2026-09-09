import { defineStore } from 'pinia'
import { setAccessToken } from '@/shared/api/http'
import type { CurrentUser } from '@/shared/types/api'

interface AuthState {
  /** 契约 LoginResponse.accessToken；统一由 http client 的拦截器持有 */
  accessToken: string | null
  user: CurrentUser | null
}

/**
 * 会话状态占位：仅维护 access token 与当前用户，
 * 首登强制改密（mustChangePassword）门控、refresh 旋转随会话模块（C20）落地。
 */
export const useAuthStore = defineStore('auth', {
  state: (): AuthState => ({
    accessToken: null,
    user: null,
  }),
  getters: {
    isAuthenticated: (state): boolean => Boolean(state.accessToken),
    role: (state): CurrentUser['role'] | null => state.user?.role ?? null,
  },
  actions: {
    setSession(accessToken: string, user: CurrentUser): void {
      this.accessToken = accessToken
      this.user = user
      setAccessToken(accessToken)
    },
    setUser(user: CurrentUser): void {
      this.user = user
    },
    clear(): void {
      this.accessToken = null
      this.user = null
      setAccessToken(null)
    },
  },
})