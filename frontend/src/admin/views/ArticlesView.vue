<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { computed, onMounted, reactive, ref } from 'vue'

import TiptapEditor from '@/admin/components/TiptapEditor.vue'
import { articleStatusMeta } from '@/admin/utils/article-status'
import { ApiError } from '@/shared/api/http'
import { useArticlesStore } from '@/shared/stores/articles'
import { useAuthStore } from '@/shared/stores/auth'
import { useCategoriesStore } from '@/shared/stores/categories'
import { ARTICLE_PERMISSIONS, type Article, type ArticleStatus } from '@/shared/types/api'

const articlesStore = useArticlesStore()
const categoriesStore = useCategoriesStore()
const authStore = useAuthStore()

function messageOf(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback
}

/* ---------- 列表：筛选 + 分页 ---------- */

const statusOptions: { value: '' | ArticleStatus; label: string }[] = [
  { value: '', label: '全部状态' },
  { value: 'draft', label: '草稿' },
  { value: 'pending_review', label: '待审核' },
  { value: 'published', label: '已发布' },
  { value: 'unpublished', label: '已下架' }
]

const statusFilter = ref<'' | ArticleStatus>('')
const keyword = ref('')
const page = ref(1)
const pageSize = ref(10)
const pageSizes = [10, 20, 50]

async function loadList() {
  try {
    await articlesStore.fetchList({
      ...(statusFilter.value ? { status: statusFilter.value } : {}),
      ...(keyword.value.trim() ? { keyword: keyword.value.trim() } : {}),
      page: page.value,
      pageSize: pageSize.value
    })
  } catch (error) {
    ElMessage.error(messageOf(error, '列表加载失败'))
  }
}

function applyFilter() {
  page.value = 1
  void loadList()
}

function resetFilter() {
  statusFilter.value = ''
  keyword.value = ''
  page.value = 1
  void loadList()
}

/* ---------- 新建/编辑草稿 ---------- */

const dialogVisible = ref(false)
const saving = ref(false)
const editingId = ref<string | null>(null)
const editingVersion = ref(0)
const editorRef = ref<InstanceType<typeof TiptapEditor> | null>(null)

const form = reactive({
  title: '',
  summary: '',
  categoryId: '',
  coverUrl: '',
  bodyHtml: ''
})

const categoryOptions = computed(() => categoriesStore.categories)

function openCreate() {
  editingId.value = null
  editingVersion.value = 0
  form.title = ''
  form.summary = ''
  form.categoryId = ''
  form.coverUrl = ''
  form.bodyHtml = ''
  dialogVisible.value = true
}

function openEdit(article: Article) {
  editingId.value = article.id
  editingVersion.value = article.version
  form.title = article.title
  form.summary = article.summary
  form.categoryId = article.categoryId
  form.coverUrl = article.coverUrl ?? ''
  form.bodyHtml = article.bodyHtml
  dialogVisible.value = true
}

function validateForm(): boolean {
  if (!form.title.trim()) {
    ElMessage.warning('请输入标题')
    return false
  }
  if (!form.summary.trim()) {
    ElMessage.warning('请输入摘要')
    return false
  }
  if (!form.categoryId) {
    ElMessage.warning('请选择分类')
    return false
  }
  if (editorRef.value?.isEmpty()) {
    ElMessage.warning('请输入正文内容')
    return false
  }
  const cover = form.coverUrl.trim()
  if (cover) {
    try {
      new URL(cover)
    } catch {
      ElMessage.warning('封面地址不是合法的 URL')
      return false
    }
  }
  return true
}

async function saveDraft() {
  if (!validateForm()) return
  saving.value = true
  try {
    const payload = {
      title: form.title.trim(),
      summary: form.summary.trim(),
      bodyHtml: form.bodyHtml,
      categoryId: form.categoryId,
      coverUrl: form.coverUrl.trim() || null
    }
    if (editingId.value) {
      await articlesStore.updateDraft(editingId.value, payload, editingVersion.value)
    } else {
      await articlesStore.createDraft(payload)
    }
    ElMessage.success('草稿已保存')
    dialogVisible.value = false
  } catch (error) {
    ElMessage.error(messageOf(error, '保存失败，请稍后重试'))
  } finally {
    saving.value = false
  }
}

/* ---------- 流转动作 ---------- */

async function confirmAction(label: string, action: () => Promise<unknown>) {
  try {
    await ElMessageBox.confirm(`确认执行「${label}」？`, label, {
      type: 'warning',
      confirmButtonText: '确认',
      cancelButtonText: '取消'
    })
  } catch {
    return
  }
  try {
    await action()
    ElMessage.success(`${label}成功`)
  } catch (error) {
    ElMessage.error(messageOf(error, `${label}失败`))
  }
}

