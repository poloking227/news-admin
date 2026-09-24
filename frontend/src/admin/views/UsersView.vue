<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { computed, onMounted, reactive, ref } from 'vue'

import { ApiError } from '@/shared/api/http'
import { useAuthStore } from '@/shared/stores/auth'
import { useUsersStore } from '@/shared/stores/users'
import { USER_PERMISSIONS, type Role, type User, type UserStatus } from '@/shared/types/api'

const usersStore = useUsersStore()
const authStore = useAuthStore()

function messageOf(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback
}

const ROLE_LABELS: Record<Role, string> = {
  admin: '管理员',
  editor: '编辑',
  reviewer: '审核员',
  operator: '运营'
}

const ROLE_TAG_TYPES: Record<Role, 'danger' | 'warning' | 'success' | 'info'> = {
  admin: 'danger',
  editor: 'warning',
  reviewer: 'success',
  operator: 'info'
}

const STATUS_LABELS: Record<UserStatus, string> = {
  active: '启用',
  disabled: '停用'
}

const STATUS_TAG_TYPES: Record<UserStatus, 'success' | 'info'> = {
  active: 'success',
  disabled: 'info'
}

/** 自保护：当前登录用户不可停用/降级 */
function isSelf(user: Pick<User, 'id'>): boolean {
  return user.id === authStore.user?.id
}

const canManage = computed(() => authStore.hasPermission(USER_PERMISSIONS.manage))

/* ---------- 列表：筛选 + 分页 ---------- */

const roleOptions: { value: '' | Role; label: string }[] = [
  { value: '', label: '全部角色' },
  { value: 'admin', label: '管理员' },
  { value: 'editor', label: '编辑' },
  { value: 'reviewer', label: '审核员' },
  { value: 'operator', label: '运营' }
]

const statusOptions: { value: '' | UserStatus; label: string }[] = [
  { value: '', label: '全部状态' },
  { value: 'active', label: '启用' },
  { value: 'disabled', label: '停用' }
]

const roleFilter = ref<'' | Role>('')
const statusFilter = ref<'' | UserStatus>('')
const keyword = ref('')
const page = ref(1)
const pageSize = ref(10)
const pageSizes = [10, 20, 50]

