import { http } from '@/shared/api/http'
import type {
  ChangePasswordRequest,
  CurrentUser,
  LoginRequest,
  LoginResponse
} from '@/shared/types/api'

/** 登录：refresh token 由后端写入 HttpOnly cookie，前端仅接收 access token */
export async function login(payload: LoginRequest): Promise<LoginResponse> {
  const { data } = await http.post<LoginResponse>('/auth/login', payload)
  return data
}

/** 刷新访问令牌：依赖 HttpOnly refresh cookie，旋转后下发新 access token 与新 cookie */
export async function refreshSession(): Promise<LoginResponse> {
  const { data } = await http.post<LoginResponse>('/auth/refresh', null)
  return data
}

/** 当前用户：含权限点与首登强制改密标记（M0） */
export async function fetchCurrentUser(): Promise<CurrentUser> {
  const { data } = await http.get<CurrentUser>('/auth/me')
  return data
}

/** 修改密码：成功后会话被吊销，需重新登录 */
export async function changePassword(payload: ChangePasswordRequest): Promise<void> {
  await http.post('/auth/change-password', payload)
}

/** 登出：吊销 refresh 族 */
export async function logout(): Promise<void> {
  await http.post('/auth/logout')
}
