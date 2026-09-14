<script setup lang="ts">
import { useRoute } from 'vue-router'

const route = useRoute()
</script>

<template>
  <section class="py-8">
    <h1 class="text-2xl font-semibold text-gray-900">文章详情</h1>
    <p class="mt-2 text-sm text-gray-500">文章 ID：{{ route.params.id }}</p>
    <p class="mt-4 rounded-lg border border-gray-200 bg-white p-4 text-sm text-gray-400">
      详情渲染（正文/封面/目录）随内容模块落地；隐藏内容一律 404 防枚举。
    </p>
  </section>
</template>
