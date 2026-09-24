import type { AuditAction } from '@/shared/types/api'

export interface AuditActionMeta {
  label: string
  /** Element Plus el-tag type */
  type: 'info' | 'warning' | 'success' | 'danger' | 'primary'
  /** 资源分组（用于筛选下拉分组展示） */
  group: string
}

/** 审计动作展示元数据：标签 + 徽标颜色 + 资源分组（与 openapi.yaml AuditAction 枚举对齐） */
export function auditActionMeta(action: AuditAction): AuditActionMeta {
  switch (action) {
    case 'login':
      return { label: '登录', type: 'success', group: '会话' }
    case 'failed_login':
      return { label: '登录失败', type: 'danger', group: '会话' }
    case 'logout':
      return { label: '登出', type: 'info', group: '会话' }
    case 'article_create':
      return { label: '创建文章', type: 'primary', group: '文章' }
    case 'article_update':
      return { label: '更新文章', type: 'primary', group: '文章' }
    case 'article_soft_delete':
      return { label: '删除文章', type: 'danger', group: '文章' }
    case 'article_submit':
      return { label: '提交审核', type: 'warning', group: '文章' }
    case 'article_approve':
      return { label: '审核通过', type: 'success', group: '文章' }
    case 'article_reject':
      return { label: '驳回文章', type: 'warning', group: '文章' }
    case 'article_unpublish':
      return { label: '下架文章', type: 'warning', group: '文章' }
    case 'article_pin':
      return { label: '文章置顶', type: 'primary', group: '文章' }
    case 'user_create':
      return { label: '创建用户', type: 'primary', group: '用户' }
    case 'user_update':
      return { label: '更新用户', type: 'primary', group: '用户' }
    case 'user_disable':
      return { label: '停用用户', type: 'danger', group: '用户' }
    case 'user_reset_password':
      return { label: '重置密码', type: 'warning', group: '用户' }
    case 'user_password_change':
      return { label: '修改密码', type: 'warning', group: '用户' }
    case 'category_create':
      return { label: '创建分类', type: 'primary', group: '分类' }
    case 'category_update':
      return { label: '更新分类', type: 'primary', group: '分类' }
    case 'category_soft_delete':
      return { label: '删除分类', type: 'danger', group: '分类' }
  }
}

/** 全部动作（按资源分组），供筛选下拉使用 */
export function auditActionGroups(): { group: string; actions: AuditAction[] }[] {
  const groups = new Map<string, AuditAction[]>()
  const all: AuditAction[] = [
    'login',
    'failed_login',
    'logout',
    'article_create',
    'article_update',
    'article_soft_delete',
    'article_submit',
    'article_approve',
    'article_reject',
    'article_unpublish',
    'article_pin',
    'user_create',
    'user_update',
    'user_disable',
    'user_reset_password',
    'user_password_change',
    'category_create',
    'category_update',
    'category_soft_delete'
  ]
  for (const action of all) {
    const { group } = auditActionMeta(action)
    const list = groups.get(group)
    if (list) list.push(action)
    else groups.set(group, [action])
  }
  return Array.from(groups, ([group, actions]) => ({ group, actions }))
}
