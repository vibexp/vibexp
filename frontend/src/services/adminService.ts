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

// Per-user insight reads behind the user detail page (#1135, #1136). Every
// resource is opaque — type, short id, team and project, never a title.
export type AdminResourceCounts = components['schemas']['AdminResourceCounts']
export type AdminUserInsights = components['schemas']['AdminUserInsights']
export type AdminUserTeamResourceCounts =
  components['schemas']['AdminUserTeamResourceCounts']
export type AdminUserCreationPoint =
  components['schemas']['AdminUserCreationPoint']
export type AdminUserResourceCreationMetrics =
  components['schemas']['AdminUserResourceCreationMetrics']
export type AdminUserAccessMetrics =
  components['schemas']['AdminUserAccessMetrics']
export type AdminTopAccessedResource =
  components['schemas']['AdminTopAccessedResource']
export type AdminTopAccessedResourcesResponse =
  components['schemas']['AdminTopAccessedResourcesResponse']
export type AdminUserTimelineEvent =
  components['schemas']['AdminUserTimelineEvent']
export type AdminUserTimelinePage =
  components['schemas']['AdminUserTimelinePage']
export type AdminUserNotificationPreferences =
  components['schemas']['AdminUserNotificationPreferences']

/** Range + bucket size shared by the per-user creation and access series. */
export type AdminUserSeriesParams = NonNullable<
  operations['getAdminUserResourceCreationMetrics']['parameters']['query']
>
export type AdminUserTopAccessedParams = NonNullable<
  operations['getAdminUserTopAccessedResources']['parameters']['query']
>
export type AdminUserTimelineParams = NonNullable<
  operations['getAdminUserTimeline']['parameters']['query']
>

// Per-project analytics and configuration behind the project detail tabs
// (#1145). Top-accessed rows reuse the opaque `AdminTopAccessedResource`.
export type AdminProjectCreationPoint =
  components['schemas']['AdminProjectCreationPoint']
export type AdminProjectResourceCreationMetrics =
  components['schemas']['AdminProjectResourceCreationMetrics']
export type AdminProjectAccessMetrics =
  components['schemas']['AdminProjectAccessMetrics']
export type AdminProjectConfig = components['schemas']['AdminProjectConfig']

/** Range + bucket size shared by the per-project creation and access series. */
export type AdminProjectSeriesParams = NonNullable<
  operations['getAdminProjectResourceCreationMetrics']['parameters']['query']
>
export type AdminProjectTopAccessedParams = NonNullable<
  operations['getAdminProjectTopAccessedResources']['parameters']['query']
>

// Read-only team configuration behind the team detail tabs (#1140, #1141).
// Secrets arrive already redacted: credentials as `has_*` booleans,
// configuration as key names only, and no free-text error.
export type AdminTeamConfigSource =
  components['schemas']['AdminTeamConfigSource']
export type AdminSearchValues = components['schemas']['AdminSearchValues']
export type AdminTeamSearchConfig =
  components['schemas']['AdminTeamSearchConfig']
export type AdminTeamAISummaryConfig =
  components['schemas']['AdminTeamAISummaryConfig']
export type AdminFreshnessRule = components['schemas']['AdminFreshnessRule']
export type AdminTeamFreshnessConfig =
  components['schemas']['AdminTeamFreshnessConfig']
export type AdminArtifactType = components['schemas']['AdminArtifactType']
export type AdminTeamArtifactTypes =
  components['schemas']['AdminTeamArtifactTypes']
export type AdminTeamSettingsAuditEntry =
  components['schemas']['AdminTeamSettingsAuditEntry']
export type AdminTeamSettingsAuditListResponse =
  components['schemas']['AdminTeamSettingsAuditListResponse']
export type AdminModelProvider = components['schemas']['AdminModelProvider']
export type AdminTeamModelProvidersConfig =
  components['schemas']['AdminTeamModelProvidersConfig']
export type AdminEmbeddingProvider =
  components['schemas']['AdminEmbeddingProvider']
export type AdminTeamEmbeddingProvidersConfig =
  components['schemas']['AdminTeamEmbeddingProvidersConfig']
export type AdminTeamEmailProviderConfig =
  components['schemas']['AdminTeamEmailProviderConfig']
export type AdminTeamGitHubConfig =
  components['schemas']['AdminTeamGitHubConfig']

/** Paging for a team's settings audit log (`page` 1-based, `limit` 1–100). */
export type AdminTeamSettingsAuditParams = NonNullable<
  operations['listAdminTeamSettingsAudit']['parameters']['query']
>

/** Which admin list a saved filter preset belongs to (#1147). */
export type AdminSavedFilterListName =
  components['schemas']['AdminSavedFilterListName']
export type AdminSavedFilterPreset =
  components['schemas']['AdminSavedFilterPreset']
export type AdminSavedFilterPresetInput =
  components['schemas']['AdminSavedFilterPresetInput']
