<script setup lang="ts">
import { ref } from 'vue'

import type { PublicArticle } from '@/shared/types/api'

const props = defineProps<{
  article: PublicArticle
}>()

/** 封面加载失败/缺失时的降级占位 */
const coverFailed = ref(false)

function coverVisible(): boolean {
  return Boolean(props.article.coverUrl) && !coverFailed.value
}
</script>

<template>
  <article
    class="overflow-hidden rounded-lg border border-gray-200 bg-white transition-shadow hover:shadow-md"
  >
    <RouterLink :to="{ name: 'public-article-detail', params: { id: article.id } }">
      <div class="flex h-40 items-center justify-center bg-gray-100">
        <img
          v-if="coverVisible()"
          :src="article.coverUrl ?? undefined"
          :alt="article.title"
          class="h-full w-full object-cover"
          loading="lazy"
          @error="coverFailed = true"
        />
        <div
          v-else
          class="flex h-full w-full flex-col items-center justify-center gap-1 bg-gradient-to-br from-gray-100 to-gray-200"
        >
          <span class="text-3xl font-semibold text-gray-300">{{ article.title.slice(0, 1) }}</span>
          <span class="text-xs text-gray-400">暂无封面</span>
        </div>
      </div>
      <div class="p-4">
        <h2 class="line-clamp-2 min-h-12 text-base font-medium text-gray-900">
          <span v-if="article.pinned" class="mr-1 text-orange-500" title="置顶">★</span>
          {{ article.title }}
        </h2>
        <p class="mt-2 line-clamp-2 text-sm text-gray-500">{{ article.summary }}</p>
        <p class="mt-3 text-xs text-gray-400">
          {{ article.categoryName }} · {{ new Date(article.publishedAt).toLocaleDateString() }}
        </p>
      </div>
    </RouterLink>
  </article>
</template>
