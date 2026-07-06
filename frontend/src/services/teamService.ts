import type { components } from '@vibexp/api-client'

import { apiClient } from '../lib/apiClient'
import type { ResourceAccessMetricsResponse } from './resourceAccessService'

// Generated wire types for the team AI-tools metrics (issue #92 hooks slice).
// The `*CountByDate` rows keep their historical names as aliases of the renamed
// generated daily-count schemas so chart consumers don't churn.
export type TeamCreationCountByDate =
  components['schemas']['TeamResourceCreationDailyCount']
export type TeamResourceCreationMetricsResponse =
  components['schemas']['TeamResourceCreationMetricsResponse']
export type TeamFeedCreationCountByDate =
  components['schemas']['TeamFeedCreationDailyCount']
export type TeamFeedCreationMetricsResponse =
  components['schemas']['TeamFeedCreationMetricsResponse']
export type TopAccessedResource =
  components['schemas']['TopAccessedResourceItem']
export type TeamTopAccessedResourcesResponse =
  components['schemas']['TeamTopAccessedResourcesResponse']

// Generated wire types for the team domain (epic #87), keeping the historical
// names as aliases of the renamed generated schemas so ~28 consumers don't churn.
// ⚠️ COLLISION TRAP: the manual `InvitationResponse` was the `{ invitation }`
// wrapper → generated `InvitationDetailsResponse`; the manual `TeamInvitation`
// (the invitation object itself) → generated `InvitationResponse`.
export type Team = components['schemas']['Team']
export type TeamMember = components['schemas']['TeamMemberDetail']
export type TeamsResponse = components['schemas']['TeamListResponse']
// `members` stays optional — the backend omits the slice when empty (Go
// omitempty), which the generated (required) field doesn't model.
export type TeamMembersResponse = Omit<
  components['schemas']['TeamMembersListResponse'],
  'members'
> & { members?: TeamMember[] }
export type TeamStats = components['schemas']['TeamStatsResponse']
// `role` stays optional — the service injects a default ('member'); the
// generated SendInvitationsRequest marks it required.
export type InviteTeamMembersRequest = Omit<
  components['schemas']['SendInvitationsRequest'],
  'role'
> & { role?: 'member' | 'admin' }
// The generated InvitationResponse marks every field optional (Go omitempty),
// but the get/list invitation endpoints always populate the invitation's
// identity + lifecycle fields (as the previous hand-written type asserted), so
// re-require them here to avoid scattering fallbacks across ~12 consumers.
export type TeamInvitation = components['schemas']['InvitationResponse'] & {
  id: string
  token: string
  team_id: string
  team_name: string
  status: InvitationStatus
  created_at: string
  expires_at: string
}
// The `{ invitation }` wrapper (generated InvitationDetailsResponse), re-typed
// so the wrapped value is the domain TeamInvitation.
export interface InvitationResponse {
  invitation: TeamInvitation
}
// `invitations` stays optional (Go omitempty) and carries the domain type.
export type PendingInvitationsResponse = Omit<
  components['schemas']['PendingInvitationsListResponse'],
  'invitations'
> & { invitations?: TeamInvitation[] }
export type AcceptInvitationResponse =
  components['schemas']['AcceptInvitationResponse']
export type CreateTeamRequest = components['schemas']['CreateTeamRequest']
export type UpdateTeamRequest = components['schemas']['UpdateTeamRequest']

// Local shapes with no generated counterpart. `InviteTeamMembersResponse` is a
// spec gap; `TeamWithMembers` is a frontend composite; the role/status unions
// exist only inline in the generated schemas (status is now `revoked`, not the
// legacy `expired`).
export type TeamRole = 'owner' | 'admin' | 'member'
export type InvitationStatus = 'pending' | 'accepted' | 'rejected' | 'revoked'
export interface InviteTeamMembersResponse {
  invitations: { email: string; invitation_id: string; status: string }[]
  success_count: number
  error_count: number
}
export interface TeamWithMembers extends Team {
  members: TeamMember[]
}

/**
 * Team service for managing teams and team invitations
 */
class TeamService {
  /**
   * Get all teams for the current user
   */
  async getTeams(): Promise<Team[]> {
    const response = await apiClient.get<TeamsResponse>('/teams')
    return response.teams
  }

  /**
   * Create a new team
   */
  async createTeam(request: CreateTeamRequest): Promise<Team> {
    return apiClient.post<Team>('/teams', request)
  }

  /**
   * Get team details (without members - use getTeamMembers for members)
   */
  async getTeamDetails(teamId: string): Promise<Team> {
    const response = await apiClient.get<Team>(`/teams/${teamId}`)
    return response
  }

  /**
   * Get team members
   */
  async getTeamMembers(teamId: string): Promise<TeamMember[]> {
    const response = await apiClient.get<TeamMembersResponse>(
      `/teams/${teamId}/members`
    )
    return response.members ?? []
  }

  /**
   * Invite members to a team
   */
  async inviteMembers(
    teamId: string,
    request: InviteTeamMembersRequest
  ): Promise<InviteTeamMembersResponse> {
    return apiClient.post<InviteTeamMembersResponse>(
      `/teams/${teamId}/invitations`,
      { ...request, role: request.role ?? 'member' }
    )
  }

