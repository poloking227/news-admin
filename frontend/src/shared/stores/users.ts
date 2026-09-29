import { defineStore } from 'pinia'
import {
  createUser as createUserApi,
  listUsers as listUsersApi,
  resetUserPassword as resetUserPasswordApi,
  setUserStatus as setUserStatusApi,
  updateUser as updateUserApi
} from '@/shared/api/users'
import type {
  ResetPasswordResponse,
  User,
  UserCreateRequest,
  UserListQuery,
  UserStatus,
  UserUpdateRequest
} from '@/shared/types/api'

interface UsersState {
  users: User[]
  total: number
  page: number
  pageSize: number
  loading: boolean
  /** 最近一次列表查询参数，供变更后刷新复用 */
  lastQuery: UserListQuery
}

const DEFAULT_PAGE_SIZE = 10

/**
 * 管理端用户状态：分页列表 + 用户 CRUD + 启停 + 重置密码。
 * 写操作成功后统一以最近查询参数刷新列表；
 * 409（自保护/冲突）与 422（业务规则）以 ApiError 向上传播，视图按错误信封展示。
 */
export const useUsersStore = defineStore('users', {
  state: (): UsersState => ({
    users: [],
    total: 0,
    page: 1,
    pageSize: DEFAULT_PAGE_SIZE,
    loading: false,
    lastQuery: { page: 1, pageSize: DEFAULT_PAGE_SIZE }
  }),
  getters: {
    pageCount(): number {
      return Math.max(1, Math.ceil(this.total / this.pageSize))
    }
  },
  actions: {
    /** 拉取列表；本次查询参数存为 lastQuery 供 refresh 复用 */
    async fetchList(query: UserListQuery = {}): Promise<void> {
      const merged: UserListQuery = {
        ...this.lastQuery,
        ...query,
        page: query.page ?? 1,
        pageSize: query.pageSize ?? this.pageSize ?? DEFAULT_PAGE_SIZE
      }
      this.lastQuery = merged
      this.loading = true
      try {
        const data = await listUsersApi(merged)
        this.users = data.items
        this.total = data.total
        this.page = data.page
        this.pageSize = data.pageSize
      } finally {
        this.loading = false
      }
    },
    /** 以最近查询参数刷新当前页 */
    async refresh(): Promise<void> {
      await this.fetchList(this.lastQuery)
    },
    /** 创建用户：临时口令开通（mustChangePassword=true 由后端置位） */
    async create(payload: UserCreateRequest): Promise<User> {
      const created = await createUserApi(payload)
      await this.refresh()
      return created
    },
    /** 更新展示名/角色 */
    async update(id: string, payload: UserUpdateRequest): Promise<User> {
      const updated = await updateUserApi(id, payload)
      await this.refresh()
      return updated
    },
    /** 启用/停用：禁止停用自己（后端 409） */
    async setStatus(id: string, status: UserStatus): Promise<User> {
      const updated = await setUserStatusApi(id, status)
      await this.refresh()
      return updated
    },
    /** 重置密码：返回一次性临时口令；被重置用户 mustChangePassword=true */
    async resetPassword(id: string): Promise<ResetPasswordResponse> {
      const result = await resetUserPasswordApi(id)
      await this.refresh()
      return result
    }
  }
})
