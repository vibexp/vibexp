import { AlertCircle, Check, Loader2, UserPlus, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { STORAGE_KEYS } from '@/constants/storageKeys'
import { useAuth } from '@/contexts/useAuth'
import { teamService } from '@/services/teamService'
import type { TeamInvitation } from '@/types/team'
import {
  GENERIC_ACCEPT_ERROR,
  INVALID_LINK_ERROR,
  type InvitationErrorView,
  mapInvitationError,
} from '@/utils/invitationErrors'
import { sessionStore } from '@/utils/storage'

export function AcceptInvitation() {
  const { token } = useParams<{ token: string }>()
  const {
    isAuthenticated,
    isLoading: authLoading,
    checkPendingInvitation,
  } = useAuth()
  const navigate = useNavigate()

  const [invitation, setInvitation] = useState<TeamInvitation | null>(null)
  const [loading, setLoading] = useState(true)
  const [errorView, setErrorView] = useState<InvitationErrorView | null>(null)
  const [processing, setProcessing] = useState(false)

  useEffect(() => {
    const fetchInvitation = async () => {
      const pendingToken = checkPendingInvitation()
      const invitationToken = token ?? pendingToken

      if (!invitationToken) {
        setErrorView(INVALID_LINK_ERROR)
        setLoading(false)
        return
      }

      // If not authenticated, stash token and send them to sign in.
      if (!authLoading && !isAuthenticated) {
        sessionStore.set(STORAGE_KEYS.PENDING_INVITATION_TOKEN, invitationToken)
        void navigate('/')
        return
      }

      if (isAuthenticated) {
        try {
          setLoading(true)
          const response =
            await teamService.getInvitationByToken(invitationToken)
          setInvitation(response.invitation)
        } catch (err) {
          console.error('Failed to fetch invitation:', err)
          setErrorView(mapInvitationError(err))
        } finally {
          setLoading(false)
        }
      }
    }

    void fetchInvitation()
  }, [token, isAuthenticated, authLoading, navigate, checkPendingInvitation])

  const handleAccept = async () => {
    if (!invitation) return
    try {
      setProcessing(true)
      const response = await teamService.acceptInvitation(invitation.token)
      sessionStore.remove(STORAGE_KEYS.PENDING_INVITATION_TOKEN)
      // Hand off to the in-app handshake (mounted inside TeamProvider)
      // which will switch the active team and show the welcome toast.
      sessionStore.set(STORAGE_KEYS.INVITATION_JUST_ACCEPTED, {
        team_id: response.team_id,
        team_name: response.team_name,
      })
      void navigate(`/settings/teams/${response.team_id}`)
    } catch (err) {
      console.error('Failed to accept invitation:', err)
      const view = mapInvitationError(err)
      // For "ok-load, fail-accept" cases keep the previous wording for
      // generic 5xx so existing tests / muscle memory don't shift.
      setErrorView(
        view.title === "Couldn't load invitation" ? GENERIC_ACCEPT_ERROR : view
      )
    } finally {
      setProcessing(false)
    }
  }

  const handleReject = async () => {
    if (!invitation) return
    try {
      setProcessing(true)
      await teamService.rejectInvitation(invitation.token)
      sessionStore.remove(STORAGE_KEYS.PENDING_INVITATION_TOKEN)
      void navigate('/', { state: { message: 'Invitation rejected' } })
    } catch (err) {
      console.error('Failed to reject invitation:', err)
      setErrorView({
        title: "Couldn't reject invitation",
        description: 'Failed to reject invitation. Please try again.',
      })
    } finally {
      setProcessing(false)
    }
  }

  if (loading || authLoading) {
    return (
      <div className="bg-background flex min-h-screen items-center justify-center p-4">
        <div className="flex flex-col items-center gap-3 text-center">
          <Loader2 className="text-primary size-10 animate-spin" />
          <p className="text-muted-foreground text-sm">Loading invitation…</p>
        </div>
      </div>
    )
  }

  if (errorView) {
    return (
      <div className="bg-background flex min-h-screen items-center justify-center p-4">
        <Card className="w-full max-w-md">
          <CardContent className="flex flex-col gap-4 p-6">
            <Alert variant="destructive">
              <AlertCircle className="size-4" />
              <AlertTitle>{errorView.title}</AlertTitle>
              <AlertDescription>{errorView.description}</AlertDescription>
            </Alert>
            <Button
              onClick={() => {
                void navigate('/')
              }}
            >
              Go to dashboard
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  if (!invitation) {
    return null
  }

  return (
    <div className="bg-background flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-lg">
        <CardHeader className="text-center">
          <div className="bg-primary text-primary-foreground mx-auto mb-2 flex size-12 items-center justify-center rounded-full">
            <UserPlus className="size-6" />
          </div>
          <CardTitle>Team invitation</CardTitle>
          <CardDescription>
            You&apos;ve been invited to join a team on VibeXP.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="bg-muted/40 space-y-3 rounded-lg border p-4">
            <InfoRow label="Team">
              <span className="font-medium">{invitation.team_name}</span>
            </InfoRow>
            {invitation.invited_by && (
              <>
                <Separator />
                <InfoRow label="Invited by">
                  <div>
                    <div className="font-medium">
                      {invitation.invited_by.name}
                    </div>
                    <div className="text-muted-foreground text-xs">
                      {invitation.invited_by.email}
                    </div>
                  </div>
                </InfoRow>
              </>
            )}
            <Separator />
            <InfoRow label="Invited email">
              {invitation.invitee_email ?? invitation.email}
            </InfoRow>
            <Separator />
            <InfoRow label="Expires">
              {new Date(invitation.expires_at).toLocaleString('en-US', {
                year: 'numeric',
                month: 'long',
                day: 'numeric',
                hour: '2-digit',
                minute: '2-digit',
              })}
            </InfoRow>
          </div>

          <div className="flex gap-2">
            <Button
              className="flex-1"
              disabled={processing}
              onClick={() => {
                void handleAccept()
              }}
            >
              <Check className="mr-2 size-4" />
              {processing ? 'Working…' : 'Accept invitation'}
            </Button>
            <Button
              variant="outline"
              className="flex-1"
              disabled={processing}
              onClick={() => {
                void handleReject()
              }}
            >
              <X className="mr-2 size-4" />
              {processing ? 'Working…' : 'Reject'}
            </Button>
          </div>

          <p className="text-muted-foreground text-center text-xs">
            By accepting, you&apos;ll be added to the team and gain access to
            shared resources.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}

function InfoRow({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="flex items-start justify-between gap-4 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <div className="text-right">{children}</div>
    </div>
  )
}
