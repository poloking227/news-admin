import { http } from '@/shared/api/http'
import type {
  Category,
  PublicArticle,
  PublicArticlePage,
  PublicListQuery,
  PublicSearchQuery
} from '@/shared/types/api'

/** 浏览端文章列表：仅 published 且未删除；置顶优先 + 发布时间倒序 */
export async function listPublicArticles(query: PublicListQuery): Promise<PublicArticlePage> {
  const { data } = await http.get<PublicArticlePage>('/public/articles', { params: query })
  return data
}

/** 浏览端文章详情：非 published 一律 404 */
export async function getPublicArticle(id: string): Promise<PublicArticle> {
  const { data } = await http.get<PublicArticle>(`/public/articles/${id}`)
  return data
}

/** 浏览端搜索：标题+摘要+正文匹配；仅 published；空关键词返回空结果 */
export async function searchPublicArticles(query: PublicSearchQuery): Promise<PublicArticlePage> {
  const { data } = await http.get<PublicArticlePage>('/public/search', { params: query })
  return data
}

/** 浏览端分类：仅包含已发布文章的未删分类 */
export async function listPublicCategories(): Promise<Category[]> {
  const { data } = await http.get<Category[]>('/public/categories')
  return data
}
