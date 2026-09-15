import { http } from '@/shared/api/http'
import type {
  Article,
  ArticleCreateRequest,
  ArticleListQuery,
  ArticlePage,
  ArticleUpdateRequest,
  PinRequest,
  RejectRequest,
  UnpublishRequest
} from '@/shared/types/api'

/** 文章列表（管理端）：过滤 + 分页 */
export async function listArticles(query: ArticleListQuery): Promise<ArticlePage> {
  const { data } = await http.get<ArticlePage>('/admin/articles', { params: query })
  return data
}

/** 文章详情（管理端） */
export async function getArticle(id: string): Promise<Article> {
  const { data } = await http.get<Article>(`/admin/articles/${id}`)
  return data
}

/** 创建草稿（创建即置 draft） */
export async function createArticle(payload: ArticleCreateRequest): Promise<Article> {
  const { data } = await http.post<Article>('/admin/articles', payload)
  return data
}

/** 更新文章（仅 draft 可编辑）；以 If-Match 头携带乐观锁 version */
export async function updateArticle(
  id: string,
  payload: ArticleUpdateRequest,
  version: number
): Promise<Article> {
  const { data } = await http.put<Article>(`/admin/articles/${id}`, payload, {
    headers: { 'If-Match': String(version) }
  })
  return data
}

/** 软删文章 */
export async function deleteArticle(id: string): Promise<void> {
  await http.delete(`/admin/articles/${id}`)
}

/** 提交审核：draft（含已驳回）| unpublished → pending_review；字段不齐全 422，非法迁移 409 */
export async function submitArticle(id: string): Promise<Article> {
  const { data } = await http.post<Article>(`/admin/articles/${id}/submit`)
  return data
}

/** 审核通过（即发布）：pending_review → published */
export async function approveArticle(id: string): Promise<Article> {
  const { data } = await http.post<Article>(`/admin/articles/${id}/approve`)
  return data
}

/** 驳回：pending_review → draft，reason 必填 ≤500 */
export async function rejectArticle(id: string, reason: string): Promise<Article> {
  const { data } = await http.post<Article>(`/admin/articles/${id}/reject`, {
    reason
  } satisfies RejectRequest)
  return data
}

/** 下架：published → unpublished，reason 可选 */
export async function unpublishArticle(id: string, reason?: string): Promise<Article> {
  const { data } = await http.post<Article>(
    `/admin/articles/${id}/unpublish`,
    reason ? ({ reason } satisfies UnpublishRequest) : undefined
  )
  return data
}

/** 置顶/取消置顶 */
export async function pinArticle(id: string, pinned: boolean): Promise<Article> {
  const { data } = await http.put<Article>(`/admin/articles/${id}/pin`, {
    pinned
  } satisfies PinRequest)
  return data
}
