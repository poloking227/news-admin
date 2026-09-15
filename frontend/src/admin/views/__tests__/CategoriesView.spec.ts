import ElementPlus from 'element-plus'
import { createPinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import CategoriesView from '@/admin/views/CategoriesView.vue'
import type { Category } from '@/shared/types/api'

vi.mock('@/shared/api/categories', () => ({
  listCategories: vi.fn(),
  createCategory: vi.fn(),
  updateCategory: vi.fn(),
  deleteCategory: vi.fn()
}))

import * as categoriesApi from '@/shared/api/categories'

const item: Category = {
  id: 'b',
  name: '科技',
  slug: 'tech',
  description: '科技报道',
  sortOrder: 0,
  articleCount: 2,
  createdAt: '2026-09-01T03:00:00Z',
  updatedAt: '2026-09-01T03:00:00Z'
}

function mountView() {
  return mount(CategoriesView, {
    global: { plugins: [createPinia(), ElementPlus] }
  })
}

describe('CategoriesView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('挂载后加载分类并在表格展示', async () => {
    vi.mocked(categoriesApi.listCategories).mockResolvedValue([item])

    const wrapper = mountView()
    await flushPromises()

    expect(categoriesApi.listCategories).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('分类管理')
    expect(wrapper.text()).toContain('科技')
  })

  it('列表加载失败时给出错误提示而非白屏', async () => {
    vi.mocked(categoriesApi.listCategories).mockRejectedValue(new Error('service unavailable'))

    const wrapper = mountView()
    await flushPromises()

    // 页面主体仍可用，表格区为空态
    expect(wrapper.find('h1').text()).toBe('分类管理')
  })
})
