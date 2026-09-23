import ElementPlus from 'element-plus'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import HomeView from '@/public/views/HomeView.vue'
import type { Category, PublicArticle } from '@/shared/types/api'

vi.mock('@/shared/api/public', () => ({
  listPublicArticles: vi.fn(),
  getPublicArticle: vi.fn(),
  searchPublicArticles: vi.fn(),
  listPublicCategories: vi.fn()
}))

import * as publicApi from '@/shared/api/public'

const category: Category = {
  id: 'c1',
  name: '时政',
  slug: 'politics',
  description: null,
  sortOrder: 0,
  articleCount: 1,
  createdAt: '2026-09-01T03:00:00Z',
  updatedAt: '2026-09-01T03:00:00Z'
}

const item: PublicArticle = {
  id: 'a1',
  title: '已发布文章标题',
  summary: '摘要内容',
  bodyHtml: '<p>正文</p>',
  categoryId: 'c1',
  categoryName: '时政',
  coverUrl: null,
  publishedAt: '2026-09-01T03:00:00Z',
  pinned: true
}

function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  return mount(HomeView, { global: { plugins: [pinia, ElementPlus] } })
}

describe('HomeView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(publicApi.listPublicArticles).mockResolvedValue({
      items: [item],
      total: 1,
      page: 1,
      pageSize: 10
    })
    vi.mocked(publicApi.listPublicCategories).mockResolvedValue([category])
  })

  it('渲染分类 chips 与文章卡片列表', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(publicApi.listPublicArticles).toHaveBeenCalledWith({ page: 1, pageSize: 10 })
    expect(publicApi.listPublicCategories).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('全部')
    expect(wrapper.text()).toContain('时政') // chip + 分类名
    expect(wrapper.text()).toContain('已发布文章标题')
    expect(wrapper.text()).toContain('★') // 置顶标记
  })

  it('空列表时显示空态提示', async () => {
    vi.mocked(publicApi.listPublicArticles).mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      pageSize: 10
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('该分类下暂无已发布内容')
  })
})
