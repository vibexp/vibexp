import type { ColumnDef } from '@tanstack/react-table'
import { AlertCircle, Calendar, Mail, Plus, Users } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { DataTable } from '@/components/DataTable'
import { EmptyState } from '@/components/EmptyState'
import { emitInvitationsChanged } from '@/components/invitations/invitationEvents'
import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useAcceptAndEnterTeam } from '@/hooks/useAcceptAndEnterTeam'
import { useErrorHandler } from '@/hooks/useErrorHandler'
import { toast } from '@/lib/toast'
import { teamService } from '@/services/teamService'
import type { Team, TeamInvitation } from '@/types'

import { CreateTeamModal } from './CreateTeamModal'
import { PendingInvitationCard } from './PendingInvitationCard'

const formatDate = (dateString: string) =>
  new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })

const roleVariant = (role: string | undefined) => {
  switch (role) {
    case 'owner':
      return 'default'
    case 'admin':
      return 'secondary'
    default:
      return 'outline'
  }
}

const capitalize = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

function buildColumns(onRowClick: (team: Team) => void): ColumnDef<Team>[] {
  return [
    {
      accessorKey: 'name',
      header: 'Name',
      cell: ({ row }) => (
        <button
          type="button"
          className="flex flex-col text-left"
          onClick={() => {
            onRowClick(row.original)
          }}
        >
          <span className="text-primary font-medium hover:underline">
            {row.original.name}
          </span>
          {row.original.description && (
            <span className="text-muted-foreground line-clamp-1 text-sm">
              {row.original.description}
            </span>
          )}
        </button>
      ),
    },
    {
      accessorKey: 'role',
      header: 'Role',
      cell: ({ row }) => {
        const role = row.original.role ?? 'member'
        return <Badge variant={roleVariant(role)}>{capitalize(role)}</Badge>
      },
    },
    {
      accessorKey: 'member_count',
      header: 'Members',
      cell: ({ row }) => {
        if (row.original.is_personal) {
          return (
            <div className="text-muted-foreground flex items-center gap-2 text-sm">
              <Users className="size-4" />
              <span>-</span>
            </div>
          )
        }
        return (
          <div className="text-muted-foreground flex items-center gap-2 text-sm">
            <Users className="size-4" />
            <span>{row.original.member_count}</span>
          </div>
        )
      },
    },
    {
      accessorKey: 'created_at',
      header: 'Created',
      cell: ({ row }) => (
        <div className="text-muted-foreground flex items-center gap-2 text-sm">
          <Calendar className="size-4" />
          <span>{formatDate(row.original.created_at)}</span>
        </div>
      ),
    },
  ]
}

export function Teams() {
  const navigate = useNavigate()
  const { handleError } = useErrorHandler()
  const acceptAndEnterTeam = useAcceptAndEnterTeam()
  const [teams, setTeams] = useState<Team[]>([])
  const [pendingInvitations, setPendingInvitations] = useState<
    TeamInvitation[]
  >([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showCreateModal, setShowCreateModal] = useState(false)

  const loadData = useCallback(async () => {
    try {
      setIsLoading(true)
      setError(null)
      const [teamsData, invitationsData] = await Promise.all([
        teamService.getTeams(),
        teamService.getPendingInvitations(),
      ])
      setTeams(Array.isArray(teamsData) ? teamsData : [])
      setPendingInvitations(
        Array.isArray(invitationsData) ? invitationsData : []
      )
    } catch (err) {
      const errorMessage =
        err instanceof Error ? err.message : 'Failed to load teams'
      setError(errorMessage)
      handleError(err, 'Failed to load teams')
      setTeams([])
      setPendingInvitations([])
    } finally {
      setIsLoading(false)
    }
  }, [handleError])

  useEffect(() => {
    void loadData()
  }, [loadData])

  const handleRowClick = (team: Team) => {
    void navigate(`/settings/teams/${team.id}`)
  }

  const handleAcceptInvitation = async (invitation: TeamInvitation) => {
    // useAcceptAndEnterTeam handles team-switch + navigation + success toast
    // and surfaces failures via toast — no need to duplicate any of that here.
    const result = await acceptAndEnterTeam(invitation.token)
    if (result.ok) {
      // Drop the now-stale invitation from local state so the UI doesn't
      // show a "Pending Invitations" entry the user can no longer act on,
      // and let other surfaces (the dashboard banner) refresh.
      setPendingInvitations(prev =>
        prev.filter(inv => inv.id !== invitation.id)
      )
      emitInvitationsChanged()
    }
  }

  const handleRejectInvitation = async (invitation: TeamInvitation) => {
    try {
      await teamService.rejectInvitation(invitation.token)
      toast.info(`Invitation to ${invitation.team_name} has been declined.`)
      setPendingInvitations(prev =>
        prev.filter(inv => inv.id !== invitation.id)
      )
      emitInvitationsChanged()
    } catch (err) {
      handleError(err, 'Failed to reject invitation')
    }
  }

  const columns = buildColumns(handleRowClick)

  return (
    <div className="space-y-6">
      <PageHeader
        title="Teams"
        description="Manage your team memberships and collaborate with others."
        actions={
          <Button
            data-testid="create-team-button"
            onClick={() => {
              setShowCreateModal(true)
            }}
          >
            <Plus className="mr-2 size-4" />
            Create team
          </Button>
        }
      />

      {error && (
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {isLoading ? (
        <Card>
          <CardContent className="space-y-3 p-6">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </CardContent>
        </Card>
      ) : (
        <>
          {pendingInvitations.length > 0 && (
            <section className="space-y-3">
              <div className="flex items-center gap-2">
                <Mail className="size-5" />
                <h2 className="text-lg font-semibold">Pending Invitations</h2>
                <Badge variant="secondary">{pendingInvitations.length}</Badge>
              </div>
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
                {pendingInvitations.map(invitation => (
                  <PendingInvitationCard
                    key={invitation.id}
                    invitation={invitation}
                    onAccept={handleAcceptInvitation}
                    onReject={handleRejectInvitation}
                  />
                ))}
              </div>
            </section>
          )}

          <section className="space-y-3">
            <div className="flex items-center gap-2">
              <Users className="size-5" />
              <h2 className="text-lg font-semibold">Your Teams</h2>
              {teams.length > 0 && (
                <Badge variant="secondary">{teams.length}</Badge>
              )}
            </div>

            {teams.length === 0 ? (
              <EmptyState
                icon={Users}
                title="No teams yet"
                description="Create your first team or wait for an invitation to join an existing team."
                actions={
                  <Button
                    onClick={() => {
                      setShowCreateModal(true)
                    }}
                  >
                    <Plus className="mr-2 size-4" />
                    Create team
                  </Button>
                }
              />
            ) : (
              <Card>
                <CardContent className="p-4">
                  <DataTable columns={columns} data={teams} />
                </CardContent>
              </Card>
            )}
          </section>
        </>
      )}

      <CreateTeamModal
        isOpen={showCreateModal}
        onClose={() => {
          setShowCreateModal(false)
        }}
        onSuccess={() => {
          void loadData()
        }}
      />
    </div>
  )
}
