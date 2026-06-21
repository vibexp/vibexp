/**
 * Team and invitation types for the VibeXP frontend
 */

/**
 * Status of a team invitation
 */
export type InvitationStatus = 'pending' | 'accepted' | 'rejected' | 'expired'

/**
 * Team member role
 */
export type TeamRole = 'owner' | 'admin' | 'member'

/**
 * Team member details
 */
export interface TeamMember {
  user_id: string
  email: string
  name: string
  role: TeamRole
  joined_at: string
  invitation_status?: 'pending' | 'accepted'
}

/**
 * Team details
 */
export interface Team {
  id: string
  name: string
  slug: string
  description: string
  // role may be absent from API responses in some contexts (e.g. when fetching
  // team details without membership context). Components must guard against
  // undefined (VIBEXP-FRONTEND-JS-5).
  role?: TeamRole
  member_count: number
  is_personal: boolean
  created_at: string
  updated_at: string
}

/**
 * Team with full member list
 */
export interface TeamWithMembers extends Team {
  members: TeamMember[]
}

/**
 * Team invitation details.
 *
 * The backend wire format uses `invitee_email` (see `models.InvitationResponse`).
 * `email` is a legacy alias kept for older consumers; new code should read
 * `invitee_email`. Both are optional on the type because different endpoints
 * historically populated different fields — readers must guard against either
 * being undefined.
 */
export interface TeamInvitation {
  id: string
  token: string
  team_id: string
  team_name: string
  /** Backend wire format field (canonical). */
  invitee_email?: string
  /** Legacy alias kept for existing consumers. */
  email?: string
  role?: TeamRole
  status: InvitationStatus
  created_at: string
  expires_at: string
  invited_by?: {
    id: string
    name: string
    email: string
  }
}

/**
 * Request to create a new team
 */
export interface CreateTeamRequest {
  name: string
  description?: string
}

/**
 * Request to update a team
 */
export interface UpdateTeamRequest {
  name?: string
  description?: string
}

/**
 * Request to invite team members
 */
export interface InviteTeamMembersRequest {
  emails: string[]
  role?: 'member' | 'admin'
}

/**
 * Response for team list
 */
export interface TeamsResponse {
  teams: Team[]
  total_count: number
  page: number
  page_size: number
}

/**
 * Response for team members
 */
export interface TeamMembersResponse {
  // members may be absent from the API response when the team has no members
  // or when the field is omitted by the server (VIBEXP-FRONTEND-JS-6).
  members?: TeamMember[]
  total_count: number
  page: number
  page_size: number
}

/**
 * Response for invite members
 */
export interface InviteTeamMembersResponse {
  invitations: {
    email: string
    invitation_id: string
    status: string
  }[]
  success_count: number
  error_count: number
}

/**
 * Response for a single invitation
 */
export interface InvitationResponse {
  invitation: TeamInvitation
}

/**
 * Response for pending invitations list
 */
export interface PendingInvitationsResponse {
  // invitations may be absent from the API response when the server omits the
  // field for users with no invitations (VIBEXP-FRONTEND-JS-4).
  invitations?: TeamInvitation[]
  total_count: number
  page: number
  page_size: number
}

/**
 * Accept invitation response
 */
export interface AcceptInvitationResponse {
  team_id: string
  team_name: string
  message: string
}

/**
 * Team-wide resource counts for the team analytics page. Mirrors the backend
 * TeamStatsResponse — like ProjectStats but team-scoped and with a projects count.
 */
export interface TeamStats {
  total_projects: number
  total_prompts: number
  total_artifacts: number
  total_blueprints: number
  total_memories: number
  total_feed_items: number
}
