<script setup lang="ts"></script>

<template>
  <div class="flex min-h-screen flex-col">
    <header class="sticky top-0 z-10 border-b border-gray-200 bg-white">
      <div class="mx-auto flex h-14 max-w-5xl items-center justify-between px-4">
        <RouterLink to="/" class="text-lg font-semibold text-gray-900">新闻管理系统</RouterLink>
        <nav class="flex items-center gap-4 text-sm text-gray-600">
          <RouterLink to="/" class="hover:text-gray-900">首页</RouterLink>
          <RouterLink :to="{ name: 'admin-login' }" class="hover:text-gray-900">管理端</RouterLink>
        </nav>
      </div>
    </header>

    <main class="mx-auto w-full max-w-5xl flex-1 px-4 py-8">
      <RouterView />
    </main>

    <footer class="border-t border-gray-200 py-4 text-center text-xs text-gray-400">
      news-admin · 内容仅供演示
    </footer>
  </div>
</template>