export type AdminSavedFilters = components['schemas']['AdminSavedFilters']
export type AdminSavedFiltersReplaceRequest =
  components['schemas']['AdminSavedFiltersReplaceRequest']

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

  /** Resource counts for one user, instance-wide and per team/project. */
  async getUserInsights(id: string): Promise<AdminUserInsights> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/users/{id}/insights', {
        params: { path: { id } },
      })
    )
  }

  /** Resources the user created, bucketed per type over a range. */
  async getUserResourceCreationMetrics(
    id: string,
    params: AdminUserSeriesParams
  ): Promise<AdminUserResourceCreationMetrics> {
    return unwrap(
      generatedClient.GET(
        '/api/v1/admin/users/{id}/resource-creation-metrics',
        { params: { path: { id }, query: params } }
      )
    )
  }

  /** The user's resource accesses, bucketed per source over a range. */
  async getUserResourceAccessMetrics(
    id: string,
    params: AdminUserSeriesParams
  ): Promise<AdminUserAccessMetrics> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/users/{id}/resource-access-metrics', {
        params: { path: { id }, query: params },
      })
    )
  }

  /** The resources the user accessed most in a range, as opaque references. */
  async getUserTopAccessedResources(
    id: string,
    params: AdminUserTopAccessedParams
  ): Promise<AdminTopAccessedResourcesResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/users/{id}/top-accessed-resources', {
        params: { path: { id }, query: params },
      })
    )
  }

  /** Resources created in the project, bucketed per type over a range. */
  async getProjectResourceCreationMetrics(
    id: string,
    params: AdminProjectSeriesParams
  ): Promise<AdminProjectResourceCreationMetrics> {
    return unwrap(
      generatedClient.GET(
        '/api/v1/admin/projects/{id}/resource-creation-metrics',
        { params: { path: { id }, query: params } }
      )
    )
  }

  /** Accesses to the project and its resources, bucketed per source. */
  async getProjectResourceAccessMetrics(
    id: string,
    params: AdminProjectSeriesParams
  ): Promise<AdminProjectAccessMetrics> {
    return unwrap(
      generatedClient.GET(
        '/api/v1/admin/projects/{id}/resource-access-metrics',
        { params: { path: { id }, query: params } }
      )
    )
  }

  /** The project's most-accessed resources in a range, as opaque references. */
  async getProjectTopAccessedResources(
    id: string,
    params: AdminProjectTopAccessedParams
  ): Promise<AdminTopAccessedResourcesResponse> {
    return unwrap(
      generatedClient.GET(
        '/api/v1/admin/projects/{id}/top-accessed-resources',
        { params: { path: { id }, query: params } }
      )
    )
  }

  /** The freshness rules that apply to a project: its own plus team-wide. */
  async getProjectConfig(id: string): Promise<AdminProjectConfig> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/projects/{id}/config', {
        params: { path: { id } },
      })
    )
  }

  /** One cursor page of the user's opaque create/update timeline, newest first. */
  async getUserTimeline(
    id: string,
    params: AdminUserTimelineParams = {}
  ): Promise<AdminUserTimelinePage> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/users/{id}/timeline', {
        params: { path: { id }, query: params },
      })
    )
  }

  /** The user's notification settings, read-only for the admin. */
  async getUserNotificationPreferences(
    id: string
  ): Promise<AdminUserNotificationPreferences> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/users/{id}/notification-preferences', {
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

  /** A team's effective search ranking settings, with the instance defaults. */
  async getTeamSearchConfig(id: string): Promise<AdminTeamSearchConfig> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/teams/{id}/config/search', {
        params: { path: { id } },
      })
    )
  }

  /** A team's effective AI summary settings and provider availability. */
  async getTeamAISummaryConfig(id: string): Promise<AdminTeamAISummaryConfig> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/teams/{id}/config/ai-summary', {
        params: { path: { id } },
      })
    )
  }

  /** A team's freshness evaluation settings and every freshness rule. */
  async getTeamFreshnessConfig(id: string): Promise<AdminTeamFreshnessConfig> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/teams/{id}/config/freshness', {
        params: { path: { id } },
      })
    )
  }

  /** The artifact types a team sees: system defaults plus its own. */
  async getTeamArtifactTypes(id: string): Promise<AdminTeamArtifactTypes> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/teams/{id}/config/artifact-types', {
        params: { path: { id } },
      })
    )
  }

  /** One page of a team's settings audit log, newest first. */
  async listTeamSettingsAudit(
    id: string,
    params: AdminTeamSettingsAuditParams = {}
  ): Promise<AdminTeamSettingsAuditListResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/teams/{id}/config/settings-audit', {
        params: { path: { id }, query: params },
      })
    )
  }

  /** A team's own model providers, credentials redacted. */
  async getTeamModelProviders(
    id: string
  ): Promise<AdminTeamModelProvidersConfig> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/teams/{id}/config/model-providers', {
        params: { path: { id } },
      })
    )
  }

  /** A team's own embedding providers plus its embedding coverage counts. */
  async getTeamEmbeddingProviders(
    id: string
  ): Promise<AdminTeamEmbeddingProvidersConfig> {
    return unwrap(
      generatedClient.GET(
        '/api/v1/admin/teams/{id}/config/embedding-providers',
        { params: { path: { id } } }
      )
    )
  }

  /** A team's effective email provider and its delivery health. */
  async getTeamEmailProvider(
    id: string
  ): Promise<AdminTeamEmailProviderConfig> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/teams/{id}/config/email-provider', {
        params: { path: { id } },
      })
    )
  }

  /** A team's GitHub App registration and installation, as last recorded. */
  async getTeamGitHubConfig(id: string): Promise<AdminTeamGitHubConfig> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/teams/{id}/config/github', {
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

  /** The calling admin's saved filter presets for one admin list (#1147). */
  async getSavedFilters(
    list: AdminSavedFilterListName
  ): Promise<AdminSavedFilters> {
    return unwrap(
      generatedClient.GET('/api/v1/admin/saved-filters/{list}', {
        params: { path: { list } },
      })
    )
  }

  /**
   * Replace the whole preset list for one admin list. A stale `version` is
   * rejected with 409 (an `ApiError` with `status === 409`) and nothing is saved.
   */
  async replaceSavedFilters(
    list: AdminSavedFilterListName,
    body: AdminSavedFiltersReplaceRequest
  ): Promise<AdminSavedFilters> {
    return unwrap(
      generatedClient.PUT('/api/v1/admin/saved-filters/{list}', {
        params: { path: { list } },
        body,
      })
    )
  }
}

export const adminService = new AdminService()
