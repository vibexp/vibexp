import type { components, operations } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'

// Generated wire types for the instance-admin domain (#316) — the OpenAPI spec
// is the single source of truth. These back the read-only `/api/v1/admin/*`
// pages; every call is authorized server-side (404 for non-admins) regardless
// of the SPA's `is_instance_admin` gating.
export type AdminInstanceCounts = components['schemas']['AdminInstanceCounts']
export type AdminExtendedCounts = components['schemas']['AdminExtendedCounts']
export type AdminBreakdownBucket = components['schemas']['AdminBreakdownBucket']
export type AdminEntityBreakdown = components['schemas']['AdminEntityBreakdown']
export type AdminTableStat = components['schemas']['AdminTableStat']
export type AdminSystemHealth = components['schemas']['AdminSystemHealth']
export type AdminDashboardOverview =
  components['schemas']['AdminDashboardOverview']
export type AdminGrowthPoint = components['schemas']['AdminGrowthPoint']
export type AdminCountPoint = components['schemas']['AdminCountPoint']
export type AdminSourcePoint = components['schemas']['AdminSourcePoint']
export type AdminDataWindow = components['schemas']['AdminDataWindow']
export type AdminTimeseriesResponse =
  components['schemas']['AdminTimeseriesResponse']

/** Query parameters for the dashboard time series (#451). */
export type AdminTimeseriesParams = NonNullable<
  operations['getAdminDashboardTimeseries']['parameters']['query']
>
export type AdminStatsResponse = components['schemas']['AdminStatsResponse']
export type AdminUserListItem = components['schemas']['AdminUserListItem']
export type AdminUserListResponse =
  components['schemas']['AdminUserListResponse']
export type AdminTeamMembership = components['schemas']['AdminTeamMembership']
export type AdminUserCreateRequest =
  components['schemas']['AdminUserCreateRequest']
export type AdminUserUpdateRequest =
  components['schemas']['AdminUserUpdateRequest']
export type AdminDeleteBlocker = components['schemas']['AdminDeleteBlocker']
export type AdminUserDeleteBlockedResponse =
  components['schemas']['AdminUserDeleteBlockedResponse']

/** Query parameters for the instance-wide user listing (#452 + #454's status). */
export type AdminUserListParams = NonNullable<
  operations['listAdminUsers']['parameters']['query']
>

/**
 * Outcome of a delete attempt.
 *
 * A refusal is **data**, not an error: the 409 body is a documented schema
 * (`AdminUserDeleteBlockedResponse`) carrying the teams that blocked it, and the
 * dialog renders them. Modelling it as a thrown error would lose the list — see
 * `deleteUser` for why `unwrap` cannot be used on this call.
 */
export type AdminUserDeleteResult =
  | { deleted: true }
  | { deleted: false; refusal: AdminUserDeleteBlockedResponse }
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

/**
 * Structural check on the 409 body.
 *
 * A 409 whose body is not the documented refusal shape falls through to the
 * normal error path rather than being reported as a refusal with no blockers —
 * failing loudly beats inventing "nothing is blocking this" when the response
 * cannot be read.
 */
function isDeleteRefusal(
  body: unknown
): body is AdminUserDeleteBlockedResponse {
  if (typeof body !== 'object' || body === null) return false
  const candidate = body as Partial<AdminUserDeleteBlockedResponse>
  return (
    typeof candidate.message === 'string' && Array.isArray(candidate.blockers)
  )
}

class AdminService {
  /** Instance-wide counts + running backend version (GET /admin/stats). */
  async getStats(): Promise<AdminStatsResponse> {
    return unwrap(generatedClient.GET('/api/v1/admin/stats', {}))
  }

  /**
   * One page of the instance-wide user listing.
   *
   * Filters, sort and pagination are server-side, so the envelope's totals
   * describe the filtered set.
   */
  async listUsers(params: AdminUserListParams): Promise<AdminUserListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/users', {
        params: { query: params },
      })
    )
  }

  /** Create a user directly. Publishes `user.created`, so the account gets its personal team and default project exactly as a self-signup would (#462). */
  async createUser(body: AdminUserCreateRequest): Promise<AdminUserDetail> {
    return unwrap(generatedClient.POST('/api/v1/admin/users', { body }))
  }

  /** Update a user's display name. */
  async updateUser(
    id: string,
    body: AdminUserUpdateRequest
  ): Promise<AdminUserDetail> {
    return unwrap(
      generatedClient.PATCH('/api/v1/admin/users/{id}', {
        params: { path: { id } },
        body,
      })
    )
  }

  /** Suspend a user: every auth entry point rejects them until reactivated (#454). */
  async suspendUser(id: string): Promise<AdminUserDetail> {
    return unwrap(
      generatedClient.POST('/api/v1/admin/users/{id}/suspend', {
        params: { path: { id } },
      })
    )
  }

  /** Lift a suspension. */
  async reactivateUser(id: string): Promise<AdminUserDetail> {
    return unwrap(
      generatedClient.POST('/api/v1/admin/users/{id}/reactivate', {
        params: { path: { id } },
      })
    )
  }

  /**
   * Hard-delete a user, or report why it was refused.
   *
   * Deliberately **not** routed through `unwrap`. The 409 body is
   * `application/json` + `AdminUserDeleteBlockedResponse`, not RFC-9457 problem
   * details, so `unwrap`'s `isProblemDetails` check fails and it collapses the
   * response into a generic `ApiError` with `code: 'UNKNOWN_ERROR'` and
   * `detail: 'HTTP 409 error'` — discarding `blockers` entirely, which is the
   * one thing the dialog needs. Every other status still goes through `unwrap`'s
   * error handling, so timeouts and 404s behave exactly as elsewhere.
   */
  async deleteUser(id: string): Promise<AdminUserDeleteResult> {
    const result = await generatedClient.DELETE('/api/v1/admin/users/{id}', {
      params: { path: { id } },
    })

    if (result.response.status === 409 && isDeleteRefusal(result.error)) {
      return { deleted: false, refusal: result.error }
    }
    // Re-resolve through unwrap so non-409 failures throw the same ApiError the
    // rest of the app handles.
    await unwrap(Promise.resolve(result))
    return { deleted: true }
  }

  /** Instance totals, per-entity breakdowns, system health and the app version. */
  async getDashboardOverview(): Promise<AdminDashboardOverview> {
    return unwrap(generatedClient.GET('/api/v1/admin/dashboard/overview', {}))
  }

  /**
   * Bucketed growth, sign-ins and access-by-source for a range.
   *
   * The response reports the range and granularity it actually used: `from` is
   * snapped down to a whole bucket, so it can precede what was asked for. Panels
   * label themselves from the response rather than from the request.
   */
  async getDashboardTimeseries(
    params: AdminTimeseriesParams
  ): Promise<AdminTimeseriesResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/dashboard/timeseries', {
        params: { query: params },
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
