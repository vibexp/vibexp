import type { components } from '@vibexp/api-client'

import { generatedClient, unwrap } from '../lib/apiClientGenerated'
import type {
  MetricsRange,
  ResourceAccessMetricsResponse,
} from './resourceAccessService'

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
//
// `token` is deliberately NOT re-required: only the pending-invitations endpoint
// returns it (see PendingTeamInvitation).
export type TeamInvitation = components['schemas']['InvitationResponse'] & {
  id: string
  team_id: string
  team_name: string
  status: InvitationStatus
  created_at: string
  expires_at: string
}
// An invitation from GET /api/v1/invitations/pending, which is the only list that
// carries the accept/reject token: `buildInvitationResponses` populates it, while
// `convertInvitationsToResponses` (team list + invite-members) omits it. Requiring
// `token` on the shared TeamInvitation asserted a field the wire never sends for
// those other lists, so an accept driven off one would have posted "" (#251).
export type PendingTeamInvitation = TeamInvitation & { token: string }
// The `{ invitation }` wrapper (generated InvitationDetailsResponse), re-typed
// so the wrapped value is the domain TeamInvitation.
export interface InvitationResponse {
  invitation: TeamInvitation
}
// `invitations` stays optional (Go omitempty) and carries the domain type.
export type PendingInvitationsResponse = Omit<
  components['schemas']['PendingInvitationsListResponse'],
  'invitations'
> & { invitations?: PendingTeamInvitation[] }
export type AcceptInvitationResponse =
  components['schemas']['AcceptInvitationResponse']
export type CreateTeamRequest = components['schemas']['CreateTeamRequest']
export type UpdateTeamRequest = components['schemas']['UpdateTeamRequest']
// The roles a member can be changed *between*. `owner` is deliberately absent:
// the API reaches it only through transfer-ownership, never a role update.
export type ChangeableTeamRole =
  components['schemas']['UpdateTeamMemberRoleRequest']['role']

