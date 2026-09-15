<script setup lang="ts"></script>

<template>
  <div class="flex min-h-screen flex-col items-center justify-center gap-2 text-center">
    <p class="text-4xl font-semibold text-gray-300">403</p>
    <p class="text-sm text-gray-500">无权访问该页面，请联系管理员</p>
    <RouterLink to="/admin/articles" class="text-sm text-blue-600 hover:underline">
      返回文章管理
    </RouterLink>
  </div>
</template>
