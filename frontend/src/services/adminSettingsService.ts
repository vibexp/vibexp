import type { components, operations } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'
import type { AdminInstanceSettingsAuditPage } from './adminService'

// Instance search ranking + AI summary settings (#1200, pages #1202). Kept out
// of `adminService.ts`, which is at the max-lines limit. Every call is
// authorized server-side (404 for non-admins).
export type AdminInstanceSearchSettings =
  components['schemas']['AdminInstanceSearchSettings']
export type AdminInstanceSearchSettingsUpdate =
  components['schemas']['AdminInstanceSearchSettingsUpdate']
export type AdminInstanceAISummarySettings =
  components['schemas']['AdminInstanceAISummarySettings']
export type AdminInstanceAISummarySettingsUpdate =
  components['schemas']['AdminInstanceAISummarySettingsUpdate']
export type AdminSiteInfo = components['schemas']['AdminSiteInfo']
export type AdminSiteInfoUpdate = components['schemas']['AdminSiteInfoUpdate']
export type AdminInstanceSettingsAuditParams = NonNullable<
  operations['listAdminSearchSettingsAudit']['parameters']['query']
>

class AdminSettingsService {
  /**
   * The instance search ranking defaults in effect, their source, the
   * built-in defaults, the validation limits and the override count.
   */
  async getSearchSettings(): Promise<AdminInstanceSearchSettings> {
    return unwrap(generatedClient.GET('/api/v1/admin/settings/search'))
  }

  /**
   * Replace the instance search ranking defaults. With `expected_version`
   * set, a save over someone else's change is rejected with 409
   * (`INSTANCE_SETTINGS_VERSION_CONFLICT`).
   */
  async updateSearchSettings(
    body: AdminInstanceSearchSettingsUpdate
  ): Promise<AdminInstanceSearchSettings> {
    return unwrap(
      generatedClient.PUT('/api/v1/admin/settings/search', { body })
    )
  }

  /** Drop the stored search defaults, so the built-in defaults apply. */
  async resetSearchSettings(): Promise<void> {
    await unwrap(generatedClient.DELETE('/api/v1/admin/settings/search'))
  }

  /** One page of the instance search settings' audit log, newest first. */
  async listSearchSettingsAudit(
    params: AdminInstanceSettingsAuditParams = {}
  ): Promise<AdminInstanceSettingsAuditPage> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/settings/search/audit', {
        params: { query: params },
      })
    )
  }

  /** The instance AI summary defaults and server budgets, as for search. */
  async getAISummarySettings(): Promise<AdminInstanceAISummarySettings> {
    return unwrap(generatedClient.GET('/api/v1/admin/settings/ai-summary'))
  }

  /** Replace the instance AI summary settings; 409 as for search. */
  async updateAISummarySettings(
    body: AdminInstanceAISummarySettingsUpdate
  ): Promise<AdminInstanceAISummarySettings> {
    return unwrap(
      generatedClient.PUT('/api/v1/admin/settings/ai-summary', { body })
    )
  }

  /** Drop the stored AI summary settings, so the built-in defaults apply. */
  async resetAISummarySettings(): Promise<void> {
    await unwrap(generatedClient.DELETE('/api/v1/admin/settings/ai-summary'))
  }

  /** One page of the instance AI summary settings' audit log, newest first. */
  async listAISummarySettingsAudit(
    params: AdminInstanceSettingsAuditParams = {}
  ): Promise<AdminInstanceSettingsAuditPage> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/settings/ai-summary/audit', {
        params: { query: params },
      })
    )
  }

  /** The instance's company branding as stored, its limits and metadata. */
  async getSiteInfo(): Promise<AdminSiteInfo> {
    return unwrap(generatedClient.GET('/api/v1/admin/settings/site-info'))
  }

  /**
   * Replace the instance's company branding; an empty string clears a value.
   * `expected_version` makes the save a compare-and-set, as for search.
   */
  async updateSiteInfo(body: AdminSiteInfoUpdate): Promise<AdminSiteInfo> {
    return unwrap(
      generatedClient.PUT('/api/v1/admin/settings/site-info', { body })
    )
  }

  /** Drop the stored site info, so the instance shows no company branding. */
  async resetSiteInfo(): Promise<void> {
    await unwrap(generatedClient.DELETE('/api/v1/admin/settings/site-info'))
  }

  /** One page of the site info's audit log, newest first. */
  async listSiteInfoAudit(
    params: AdminInstanceSettingsAuditParams = {}
  ): Promise<AdminInstanceSettingsAuditPage> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/settings/site-info/audit', {
        params: { query: params },
      })
    )
  }
}

export const adminSettingsService = new AdminSettingsService()
