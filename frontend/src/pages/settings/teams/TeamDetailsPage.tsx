import {
  ArrowLeft,
  BarChart3,
  Calendar,
  Edit,
  Info,
  Trash2,
  UserPlus,
  Users,
} from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { PageHeader } from '@/components/PageHeader'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/lib/toast'
import { teamService } from '@/services/teamService'
import type { Team, TeamInvitation, TeamMember } from '@/types'
import { ApiError } from '@/types/errors'

import { DeleteTeamModal } from './DeleteTeamModal'
import { EditTeamModal } from './EditTeamModal'
import { InviteTeamMembersModal } from './InviteTeamMembersModal'
import { mergeMembersAndInvitations } from './teamMemberMerge'
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

export function TeamDetailsPage() {
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()

  const [team, setTeam] = useState<Team | null>(null)
  const [members, setMembers] = useState<TeamMember[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showInviteModal, setShowInviteModal] = useState(false)
  const [showEditModal, setShowEditModal] = useState(false)
  const [showDeleteModal, setShowDeleteModal] = useState(false)

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
  }, [loadTeamDetails])

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

  const handleInviteMembers = async (emails: string[]) => {
    if (!id) return

    await teamService.inviteMembers(id, { emails })
    toast.success(
      `Sent ${String(emails.length)} invitation${emails.length > 1 ? 's' : ''}`
    )
    setShowInviteModal(false)
    void loadTeamDetails()
  }

  if (isLoading) {
    return <TeamDetailsSkeleton />
  }

  if (error || !team) {
    return (
      <div className="space-y-4">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => {
            void navigate('/settings/teams')
          }}
        >
          <ArrowLeft className="mr-2 size-4" />
          Back to Teams
        </Button>
        <Alert variant="destructive">
          <AlertTitle>Failed to load team</AlertTitle>
          <AlertDescription>
            {error ?? 'Team not found or could not be loaded.'}
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  const isOwner = team.role === 'owner'

  return (
    <div className="space-y-6">
      <Button
        variant="ghost"
        size="sm"
        onClick={() => {
          void navigate('/settings/teams')
        }}
      >
        <ArrowLeft className="mr-2 size-4" />
        Back to Teams
      </Button>

      <PageHeader
        title={team.name}
        description="Team details and member management"
        actions={
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              void navigate(`/settings/teams/${team.id}/analytics`)
            }}
          >
            <BarChart3 className="mr-2 size-4" />
            Analytics
          </Button>
        }
      />

      {team.is_personal ? (
        <Alert>
          <Info className="size-4" />
          <AlertTitle>Private workspace</AlertTitle>
          <AlertDescription>
            Your private workspace for private projects and resources. Private
            workspace doesn&apos;t allow to invite members. You can create a
            separate team to share resources from{' '}
            <a href="/settings/teams" className="underline hover:no-underline">
              here
            </a>
            .
          </AlertDescription>
        </Alert>
      ) : (
        team.description && (
          <Card>
            <CardContent className="flex items-start gap-3 p-4">
              <Info className="text-muted-foreground mt-0.5 size-4 shrink-0" />
              <p className="text-muted-foreground text-sm leading-relaxed">
                {team.description}
              </p>
            </CardContent>
          </Card>
        )
      )}

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-muted-foreground flex items-center gap-2 text-sm font-medium">
              <Users className="size-4" />
              Total members
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-semibold">{members.length}</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-muted-foreground flex items-center gap-2 text-sm font-medium">
              <Calendar className="size-4" />
              Created
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm font-medium">{formatDate(team.created_at)}</p>
          </CardContent>
        </Card>
      </div>

      <section className="space-y-3">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <h2 className="flex items-center gap-2 text-lg font-semibold">
            <Users className="size-5" />
            Team Members
          </h2>

          {isOwner && (
            <div className="flex flex-wrap items-center gap-2">
              {!team.is_personal && (
                <Button
                  variant="outline"
                  size="sm"
                  data-testid="delete-team-button"
                  onClick={() => {
                    setShowDeleteModal(true)
                  }}
                >
                  <Trash2 className="mr-2 size-4" />
                  Delete team
                </Button>
              )}
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setShowEditModal(true)
                }}
              >
                <Edit className="mr-2 size-4" />
                Edit team
              </Button>
              {!team.is_personal && (
                <Button
                  size="sm"
                  onClick={() => {
                    setShowInviteModal(true)
                  }}
                >
                  <UserPlus className="mr-2 size-4" />
                  Invite members
                </Button>
              )}
            </div>
          )}
        </div>

        <TeamMembersList
          members={members}
          isOwner={isOwner}
          onRemoveMember={isOwner ? handleRemoveMember : undefined}
        />
      </section>

      <InviteTeamMembersModal
        isOpen={showInviteModal}
        teamName={team.name}
        onClose={() => {
          setShowInviteModal(false)
        }}
        onSubmit={handleInviteMembers}
      />

      {showEditModal && (
        <EditTeamModal
          isOpen={showEditModal}
          team={team}
          onClose={() => {
            setShowEditModal(false)
          }}
          onSuccess={() => {
            void loadTeamDetails()
          }}
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
            void navigate('/settings/teams')
          }}
        />
      )}
    </div>
  )
}
