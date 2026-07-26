import { Info } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useTeam } from '@/contexts/TeamContext'
import { useAuth } from '@/contexts/useAuth'
import { usePermissions } from '@/hooks/usePermissions'
import { toast } from '@/lib/toast'
import type {
  ChangeableTeamRole,
  Team,
  TeamInvitation,
} from '@/services/teamService'
import { teamService } from '@/services/teamService'
import { ApiError } from '@/types/errors'

import {
  mergeMembersAndInvitations,
  type RosterMember,
} from './teamMemberMerge'
import { TeamMembersList } from './TeamMembersList'

const formatDate = (dateString: string) =>
  new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })

function TeamDetailsSkeleton() {
  return (
    <div className="space-y-6">
      <Skeleton className="h-8 w-32" />
      <Skeleton className="h-12 w-2/3" />
      <Skeleton className="h-24 w-full" />
      <Skeleton className="h-64 w-full" />
    </div>
  )
}

/**
 * The Overview tab of a team (`/teams/:id`).
 *
 * Since #666 this is content only: page identity, the team-level actions and
 * the modals that back them belong to `TeamScopeHeader`, which the scope layout
 * renders above the tab bar on every team route. What is left here is what the
 * Overview tab is actually about — the team's description, its counters, and
 * the member roster.
 */
