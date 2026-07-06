import { Mail, X } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { STORAGE_KEYS } from '@/constants/storageKeys'
import { useAcceptAndEnterTeam } from '@/hooks/useAcceptAndEnterTeam'
import type { TeamInvitation } from '@/services/teamService'
import { teamService } from '@/services/teamService'
import { sessionStore } from '@/utils/storage'

import {
  emitInvitationsChanged,
  onInvitationsChanged,
} from './invitationEvents'

/**
 * Read the dismissed-invitation-id list. Falls back to an empty array on
 * any read/parse failure so a stale localStorage entry can never break the
 * banner.
 */
function readDismissedIds(): string[] {
  const ids = sessionStore.getJSON<string[]>(
    STORAGE_KEYS.INVITATION_BANNER_DISMISSED
  )
  return Array.isArray(ids) ? ids : []
}

function persistDismissedIds(ids: string[]): void {
  sessionStore.set(STORAGE_KEYS.INVITATION_BANNER_DISMISSED, ids)
}

/**
 * Pending-invitations dashboard banner.
 *
 * Mounted once inside the v2 `Layout` so it sits above page content and
 * survives client-side navigations without flicker. Renders nothing when
 * the user has no actionable pending invitations or when every pending
 * invitation has been dismissed.
 */
export function PendingInvitationsBanner() {
  const navigate = useNavigate()
  const acceptAndEnterTeam = useAcceptAndEnterTeam()

  const [invitations, setInvitations] = useState<TeamInvitation[]>([])
  const [dismissedIds, setDismissedIds] = useState<string[]>(() =>
    readDismissedIds()
  )
  const [acceptingId, setAcceptingId] = useState<string | null>(null)

  const fetchInvitations = useCallback(async () => {
    try {
      const list = await teamService.getPendingInvitations()
      setInvitations(Array.isArray(list) ? list : [])
    } catch (error) {
      // Banner is non-critical; a fetch failure should never break the page.
      console.error('Failed to load pending invitations:', error)
      setInvitations([])
    }
  }, [])

  useEffect(() => {
    void fetchInvitations()
    const unsubscribe = onInvitationsChanged(() => {
      void fetchInvitations()
    })
    return unsubscribe
  }, [fetchInvitations])

  const visible = useMemo(
    () => invitations.filter(inv => !dismissedIds.includes(inv.id)),
    [invitations, dismissedIds]
  )

  const dismissOne = (id: string) => {
    setDismissedIds(prev => {
      if (prev.includes(id)) return prev
      const next = [...prev, id]
      persistDismissedIds(next)
      return next
    })
  }

  const dismissAll = () => {
    setDismissedIds(prev => {
      const next = Array.from(new Set([...prev, ...visible.map(v => v.id)]))
      persistDismissedIds(next)
      return next
    })
  }

  const onAccept = async (invitation: TeamInvitation) => {
    setAcceptingId(invitation.id)
    try {
      const result = await acceptAndEnterTeam(invitation.token)
      if (result.ok) {
        // Drop the now-stale invitation from local state and emit so any
        // peer banner / Teams page refreshes.
        setInvitations(prev => prev.filter(inv => inv.id !== invitation.id))
        emitInvitationsChanged()
      }
    } finally {
      setAcceptingId(prev => (prev === invitation.id ? null : prev))
    }
  }

  if (visible.length === 0) return null

  return (
    <div className="mb-4">
      {visible.length === 1 ? (
        <SingleInvitationBanner
          invitation={visible[0]}
          accepting={acceptingId === visible[0].id}
          onAccept={() => {
            void onAccept(visible[0])
          }}
          onDismiss={() => {
            dismissOne(visible[0].id)
          }}
        />
      ) : (
        <MultipleInvitationsBanner
          count={visible.length}
          onReview={() => {
            void navigate('/settings/teams')
          }}
          onDismiss={dismissAll}
        />
      )}
    </div>
  )
}

interface SingleInvitationBannerProps {
  invitation: TeamInvitation
  accepting: boolean
  onAccept: () => void
  onDismiss: () => void
}

function SingleInvitationBanner({
  invitation,
  accepting,
  onAccept,
  onDismiss,
}: SingleInvitationBannerProps) {
  const inviterName = invitation.invited_by?.name ?? 'A teammate'

  return (
    <Card>
      <CardContent className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-start gap-3">
          <Mail
            aria-hidden="true"
            className="text-primary mt-0.5 size-5 shrink-0"
          />
          <p className="text-sm">
            <span className="font-medium">{inviterName}</span>
            {' invited you to '}
            <span className="font-medium">{invitation.team_name}</span>.
          </p>
        </div>
        <div className="flex items-center gap-2 self-end sm:self-auto">
          <Button size="sm" disabled={accepting} onClick={onAccept}>
            {accepting ? 'Joining…' : 'Accept'}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            aria-label="Dismiss invitation"
            disabled={accepting}
            onClick={onDismiss}
          >
            <X className="size-4" />
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

interface MultipleInvitationsBannerProps {
  count: number
  onReview: () => void
  onDismiss: () => void
}

function MultipleInvitationsBanner({
  count,
  onReview,
  onDismiss,
}: MultipleInvitationsBannerProps) {
  return (
    <Card>
      <CardContent className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-start gap-3">
          <Mail
            aria-hidden="true"
            className="text-primary mt-0.5 size-5 shrink-0"
          />
          <p className="text-sm">
            You have{' '}
            <span className="font-medium">
              {count} pending team invitations
            </span>
            .
          </p>
        </div>
        <div className="flex items-center gap-2 self-end sm:self-auto">
          <Button size="sm" onClick={onReview}>
            Review
          </Button>
          <Button
            size="sm"
            variant="ghost"
            aria-label="Dismiss all invitations"
            onClick={onDismiss}
          >
            Dismiss all
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