// Local shapes with no generated counterpart. `TeamWithMembers` is a frontend
// composite; the role/status unions exist only inline in the generated schemas
// (status is now `revoked`, not the legacy `expired`).
export type TeamRole = 'owner' | 'admin' | 'member'
export type InvitationStatus = 'pending' | 'accepted' | 'rejected' | 'revoked'
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
    return (await unwrap(generatedClient.GET('/api/v1/teams', {}))).teams
  }

  /**
   * Create a new team
   */
  async createTeam(request: CreateTeamRequest): Promise<Team> {
    return unwrap(generatedClient.POST('/api/v1/teams', { body: request }))
  }

  /**
   * Get team details (without members - use getTeamMembers for members)
   */
  async getTeamDetails(teamId: string): Promise<Team> {
    return unwrap(
      generatedClient.GET('/api/v1/teams/{id}', {
        params: { path: { id: teamId } },
      })
    )
  }

  /**
   * Get team members
   */
  async getTeamMembers(teamId: string): Promise<TeamMember[]> {
    // The generated response types `members` as required, but the backend omits
    // an empty slice (Go omitempty), so read it defensively.
    const response = (await unwrap(
      generatedClient.GET('/api/v1/teams/{id}/members', {
        params: { path: { id: teamId } },
      })
    )) as TeamMembersResponse
    return response.members ?? []
  }

  /**
   * Invite members to a team. The endpoint returns the created invitations as a
   * raw array (`InvitationResponse[]`); the backend populates the identity /
   * lifecycle fields the domain TeamInvitation requires.
   */
  async inviteMembers(
    teamId: string,
    request: InviteTeamMembersRequest
  ): Promise<TeamInvitation[]> {
    return (await unwrap(
      generatedClient.POST('/api/v1/teams/{id}/invitations', {
        params: { path: { id: teamId } },
        body: { ...request, role: request.role ?? 'member' },
      })
    )) as TeamInvitation[]
  }

  /**
   * Get all invitations for a team (any status). Owners and admins can call this.
   *
   * Unlike {@link getPendingInvitations}, this endpoint returns a raw JSON array
   * (not a `{invitations: [...]}` envelope) — wire shape is fixed by
   * `handleListTeamInvitations` in `team_invitation_handlers.go`.
   */
  async getTeamInvitations(teamId: string): Promise<TeamInvitation[]> {
    // The backend always populates the invitation identity/lifecycle fields the
    // domain TeamInvitation requires (see the type note above).
    return (await unwrap(
      generatedClient.GET('/api/v1/teams/{id}/invitations', {
        params: { path: { id: teamId } },
      })
    )) as TeamInvitation[]
  }

  /**
   * Get all pending invitations for the current user.
   *
   * The only invitation list that carries the accept/reject token — hence
   * {@link PendingTeamInvitation} rather than {@link TeamInvitation}.
   */
  async getPendingInvitations(): Promise<PendingTeamInvitation[]> {
    const response = (await unwrap(
      generatedClient.GET('/api/v1/invitations/pending', {})
    )) as PendingInvitationsResponse
    return response.invitations ?? []
  }

  /**
   * Get invitation details by token
   */
  async getInvitationByToken(token: string): Promise<InvitationResponse> {
    return (await unwrap(
      generatedClient.GET('/api/v1/invitations/{token}', {
        params: { path: { token } },
      })
    )) as unknown as InvitationResponse
  }

  /**
   * Accept a team invitation
   */
  async acceptInvitation(token: string): Promise<AcceptInvitationResponse> {
    return unwrap(
      generatedClient.POST('/api/v1/invitations/{token}/accept', {
        params: { path: { token } },
      })
    )
  }

  /**
   * Reject a team invitation
   */
  async rejectInvitation(token: string): Promise<void> {
    await unwrap(
      generatedClient.POST('/api/v1/invitations/{token}/reject', {
        params: { path: { token } },
      })
    )
  }

  /**
   * Leave a team
   */
  async leaveTeam(teamId: string, userId: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/teams/{id}/members/{userId}', {
        params: { path: { id: teamId, userId } },
      })
    )
  }

  /**
   * Remove a member from a team
   */
  async removeMember(teamId: string, userId: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/teams/{id}/members/{userId}', {
        params: { path: { id: teamId, userId } },
      })
    )
  }

  /**
   * Change a member's role between member and admin (#222).
   *
   * The owner's role is not changeable this way — transferring ownership is a
   * separate operation.
   */
  async updateMemberRole(
    teamId: string,
    userId: string,
    role: ChangeableTeamRole
  ): Promise<void> {
    await unwrap(
      generatedClient.PATCH('/api/v1/teams/{id}/members/{userId}/role', {
        params: { path: { id: teamId, userId } },
        body: { role },
      })
    )
  }

  /**
   * Transfer team ownership to an existing member (#222).
   *
   * The caller is demoted to admin, so callers must refresh the team list
   * afterwards: their own permissions have changed.
   */
  async transferOwnership(teamId: string, newOwnerId: string): Promise<Team> {
    const response = await unwrap(
      generatedClient.POST('/api/v1/teams/{id}/transfer-ownership', {
        params: { path: { id: teamId } },
        body: { new_owner_id: newOwnerId },
      })
    )
    return response.team
  }

  /**
   * Update a team
   */
  async updateTeam(teamId: string, request: UpdateTeamRequest): Promise<Team> {
    return unwrap(
      generatedClient.PUT('/api/v1/teams/{id}', {
        params: { path: { id: teamId } },
        body: request,
      })
    )
  }

  /**
   * Delete a team
   */
  async deleteTeam(teamId: string): Promise<void> {
    await unwrap(
      generatedClient.DELETE('/api/v1/teams/{id}', {
        params: { path: { id: teamId } },
      })
    )
  }

  /**
   * Get team-wide resource counts for the team analytics page. The endpoint
   * returns the bare stats object (no envelope), matching getProjectStats.
   */
  async getTeamStats(teamId: string): Promise<TeamStats> {
    return unwrap(
      generatedClient.GET('/api/v1/teams/{id}/stats', {
        params: { path: { id: teamId } },
      })
    )
  }

  /**
   * Get team-wide daily resource-creation metrics (prompts/artifacts/blueprints/
   * memories/projects).
   */
  async getTeamResourceCreationMetrics(
    teamId: string,
    range = '30d',
    signal?: AbortSignal
  ): Promise<TeamResourceCreationMetricsResponse> {
    return unwrap(
      generatedClient.GET('/api/v1/teams/{id}/resource-creation-metrics', {
        params: {
          path: { id: teamId },
          query: { range: range as MetricsRange },
        },
        signal,
      })
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
    return unwrap(
      generatedClient.GET('/api/v1/teams/{id}/resource-access-metrics', {
        params: {
          path: { id: teamId },
          query: { range: range as MetricsRange },
        },
        signal,
      })
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
    return unwrap(
      generatedClient.GET('/api/v1/teams/{id}/feed-creation-metrics', {
        params: {
          path: { id: teamId },
          query: { range: range as MetricsRange },
        },
        signal,
      })
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
    return unwrap(
      generatedClient.GET('/api/v1/teams/{id}/top-accessed-resources', {
        params: {
          path: { id: teamId },
          query: {
            range: range as MetricsRange,
            limit,
            // `source` is a fixed channel selector; 'all'/undefined mean no filter.
            source:
              source != null && source !== 'all'
                ? (source as 'web' | 'cli' | 'mcp' | 'api')
                : undefined,
          },
        },
        signal,
      })
    )
  }
}

// Export singleton instance
export const teamService = new TeamService()
