import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { ApiError } from '@/shared/api/http'
import { usePublicArticle } from '@/shared/composables/usePublicArticle'
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
  title: '详情文章',
  summary: '摘要',
  bodyHtml: '<p>正文</p>',
  categoryId: 'c1',
  categoryName: '时政',
  coverUrl: null,
  publishedAt: '2026-09-01T03:00:00Z',
  pinned: false
}

const mockedGet = vi.mocked(publicApi.getPublicArticle)

describe('usePublicArticle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('加载成功：article 就绪且 notFound=false', async () => {
    mockedGet.mockResolvedValue(item)

    const { article, loading, notFound, load } = usePublicArticle()
    await load('a1')

    expect(mockedGet).toHaveBeenCalledWith('a1')
    await nextTick()
    expect(article.value?.title).toBe('详情文章')
    expect(notFound.value).toBe(false)
    expect(loading.value).toBe(false)
  })

  it('后端 404（非 published/已删除）：标记 notFound 供前端跳 404 页', async () => {
    mockedGet.mockRejectedValue(new ApiError({ code: 'NOT_FOUND', message: '不存在' }, 404))

    const { article, notFound, load } = usePublicArticle()
    await load('hidden-id')

    expect(article.value).toBeNull()
    expect(notFound.value).toBe(true)
  })

  it('非 404 错误（如网络异常）向上抛出而非标记 404', async () => {
    mockedGet.mockRejectedValue(new ApiError({ code: 'RATE_LIMITED', message: '限流' }, 429))

    const { notFound, load } = usePublicArticle()
    await expect(load('a1')).rejects.toMatchObject({ code: 'RATE_LIMITED' })
    expect(notFound.value).toBe(false)
  })
})
