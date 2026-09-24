import { http } from '@/shared/api/http'
import type { AuditLogListQuery, AuditLogPage } from '@/shared/types/api'

/** 审计日志查询（管理端）：actor/action/resourceType/resourceId/时间窗 + 分页 */
export async function listAuditLogs(query: AuditLogListQuery): Promise<AuditLogPage> {
  const { data } = await http.get<AuditLogPage>('/admin/audit-logs', { params: query })
  return data
}
