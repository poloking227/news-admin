import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { usePublicArticlesStore } from '@/shared/stores/publicArticles'
import type { PublicArticle } from '@/shared/types/api'

vi.mock('@/shared/api/public', () => ({
  listPublicArticles: vi.fn(),
  getPublicArticle: vi.fn(),
  searchPublicArticles: vi.fn(),
  listPublicCategories: vi.fn()
}))

import * as publicApi from '@/shared/api/public'

const item: PublicArticle = {
  id: 'a1',
  title: '已发布文章',
  summary: '摘要',
  bodyHtml: '<p>正文</p>',
  categoryId: 'c1',
  categoryName: '时政',
  coverUrl: null,
  publishedAt: '2026-09-01T03:00:00Z',
  pinned: false
}

const mockedList = vi.mocked(publicApi.listPublicArticles)
const mockedSearch = vi.mocked(publicApi.searchPublicArticles)

describe('public articles store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    mockedList.mockResolvedValue({ items: [item], total: 1, page: 1, pageSize: 10 })
    mockedSearch.mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 10 })
  })

  it('fetchList 携带分类筛选与分页参数并填充列表', async () => {
    const store = usePublicArticlesStore()
    await store.fetchList({ categoryId: 'c1', page: 2, pageSize: 20 })

    expect(mockedList).toHaveBeenCalledWith({ categoryId: 'c1', page: 2, pageSize: 20 })
    expect(store.articles).toHaveLength(1)
    expect(store.total).toBe(1)
    expect(store.pageSize).toBe(10) // 以响应为准
  })

  it('fetchList 未指定分页时使用默认 page=1 与当前 pageSize', async () => {
    const store = usePublicArticlesStore()
    await store.fetchList()

    expect(mockedList).toHaveBeenCalledWith({ page: 1, pageSize: 10 })
  })

  it('search 携带关键词与分页并填充搜索结果', async () => {
    const store = usePublicArticlesStore()
    await store.search({ q: '能源', page: 1, pageSize: 10 })

    expect(mockedSearch).toHaveBeenCalledWith({ q: '能源', page: 1, pageSize: 10 })
    expect(store.total).toBe(0)
    expect(store.articles).toHaveLength(0)
  })

  it('搜索失败时向上抛出错误并复位 loading', async () => {
    mockedSearch.mockRejectedValue(new Error('upstream'))
    const store = usePublicArticlesStore()

    await expect(store.search({ q: 'x' })).rejects.toThrow('upstream')
    expect(store.loading).toBe(false)
  })
})
