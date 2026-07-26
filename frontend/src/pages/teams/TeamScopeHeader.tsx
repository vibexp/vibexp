import {
  ChevronLeft,
  Crown,
  Plus,
  SlidersHorizontal,
  Trash2,
} from 'lucide-react'
import { useCallback, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { usePermissions } from '@/hooks/usePermissions'
import { toast } from '@/lib/toast'
import type { Team, TeamMember } from '@/services/teamService'
import { teamService } from '@/services/teamService'

import { DeleteTeamModal } from './DeleteTeamModal'
import { EditTeamModal } from './EditTeamModal'
import { InviteTeamMembersModal } from './InviteTeamMembersModal'
import { TransferOwnershipModal } from './TransferOwnershipModal'

const formatCreatedAt = (dateString: string) =>
  new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })

/** Single letter for the identity tile; `?` for a name that is only whitespace. */
const initialOf = (name: string) => (name.trim().charAt(0) || '?').toUpperCase()

/**
 * The meta line under the team name.
 *
 * `member_count` is only populated on **list** responses (0 or absent on a
 * single-team read), so the member clause is omitted rather than rendered as
 * "0 members" when the team was resolved by the layout's deep-link fallback.
 */
function metaLine(team: Team): string {
  const parts: string[] = []
  const count = team.member_count ?? 0
  if (count > 0) {
    parts.push(`${String(count)} member${count === 1 ? '' : 's'}`)
  }
  parts.push(`created ${formatCreatedAt(team.created_at)}`)
  return parts.join(' · ')
}

/** `owner` -> `Owner`. Empty on older responses that omit the role (#224). */
function roleLabel(role: string | undefined): string | null {
  if (!role) return null
  return role.charAt(0).toUpperCase() + role.slice(1)
}

/**
 * Page header for the whole `/teams/:id/**` scope (#666).
 *
 * One header block owns the top of the page, in the order the design fixes:
 * escape hatch -> identity -> actions -> (tabs, rendered by the layout right
 * below this). It replaces three competing blocks that used to sit above the
 * team name: the tab bar, a "Back to Teams" button, and a generic `PageHeader`
 * whose description was the same string for every team.
 *
 * It lives in the layout, not in the Overview page, because it is the identity
 * of the *scope* — the team name and its actions stay put while you move
 * between Overview / Projects / Analytics / Settings.
 *
 * Gating is on the server's `permissions` array via `usePermissions(team)`,
 * never on `role` (#224) — and on the team the layout resolved from the URL,
 * not the ambient one (#584).
 */
export function TeamScopeHeader({
  team,
  onTeamChanged,
}: Readonly<{
  team: Team
  /**
   * Something about the team or its roster changed. The layout refreshes the
   * cached team list (permissions, name, member count) and re-runs the active
   * tab's own load.
   */
  onTeamChanged: () => void
}>) {
  const navigate = useNavigate()
  const { can } = usePermissions(team)

  const [showEditModal, setShowEditModal] = useState(false)
  const [showInviteModal, setShowInviteModal] = useState(false)
  const [showDeleteModal, setShowDeleteModal] = useState(false)
  const [showTransferModal, setShowTransferModal] = useState(false)
  const [transferCandidates, setTransferCandidates] = useState<TeamMember[]>([])

  const canUpdateTeam = can('team.update')
  const canInvite = can('member.invite') && !team.is_personal
  const canDeleteTeam = can('team.delete') && !team.is_personal
  const canTransferOwnership = can('team.transfer') && !team.is_personal
  const hasOverflow = canDeleteTeam || canTransferOwnership

  // The roster is only needed to pick a transfer target, so it is fetched when
  // the menu item is used rather than on every team route the header renders on.
  const openTransfer = useCallback(async () => {
    try {
      setTransferCandidates(await teamService.getTeamMembers(team.id))
      setShowTransferModal(true)
    } catch (error) {
      console.error('Failed to load members for ownership transfer', error)
      toast.error('Failed to load team members')
    }
  }, [team.id])

  const handleInviteMembers = async (emails: string[]) => {
    await teamService.inviteMembers(team.id, { emails })
    toast.success(
      `Sent ${String(emails.length)} invitation${emails.length > 1 ? 's' : ''}`
    )
    setShowInviteModal(false)
    onTeamChanged()
  }

  const role = roleLabel(team.role)

  return (
    <header className="pb-6">
      <Button
        asChild
        variant="ghost"
        size="sm"
        className="text-muted-foreground hover:text-foreground -ml-3"
      >
        <Link to="/teams">
          <ChevronLeft className="mr-1 size-4" />
          All teams
        </Link>
      </Button>

      <div className="mt-3 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 items-center gap-4">
          <div
            aria-hidden="true"
            className="bg-primary text-primary-foreground flex size-12 shrink-0 items-center justify-center rounded-md text-lg font-semibold"
          >
            {initialOf(team.name)}
          </div>
          <div className="min-w-0 space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="truncate text-3xl font-bold tracking-tight">
                {team.name}
              </h1>
              {role && (
                <Badge
                  variant="outline"
                  className="text-muted-foreground border-border"
                >
                  {role}
                </Badge>
              )}
            </div>
            <p className="text-muted-foreground text-sm">{metaLine(team)}</p>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-2">
          {canUpdateTeam && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setShowEditModal(true)
              }}
            >
              Edit team
            </Button>
          )}
          {canInvite && (
            <Button
              size="sm"
              onClick={() => {
                setShowInviteModal(true)
              }}
            >
              <Plus className="mr-2 size-4" />
              Invite members
            </Button>
          )}
          {hasOverflow && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="outline"
                  size="icon-sm"
                  aria-label="More team actions"
                  data-testid="team-actions-menu"
                >
                  <SlidersHorizontal className="size-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                {canTransferOwnership && (
                  <DropdownMenuItem
                    data-testid="transfer-ownership-button"
                    onSelect={() => {
                      void openTransfer()
                    }}
                  >
                    <Crown className="size-4" />
                    Transfer ownership
                  </DropdownMenuItem>
                )}
                {canDeleteTeam && (
                  <DropdownMenuItem
                    className="text-destructive focus:text-destructive"
                    data-testid="delete-team-button"
                    onSelect={() => {
                      setShowDeleteModal(true)
                    }}
                  >
                    <Trash2 className="size-4" />
                    Delete team
                  </DropdownMenuItem>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
      </div>

      <InviteTeamMembersModal
        isOpen={showInviteModal}
        teamName={team.name}
        onClose={() => {
          setShowInviteModal(false)
        }}
        onSubmit={handleInviteMembers}
      />

      <TransferOwnershipModal
        isOpen={showTransferModal}
        team={team}
        members={transferCandidates}
        onClose={() => {
          setShowTransferModal(false)
        }}
        onSuccess={() => {
          onTeamChanged()
        }}
      />

      {showEditModal && (
        <EditTeamModal
          isOpen={showEditModal}
          team={team}
          onClose={() => {
            setShowEditModal(false)
          }}
          onSuccess={onTeamChanged}
        />
      )}

      {showDeleteModal && (
        <DeleteTeamModal
          isOpen={showDeleteModal}
          team={team}
          onClose={() => {
            setShowDeleteModal(false)
          }}
          onSuccess={() => {
            void navigate('/teams')
          }}
        />
      )}
    </header>
  )
}