export function TeamDetailsPage({
  reloadToken = 0,
}: Readonly<{
  /**
   * Bumped by `TeamScopeLayout` when a header action changed the team or its
   * roster (an invite sent, ownership transferred, the team renamed), so this
   * tab refetches rather than showing what the header just superseded.
   */
  reloadToken?: number
}> = {}) {
  const { id } = useParams<{ id: string }>()
  const { refreshTeams } = useTeam()
  const { user } = useAuth()

  const [team, setTeam] = useState<Team | null>(null)
  const [members, setMembers] = useState<RosterMember[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // This page can show any team the user belongs to, not just the current one,
  // so gate on the team it fetched. While loading (`team` is null) nothing is
  // permitted, which is the safe default for a page full of destructive actions.
  const { can } = usePermissions(team)

  const loadTeamDetails = useCallback(async () => {
    if (!id) return

    try {
      setIsLoading(true)
      setError(null)

      // Only swallow 403 from /teams/{id}/invitations (non-owners legitimately
      // can't see invitations) so the page still renders for them. Server/
      // network failures are surfaced to a toast and logged; the page is still
      // rendered without pending rows so members/details remain visible.
      const [teamData, membersData, invitationsData] = await Promise.all([
        teamService.getTeamDetails(id),
        teamService.getTeamMembers(id),
        teamService
          .getTeamInvitations(id)
          .catch((err: unknown): TeamInvitation[] => {
            if (err instanceof ApiError && err.status === 403) return []
            console.error('Failed to load team invitations', err)
            toast.error('Failed to load pending invitations')
            return []
          }),
      ])

      setTeam(teamData)
      setMembers(mergeMembersAndInvitations(membersData, invitationsData))
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to load team details'
      )
    } finally {
      setIsLoading(false)
    }
  }, [id])

  useEffect(() => {
    void loadTeamDetails()
    // `reloadToken` is a signal, not data: it is in the dependency list purely
    // so a header action (invite, transfer, rename) refetches this tab.
  }, [loadTeamDetails, reloadToken])

  const handleRemoveMember = async (userId: string) => {
    if (!id) return

    try {
      await teamService.removeMember(id, userId)
      toast.success('Member removed successfully')
      void loadTeamDetails()
    } catch (err) {
      const errorMessage =
        err instanceof Error ? err.message : 'Failed to remove member'
      toast.error(errorMessage)
      throw err
    }
  }

  const handleChangeRole = async (userId: string, role: ChangeableTeamRole) => {
    if (!id) return

    // Optimistic: the dropdown should settle immediately. Snapshot first so a
    // rejected change (e.g. the caller lost the permission meanwhile) puts the
    // row back rather than leaving the UI asserting a role the server refused.
    const previousMembers = members
    setMembers(current =>
      current.map(member =>
        member.user_id === userId ? { ...member, role } : member
      )
    )

    try {
      await teamService.updateMemberRole(id, userId, role)
      toast.success(`Role updated to ${role}`)

      // Nothing else on the page depends on another member's role, so the
      // optimistic row above is the whole update — refetching here would
      // replace the page with a loading skeleton and undo the point of it.
      //
      // Demoting YOURSELF is different: the backend only protects the owner's
      // role (TeamService.UpdateMemberRole), so an admin may hand away their
      // own permissions. Resync both this page's gates and the cached team
      // list, or the rest of the SPA keeps offering admin actions that now 403.
      if (userId === user?.id) {
        await loadTeamDetails()
        await refreshTeams()
      }
    } catch (err) {
      setMembers(previousMembers)
      const errorMessage =
        err instanceof Error ? err.message : 'Failed to update role'
      toast.error(errorMessage)
    }
  }

  if (isLoading) {
    return <TeamDetailsSkeleton />
  }

  if (error || !team) {
    // No back link here any more: the scope header above renders one on every
    // team route, and two escape hatches was the confusion #666 removed.
    return (
      <div className="space-y-4">
        <Alert variant="destructive">
          <AlertTitle>Failed to load team</AlertTitle>
          <AlertDescription>
            {error ?? 'Team not found or could not be loaded.'}
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  // Gate on the permissions the server computed for THIS team (which may not be
  // the current team), never on `role` — the matrix lives on the backend (#224).
  const canInvite = can('member.invite')
  const canRemoveMember = can('member.remove')
  const canManageRoles = can('member.role.update')

  const acceptedMemberCount = members.filter(
    member => member.invitation_status !== 'pending'
  ).length

  return (
    <div className="space-y-6">
      {team.is_personal ? (
        <Alert>
          <Info className="size-4" />
          <AlertTitle>Private workspace</AlertTitle>
          <AlertDescription>
            Your private workspace for private projects and resources. Private
            workspace doesn&apos;t allow to invite members. You can create a
            separate team to share resources from{' '}
            <a href="/teams" className="underline hover:no-underline">
              here
            </a>
            {'.'}
          </AlertDescription>
        </Alert>
      ) : (
        team.description && (
          <Card className="bg-transparent shadow-none">
            <CardContent className="text-muted-foreground flex items-center gap-3 p-4 text-sm">
              <Info className="size-4 shrink-0" />
              <p className="leading-relaxed">{team.description}</p>
            </CardContent>
          </Card>
        )
      )}

      {/* Unfilled, shadowless panels: the design puts every content block on
          the page background with a single hairline border, so a `bg-card`
          fill would read as a raised surface it never intends (and in dark
          mode `--card` is visibly lighter than `--background`). */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <Card className="bg-transparent shadow-none">
          <CardContent className="p-4">
            <p className="text-muted-foreground text-sm">Total members</p>
            {/* Accepted memberships only, which is what the server's
                `member_count` counts and therefore what the scope header's meta
                line says. Counting the whole roster here instead made the two
                disagree on screen (header "1 member", card "2") whenever an
                invitation was still pending — the pending rows are in the table
                below, badged as such. */}
            <p className="mt-2 text-2xl font-bold">{acceptedMemberCount}</p>
          </CardContent>
        </Card>
        <Card className="bg-transparent shadow-none">
          <CardContent className="p-4">
            <p className="text-muted-foreground text-sm">Created</p>
            <p className="mt-2 text-base font-semibold">
              {formatDate(team.created_at)}
            </p>
          </CardContent>
        </Card>
      </div>

      {/* The roster's title sits on the table's own toolbar row rather than
          floating above it, and the team-level actions that used to sit beside
          it now live in the scope header (#666). */}
      <TeamMembersList
        title="Team members"
        members={members}
        canManageRoles={canManageRoles}
        canRemoveMember={canRemoveMember}
        canCopyInviteLink={canInvite}
        onRemoveMember={canRemoveMember ? handleRemoveMember : undefined}
        onChangeRole={canManageRoles ? handleChangeRole : undefined}
      />
    </div>
  )
}
