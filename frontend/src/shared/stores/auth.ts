import { defineStore } from 'pinia'
import {
  getCsrfToken,
  refreshAccessToken,
  setAccessToken,
  syncCsrfFromCookie
} from '@/shared/api/http'
import {
  changePassword as changePasswordApi,
  fetchCurrentUser as fetchCurrentUserApi,
  login as loginApi,
  logout as logoutApi
} from '@/shared/api/auth'
import type { CurrentUser, LoginRequest } from '@/shared/types/api'

interface AuthState {
  /** 契约 LoginResponse.accessToken；按契约仅内存持有（1h），不落持久化 */
  accessToken: string | null
  user: CurrentUser | null
}

/**
 * 会话状态：access token 内存持有 + refresh cookie（HttpOnly，由浏览器管理）。
 * 首登强制改密（M0）门控：mustChangePassword 为 true 时仅改密/登出/me 放行，
 * 其余管理行为由路由守卫拦截跳转改密页。
 */
export const useAuthStore = defineStore('auth', {
  state: (): AuthState => ({
    accessToken: null,
    user: null
  }),
  getters: {
    isAuthenticated: (state): boolean => Boolean(state.accessToken),
    role: (state): CurrentUser['role'] | null => state.user?.role ?? null,
    /** 首登强制改密标记（M0），true 时路由守卫仅放行改密与登出 */
    mustChangePassword: (state): boolean => Boolean(state.user?.mustChangePassword),
    hasPermission:
      (state) =>
      (permission: string): boolean =>
        Boolean(state.user?.permissions.includes(permission))
  },
  actions: {
    async login(payload: LoginRequest): Promise<void> {
      const { accessToken, user } = await loginApi(payload)
      // 登录响应会以 Set-Cookie 下发双提交 CSRF 值；刷新/登出端点依赖其回传
      syncCsrfFromCookie()
      this.setSession(accessToken, user)
    },
    setSession(accessToken: string, user: CurrentUser): void {
      this.accessToken = accessToken
      this.user = user
      setAccessToken(accessToken)
    },
    setUser(user: CurrentUser): void {
      this.user = user
    },
    /** 拉取当前用户（权限点/角色/强制改密标记以服务端为准） */
    async fetchMe(): Promise<void> {
      this.setUser(await fetchCurrentUserApi())
    },
    /**
     * 恢复会话：页面刷新后 access token（内存）丢失，凭 refresh cookie 静默恢复。
     * 无 CSRF 双提交值（即从未登录）时不发起请求。
     */
    async restoreSession(): Promise<boolean> {
      if (this.accessToken) return true
      syncCsrfFromCookie()
      if (!getCsrfToken()) return false
      const token = await refreshAccessToken()
      if (!token) return false
      try {
        this.setSession(token, await fetchCurrentUserApi())
        return true
      } catch {
        this.clear()
        return false
      }
    },
    /** 修改密码：成功后后端吊销会话，前端清空并提示重新登录 */
    async changePassword(oldPassword: string, newPassword: string): Promise<void> {
      await changePasswordApi({ oldPassword, newPassword })
      this.clear()
    },
    /** 登出：吊销 refresh 族；服务端吊销失败（如网络异常）不阻断本地会话清理 */
    async logout(): Promise<void> {
      try {
        await logoutApi()
      } catch {
        // 本地会话必须清理，登出不因服务端异常而失败
      } finally {
        this.clear()
      }
    },
    clear(): void {
      this.accessToken = null
      this.user = null
      setAccessToken(null)
    }
  }
})
