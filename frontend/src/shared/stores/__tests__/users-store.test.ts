import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useUsersStore } from '@/shared/stores/users'
import type { User } from '@/shared/types/api'

vi.mock('@/shared/api/users', () => ({
  listUsers: vi.fn(),
  createUser: vi.fn(),
  updateUser: vi.fn(),
  setUserStatus: vi.fn(),
  resetUserPassword: vi.fn()
}))

import * as usersApi from '@/shared/api/users'

const admin: User = {
  id: 'u1',
  username: 'admin',
  displayName: '管理员',
  role: 'admin',
  status: 'active',
  createdAt: '2026-09-01T03:00:00Z',
  updatedAt: '2026-09-01T03:00:00Z'
}

const editor: User = {
  id: 'u2',
  username: 'editor1',
  displayName: '编辑一',
  role: 'editor',
  status: 'active',
  createdAt: '2026-09-02T03:00:00Z',
  updatedAt: '2026-09-02T03:00:00Z'
}

const returned = (user: User): User => user

const mockedList = vi.mocked(usersApi.listUsers)
const mockedCreate = vi.mocked(usersApi.createUser)
const mockedUpdate = vi.mocked(usersApi.updateUser)
const mockedSetStatus = vi.mocked(usersApi.setUserStatus)
const mockedResetPassword = vi.mocked(usersApi.resetUserPassword)

describe('users store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockedList.mockResolvedValue({ items: [admin, editor], total: 2, page: 1, pageSize: 10 })
  })

  it('fetchList 携带筛选与分页参数并填充列表', async () => {
    const store = useUsersStore()
    await store.fetchList({
      role: 'editor',
      status: 'active',
      keyword: '编辑',
      page: 2,
      pageSize: 20
    })

    expect(mockedList).toHaveBeenCalledWith({
      role: 'editor',
      status: 'active',
      keyword: '编辑',
      page: 2,
      pageSize: 20
    })
    expect(store.users).toHaveLength(2)
    expect(store.total).toBe(2)
    expect(store.pageSize).toBe(10) // 以响应为准
  })

  it('未指定分页时默认 page=1/pageSize=10', async () => {
    const store = useUsersStore()
    await store.fetchList()

    expect(mockedList).toHaveBeenCalledWith({ page: 1, pageSize: 10 })
  })

  it('create 以临时口令开通用户（mustChangePassword 由后端置位）并刷新列表', async () => {
    mockedCreate.mockResolvedValue(returned(editor))
    const store = useUsersStore()
    await store.fetchList({ role: 'editor' })

    await store.create({
      username: 'editor2',
      displayName: '编辑二',
      role: 'editor',
      password: 'TempPass123'
    })

    expect(mockedCreate).toHaveBeenCalledWith({
      username: 'editor2',
      displayName: '编辑二',
      role: 'editor',
      password: 'TempPass123'
    })
    // 创建后 refresh 复用筛选参数
    expect(mockedList).toHaveBeenLastCalledWith({ role: 'editor', page: 1, pageSize: 10 })
  })

  it('update 调用更新接口并刷新', async () => {
    mockedUpdate.mockResolvedValue(returned({ ...editor, role: 'reviewer' }))
    const store = useUsersStore()

    await store.update('u2', { displayName: '新展示名', role: 'reviewer' })

    expect(mockedUpdate).toHaveBeenCalledWith('u2', { displayName: '新展示名', role: 'reviewer' })
    expect(mockedList).toHaveBeenCalled()
  })

  it('setStatus 启停用户并刷新（停用后其会话即时失效）', async () => {
    mockedSetStatus.mockResolvedValue(returned({ ...editor, status: 'disabled' }))
    const store = useUsersStore()

    await store.setStatus('u2', 'disabled')

    expect(mockedSetStatus).toHaveBeenCalledWith('u2', 'disabled')
    expect(mockedList).toHaveBeenCalled()
  })

  it('resetPassword 返回一次性临时口令并刷新', async () => {
    mockedResetPassword.mockResolvedValue({ temporaryPassword: 'NewTemp456' })
    const store = useUsersStore()

    const result = await store.resetPassword('u2')

    expect(mockedResetPassword).toHaveBeenCalledWith('u2')
    expect(result).toEqual({ temporaryPassword: 'NewTemp456' })
    expect(mockedList).toHaveBeenCalled()
  })
})