async function loadList() {
  try {
    await usersStore.fetchList({
      ...(roleFilter.value ? { role: roleFilter.value } : {}),
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
  roleFilter.value = ''
  statusFilter.value = ''
  keyword.value = ''
  page.value = 1
  void loadList()
}

/* ---------- 新建 ---------- */

const createVisible = ref(false)
const saving = ref(false)
const createForm = reactive({
  username: '',
  displayName: '',
  role: '' as '' | Role,
  password: ''
})

const tempResultVisible = ref(false)
const tempResult = reactive({ username: '', temporaryPassword: '' })

function openCreate() {
  createForm.username = ''
  createForm.displayName = ''
  createForm.role = ''
  createForm.password = ''
  createVisible.value = true
}

function validateCreate(): boolean {
  if (!createForm.username.trim()) {
    ElMessage.warning('请输入用户名')
    return false
  }
  if (!createForm.displayName.trim()) {
    ElMessage.warning('请输入展示名')
    return false
  }
  if (!createForm.role) {
    ElMessage.warning('请选择角色')
    return false
  }
  if (createForm.password.length < 8 || createForm.password.length > 72) {
    ElMessage.warning('临时口令长度须为 8-72 位')
    return false
  }
  return true
}

async function submitCreate() {
  if (!validateCreate()) return
  const role = createForm.role
  if (!role) return // validateCreate 已拦截空角色，此处仅类型收窄
  saving.value = true
  try {
    await usersStore.create({
      username: createForm.username.trim(),
      displayName: createForm.displayName.trim(),
      role,
      password: createForm.password
    })
    // 临时口令仅admin设置时可见一次，创建成功即回显
    tempResult.username = createForm.username.trim()
    tempResult.temporaryPassword = createForm.password
    createVisible.value = false
    tempResultVisible.value = true
  } catch (error) {
    ElMessage.error(messageOf(error, '创建用户失败'))
  } finally {
    saving.value = false
  }
}

/* ---------- 编辑 ---------- */

const editVisible = ref(false)
const editId = ref<string | null>(null)
const editForm = reactive({ displayName: '', role: '' as '' | Role })
const savingEdit = ref(false)

const editingSelf = computed(() => Boolean(editId.value && isSelf({ id: editId.value })))

function openEdit(user: User) {
  editId.value = user.id
  editForm.displayName = user.displayName
  editForm.role = user.role
  editVisible.value = true
}

function validateEdit(): boolean {
  if (!editForm.displayName.trim()) {
    ElMessage.warning('请输入展示名')
    return false
  }
  return true
}

async function submitEdit() {
  if (!validateEdit()) return
  const role = editForm.role
  if (!role) return
  savingEdit.value = true
  try {
    await usersStore.update(editId.value!, {
      displayName: editForm.displayName.trim(),
      role
    })
    ElMessage.success('用户已更新')
    editVisible.value = false
  } catch (error) {
    ElMessage.error(messageOf(error, '更新失败'))
  } finally {
    savingEdit.value = false
  }
}

/* ---------- 启停（自保护：不可停用自己） ---------- */

async function handleToggleStatus(user: User) {
  if (isSelf(user) && user.status === 'active') {
    ElMessage.warning('不能停用当前登录用户')
    return
  }
  const disable = user.status === 'active'
  try {
    await ElMessageBox.confirm(
      disable
        ? `停用后「${user.displayName}」将无法登录，其会话即时失效。`
        : `确认启用「${user.displayName}」？`,
      disable ? '停用用户' : '启用用户',
      { type: 'warning', confirmButtonText: '确认', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  try {
    await usersStore.setStatus(user.id, disable ? 'disabled' : 'active')
    ElMessage.success(disable ? '已停用' : '已启用')
  } catch (error) {
    ElMessage.error(messageOf(error, '操作失败'))
  }
}

/* ---------- 重置密码 ---------- */

const resetResultVisible = ref(false)
const resetResult = reactive({ displayName: '', temporaryPassword: '' })

async function handleResetPassword(user: User) {
  try {
    await ElMessageBox.confirm(
      `将为「${user.displayName}」生成临时口令并撤销其现有会话；该用户首登必须改密。`,
      '重置密码',
      { type: 'warning', confirmButtonText: '重置', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  try {
    const { temporaryPassword } = await usersStore.resetPassword(user.id)
    resetResult.displayName = user.displayName
    resetResult.temporaryPassword = temporaryPassword
    resetResultVisible.value = true
  } catch (error) {
    ElMessage.error(messageOf(error, '重置密码失败'))
  }
}

/* ---------- 复制 ---------- */

async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    /* 降级到 execCommand */
  }
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.style.position = 'fixed'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.select()
  const ok = document.execCommand('copy')
  document.body.removeChild(textarea)
  return ok
}

async function copyTempPassword(text: string) {
  const ok = await copyText(text)
  if (ok) ElMessage.success('已复制')
  else ElMessage.error('复制失败，请手动复制')
}

onMounted(() => {
  void loadList()
})
</script>

<template>
  <section>
    <div class="flex items-center justify-between">
      <h1 class="text-lg font-semibold text-gray-900">用户管理</h1>
      <el-button v-if="canManage" type="primary" @click="openCreate">新建用户</el-button>
    </div>

    <!-- 筛选条 -->
    <div class="mt-4 flex flex-wrap items-center gap-2">
      <el-select v-model="roleFilter" class="w-32" placeholder="全部角色" @change="applyFilter">
        <el-option
          v-for="option in roleOptions"
          :key="option.value"
          :value="option.value"
          :label="option.label"
        />
      </el-select>
      <el-select v-model="statusFilter" class="w-32" placeholder="全部状态" @change="applyFilter">
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
        placeholder="搜索用户名/展示名"
        clearable
        @keyup.enter="applyFilter"
        @clear="applyFilter"
      />
      <el-button @click="applyFilter">查询</el-button>
      <el-button @click="resetFilter">重置</el-button>
    </div>

    <!-- 列表 -->
    <el-card class="mt-4" shadow="never">
      <el-table v-loading="usersStore.loading" :data="usersStore.users" stripe>
        <el-table-column prop="username" label="用户名" min-width="140" />
        <el-table-column prop="displayName" label="展示名" min-width="120" />
        <el-table-column label="角色" width="100">
          <template #default="{ row }">
            <el-tag :type="ROLE_TAG_TYPES[row.role as Role]" size="small">
              {{ ROLE_LABELS[row.role as Role] }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="STATUS_TAG_TYPES[row.status as UserStatus]" size="small">
              {{ STATUS_LABELS[row.status as UserStatus] }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="170">
          <template #default="{ row }">
            {{ new Date(row.createdAt).toLocaleString() }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-tooltip content="不能停用当前登录用户" :disabled="!isSelf(row)">
              <span>
                <el-button
                  link
                  type="danger"
                  :disabled="isSelf(row) && row.status === 'active'"
                  @click="handleToggleStatus(row)"
                >
                  {{ row.status === 'active' ? '停用' : '启用' }}
                </el-button>
              </span>
            </el-tooltip>
            <el-button link type="warning" @click="handleResetPassword(row)">重置密码</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="mt-4 flex justify-end">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="usersStore.total"
          :page-sizes="pageSizes"
          layout="total, sizes, prev, pager, next"
          @current-change="loadList"
          @size-change="applyFilter"
        />
      </div>
    </el-card>

    <!-- 新建对话框 -->
    <el-dialog v-model="createVisible" title="新建用户" width="480px">
      <div class="space-y-4">
        <div>
          <label class="mb-1 block text-sm font-medium text-gray-700">
            用户名 <span class="text-red-500">*</span>
          </label>
          <el-input
            v-model="createForm.username"
            maxlength="50"
            show-word-limit
            placeholder="登录用户名"
          />
        </div>
        <div>
          <label class="mb-1 block text-sm font-medium text-gray-700">
            展示名 <span class="text-red-500">*</span>
          </label>
          <el-input v-model="createForm.displayName" maxlength="50" placeholder="显示名称" />
        </div>
        <div>
          <label class="mb-1 block text-sm font-medium text-gray-700">
            角色 <span class="text-red-500">*</span>
          </label>
          <el-select v-model="createForm.role" class="w-full" placeholder="请选择角色">
            <el-option
              v-for="option in roleOptions.filter((o) => o.value)"
              :key="option.value"
              :value="option.value"
              :label="option.label"
            />
          </el-select>
        </div>
        <div>
          <label class="mb-1 block text-sm font-medium text-gray-700">
            临时口令 <span class="text-red-500">*</span>
          </label>
          <el-input
            v-model="createForm.password"
            type="password"
            show-password
            maxlength="72"
            placeholder="8-72 位"
          />
          <p class="mt-1 text-xs text-gray-400">
            创建成功后仅在此展示一次；该用户首次登录必须修改密码。
          </p>
        </div>
      </div>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submitCreate">创建用户</el-button>
      </template>
    </el-dialog>

    <!-- 新建成功：临时口令回显（仅一次） -->
    <el-dialog v-model="tempResultVisible" title="用户已创建" width="440px">
      <p class="text-sm text-gray-700">
        用户 <span class="font-medium">{{ tempResult.username }}</span> 创建成功。请将临时口令
        转交给该用户：
      </p>
      <div class="mt-3 flex items-center gap-2">
        <el-input :model-value="tempResult.temporaryPassword" readonly>
          <template #append>
            <el-button @click="copyTempPassword(tempResult.temporaryPassword)">复制</el-button>
          </template>
        </el-input>
      </div>
      <el-alert
        class="mt-3"
        type="warning"
        :closable="false"
        show-icon
        title="该用户首次登录必须修改密码（首登强制改密）。"
      />
      <template #footer>
        <el-button type="primary" @click="tempResultVisible = false">完成</el-button>
      </template>
    </el-dialog>

    <!-- 编辑对话框 -->
    <el-dialog v-model="editVisible" title="编辑用户" width="440px">
      <div class="space-y-4">
        <div>
          <label class="mb-1 block text-sm font-medium text-gray-700">
            展示名 <span class="text-red-500">*</span>
          </label>
          <el-input v-model="editForm.displayName" maxlength="50" placeholder="显示名称" />
        </div>
        <div>
          <label class="mb-1 block text-sm font-medium text-gray-700">
            角色 <span class="text-red-500">*</span>
          </label>
          <el-select
            v-model="editForm.role"
            class="w-full"
            :disabled="editingSelf"
            placeholder="请选择角色"
          >
            <el-option
              v-for="option in roleOptions.filter((o) => o.value)"
              :key="option.value"
              :value="option.value"
              :label="option.label"
            />
          </el-select>
          <p v-if="editingSelf" class="mt-1 text-xs text-gray-400">
            不能降级当前登录用户，角色不可修改。
          </p>
        </div>
      </div>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" :loading="savingEdit" @click="submitEdit">保存</el-button>
      </template>
    </el-dialog>

    <!-- 重置密码结果 -->
    <el-dialog v-model="resetResultVisible" title="密码已重置" width="440px">
      <p class="text-sm text-gray-700">
        已为 <span class="font-medium">{{ resetResult.displayName }}</span> 生成临时口令：
      </p>
      <div class="mt-3 flex items-center gap-2">
        <el-input :model-value="resetResult.temporaryPassword" readonly>
          <template #append>
            <el-button @click="copyTempPassword(resetResult.temporaryPassword)">复制</el-button>
          </template>
        </el-input>
      </div>
      <el-alert
        class="mt-3"
        type="warning"
        :closable="false"
        show-icon
        title="原会话已撤销；该用户首次登录必须修改密码（首登强制改密）。"
      />
      <template #footer>
        <el-button type="primary" @click="resetResultVisible = false">完成</el-button>
      </template>
    </el-dialog>
  </section>
</template>
