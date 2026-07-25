import { Calendar, Check, Mail, User, X } from 'lucide-react'
import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { PendingTeamInvitation } from '@/services/teamService'

interface PendingInvitationCardProps {
  invitation: PendingTeamInvitation
  onAccept: (invitation: PendingTeamInvitation) => Promise<void>
  onReject: (invitation: PendingTeamInvitation) => Promise<void>
}

const formatDate = (dateString: string) =>
  new Date(dateString).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })

export function PendingInvitationCard({
  invitation,
  onAccept,
  onReject,
}: Readonly<PendingInvitationCardProps>) {
  const [isAccepting, setIsAccepting] = useState(false)
  const [isRejecting, setIsRejecting] = useState(false)

  const handleAccept = async () => {
    setIsAccepting(true)
    try {
      await onAccept(invitation)
    } finally {
      setIsAccepting(false)
    }
  }

  const handleReject = async () => {
    setIsRejecting(true)
    try {
      await onReject(invitation)
    } finally {
      setIsRejecting(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Mail className="size-4" />
          {invitation.team_name}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="text-muted-foreground space-y-1.5 text-sm">
          {invitation.invited_by && (
            <div className="flex items-center gap-2">
              <User className="size-4" />
              <span>
                Invited by {invitation.invited_by.name} (
                {invitation.invited_by.email})
              </span>
            </div>
          )}
          <div className="flex items-center gap-2">
            <Calendar className="size-4" />
            <span>Received on {formatDate(invitation.created_at)}</span>
          </div>
          <div className="flex items-center gap-2">
            <Calendar className="size-4" />
            <span>Expires on {formatDate(invitation.expires_at)}</span>
          </div>
        </div>

        <div className="flex gap-2">
          <Button
            size="sm"
            onClick={() => {
              void handleAccept()
            }}
            disabled={isAccepting || isRejecting}
            className="flex-1"
          >
            <Check className="mr-2 size-4" />
            {isAccepting ? 'Accepting…' : 'Accept'}
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              void handleReject()
            }}
            disabled={isAccepting || isRejecting}
            className="flex-1"
          >
            <X className="mr-2 size-4" />
            {isRejecting ? 'Declining…' : 'Decline'}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
