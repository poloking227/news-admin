import { defineStore } from 'pinia'
import {
  approveArticle as approveArticleApi,
  createArticle as createArticleApi,
  deleteArticle as deleteArticleApi,
  listArticles as listArticlesApi,
  pinArticle as pinArticleApi,
  rejectArticle as rejectArticleApi,
  submitArticle as submitArticleApi,
  unpublishArticle as unpublishArticleApi,
  updateArticle as updateArticleApi
} from '@/shared/api/articles'
import type {
  Article,
  ArticleCreateRequest,
  ArticleListQuery,
  ArticleUpdateRequest
} from '@/shared/types/api'

interface ArticlesState {
  articles: Article[]
  total: number
  page: number
  pageSize: number
  loading: boolean
  /** 最近一次列表查询参数，供流转/变更后刷新 */
  lastQuery: ArticleListQuery
}

const DEFAULT_PAGE_SIZE = 10

/**
 * 文章管理端状态：分页列表 + 草稿 CRUD + 四态流转。
 * 所有写操作成功后统一以最近查询参数刷新列表（状态/排序可能变化，刷新最可靠）；
 * 409（非法迁移）与 422（业务规则）以 ApiError 向上传播，视图按错误信封展示。
 */
export const useArticlesStore = defineStore('articles', {
  state: (): ArticlesState => ({
    articles: [],
    total: 0,
    page: 1,
    pageSize: DEFAULT_PAGE_SIZE,
    loading: false,
    lastQuery: { page: 1, pageSize: DEFAULT_PAGE_SIZE }
  }),
  getters: {
    pageCount(): number {
      return Math.max(1, Math.ceil(this.total / this.pageSize))
    }
  },
  actions: {
    /** 拉取列表；本次查询参数存为 lastQuery 供 refresh 复用 */
    async fetchList(query: ArticleListQuery = {}): Promise<void> {
      const merged: ArticleListQuery = {
        ...this.lastQuery,
        ...query,
        page: query.page ?? 1,
        pageSize: query.pageSize ?? this.pageSize ?? DEFAULT_PAGE_SIZE
      }
      this.lastQuery = merged
      this.loading = true
      try {
        const data = await listArticlesApi(merged)
        this.articles = data.items
        this.total = data.total
        this.page = data.page
        this.pageSize = data.pageSize
      } finally {
        this.loading = false
      }
    },
    /** 以最近查询参数刷新当前页 */
    async refresh(): Promise<void> {
      await this.fetchList(this.lastQuery)
    },
    /** 保存草稿（新建：创建即置 draft） */
    async createDraft(payload: ArticleCreateRequest): Promise<Article> {
      const created = await createArticleApi(payload)
      await this.refresh()
      return created
    },
    /** 保存草稿（编辑，仅 draft 可编辑）；乐观锁版本经 If-Match 回传 */
    async updateDraft(
      id: string,
      payload: ArticleUpdateRequest,
      version: number
    ): Promise<Article> {
      const updated = await updateArticleApi(id, payload, version)
      await this.refresh()
      return updated
    },
    /** 软删文章 */
    async softDelete(id: string): Promise<void> {
      await deleteArticleApi(id)
      await this.refresh()
    },
    /** 提交审核：draft（含已驳回）| unpublished → pending_review */
    async submit(id: string): Promise<Article> {
      const article = await submitArticleApi(id)
      await this.refresh()
      return article
    },
    /** 审核通过（即发布）：pending_review → published */
    async approve(id: string): Promise<Article> {
      const article = await approveArticleApi(id)
      await this.refresh()
      return article
    },
    /** 驳回：pending_review → draft；理由必填（≤500 由服务端校验） */
    async reject(id: string, reason: string): Promise<Article> {
      if (!reason.trim()) {
        throw new Error('驳回理由必填')
      }
      const article = await rejectArticleApi(id, reason.trim())
      await this.refresh()
      return article
    },
    /** 下架：published → unpublished，reason 可选 */
    async unpublish(id: string, reason?: string): Promise<Article> {
      const article = await unpublishArticleApi(id, reason)
      await this.refresh()
      return article
    },
    /** 置顶/取消置顶 */
    async togglePin(id: string, pinned: boolean): Promise<Article> {
      const article = await pinArticleApi(id, pinned)
      await this.refresh()
      return article
    }
  }
})
