import { http } from '@/shared/api/http'
import type { Category, CategoryCreateRequest, CategoryUpdateRequest } from '@/shared/types/api'

/** 分类列表（管理端）：含文章数；不含软删分类 */
export async function listCategories(): Promise<Category[]> {
  const { data } = await http.get<Category[]>('/admin/categories')
  return data
}

/** 创建分类 */
export async function createCategory(payload: CategoryCreateRequest): Promise<Category> {
  const { data } = await http.post<Category>('/admin/categories', payload)
  return data
}

/** 更新分类 */
export async function updateCategory(
  id: string,
  payload: CategoryUpdateRequest
): Promise<Category> {
  const { data } = await http.put<Category>(`/admin/categories/${id}`, payload)
  return data
}

/** 软删分类：存在已发布文章时返回 409（需先迁移内容） */
export async function deleteCategory(id: string): Promise<void> {
  await http.delete(`/admin/categories/${id}`)
}
