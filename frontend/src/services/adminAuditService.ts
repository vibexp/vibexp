import type { components, operations } from '@vibexp/api-client'

import {
  generatedClient,
  longRunningClient,
  unwrap,
  unwrapWithResponse,
} from '@/lib/apiClientGenerated'
import type { AdminCsvExport, SameShape } from '@/services/adminService'
import { toCsvExport } from '@/services/adminService'

/**
 * The instance audit log for instance admins (#1516, UI #1517). Kept beside
 * `adminService` rather than in it, which is already at the file-size limit;
 * the CSV handling is the same `toCsvExport` every admin export uses.
 */

export type AdminAuditEvent = components['schemas']['AdminAuditEvent']
export type AdminAuditEventPage = components['schemas']['AdminAuditEventPage']
export type AdminAuditListParams = NonNullable<
  operations['listAdminAuditEvents']['parameters']['query']
>
export type AdminAuditExportParams = NonNullable<
  operations['exportAdminAuditEvents']['parameters']['query']
>

/**
 * Compile-time parity, like `ADMIN_EXPORT_PARAMS_MATCH_LIST`: the export takes
 * exactly the list's query minus its cursor and page size, so a filter added to
 * one op and not the other fails `tsc -b` instead of an export silently
 * ignoring a filter the table on screen applies.
 */
export const ADMIN_AUDIT_EXPORT_PARAMS_MATCH_LIST: SameShape<
  Omit<AdminAuditListParams, 'cursor' | 'limit'>,
  AdminAuditExportParams
> = true

export const adminAuditService = {
  /**
   * One page of the instance audit log, newest first. Pass the previous
   * page's `next_cursor` as `cursor` to continue; there is no total.
   */
  async listEvents(params: AdminAuditListParams): Promise<AdminAuditEventPage> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/audit-events', {
        params: { query: params },
      })
    )
  },

  /**
   * The filtered audit log as CSV. It streams the whole filtered set (capped
   * server-side), so it goes through `longRunningClient` like the other admin
   * exports.
   */
  async exportEvents(params: AdminAuditExportParams): Promise<AdminCsvExport> {
    const { data, response } = await unwrapWithResponse(
      longRunningClient.GET('/api/v1/admin/audit-events/export', {
        params: { query: params },
        parseAs: 'blob',
      })
    )
    return toCsvExport(data, response, 'audit')
  },
}
