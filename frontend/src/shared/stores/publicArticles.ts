import { defineStore } from 'pinia'
import {
  listPublicArticles as listPublicArticlesApi,
  searchPublicArticles as searchPublicArticlesApi
} from '@/shared/api/public'
import type { PublicArticle, PublicListQuery, PublicSearchQuery } from '@/shared/types/api'

interface PublicArticlesState {
  articles: PublicArticle[]
  total: number
  page: number
  pageSize: number
  loading: boolean
}

/**
 * 浏览端文章列表/搜索结果：与首页/搜索页共用。
 * 排序由服务端保证（置顶优先 + 发布时间倒序）。
 */
export const usePublicArticlesStore = defineStore('publicArticles', {
  state: (): PublicArticlesState => ({
    articles: [],
    total: 0,
    page: 1,
    pageSize: 10,
    loading: false
  }),
  actions: {
    async fetchList(query: PublicListQuery = {}): Promise<void> {
      this.loading = true
      try {
        const data = await listPublicArticlesApi({
          page: query.page ?? 1,
          pageSize: query.pageSize ?? this.pageSize,
          ...(query.categoryId ? { categoryId: query.categoryId } : {}),
          ...(query.keyword ? { keyword: query.keyword } : {})
        })
        this.articles = data.items
        this.total = data.total
        this.page = data.page
        this.pageSize = data.pageSize
      } finally {
        this.loading = false
      }
    },
    async search(query: PublicSearchQuery): Promise<void> {
      this.loading = true
      try {
        const data = await searchPublicArticlesApi({
          q: query.q,
          page: query.page ?? 1,
          pageSize: query.pageSize ?? this.pageSize
        })
        this.articles = data.items
        this.total = data.total
        this.page = data.page
        this.pageSize = data.pageSize
      } finally {
        this.loading = false
      }
    }
  }
})
