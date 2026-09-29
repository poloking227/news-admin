import ElementPlus, { ElSelect } from 'element-plus'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import UsersView from '@/admin/views/UsersView.vue'
import { useAuthStore } from '@/shared/stores/auth'
import type { CurrentUser, User } from '@/shared/types/api'

vi.mock('@/shared/api/users', () => ({
  listUsers: vi.fn(),
  createUser: vi.fn(),
  updateUser: vi.fn(),
  setUserStatus: vi.fn(),
  resetUserPassword: vi.fn()
}))

import * as usersApi from '@/shared/api/users'

const adminUser: CurrentUser = {
  id: 'u1',
  username: 'admin',
  displayName: '管理员',
  role: 'admin',
  status: 'active',
  permissions: ['users:manage'],
  mustChangePassword: false,
  createdAt: '2026-09-01T03:00:00Z',
  updatedAt: '2026-09-01T03:00:00Z'
}

const selfUser: User = {
  id: 'u1',
  username: 'admin',
  displayName: '管理员',
  role: 'admin',
  status: 'active',
  createdAt: '2026-09-01T03:00:00Z',
  updatedAt: '2026-09-01T03:00:00Z'
}

const otherUser: User = {
  id: 'u2',
  username: 'editor1',
  displayName: '编辑一',
  role: 'editor',
  status: 'active',
  createdAt: '2026-09-02T03:00:00Z',
  updatedAt: '2026-09-02T03:00:00Z'
}

function mockList(users: User[]) {
  vi.mocked(usersApi.listUsers).mockResolvedValue({
    items: users,
    total: users.length,
    page: 1,
    pageSize: 10
  })
}

function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setSession('token-1', adminUser)
  return mount(UsersView, {
    global: { plugins: [pinia, ElementPlus] }
  })
}

describe('UsersView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockList([selfUser, otherUser])
  })

  it('挂载后加载用户列表并渲染用户名/角色/状态', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(usersApi.listUsers).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('用户管理')
    expect(wrapper.text()).toContain('admin')
    expect(wrapper.text()).toContain('编辑一')
    expect(wrapper.text()).toContain('管理员')
    expect(wrapper.text()).toContain('编辑')
    expect(wrapper.text()).toContain('启用')
  })

  it('自保护：当前登录用户的停用按钮禁用，其他用户可停用', async () => {
    const wrapper = mountView()
    await flushPromises()

    const disableButtons = wrapper.findAll('button').filter((b) => b.text().trim() === '停用')
    expect(disableButtons.length).toBe(2)
    // 首行（自己）停用按钮禁用；第二行（他人）可用
    expect(disableButtons[0].attributes('disabled')).toBeDefined()
    expect(disableButtons[1].attributes('disabled')).toBeUndefined()
  })

  it('新建用户：提交携带临时口令并回显于结果框（仅一次）', async () => {
    vi.mocked(usersApi.createUser).mockResolvedValue(otherUser)

    const wrapper = mountView()
    await flushPromises()

    const createButton = wrapper.findAll('button').find((b) => b.text().includes('新建用户'))
    expect(createButton).toBeTruthy()
    await createButton!.trigger('click')
    await flushPromises()

    await wrapper.find('input[placeholder="登录用户名"]').setValue('newuser')
    await wrapper.find('input[placeholder="显示名称"]').setValue('新用户')
    await wrapper.find('input[placeholder="8-72 位"]').setValue('TempPass123')

    // 角色下拉：新建对话框中的最后一个 ElSelect（v-model 直接置值）
    const selects = wrapper.findAllComponents(ElSelect)
    await selects.at(-1)!.vm.$emit('update:modelValue', 'editor')
    await flushPromises()

    const submit = wrapper.findAll('button').find((b) => b.text().includes('创建用户'))
    await submit!.trigger('click')
    await flushPromises()

    expect(usersApi.createUser).toHaveBeenCalledWith({
      username: 'newuser',
      displayName: '新用户',
      role: 'editor',
      password: 'TempPass123'
    })
    // 创建成功后回显临时口令（readonly input 值）与首登强制改密提示
    const passwordInput = wrapper.findAll('input[readonly]').at(-1)!
    expect((passwordInput.element as HTMLInputElement).value).toBe('TempPass123')
    expect(wrapper.text()).toContain('该用户首次登录必须修改密码')
  })

  it('列表加载失败时给出错误提示而非白屏', async () => {
    vi.mocked(usersApi.listUsers).mockRejectedValue(new Error('service unavailable'))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('h1').text()).toBe('用户管理')
  })
})
