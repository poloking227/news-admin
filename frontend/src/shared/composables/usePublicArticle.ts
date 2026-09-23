import { ref } from 'vue'

import { getPublicArticle } from '@/shared/api/public'
import { ApiError } from '@/shared/api/http'
import type { PublicArticle } from '@/shared/types/api'

/**
 * 浏览端文章详情（供详情页使用）。
 * 非 published 一律 404：notFound=true（前端跳 404 页，不暴露存在性）。
 */
export function usePublicArticle() {
  const article = ref<PublicArticle | null>(null)
  const loading = ref(false)
  const notFound = ref(false)

  async function load(id: string): Promise<void> {
    loading.value = true
    notFound.value = false
    article.value = null
    try {
      article.value = await getPublicArticle(id)
    } catch (error) {
      if (error instanceof ApiError && error.status === 404) {
        notFound.value = true
        return
      }
      throw error
    } finally {
      loading.value = false
    }
  }

  function reset(): void {
    article.value = null
    notFound.value = false
    loading.value = false
  }

  return { article, loading, notFound, load, reset }
}
