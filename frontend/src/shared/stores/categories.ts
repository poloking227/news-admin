import { defineStore } from 'pinia'
import {
  createCategory as createCategoryApi,
  deleteCategory as deleteCategoryApi,
  listCategories as listCategoriesApi,
  updateCategory as updateCategoryApi
} from '@/shared/api/categories'
import type { Category, CategoryCreateRequest, CategoryUpdateRequest } from '@/shared/types/api'

interface CategoriesState {
  categories: Category[]
  loading: boolean
}

/** 管理端分类状态：列表为契约返回的完整数组（含文章数、不含软删） */
export const useCategoriesStore = defineStore('categories', {
  state: (): CategoriesState => ({
    categories: [],
    loading: false
  }),
  getters: {
    /** 契约无序要求，按 sortOrder 升序展示 */
    sortedCategories: (state): Category[] =>
      [...state.categories].sort((a, b) => a.sortOrder - b.sortOrder)
  },
  actions: {
    async fetchCategories(): Promise<void> {
      this.loading = true
      try {
        this.categories = await listCategoriesApi()
      } finally {
        this.loading = false
      }
    },
    async createCategory(payload: CategoryCreateRequest): Promise<void> {
      const created = await createCategoryApi(payload)
      this.categories.push(created)
    },
    async updateCategory(id: string, payload: CategoryUpdateRequest): Promise<void> {
      const updated = await updateCategoryApi(id, payload)
      const index = this.categories.findIndex((c) => c.id === id)
      if (index >= 0) this.categories[index] = updated
    },
    /** 软删：成功后从列表移除；有关联内容时后端 409 会在调用方以 ApiError 呈现 */
    async removeCategory(id: string): Promise<void> {
      await deleteCategoryApi(id)
      this.categories = this.categories.filter((c) => c.id !== id)
    }
  }
})
