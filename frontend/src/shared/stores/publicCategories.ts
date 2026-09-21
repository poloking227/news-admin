import { defineStore } from 'pinia'
import { listPublicCategories as listPublicCategoriesApi } from '@/shared/api/public'
import type { Category } from '@/shared/types/api'

interface PublicCategoriesState {
  categories: Category[]
  loading: boolean
}

/** 浏览端分类（仅含已发布文章的未删分类），用于首页筛选 chip */
export const usePublicCategoriesStore = defineStore('publicCategories', {
  state: (): PublicCategoriesState => ({
    categories: [],
    loading: false
  }),
  actions: {
    async fetchCategories(): Promise<void> {
      this.loading = true
      try {
        this.categories = await listPublicCategoriesApi()
      } finally {
        this.loading = false
      }
    }
  }
})
