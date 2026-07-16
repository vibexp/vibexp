import type { TeamInvitation, TeamMember } from '@/services/teamService'

/**
 * A roster row: an accepted member, or a pending invitation rendered in the same
 * table. Pending rows carry `invitationToken` (populated by
 * GET /api/v1/teams/{id}/invitations for callers holding `member.invite`) so the
 * roster can offer a copyable accept link; accepted rows leave it undefined.
 */
export type RosterMember = TeamMember & { invitationToken?: string }

/**
 * Canonical email key for de-dupe: trim then lower-case. Matches against
 * potential whitespace asymmetry between `members.email` and
 * `invitations.invitee_email` (the backend does not consistently normalize
 * email at the team-invitation layer).
 */
const canonEmail = (email: string): string => email.trim().toLowerCase()

/**
 * Build a display name from an email address — uses the local part before '@',
 * falling back to the full email if no '@' is present.
 */
const displayNameFromEmail = (email: string): string => {
  const at = email.indexOf('@')
  return at > 0 ? email.slice(0, at) : email
}

/**
 * Convert a pending invitation into a TeamMember-shaped row so it can render
 * in the same table as accepted members. The synthetic `inv:<id>` user_id
 * keeps row keys stable in TanStack Table and lets the Remove action guard
 * itself by checking `invitation_status`.
 */
export const pendingInvitationToMember = (
  invitation: TeamInvitation
): RosterMember => {
  const email = invitation.invitee_email ?? ''
  return {
    user_id: `inv:${invitation.id}`,
    email,
    name: displayNameFromEmail(email),
    role: invitation.role ?? 'member',
    joined_at: invitation.created_at,
    invitation_status: 'pending',
    // Present only when the caller may see it (member.invite); the roster gates
    // the copy action on both this and the permission.
    invitationToken: invitation.token,
  }
}

/**
 * Merge accepted members with pending invitation rows, de-duped by email so an
 * accepted member is never shadowed by a stale pending row for the same person.
 *
 * - Only invitations with `status === 'pending'` are surfaced (revoked, expired,
 *   accepted, rejected ones drop out).
 * - Empty-email pending entries are dropped (defensive — the wire format
 *   should always include `invitee_email`).
 * - De-dupe is case-insensitive on email.
 */
export const mergeMembersAndInvitations = (
  members: TeamMember[],
  invitations: TeamInvitation[]
): RosterMember[] => {
  const acceptedEmails = new Set(
    members.map(m => canonEmail(m.email)).filter(e => e.length > 0)
  )
  const pendingRows = invitations
    .filter(inv => inv.status === 'pending')
    .map(pendingInvitationToMember)
    .filter(row => {
      const key = canonEmail(row.email)
      if (key.length === 0) return false
      return !acceptedEmails.has(key)
    })
  return [...members, ...pendingRows]
}
