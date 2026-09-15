import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/shared/api/http'
import { useCategoriesStore } from '@/shared/stores/categories'
import type { Category } from '@/shared/types/api'

vi.mock('@/shared/api/categories', () => ({
  listCategories: vi.fn(),
  createCategory: vi.fn(),
  updateCategory: vi.fn(),
  deleteCategory: vi.fn()
}))

import * as categoriesApi from '@/shared/api/categories'

const itemA: Category = {
  id: 'a',
  name: '时政',
  slug: 'politics',
  description: null,
  sortOrder: 2,
  articleCount: 3,
  createdAt: '2026-09-01T03:00:00Z',
  updatedAt: '2026-09-01T03:00:00Z'
}

const itemB: Category = {
  id: 'b',
  name: '科技',
  slug: 'tech',
  description: '科技报道',
  sortOrder: 0,
  articleCount: 0,
  createdAt: '2026-09-01T03:00:00Z',
  updatedAt: '2026-09-01T03:00:00Z'
}

const mockedList = vi.mocked(categoriesApi.listCategories)
const mockedCreate = vi.mocked(categoriesApi.createCategory)
const mockedUpdate = vi.mocked(categoriesApi.updateCategory)
const mockedDelete = vi.mocked(categoriesApi.deleteCategory)

describe('categories store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('fetchCategories 加载分类列表并按 sortOrder 排序', async () => {
    mockedList.mockResolvedValue([itemA, itemB])

    const store = useCategoriesStore()
    await store.fetchCategories()

    expect(mockedList).toHaveBeenCalledTimes(1)
    expect(store.categories).toHaveLength(2)
    expect(store.sortedCategories.map((c) => c.id)).toEqual(['b', 'a'])
  })

  it('fetchCategories 请求失败时向上抛出并复位 loading', async () => {
    mockedList.mockRejectedValue(
      new ApiError({ code: 'UNAUTHENTICATED', message: '会话失效' }, 401)
    )

    const store = useCategoriesStore()
    await expect(store.fetchCategories()).rejects.toMatchObject({ code: 'UNAUTHENTICATED' })
    expect(store.loading).toBe(false)
  })

  it('createCategory 成功后追加到列表', async () => {
    mockedCreate.mockResolvedValue(itemB)

    const store = useCategoriesStore()
    await store.createCategory({ name: '科技', slug: 'tech' })

    expect(mockedCreate).toHaveBeenCalledWith({ name: '科技', slug: 'tech' })
    expect(store.categories).toHaveLength(1)
    expect(store.categories[0].slug).toBe('tech')
  })

  it('updateCategory 成功后替换列表中对应项', async () => {
    mockedUpdate.mockResolvedValue({ ...itemB, name: '信息技术' })

    const store = useCategoriesStore()
    store.categories = [itemB]
    await store.updateCategory('b', { name: '信息技术' })

    expect(store.categories[0].name).toBe('信息技术')
  })

  it('removeCategory 成功后从列表移除', async () => {
    mockedDelete.mockResolvedValue()

    const store = useCategoriesStore()
    store.categories = [itemA, itemB]
    await store.removeCategory('a')

    expect(mockedDelete).toHaveBeenCalledWith('a')
    expect(store.categories.map((c) => c.id)).toEqual(['b'])
  })

  it('removeCategory 遇到 409（有关联内容）时抛出 ApiError 且列表不变', async () => {
    mockedDelete.mockRejectedValue(
      new ApiError({ code: 'CONFLICT', message: '分类下存在文章' }, 409)
    )

    const store = useCategoriesStore()
    store.categories = [itemA, itemB]
    await expect(store.removeCategory('a')).rejects.toMatchObject({ code: 'CONFLICT' })
    expect(store.categories).toHaveLength(2)
  })
})
