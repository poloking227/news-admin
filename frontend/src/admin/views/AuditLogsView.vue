<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { onMounted, ref } from 'vue'

import { auditActionGroups, auditActionMeta } from '@/admin/utils/audit-action'
import { ApiError } from '@/shared/api/http'
import { useAuditLogsStore } from '@/shared/stores/auditLogs'
import type { AuditAction, AuditLog } from '@/shared/types/api'

const auditLogsStore = useAuditLogsStore()

function messageOf(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback
}

/* ---------- 筛选 + 分页 ---------- */

const actionGroups = auditActionGroups()

const actionFilter = ref<'' | AuditAction>('')
const actorId = ref('')
/** 时间窗：el-date-picker value-format=x（毫秒时间戳），查询时转 UTC ISO */
const timeRange = ref<[number, number] | null>(null)
const page = ref(1)
const pageSize = ref(10)
const pageSizes = [10, 20, 50]

async function loadList() {
  try {
    await auditLogsStore.fetchList({
      ...(actionFilter.value ? { action: actionFilter.value } : {}),
      ...(actorId.value.trim() ? { actorId: actorId.value.trim() } : {}),
      ...(timeRange.value
        ? {
            from: new Date(timeRange.value[0]).toISOString(),
            to: new Date(timeRange.value[1]).toISOString()
          }
        : {}),
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
  actionFilter.value = ''
  actorId.value = ''
  timeRange.value = null
  page.value = 1
  void loadList()
}

/* ---------- 变更摘要 ---------- */

/** 单侧对象 → 截断 JSON；null 视为无 */
function compactJson(value: Record<string, unknown> | null): string {
  if (!value) return '—'
  const text = JSON.stringify(value)
  return text.length > 120 ? `${text.slice(0, 120)}…` : text
}

/** 展开行完整 before/after：格式化 JSON；null 视为无 */
function fullJson(value: Record<string, unknown> | null): string {
  return value ? JSON.stringify(value, null, 2) : '—'
}

/** 变更摘要：before → after 简写；无变化对象则保留有值一侧 */
function changeSummary(log: AuditLog): string {
  if (log.before && log.after) {
    return `${compactJson(log.before)} → ${compactJson(log.after)}`
  }
  if (log.after) return `+ ${compactJson(log.after)}`
  if (log.before) return `- ${compactJson(log.before)}`
  return '—'
}

function formatTime(value: string): string {
  return new Date(value).toLocaleString()
}

onMounted(() => {
  void loadList()
})
</script>

<template>
  <section>
    <div class="flex items-center justify-between">
      <h1 class="text-lg font-semibold text-gray-900">审计日志</h1>
    </div>

    <!-- 筛选条 -->
    <div class="mt-4 flex flex-wrap items-center gap-2">
      <el-select
        v-model="actionFilter"
        class="w-40"
        placeholder="全部动作"
        clearable
        @change="applyFilter"
      >
        <el-option-group v-for="group in actionGroups" :key="group.group" :label="group.group">
          <el-option
            v-for="action in group.actions"
            :key="action"
            :value="action"
            :label="auditActionMeta(action).label"
          />
        </el-option-group>
      </el-select>
      <el-input
        v-model="actorId"
        class="w-56"
        placeholder="操作者 ID（UUID）"
        clearable
        @keyup.enter="applyFilter"
        @clear="applyFilter"
      />
      <el-date-picker
        v-model="timeRange"
        type="datetimerange"
        value-format="x"
        range-separator="至"
        start-placeholder="开始时间"
        end-placeholder="结束时间"
        class="w-[380px]"
      />
      <el-button @click="applyFilter">查询</el-button>
      <el-button @click="resetFilter">重置</el-button>
    </div>

    <!-- 列表 -->
    <el-card class="mt-4" shadow="never">
      <el-table v-loading="auditLogsStore.loading" :data="auditLogsStore.logs" stripe>
        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="grid grid-cols-2 gap-4 px-4 py-2">
              <div>
                <p class="mb-1 text-xs font-medium text-gray-500">变更前（before）</p>
                <pre
                  class="max-h-48 overflow-auto whitespace-pre-wrap rounded bg-gray-50 p-2 text-xs text-gray-700"
                  >{{ fullJson(row.before) }}</pre>
              </div>
              <div>
                <p class="mb-1 text-xs font-medium text-gray-500">变更后（after）</p>
                <pre
                  class="max-h-48 overflow-auto whitespace-pre-wrap rounded bg-gray-50 p-2 text-xs text-gray-700"
                  >{{ fullJson(row.after) }}</pre>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column label="动作" width="120">
          <template #default="{ row }">
            <el-tag :type="auditActionMeta(row.action).type" size="small">
              {{ auditActionMeta(row.action).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作者" min-width="140">
          <template #default="{ row }">
            <div>
              <div class="text-sm text-gray-900">{{ row.actorName }}</div>
              <div class="text-xs text-gray-400">{{ row.actorId }}</div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="resourceType" label="资源类型" width="120" />
        <el-table-column prop="resourceId" label="资源 ID" min-width="150" show-overflow-tooltip />
        <el-table-column label="变更摘要" min-width="240">
          <template #default="{ row }">
            <span class="text-xs text-gray-600" :title="changeSummary(row)">
              {{ changeSummary(row) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="ip" label="IP" width="130" show-overflow-tooltip />
        <el-table-column label="时间" width="170">
          <template #default="{ row }">
            {{ formatTime(row.createdAt) }}
          </template>
        </el-table-column>
      </el-table>

      <div class="mt-4 flex justify-end">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="auditLogsStore.total"
          :page-sizes="pageSizes"
          layout="total, sizes, prev, pager, next"
          @current-change="loadList"
          @size-change="applyFilter"
        />
      </div>
    </el-card>
  </section>
</template>
