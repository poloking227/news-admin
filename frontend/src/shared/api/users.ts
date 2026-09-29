import { http } from '@/shared/api/http'
import type {
  ResetPasswordResponse,
  User,
  UserCreateRequest,
  UserListQuery,
  UserPage,
  UserStatus,
  UserStatusUpdateRequest,
  UserUpdateRequest
} from '@/shared/types/api'

/** 用户列表（管理端）：role/status/keyword 过滤 + 分页 */
export async function listUsers(query: UserListQuery): Promise<UserPage> {
  const { data } = await http.get<UserPage>('/admin/users', { params: query })
  return data
}

/** 创建用户：以临时口令开通，mustChangePassword=true（首登强制改密） */
export async function createUser(payload: UserCreateRequest): Promise<User> {
  const { data } = await http.post<User>('/admin/users', payload)
  return data
}

/** 更新用户：修改展示名/角色；禁止降级/停用自己（后端 409） */
export async function updateUser(id: string, payload: UserUpdateRequest): Promise<User> {
  const { data } = await http.put<User>(`/admin/users/${id}`, payload)
  return data
}

/** 启用/停用用户：禁止停用自己；停用后其会话即时失效 */
export async function setUserStatus(id: string, status: UserStatus): Promise<User> {
  const { data } = await http.patch<User>(`/admin/users/${id}/status`, {
    status
  } satisfies UserStatusUpdateRequest)
  return data
}

/** 重置密码：生成临时口令并返回；被重置用户 mustChangePassword=true，撤销其现有会话 */
export async function resetUserPassword(id: string): Promise<ResetPasswordResponse> {
  const { data } = await http.post<ResetPasswordResponse>(`/admin/users/${id}/reset-password`)
  return data
}