  /**
   * Get all invitations for a team (any status). Owners and admins can call this.
   *
   * Unlike {@link getPendingInvitations}, this endpoint returns a raw JSON array
   * (not a `{invitations: [...]}` envelope) — wire shape is fixed by
   * `handleListTeamInvitations` in `team_invitation_handlers.go`.
   */
  async getTeamInvitations(teamId: string): Promise<TeamInvitation[]> {
    return apiClient.get<TeamInvitation[]>(`/teams/${teamId}/invitations`)
  }

  /**
   * Get all pending invitations for the current user
   */
  async getPendingInvitations() {
    const response = await apiClient.get<PendingInvitationsResponse>(
      '/invitations/pending'
    )
    return response.invitations ?? []
  }

  /**
   * Get invitation details by token
   */
  async getInvitationByToken(token: string): Promise<InvitationResponse> {
    return apiClient.get<InvitationResponse>(
      `/invitations/${encodeURIComponent(token)}`
    )
  }

  /**
   * Accept a team invitation
   */
  async acceptInvitation(token: string): Promise<AcceptInvitationResponse> {
    return apiClient.post<AcceptInvitationResponse>(
      `/invitations/${encodeURIComponent(token)}/accept`
    )
  }

  /**
   * Reject a team invitation
   */
  async rejectInvitation(token: string): Promise<void> {
    await apiClient.post<Record<string, never>>(
      `/invitations/${encodeURIComponent(token)}/reject`
    )
  }

  /**
   * Leave a team
   */
  async leaveTeam(teamId: string, userId: string): Promise<void> {
    await apiClient.delete<Record<string, never>>(
      `/teams/${teamId}/members/${userId}`
    )
  }

  /**
   * Remove a member from a team
   */
  async removeMember(teamId: string, userId: string): Promise<void> {
    await apiClient.delete<Record<string, never>>(
      `/teams/${teamId}/members/${userId}`
    )
  }

  /**
   * Update a team
   */
  async updateTeam(teamId: string, request: UpdateTeamRequest): Promise<Team> {
    return apiClient.put<Team>(`/teams/${teamId}`, request)
  }

  /**
   * Delete a team
   */
  async deleteTeam(teamId: string): Promise<void> {
    await apiClient.delete<Record<string, never>>(`/teams/${teamId}`)
  }

  /**
   * Get team-wide resource counts for the team analytics page. The endpoint
   * returns the bare stats object (no envelope), matching getProjectStats.
   */
  async getTeamStats(teamId: string): Promise<TeamStats> {
    return apiClient.get<TeamStats>(
      `/teams/${encodeURIComponent(teamId)}/stats`
    )
  }

  /**
   * Get team-wide daily resource-creation metrics (prompts/artifacts/blueprints/
   * memories/projects). Mirrors resourceCreationService — plain apiClient, the
   * team metrics domain is not on the generated typed client.
   */
  async getTeamResourceCreationMetrics(
    teamId: string,
    range = '30d',
    signal?: AbortSignal
  ): Promise<TeamResourceCreationMetricsResponse> {
    const params = new URLSearchParams({ range })
    return apiClient.get<TeamResourceCreationMetricsResponse>(
      `/teams/${encodeURIComponent(teamId)}/resource-creation-metrics?${params.toString()}`,
      { signal }
    )
  }

  /**
   * Get team-wide daily access metrics (web/cli/mcp/api breakdown across every
   * resource in the team). Reuses the per-resource access response shape.
   */
  async getTeamResourceAccessMetrics(
    teamId: string,
    range = '30d',
    signal?: AbortSignal
  ): Promise<ResourceAccessMetricsResponse> {
    const params = new URLSearchParams({ range })
    return apiClient.get<ResourceAccessMetricsResponse>(
      `/teams/${encodeURIComponent(teamId)}/resource-access-metrics?${params.toString()}`,
      { signal }
    )
  }

  /**
   * Get team-wide daily feed-creation metrics (feeds + feed items created over
   * time). Mirrors the resource-creation/access metrics methods.
   */
  async getTeamFeedCreationMetrics(
    teamId: string,
    range = '30d',
    signal?: AbortSignal
  ): Promise<TeamFeedCreationMetricsResponse> {
    const params = new URLSearchParams({ range })
    return apiClient.get<TeamFeedCreationMetricsResponse>(
      `/teams/${encodeURIComponent(teamId)}/feed-creation-metrics?${params.toString()}`,
      { signal }
    )
  }

  /**
   * Get the team's most-accessed resources over a range, ranked by access count.
   * `limit` defaults to the backend default (5) and is capped server-side at 50.
   * `source` restricts the ranking to a single access channel (web/cli/mcp/api);
   * omitted or 'all' aggregates across channels.
   */
  async getTeamTopAccessedResources(
    teamId: string,
    range = '30d',
    limit?: number,
    source?: string,
    signal?: AbortSignal
  ): Promise<TeamTopAccessedResourcesResponse> {
    const params = new URLSearchParams({ range })
    if (limit != null) params.set('limit', String(limit))
    if (source != null && source !== 'all') params.set('source', source)
    return apiClient.get<TeamTopAccessedResourcesResponse>(
      `/teams/${encodeURIComponent(teamId)}/top-accessed-resources?${params.toString()}`,
      { signal }
    )
  }
}

// Export singleton instance
export const teamService = new TeamService()
