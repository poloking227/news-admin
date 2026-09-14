<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'

import { ApiError } from '@/shared/api/http'
import { useAuthStore } from '@/shared/stores/auth'

const authStore = useAuthStore()
const router = useRouter()

const form = reactive({
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
})

const submitting = ref(false)

const forced = authStore.mustChangePassword

async function handleSubmit() {
  if (!form.oldPassword || !form.newPassword) {
    ElMessage.warning('请输入当前密码和新密码')
    return
  }
  if (form.newPassword !== form.confirmPassword) {
    ElMessage.warning('两次输入的新密码不一致')
    return
  }
  submitting.value = true
  try {
    await authStore.changePassword(form.oldPassword, form.newPassword)
    ElMessage.success('密码已修改，请使用新密码重新登录')
    await router.push({ name: 'admin-login' })
  } catch (error) {
    const message = error instanceof ApiError ? error.message : '修改失败，请稍后重试'
    ElMessage.error(message)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="mx-auto w-full max-w-sm py-16">
    <div class="rounded-lg border border-gray-200 bg-white p-8 shadow-sm">
      <h1 class="text-center text-xl font-semibold text-gray-900">修改密码</h1>
      <el-alert
        v-if="forced"
        class="mt-4"
        type="warning"
        :closable="false"
        title="首次登录须修改初始密码后方可继续"
      />
      <el-form class="mt-6" label-position="top" @submit.prevent="handleSubmit">
        <el-form-item label="当前密码">
          <el-input
            v-model="form.oldPassword"
            name="old-password"
            type="password"
            autocomplete="current-password"
            show-password
            @keyup.enter="handleSubmit"
          />
        </el-form-item>
        <el-form-item label="新密码">
          <el-input
            v-model="form.newPassword"
            name="new-password"
            type="password"
            autocomplete="new-password"
            placeholder="至少 8 位"
            show-password
            @keyup.enter="handleSubmit"
          />
        </el-form-item>
        <el-form-item label="确认新密码">
          <el-input
            v-model="form.confirmPassword"
            name="confirm-password"
            type="password"
            autocomplete="new-password"
            show-password
            @keyup.enter="handleSubmit"
          />
        </el-form-item>
        <el-button class="mt-2 w-full" type="primary" native-type="submit" :loading="submitting">
          确认修改
        </el-button>
      </el-form>
    </div>
  </div>
</template>
