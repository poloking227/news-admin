<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { onMounted, reactive, ref } from 'vue'

import { ApiError } from '@/shared/api/http'
import { useCategoriesStore } from '@/shared/stores/categories'
import type { Category } from '@/shared/types/api'

const store = useCategoriesStore()

const dialogVisible = ref(false)
const submitting = ref(false)
const editingId = ref<string | null>(null)

const form = reactive({
  name: '',
  slug: '',
  description: '',
  sortOrder: 0
})

function openCreate() {
  editingId.value = null
  form.name = ''
  form.slug = ''
  form.description = ''
  form.sortOrder = 0
  dialogVisible.value = true
}

function openEdit(category: Category) {
  editingId.value = category.id
  form.name = category.name
  form.slug = category.slug
  form.description = category.description ?? ''
  form.sortOrder = category.sortOrder
  dialogVisible.value = true
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback
}

async function handleSubmit() {
  if (!form.name.trim()) {
    ElMessage.warning('请输入分类名称')
    return
  }
  if (!form.slug.trim()) {
    ElMessage.warning('请输入分类标识（slug）')
    return
  }

  submitting.value = true
  try {
    const payload = {
      name: form.name.trim(),
      slug: form.slug.trim(),
      description: form.description.trim() || null,
      sortOrder: form.sortOrder
    }
    if (editingId.value) {
      await store.updateCategory(editingId.value, payload)
      ElMessage.success('分类已更新')
    } else {
      await store.createCategory(payload)
      ElMessage.success('分类已创建')
    }
    dialogVisible.value = false
  } catch (error) {
    ElMessage.error(errorMessage(error, '保存失败，请稍后重试'))
  } finally {
    submitting.value = false
  }
}

async function handleDelete(category: Category) {
  try {
    await ElMessageBox.confirm(`删除分类「${category.name}」？删除后不可恢复。`, '删除分类', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
  } catch {
    return // 用户取消
  }

  try {
    await store.removeCategory(category.id)
    ElMessage.success('分类已删除')
  } catch (error) {
    if (error instanceof ApiError && error.status === 409) {
      ElMessage.warning('该分类下仍有内容，请先迁移分类下内容再删除')
    } else {
      ElMessage.error(errorMessage(error, '删除失败，请稍后重试'))
    }
  }
}

onMounted(() => {
  store.fetchCategories().catch((error: unknown) => {
    ElMessage.error(errorMessage(error, '分类列表加载失败'))
  })
})
</script>

<template>
  <section>
    <div class="flex items-center justify-between">
      <h1 class="text-lg font-semibold text-gray-900">分类管理</h1>
      <el-button type="primary" @click="openCreate">新建分类</el-button>
    </div>

    <el-card class="mt-4" shadow="never">
      <el-table v-loading="store.loading" :data="store.sortedCategories" stripe>
        <el-table-column prop="name" label="名称" min-width="120" />
        <el-table-column prop="slug" label="标识" min-width="120" />
        <el-table-column prop="description" label="描述" min-width="160" show-overflow-tooltip />
        <el-table-column prop="sortOrder" label="排序" width="80" />
        <el-table-column prop="articleCount" label="文章数" width="90" />
        <el-table-column label="更新时间" min-width="160">
          <template #default="{ row }">
            {{ new Date(row.updatedAt).toLocaleString() }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="handleDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog
      v-model="dialogVisible"
      :title="editingId ? '编辑分类' : '新建分类'"
      width="480px"
      destroy-on-close
    >
      <el-form label-position="top">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" name="name" maxlength="50" placeholder="分类名称" />
        </el-form-item>
        <el-form-item label="标识（slug）" required>
          <el-input v-model="form.slug" name="slug" maxlength="50" placeholder="如 technology" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input
            v-model="form.description"
            name="description"
            type="textarea"
            :rows="3"
            maxlength="200"
            placeholder="可选"
          />
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sortOrder" :min="0" name="sort-order" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">保存</el-button>
      </template>
    </el-dialog>
  </section>
</template>
