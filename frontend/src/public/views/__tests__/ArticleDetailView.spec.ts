import ElementPlus from 'element-plus'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import ArticleDetailView from '@/public/views/ArticleDetailView.vue'
import { ApiError } from '@/shared/api/http'
import type { PublicArticle } from '@/shared/types/api'

const replaceMock = vi.fn()

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: 'a1' } }),
  useRouter: () => ({ replace: replaceMock })
}))

vi.mock('@/shared/api/public', () => ({
  listPublicArticles: vi.fn(),
  getPublicArticle: vi.fn(),
  searchPublicArticles: vi.fn(),
  listPublicCategories: vi.fn()
}))

import * as publicApi from '@/shared/api/public'

const item: PublicArticle = {
  id: 'a1',
  title: '详情页标题',
  summary: '摘要',
  bodyHtml: '<p>正文段一</p><script>alert(1)</script>',
  categoryId: 'c1',
  categoryName: '时政',
  coverUrl: null,
  publishedAt: '2026-09-01T03:00:00Z',
  pinned: true,
  authorDisplayName: '张三'
}

function mountView() {
  return mount(ArticleDetailView, { global: { plugins: [ElementPlus] } })
}

describe('ArticleDetailView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    replaceMock.mockReset()
  })

  it('渲染标题/作者/正文，且 DOMPurify 移除危险标签', async () => {
    vi.mocked(publicApi.getPublicArticle).mockResolvedValue(item)

    const wrapper = mountView()
    await flushPromises()

    expect(publicApi.getPublicArticle).toHaveBeenCalledWith('a1')
    expect(wrapper.text()).toContain('详情页标题')
    expect(wrapper.text()).toContain('张三')
    expect(wrapper.text()).toContain('正文段一')
    expect(wrapper.html()).not.toContain('<script')
    expect(replaceMock).not.toHaveBeenCalled()
  })

  it('直连非 published：后端 404 → 前端跳 404 页', async () => {
    vi.mocked(publicApi.getPublicArticle).mockRejectedValue(
      new ApiError({ code: 'NOT_FOUND', message: '不存在' }, 404)
    )

    const wrapper = mountView()
    await flushPromises()

    expect(replaceMock).toHaveBeenCalledWith({ name: 'not-found' })
    expect(wrapper.text()).not.toContain('详情页标题')
  })
})
