<script setup lang="ts">
import { ref } from 'vue'

import ArticleCard from '@/public/components/ArticleCard.vue'
import PaginationBar from '@/shared/components/PaginationBar.vue'
import { usePublicArticlesStore } from '@/shared/stores/publicArticles'

const articlesStore = usePublicArticlesStore()

const keyword = ref('')
const page = ref(1)
const pageSize = ref(10)
const submittedKeyword = ref('')
const hasSearched = ref(false)

/** 空关键词不触发搜索 */
async function doSearch() {
  const q = keyword.value.trim()
  if (!q) return
  submittedKeyword.value = q
  page.value = 1
  await articlesStore.search({ q, page: page.value, pageSize: pageSize.value })
  hasSearched.value = true
}

async function changePage() {
  if (!submittedKeyword.value) return
  await articlesStore.search({
    q: submittedKeyword.value,
    page: page.value,
    pageSize: pageSize.value
  })
}
</script>

<template>
  <section class="py-4">
    <h1 class="text-2xl font-semibold text-gray-900">搜索</h1>

    <form class="mt-4 flex gap-2" @submit.prevent="doSearch">
      <el-input
        v-model="keyword"
        class="max-w-md"
        placeholder="输入关键词，匹配标题/摘要/正文"
        clearable
      />
      <el-button type="primary" native-type="submit">搜索</el-button>
    </form>

    <div v-loading="articlesStore.loading" class="mt-6 min-h-40">
      <template v-if="hasSearched">
        <p class="text-sm text-gray-500">
          「{{ submittedKeyword }}」的结果（共 {{ articlesStore.total }} 条）
        </p>
        <div
          v-if="articlesStore.articles.length > 0"
          class="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-3"
        >
          <ArticleCard
            v-for="article in articlesStore.articles"
            :key="article.id"
            :article="article"
          />
        </div>
        <div
          v-else-if="!articlesStore.loading"
          class="mt-4 rounded-lg border border-gray-200 bg-white p-12 text-center text-sm text-gray-400"
        >
          未找到与「{{ submittedKeyword }}」相关的内容
        </div>
      </template>
      <div
        v-else
        class="rounded-lg border border-gray-200 bg-white p-12 text-center text-sm text-gray-400"
      >
        输入关键词开始搜索（支持标题、摘要与正文内容）
      </div>
    </div>

    <div v-if="hasSearched && articlesStore.total > 0" class="mt-6">
      <PaginationBar
        v-model:page="page"
        v-model:page-size="pageSize"
        :total="articlesStore.total"
        @change="changePage"
      />
    </div>
  </section>
</template>
