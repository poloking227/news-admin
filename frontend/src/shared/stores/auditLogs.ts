import { defineStore } from 'pinia'
import { listAuditLogs as listAuditLogsApi } from '@/shared/api/auditLogs'
import type { AuditLog, AuditLogListQuery } from '@/shared/types/api'

interface AuditLogsState {
  logs: AuditLog[]
  total: number
  page: number
  pageSize: number
  loading: boolean
  /** 最近一次列表查询参数，供刷新复用 */
  lastQuery: AuditLogListQuery
}

const DEFAULT_PAGE_SIZE = 10

/** 管理端审计日志状态：只读查询，筛选 + 分页 */
export const useAuditLogsStore = defineStore('auditLogs', {
  state: (): AuditLogsState => ({
    logs: [],
    total: 0,
    page: 1,
    pageSize: DEFAULT_PAGE_SIZE,
    loading: false,
    lastQuery: { page: 1, pageSize: DEFAULT_PAGE_SIZE }
  }),
  getters: {
    pageCount(): number {
      return Math.max(1, Math.ceil(this.total / this.pageSize))
    }
  },
  actions: {
    /** 拉取列表；本次查询参数存为 lastQuery 供 refresh 复用 */
    async fetchList(query: AuditLogListQuery = {}): Promise<void> {
      const merged: AuditLogListQuery = {
        ...this.lastQuery,
        ...query,
        page: query.page ?? 1,
        pageSize: query.pageSize ?? this.pageSize ?? DEFAULT_PAGE_SIZE
      }
      this.lastQuery = merged
      this.loading = true
      try {
        const data = await listAuditLogsApi(merged)
        this.logs = data.items
        this.total = data.total
        this.page = data.page
        this.pageSize = data.pageSize
      } finally {
        this.loading = false
      }
    },
    /** 以最近查询参数刷新当前页 */
    async refresh(): Promise<void> {
      await this.fetchList(this.lastQuery)
    }
  }
})
