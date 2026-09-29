import ElementPlus, { ElSelect } from 'element-plus'
import { createPinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AuditLogsView from '@/admin/views/AuditLogsView.vue'
import type { AuditLog } from '@/shared/types/api'

vi.mock('@/shared/api/auditLogs', () => ({
  listAuditLogs: vi.fn()
}))

import * as auditLogsApi from '@/shared/api/auditLogs'

const approveLog: AuditLog = {
  id: 1,
  actorId: 'u1',
  actorName: '管理员',
  action: 'article_approve',
  resourceType: 'article',
  resourceId: 'art-1',
  before: { status: 'pending_review' },
  after: { status: 'published' },
  ip: '127.0.0.1',
  createdAt: '2026-09-20T10:00:00Z'
}

const createLog: AuditLog = {
  id: 2,
  actorId: 'u2',
  actorName: '编辑一',
  action: 'user_create',
  resourceType: 'user',
  resourceId: 'u3',
  before: null,
  after: { username: 'newuser', role: 'editor' },
  ip: '10.0.0.8',
  createdAt: '2026-09-21T04:00:00Z'
}

function mockList(logs: AuditLog[]) {
  vi.mocked(auditLogsApi.listAuditLogs).mockResolvedValue({
    items: logs,
    total: logs.length,
    page: 1,
    pageSize: 10
  })
}

function mountView() {
  return mount(AuditLogsView, {
    global: { plugins: [createPinia(), ElementPlus] }
  })
}

describe('AuditLogsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockList([approveLog, createLog])
  })

  it('挂载后加载审计记录并展示动作/操作者/资源/变更摘要', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(auditLogsApi.listAuditLogs).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('审计日志')
    expect(wrapper.text()).toContain('审核通过')
    expect(wrapper.text()).toContain('创建用户')
    expect(wrapper.text()).toContain('管理员')
    expect(wrapper.text()).toContain('编辑一')
    expect(wrapper.text()).toContain('article')
    // 变更摘要：before → after
    expect(wrapper.text()).toContain('pending_review')
    expect(wrapper.text()).toContain('published')
  })

  it('按动作筛选后重新查询（page 复位为 1）', async () => {
    const wrapper = mountView()
    await flushPromises()

    // 动作下拉：v-model 直接置值（jsdom 下不依赖 teleport 下拉点选）
    const select = wrapper.findAllComponents(ElSelect).at(0)
    expect(select).toBeTruthy()
    await select!.vm.$emit('update:modelValue', 'article_approve')
    await flushPromises()

    const queryButton = wrapper.findAll('button').find((b) => b.text().trim() === '查询')
    await queryButton!.trigger('click')
    await flushPromises()

    expect(auditLogsApi.listAuditLogs).toHaveBeenLastCalledWith({
      action: 'article_approve',
      page: 1,
      pageSize: 10
    })
  })

  it('按操作者 ID 筛选：输入后回车查询', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.find('input[placeholder="操作者 ID（UUID）"]').setValue('u2')
    await wrapper.find('input[placeholder="操作者 ID（UUID）"]').trigger('keyup.enter')
    await flushPromises()

    expect(auditLogsApi.listAuditLogs).toHaveBeenLastCalledWith({
      actorId: 'u2',
      page: 1,
      pageSize: 10
    })
  })
})
