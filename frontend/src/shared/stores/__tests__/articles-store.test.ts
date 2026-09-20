import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/shared/api/http'
import { useArticlesStore } from '@/shared/stores/articles'
import type { Article } from '@/shared/types/api'

vi.mock('@/shared/api/articles', () => ({
  listArticles: vi.fn(),
  getArticle: vi.fn(),
  createArticle: vi.fn(),
  updateArticle: vi.fn(),
  deleteArticle: vi.fn(),
  submitArticle: vi.fn(),
  approveArticle: vi.fn(),
  rejectArticle: vi.fn(),
  unpublishArticle: vi.fn(),
  pinArticle: vi.fn()
}))

import * as articlesApi from '@/shared/api/articles'

const draft: Article = {
  id: 'd1',
  title: '草稿文章',
  summary: '摘要',
  bodyHtml: '<p>内容</p>',
  categoryId: 'c1',
  categoryName: '时政',
  coverUrl: null,
  status: 'draft',
  rejectReason: null,
  rejectedAt: null,
  pinned: false,
  pinnedAt: null,
  submittedAt: null,
  publishedAt: null,
  unpublishedAt: null,
  createdBy: 'u1',
  createdByName: 'editor1',
  updatedBy: null,
  version: 1,
  createdAt: '2026-09-01T03:00:00Z',
  updatedAt: '2026-09-01T03:00:00Z'
}

const returned = (article: Article): Article => article

const mockedList = vi.mocked(articlesApi.listArticles)
const mockedCreate = vi.mocked(articlesApi.createArticle)
const mockedUpdate = vi.mocked(articlesApi.updateArticle)
const mockedDelete = vi.mocked(articlesApi.deleteArticle)
const mockedSubmit = vi.mocked(articlesApi.submitArticle)
const mockedApprove = vi.mocked(articlesApi.approveArticle)
const mockedReject = vi.mocked(articlesApi.rejectArticle)
const mockedUnpublish = vi.mocked(articlesApi.unpublishArticle)
const mockedPin = vi.mocked(articlesApi.pinArticle)

describe('articles store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockedList.mockResolvedValue({ items: [draft], total: 1, page: 1, pageSize: 10 })
  })

  it('fetchList 携带筛选与分页参数并填充列表', async () => {
    const store = useArticlesStore()
    await store.fetchList({ status: 'pending_review', page: 2, pageSize: 20 })

    expect(mockedList).toHaveBeenCalledWith({
      status: 'pending_review',
      page: 2,
      pageSize: 20
    })
    expect(store.articles).toHaveLength(1)
    expect(store.total).toBe(1)
    expect(store.page).toBe(1) // 以响应为准
    expect(store.pageSize).toBe(10)
  })

  it('未指定分页时默认 page=1/pageSize=10', async () => {
    const store = useArticlesStore()
    await store.fetchList()

    expect(mockedList).toHaveBeenCalledWith({ page: 1, pageSize: 10 })
  })

  it('createDraft 保存草稿后以最近查询刷新列表', async () => {
    mockedCreate.mockResolvedValue(returned(draft))
    const store = useArticlesStore()
    await store.fetchList({ status: 'draft' })

    await store.createDraft({
      title: '新草稿',
      summary: 's',
      bodyHtml: '<p>x</p>',
      categoryId: 'c1'
    })

    expect(mockedCreate).toHaveBeenCalledWith({
      title: '新草稿',
      summary: 's',
      bodyHtml: '<p>x</p>',
      categoryId: 'c1'
    })
    // 创建后 refresh 复用筛选参数
    expect(mockedList).toHaveBeenLastCalledWith({ status: 'draft', page: 1, pageSize: 10 })
  })

  it('updateDraft 携带乐观锁版本调用更新接口并刷新', async () => {
    mockedUpdate.mockResolvedValue(returned({ ...draft, version: 2 }))
    const store = useArticlesStore()

    await store.updateDraft('d1', { title: '改标题' }, 1)

    expect(mockedUpdate).toHaveBeenCalledWith('d1', { title: '改标题' }, 1)
    expect(mockedList).toHaveBeenCalled()
  })

  it('四态流转：submit/approve/unpublish/togglePin 各自调用接口并刷新', async () => {
    mockedSubmit.mockResolvedValue(returned({ ...draft, status: 'pending_review' }))
    mockedApprove.mockResolvedValue(returned({ ...draft, status: 'published' }))
    mockedUnpublish.mockResolvedValue(returned({ ...draft, status: 'unpublished' }))
    mockedPin.mockResolvedValue(returned({ ...draft, pinned: true }))

    const store = useArticlesStore()

    await store.submit('d1')
    expect(mockedSubmit).toHaveBeenCalledWith('d1')

    await store.approve('d1')
    expect(mockedApprove).toHaveBeenCalledWith('d1')

    await store.unpublish('d1', '内容过期')
    expect(mockedUnpublish).toHaveBeenCalledWith('d1', '内容过期')

    await store.togglePin('d1', true)
    expect(mockedPin).toHaveBeenCalledWith('d1', true)
    // submit/approve/unpublish/togglePin 各触发一次以 lastQuery 刷新
    expect(mockedList.mock.calls.length).toBe(4)
  })

  it('reject 理由为空时拒绝执行且不调用接口', async () => {
    const store = useArticlesStore()

    await expect(store.reject('d1', '   ')).rejects.toThrow('驳回理由必填')
    expect(mockedReject).not.toHaveBeenCalled()
    expect(mockedList).not.toHaveBeenCalled()
  })

  it('reject 携带理由（trim）调用接口并刷新', async () => {
    mockedReject.mockResolvedValue(returned({ ...draft, rejectReason: '缺来源', rejectedAt: 'x' }))
    const store = useArticlesStore()

    await store.reject('d1', '  缺来源  ')

    expect(mockedReject).toHaveBeenCalledWith('d1', '缺来源')
    expect(mockedList).toHaveBeenCalled()
  })

  it('流转遇到 409 非法迁移：ApiError 向上传播且列表不刷新', async () => {
    mockedSubmit.mockRejectedValue(
      new ApiError({ code: 'CONFLICT', message: '当前状态不允许提交' }, 409)
    )
    const store = useArticlesStore()
    await store.fetchList({ status: 'pending_review' })
    const callsBefore = mockedList.mock.calls.length

    await expect(store.submit('d1')).rejects.toMatchObject({
      code: 'CONFLICT',
      message: '当前状态不允许提交'
    })

    expect(mockedList.mock.calls.length).toBe(callsBefore)
    expect(store.articles).toHaveLength(1)
  })

  it('softDelete 调用软删接口并刷新', async () => {
    mockedDelete.mockResolvedValue()
    const store = useArticlesStore()

    await store.softDelete('d1')

    expect(mockedDelete).toHaveBeenCalledWith('d1')
    expect(mockedList).toHaveBeenCalled()
  })
})
