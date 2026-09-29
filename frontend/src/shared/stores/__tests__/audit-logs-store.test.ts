import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useAuditLogsStore } from '@/shared/stores/auditLogs'
import type { AuditLog } from '@/shared/types/api'

vi.mock('@/shared/api/auditLogs', () => ({
  listAuditLogs: vi.fn()
}))

import * as auditLogsApi from '@/shared/api/auditLogs'

const log: AuditLog = {
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

const mockedList = vi.mocked(auditLogsApi.listAuditLogs)

describe('audit logs store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockedList.mockResolvedValue({ items: [log], total: 1, page: 1, pageSize: 10 })
  })

  it('fetchList 携带筛选（action/actorId/时间窗）与分页参数并填充列表', async () => {
    const store = useAuditLogsStore()
    await store.fetchList({
      action: 'article_approve',
      actorId: 'u1',
      from: '2026-09-20T00:00:00Z',
      to: '2026-09-21T00:00:00Z',
      page: 2,
      pageSize: 20
    })

    expect(mockedList).toHaveBeenCalledWith({
      action: 'article_approve',
      actorId: 'u1',
      from: '2026-09-20T00:00:00Z',
      to: '2026-09-21T00:00:00Z',
      page: 2,
      pageSize: 20
    })
    expect(store.logs).toHaveLength(1)
    expect(store.logs[0].action).toBe('article_approve')
    expect(store.total).toBe(1)
  })

  it('未指定分页时默认 page=1/pageSize=10', async () => {
    const store = useAuditLogsStore()
    await store.fetchList()

    expect(mockedList).toHaveBeenCalledWith({ page: 1, pageSize: 10 })
  })

  it('refresh 复用最近一次查询参数', async () => {
    const store = useAuditLogsStore()
    await store.fetchList({ action: 'user_create' })

    await store.refresh()

    expect(mockedList).toHaveBeenLastCalledWith({ action: 'user_create', page: 1, pageSize: 10 })
  })
})
