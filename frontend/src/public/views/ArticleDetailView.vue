<script setup lang="ts">
import DOMPurify from 'dompurify'
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { usePublicArticle } from '@/shared/composables/usePublicArticle'

const route = useRoute()
const router = useRouter()
const { article, loading, notFound, load } = usePublicArticle()

/** 服务端已白名单清洗；渲染前客户端 DOMPurify 再清洗（纵深防御） */
const sanitizedBodyHtml = computed(() => DOMPurify.sanitize(article.value?.bodyHtml ?? ''))

watch(
  () => route.params.id,
  (id) => {
    if (typeof id === 'string') {
      void load(id).catch(() => {
        /* 非 404 网络错误保持当前页，避免白屏 */
      })
    }
  },
  { immediate: true }
)

watch(
  notFound,
  (value) => {
    // 直连非 published（或已删除）详情：后端 404 → 前端跳 404 页（不暴露存在性）
    if (value) {
      void router.replace({ name: 'not-found' })
    }
  },
  { immediate: false }
)
</script>

<template>
  <section v-loading="loading" class="py-4">
    <template v-if="article">
      <div class="flex items-start gap-1">
        <span v-if="article.pinned" class="text-lg text-orange-500" title="置顶">★</span>
        <h1 class="text-3xl font-semibold text-gray-900">{{ article.title }}</h1>
      </div>

      <p class="mt-3 text-sm text-gray-500">
        {{ article.categoryName }}
        <span v-if="article.authorDisplayName"> · {{ article.authorDisplayName }}</span>
        · {{ new Date(article.publishedAt).toLocaleString() }}
      </p>

      <img
        v-if="article.coverUrl"
        :src="article.coverUrl"
        :alt="article.title"
        class="mt-6 max-h-96 w-full rounded-lg object-cover"
      />

      <div class="mt-8 max-w-none">
        <!-- 正文：Tiptap 生成（服务端白名单清洗）+ DOMPurify 客户端再清洗 -->
        <div class="prose" v-html="sanitizedBodyHtml"></div>
      </div>
    </template>
  </section>
</template>
