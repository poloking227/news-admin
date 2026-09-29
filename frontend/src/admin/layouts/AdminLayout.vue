<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'

import { useAuthStore } from '@/shared/stores/auth'
import { AUDIT_PERMISSIONS, USER_PERMISSIONS } from '@/shared/types/api'

const authStore = useAuthStore()
const router = useRouter()

/** 导航按权限点过滤：文章/分类沿用既有角色路由门槛，用户/审计按 users:manage / audit:read 权限点门控 */
const navItems = computed(() => {
  const items: { name: string; label: string }[] = [
    { name: 'admin-articles', label: '文章管理' },
    { name: 'admin-categories', label: '分类管理' }
  ]
  if (authStore.hasPermission(USER_PERMISSIONS.manage)) {
    items.push({ name: 'admin-users', label: '用户管理' })
  }
  if (authStore.hasPermission(AUDIT_PERMISSIONS.read)) {
    items.push({ name: 'admin-audit-logs', label: '审计日志' })
  }
  return items
})

async function handleLogout() {
  await authStore.logout()
  await router.push({ name: 'admin-login' })
}
</script>

<template>
  <div class="flex min-h-screen">
    <aside class="flex w-56 shrink-0 flex-col border-r border-gray-200 bg-white">
      <div
        class="flex h-14 items-center border-b border-gray-200 px-4 text-base font-semibold text-gray-900"
      >
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
      <header class="flex h-14 items-center justify-between border-b border-gray-200 bg-white px-6">
        <div class="text-sm text-gray-500">
          <template v-if="authStore.user">
            {{ authStore.user.displayName }}（{{ authStore.user.role }}）
          </template>
          <template v-else>加载中…</template>
        </div>
        <div class="flex items-center gap-3">
          <RouterLink
            :to="{ name: 'admin-change-password' }"
            class="text-sm text-gray-600 hover:text-gray-900"
          >
            修改密码
          </RouterLink>
          <el-button size="small" @click="handleLogout">退出登录</el-button>
        </div>
      </header>
      <main class="flex-1 overflow-y-auto p-6">
        <RouterView />
      </main>
    </div>
  </div>
</template>