const rejectDialog = reactive({ visible: false, id: '', reason: '' })

function openReject(article: Article) {
  rejectDialog.id = article.id
  rejectDialog.reason = ''
  rejectDialog.visible = true
}

async function confirmReject() {
  const reason = rejectDialog.reason.trim()
  if (!reason) {
    ElMessage.warning('请填写驳回理由')
    return
  }
  if (reason.length > 500) {
    ElMessage.warning('驳回理由不能超过 500 字')
    return
  }
  try {
    await articlesStore.reject(rejectDialog.id, reason)
    rejectDialog.visible = false
    ElMessage.success('已驳回')
  } catch (error) {
    ElMessage.error(messageOf(error, '驳回失败'))
  }
}

async function handleUnpublish(article: Article) {
  try {
    const { value } = await ElMessageBox.prompt('下架原因（可选，500 字内）', '下架文章', {
      confirmButtonText: '下架',
      cancelButtonText: '取消',
      inputType: 'textarea',
      inputPlaceholder: '原因不超过 500 字',
      inputValidator: (reason: string) => reason.length <= 500 || '下架原因不能超过 500 字'
    })
    await articlesStore.unpublish(article.id, value?.trim() || undefined)
    ElMessage.success('已下架')
  } catch (error) {
    if (error === 'cancel' || error === 'close') return
    ElMessage.error(messageOf(error, '下架失败'))
  }
}

async function handleDelete(article: Article) {
  await confirmAction('删除（软删）', () => articlesStore.softDelete(article.id))
}

async function handlePinToggle(article: Article, pinned: boolean) {
  try {
    await articlesStore.togglePin(article.id, pinned)
    ElMessage.success(pinned ? '已置顶' : '已取消置顶')
  } catch (error) {
    ElMessage.error(messageOf(error, '置顶操作失败'))
  }
}

/* ---------- 权限与显示矩阵 ---------- */

const can = {
  update: () => authStore.hasPermission(ARTICLE_PERMISSIONS.update),
  submit: () => authStore.hasPermission(ARTICLE_PERMISSIONS.submit),
  approve: () => authStore.hasPermission(ARTICLE_PERMISSIONS.approve),
  reject: () => authStore.hasPermission(ARTICLE_PERMISSIONS.reject),
  unpublish: () => authStore.hasPermission(ARTICLE_PERMISSIONS.unpublish),
  pin: () => authStore.hasPermission(ARTICLE_PERMISSIONS.pin)
}

/** 编辑仅 draft（含已驳回）；正文编辑权属 editor/admin（articles:update） */
function canEdit(article: Article): boolean {
  return article.status === 'draft' && can.update()
}

/** 提交审核：draft（含已驳回）| unpublished → pending_review */
function canSubmit(article: Article): boolean {
  return can.submit() && (article.status === 'draft' || article.status === 'unpublished')
}

function canReview(article: Article): boolean {
  return article.status === 'pending_review'
}

function canUnpublish(article: Article): boolean {
  return article.status === 'published' && can.unpublish()
}

function canPinRow(article: Article): boolean {
  return article.status === 'published' && can.pin()
}

onMounted(() => {
  void loadList()
  categoriesStore.fetchCategories().catch(() => {
    /* 分类下拉加载失败不影响列表 */
  })
})
</script>

