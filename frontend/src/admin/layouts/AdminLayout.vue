<script setup lang="ts">
const navItems = [
  { name: 'admin-articles', label: '文章管理' },
  { name: 'admin-categories', label: '分类管理' },
  { name: 'admin-users', label: '用户管理' },
  { name: 'admin-audit-logs', label: '审计日志' },
]
</script>

<template>
  <div class="flex min-h-screen">
    <aside class="flex w-56 shrink-0 flex-col border-r border-gray-200 bg-white">
      <div class="flex h-14 items-center border-b border-gray-200 px-4 text-base font-semibold text-gray-900">
        管理台
      </div>
      <nav class="flex-1 px-2 py-3">
        <RouterLink
          v-for="item in navItems"
          :key="item.name"
          :to="{ name: item.name }"
          class="mb-1 block rounded-md px-3 py-2 text-sm text-gray-600 hover:bg-gray-100 hover:text-gray-900"
          active-class="bg-gray-100 font-medium text-gray-900"
        >
          {{ item.label }}
        </RouterLink>
      </nav>
    </aside>

    <div class="flex min-w-0 flex-1 flex-col">
      <header class="flex h-14 items-center justify-end border-b border-gray-200 bg-white px-6">
        <span class="text-sm text-gray-500">开发骨架 · 会话模块待接入</span>
      </header>
      <main class="flex-1 overflow-y-auto p-6">
        <RouterView />
      </main>
    </div>
  </div>
</template>