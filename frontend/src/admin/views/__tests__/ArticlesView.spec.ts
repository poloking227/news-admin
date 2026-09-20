import ElementPlus from 'element-plus'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import ArticlesView from '@/admin/views/ArticlesView.vue'
import { useAuthStore } from '@/shared/stores/auth'
import type { Article, Category, CurrentUser } from '@/shared/types/api'

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

vi.mock('@/shared/api/categories', () => ({
  listCategories: vi.fn(),
  createCategory: vi.fn(),
  updateCategory: vi.fn(),
  deleteCategory: vi.fn()
}))

import * as articlesApi from '@/shared/api/articles'
import * as categoriesApi from '@/shared/api/categories'

const adminUser: CurrentUser = {
  id: 'u1',
  username: 'admin',
  displayName: '管理员',
  role: 'admin',
  status: 'active',
  permissions: [
    'articles:update',
    'articles:submit',
    'articles:approve',
    'articles:reject',
    'articles:unpublish',
    'articles:pin'
  ],
  mustChangePassword: false,
  createdAt: '2026-09-01T03:00:00Z',
  updatedAt: '2026-09-01T03:00:00Z'
}

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

const draftArticle: Article = {
  id: 'd1',
  title: '草稿标题示例',
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
  createdByName: '管理员',
  updatedBy: null,
  version: 1,
  createdAt: '2026-09-01T03:00:00Z',
  updatedAt: '2026-09-01T03:00:00Z'
}

function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setSession('token-1', adminUser)
  return mount(ArticlesView, {
    global: {
      plugins: [pinia, ElementPlus],
      stubs: {
        TiptapEditor: { template: '<div data-testid="tiptap-stub"></div>' }
      }
    }
  })
}

describe('ArticlesView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(articlesApi.listArticles).mockResolvedValue({
      items: [draftArticle],
      total: 1,
      page: 1,
      pageSize: 10
    })
    vi.mocked(categoriesApi.listCategories).mockResolvedValue([category])
  })

  it('挂载后渲染文章列表与状态徽标（草稿）', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(articlesApi.listArticles).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('文章管理')
    expect(wrapper.text()).toContain('草稿标题示例')
    expect(wrapper.text()).toContain('草稿')
  })

  it('点击编辑打开对话框并回填文章内容', async () => {
    const wrapper = mountView()
    await flushPromises()

    const editButton = wrapper.findAll('button').find((b) => b.text().includes('编辑'))
    expect(editButton).toBeTruthy()
    await editButton!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('编辑文章')
    const titleInput = wrapper.find('input[placeholder="文章标题"]')
    expect((titleInput.element as HTMLInputElement).value).toBe('草稿标题示例')
    // 分类回填：el-select 显示选中分类名
    expect(wrapper.text()).toContain('时政')
  })
})
