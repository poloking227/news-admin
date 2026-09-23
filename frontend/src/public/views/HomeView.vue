<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import ArticleCard from '@/public/components/ArticleCard.vue'
import PaginationBar from '@/shared/components/PaginationBar.vue'
import { usePublicArticlesStore } from '@/shared/stores/publicArticles'
import { usePublicCategoriesStore } from '@/shared/stores/publicCategories'

const articlesStore = usePublicArticlesStore()
const categoriesStore = usePublicCategoriesStore()

const page = ref(1)
const pageSize = ref(10)
const activeCategoryId = ref('')

/** 分类 chips：全部 + published-only 分类（异步加载后随响应更新） */
const categoryChips = computed(() => [
  { value: '', label: '全部' },
  ...categoriesStore.categories.map((category) => ({
    value: category.id,
    label: category.name
  }))
])

async function loadList() {
  await articlesStore.fetchList({
    ...(activeCategoryId.value ? { categoryId: activeCategoryId.value } : {}),
    page: page.value,
    pageSize: pageSize.value
  })
}

function selectCategory(value: string) {
  activeCategoryId.value = value
  page.value = 1
  void loadList()
}

onMounted(() => {
  void loadList()
  void categoriesStore.fetchCategories()
})
</script>

<template>
  <section class="py-4">
    <h1 class="text-2xl font-semibold text-gray-900">最新发布</h1>

    <!-- 分类筛选 chips -->
    <div class="mt-4 flex flex-wrap items-center gap-2">
      <button
        v-for="chip in categoryChips"
        :key="chip.value"
        type="button"
        class="rounded-full border px-4 py-1 text-sm transition-colors"
        :class="
          activeCategoryId === chip.value
            ? 'border-gray-900 bg-gray-900 text-white'
            : 'border-gray-300 bg-white text-gray-600 hover:border-gray-400'
        "
        @click="selectCategory(chip.value)"
      >
        {{ chip.label }}
      </button>
    </div>

    <!-- 文章列表 -->
    <div v-loading="articlesStore.loading" class="mt-6 min-h-40">
      <div
        v-if="articlesStore.articles.length > 0"
        class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3"
      >
        <ArticleCard
          v-for="article in articlesStore.articles"
          :key="article.id"
          :article="article"
        />
      </div>
      <div
        v-else-if="!articlesStore.loading"
        class="rounded-lg border border-gray-200 bg-white p-12 text-center text-sm text-gray-400"
      >
        该分类下暂无已发布内容
      </div>
    </div>

    <div v-if="articlesStore.total > 0" class="mt-6">
      <PaginationBar
        v-model:page="page"
        v-model:page-size="pageSize"
        :total="articlesStore.total"
        @change="loadList"
      />
    </div>
  </section>
</template>
