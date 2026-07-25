import type { components, operations } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the instance-admin domain (#316) — the OpenAPI spec
// is the single source of truth. These back the read-only `/api/v1/admin/*`
// pages; every call is authorized server-side (404 for non-admins) regardless
// of the SPA's `is_instance_admin` gating.
export type AdminInstanceCounts = components['schemas']['AdminInstanceCounts']
export type AdminStatsResponse = components['schemas']['AdminStatsResponse']
export type AdminUserListItem = components['schemas']['AdminUserListItem']
export type AdminUserListResponse =
  components['schemas']['AdminUserListResponse']
export type AdminTeamMembership = components['schemas']['AdminTeamMembership']
export type AdminUserDetail = components['schemas']['AdminUserDetail']
export type AdminTeamOwner = components['schemas']['AdminTeamOwner']
export type AdminTeamListItem = components['schemas']['AdminTeamListItem']
export type AdminTeamListResponse =
  components['schemas']['AdminTeamListResponse']
export type AdminTeamMember = components['schemas']['AdminTeamMember']
export type AdminTeamDetail = components['schemas']['AdminTeamDetail']

/**
 * Query parameters for the instance-wide team listing (#452).
 *
 * Taken straight off the generated operation rather than restated, so a spec
 * change surfaces here as a type error instead of a silently ignored filter.
 * `created_from`/`created_to` are RFC 3339 instants — the caller converts a
 * date-only range with `rangeToInstants`, which puts the upper bound at local
 * end-of-day so a single-day filter is not empty.
 */
export type AdminTeamListParams = NonNullable<
  operations['listAdminTeams']['parameters']['query']
>

export type AdminProjectTeam = components['schemas']['AdminProjectTeam']
export type AdminProjectListItem = components['schemas']['AdminProjectListItem']
export type AdminProjectListResponse =
  components['schemas']['AdminProjectListResponse']
export type AdminProjectResourceCounts =
  components['schemas']['AdminProjectResourceCounts']
export type AdminProjectDetail = components['schemas']['AdminProjectDetail']

/** Query parameters for the instance-wide project listing (#453). */
export type AdminProjectListParams = NonNullable<
  operations['listAdminProjects']['parameters']['query']
>

class AdminService {
  /** Instance-wide counts + running backend version (GET /admin/stats). */
  async getStats(): Promise<AdminStatsResponse> {
    return unwrap(generatedClient.GET('/api/v1/admin/stats', {}))
  }

  /** One page of the instance-wide user listing, newest first. */
  async listUsers(page: number, limit: number): Promise<AdminUserListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/users', {
        params: { query: { page, limit } },
      })
    )
  }

  /** A single user with their team memberships. */
  async getUser(id: string): Promise<AdminUserDetail> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/users/{id}', {
        params: { path: { id } },
      })
    )
  }

  /**
   * One page of the instance-wide team listing.
   *
   * Filters, sort and pagination are all server-side, so the envelope's totals
   * describe the filtered set rather than the instance.
   */
  async listTeams(params: AdminTeamListParams): Promise<AdminTeamListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/teams', {
        params: { query: params },
      })
    )
  }

  /** A single team with its owner and member list. */
  async getTeam(id: string): Promise<AdminTeamDetail> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/teams/{id}', {
        params: { path: { id } },
      })
    )
  }

  /**
   * One page of the instance-wide project listing.
   *
   * Filters, sort and pagination are server-side, so the envelope's totals
   * describe the filtered set.
   */
  async listProjects(
    params: AdminProjectListParams
  ): Promise<AdminProjectListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/projects', {
        params: { query: params },
      })
    )
  }

  /** A single project with its team, creator and per-type resource counts. */
  async getProject(id: string): Promise<AdminProjectDetail> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/projects/{id}', {
        params: { path: { id } },
      })
    )
  }
}

export const adminService = new AdminService()