<template>
  <section>
    <div class="flex items-center justify-between">
      <h1 class="text-lg font-semibold text-gray-900">文章管理</h1>
      <el-button v-if="can.update()" type="primary" @click="openCreate">新建文章</el-button>
    </div>

    <!-- 筛选条 -->
    <div class="mt-4 flex flex-wrap items-center gap-2">
      <el-select v-model="statusFilter" class="w-36" placeholder="全部状态" @change="applyFilter">
        <el-option
          v-for="option in statusOptions"
          :key="option.value"
          :value="option.value"
          :label="option.label"
        />
      </el-select>
      <el-input
        v-model="keyword"
        class="w-56"
        placeholder="搜索标题/正文"
        clearable
        @keyup.enter="applyFilter"
        @clear="applyFilter"
      />
      <el-button @click="applyFilter">查询</el-button>
      <el-button @click="resetFilter">重置</el-button>
    </div>

    <!-- 列表 -->
    <el-card class="mt-4" shadow="never">
      <el-table v-loading="articlesStore.loading" :data="articlesStore.articles" stripe>
        <el-table-column label="标题" min-width="220">
          <template #default="{ row }">
            <span v-if="row.pinned" class="mr-1 text-orange-500" title="已置顶">★</span>
            {{ row.title }}
          </template>
        </el-table-column>
        <el-table-column prop="categoryName" label="分类" width="110" />
        <el-table-column label="状态" width="130">
          <template #default="{ row }">
            <el-tag :type="articleStatusMeta(row).type" size="small">
              {{ articleStatusMeta(row).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="createdByName" label="作者" width="110" />
        <el-table-column label="创建时间" width="160">
          <template #default="{ row }">
            {{ new Date(row.createdAt).toLocaleString() }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-button v-if="canEdit(row)" link type="primary" @click="openEdit(row)">
              编辑
            </el-button>
            <el-button
              v-if="canSubmit(row)"
              link
              type="primary"
              @click="confirmAction('提交审核', () => articlesStore.submit(row.id))"
            >
              提交
            </el-button>
            <el-button
              v-if="canReview(row) && can.approve()"
              link
              type="success"
              @click="confirmAction('审核通过并发布', () => articlesStore.approve(row.id))"
            >
              通过
            </el-button>
            <el-button
              v-if="canReview(row) && can.reject()"
              link
              type="warning"
              @click="openReject(row)"
            >
              驳回
            </el-button>
            <el-button v-if="canUnpublish(row)" link type="warning" @click="handleUnpublish(row)">
              下架
            </el-button>
            <el-button v-if="canEdit(row)" link type="danger" @click="handleDelete(row)">
              删除
            </el-button>
            <el-switch
              v-if="canPinRow(row)"
              :model-value="row.pinned"
              inline-prompt
              active-text="顶"
              inactive-text="—"
              style="margin-left: 8px; vertical-align: middle"
              @change="(value: boolean) => handlePinToggle(row, value)"
            />
          </template>
        </el-table-column>
      </el-table>

      <div class="mt-4 flex justify-end">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="articlesStore.total"
          :page-sizes="pageSizes"
          layout="total, sizes, prev, pager, next"
          @current-change="loadList"
          @size-change="applyFilter"
        />
      </div>
    </el-card>

    <!-- 编辑/新建对话框 -->
    <el-dialog
      v-model="dialogVisible"
      :title="editingId ? '编辑文章' : '新建文章'"
      width="720px"
      top="6vh"
    >
      <div class="space-y-4">
        <div class="flex gap-3">
          <div class="flex-1">
            <label class="mb-1 block text-sm font-medium text-gray-700">
              标题 <span class="text-red-500">*</span>
            </label>
            <el-input v-model="form.title" maxlength="200" show-word-limit placeholder="文章标题" />
          </div>
          <div class="w-44">
            <label class="mb-1 block text-sm font-medium text-gray-700">
              分类 <span class="text-red-500">*</span>
            </label>
            <el-select v-model="form.categoryId" class="w-full" placeholder="必选">
              <el-option
                v-for="category in categoryOptions"
                :key="category.id"
                :value="category.id"
                :label="category.name"
              />
            </el-select>
          </div>
        </div>
        <div>
          <label class="mb-1 block text-sm font-medium text-gray-700">
            摘要 <span class="text-red-500">*</span>
          </label>
          <el-input
            v-model="form.summary"
            type="textarea"
            :rows="2"
            maxlength="500"
            show-word-limit
            placeholder="文章摘要（500 字内）"
          />
        </div>
        <div>
          <label class="mb-1 block text-sm font-medium text-gray-700">封面（外链 URL）</label>
          <el-input
            v-model="form.coverUrl"
            maxlength="2048"
            placeholder="https://example.com/cover.jpg（可选）"
          />
        </div>
        <div>
          <label class="mb-1 block text-sm font-medium text-gray-700">
            正文 <span class="text-red-500">*</span>
          </label>
          <TiptapEditor ref="editorRef" v-model="form.bodyHtml" placeholder="请输入正文…" />
          <p class="mt-1 text-xs text-gray-400">
            支持加粗/斜体/标题/列表/引用；纯文本自动提取用于搜索，HTML 由服务端白名单消毒。
          </p>
        </div>
      </div>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveDraft">保存草稿</el-button>
      </template>
    </el-dialog>

    <!-- 驳回对话框 -->
    <el-dialog v-model="rejectDialog.visible" title="驳回文章" width="480px">
      <el-input
        v-model="rejectDialog.reason"
        type="textarea"
        :rows="4"
        maxlength="500"
        show-word-limit
        placeholder="请填写驳回理由（必填，500 字内），作者将看到「已驳回待修改」"
      />
      <template #footer>
        <el-button @click="rejectDialog.visible = false">取消</el-button>
        <el-button type="warning" @click="confirmReject">确认驳回</el-button>
      </template>
    </el-dialog>
  </section>
</template>
