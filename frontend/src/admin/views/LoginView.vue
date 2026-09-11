<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { ApiError } from '@/shared/api/http'
import { useAuthStore } from '@/shared/stores/auth'

const authStore = useAuthStore()
const route = useRoute()
const router = useRouter()

const form = reactive({
  username: '',
  password: ''
})

const submitting = ref(false)

async function handleSubmit() {
  if (!form.username || !form.password) {
    ElMessage.warning('请输入账号和密码')
    return
  }
  submitting.value = true
  try {
    await authStore.login({ username: form.username, password: form.password })
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : ''
    const target = authStore.mustChangePassword
      ? { name: 'admin-change-password' }
      : redirect
        ? { path: redirect }
        : { name: 'admin-articles' }
    await router.push(target)
  } catch (error) {
    const message = error instanceof ApiError ? error.message : '登录失败，请稍后重试'
    ElMessage.error(message)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="mx-auto w-full max-w-sm py-16">
    <div class="rounded-lg border border-gray-200 bg-white p-8 shadow-sm">
      <h1 class="text-center text-xl font-semibold text-gray-900">管理端登录</h1>
      <el-form class="mt-8" label-position="top" @submit.prevent="handleSubmit">
        <el-form-item label="账号">
          <el-input
            v-model="form.username"
            name="username"
            autocomplete="username"
            placeholder="请输入账号"
            @keyup.enter="handleSubmit"
          />
        </el-form-item>
        <el-form-item label="密码">
          <el-input
            v-model="form.password"
            name="password"
            type="password"
            autocomplete="current-password"
            placeholder="请输入密码"
            show-password
            @keyup.enter="handleSubmit"
          />
        </el-form-item>
        <el-button class="mt-2 w-full" type="primary" native-type="submit" :loading="submitting">
          登录
        </el-button>
      </el-form>
    </div>
  </div>
</template>
