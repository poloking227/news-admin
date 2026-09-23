import ElementPlus from 'element-plus'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import SearchView from '@/public/views/SearchView.vue'

vi.mock('@/shared/api/public', () => ({
  listPublicArticles: vi.fn(),
  getPublicArticle: vi.fn(),
  searchPublicArticles: vi.fn(),
  listPublicCategories: vi.fn()
}))

import * as publicApi from '@/shared/api/public'

function mountView() {
  const pinia = createPinia()
  setActivePinia(pinia)
  return mount(SearchView, { global: { plugins: [pinia, ElementPlus] } })
}

describe('SearchView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('初始状态显示引导空态且不触发搜索', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('输入关键词开始搜索')
    expect(publicApi.searchPublicArticles).not.toHaveBeenCalled()
  })

  it('空关键词提交不触发搜索请求', async () => {
    const wrapper = mountView()
    const input = wrapper.find('input')
    await input.setValue('   ')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(publicApi.searchPublicArticles).not.toHaveBeenCalled()
  })

  it('非空关键词提交后展示结果与摘要行', async () => {
    vi.mocked(publicApi.searchPublicArticles).mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      pageSize: 10
    })

    const wrapper = mountView()
    await wrapper.find('input').setValue('能源')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(publicApi.searchPublicArticles).toHaveBeenCalledWith({
      q: '能源',
      page: 1,
      pageSize: 10
    })
    expect(wrapper.text()).toContain('「能源」的结果')
    expect(wrapper.text()).toContain('未找到与「能源」相关的内容')
  })
})
