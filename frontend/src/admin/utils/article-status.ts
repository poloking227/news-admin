import type { Article } from '@/shared/types/api'

export interface ArticleStatusMeta {
  label: string
  /** Element Plus el-tag type */
  type: 'info' | 'warning' | 'success' | 'danger'
}

/**
 * 四态徽标：契约无独立 rejected 枚举，
 * status=draft 且 rejectReason 非空时显示「已驳回待修改」。
 */
export function articleStatusMeta(
  article: Pick<Article, 'status' | 'rejectReason'>
): ArticleStatusMeta {
  if (article.status === 'draft') {
    return article.rejectReason
      ? { label: '已驳回待修改', type: 'warning' }
      : { label: '草稿', type: 'info' }
  }
  switch (article.status) {
    case 'pending_review':
      return { label: '待审核', type: 'warning' }
    case 'published':
      return { label: '已发布', type: 'success' }
    case 'unpublished':
      return { label: '已下架', type: 'danger' }
  }
  return { label: article.status, type: 'info' }
}
