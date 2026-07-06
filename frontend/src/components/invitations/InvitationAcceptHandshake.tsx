import { useEffect, useRef } from 'react'

import { STORAGE_KEYS } from '@/constants/storageKeys'
import { useTeam } from '@/contexts/TeamContext'
import { toast } from '@/lib/toast'
import type { Team } from '@/services/teamService'
import { teamService } from '@/services/teamService'
import { sessionStore } from '@/utils/storage'

import { emitInvitationsChanged } from './invitationEvents'

interface JustAcceptedStash {
  team_id: string
  team_name: string
}

function readStash(): JustAcceptedStash | null {
  const value = sessionStore.getJSON<JustAcceptedStash>(
    STORAGE_KEYS.INVITATION_JUST_ACCEPTED
  )
  if (!value || typeof value !== 'object') return null
  if (
    typeof value.team_id !== 'string' ||
    typeof value.team_name !== 'string'
  ) {
    return null
  }
  return value
}

/**
 * Receives the cross-context handoff from `AcceptInvitation` (which lives in
 * `BareLayout`, outside `TeamProvider`) and finishes the "enter team" flow:
 *   - looks up the joined team in the current list (or refreshes it),
 *   - calls `setCurrentTeam`,
 *   - shows the welcome toast,
 *   - emits an invitations-changed event so the banner refetches.
 *
 * Renders nothing.
 */
export function InvitationAcceptHandshake() {
  const { teams, refreshTeams, setCurrentTeam } = useTeam()
  // Guard against double-firing under React 18 StrictMode (effects run twice
  // in dev) and against re-runs when `teams` reference changes.
  const processedRef = useRef(false)

  useEffect(() => {
    if (processedRef.current) return
    const stash = readStash()
    if (!stash) return
    processedRef.current = true

    sessionStore.remove(STORAGE_KEYS.INVITATION_JUST_ACCEPTED)

    const finish = async () => {
      let team: Team | null = teams.find(t => t.id === stash.team_id) ?? null

      if (!team) {
        try {
          const refreshed = await refreshTeams()
          team = refreshed.find(t => t.id === stash.team_id) ?? null
        } catch (refreshErr) {
          console.error(
            'Failed to refresh teams after invitation accept:',
            refreshErr
          )
        }
      }

      if (!team) {
        try {
          team = await teamService.getTeamDetails(stash.team_id)
        } catch (lookupErr) {
          console.error('Failed to load just-joined team details:', lookupErr)
        }
      }

      if (team) {
        setCurrentTeam(team)
      }

      toast.success(`Welcome to ${stash.team_name}!`)
      emitInvitationsChanged()
    }

    void finish()
    // No cleanup-cancellation: under React 18 StrictMode dev the cleanup fires
    // before the second invocation, which would silence the toast. The stash
    // is single-shot (cleared above), processedRef prevents the second mount
    // from re-entering, and the worst case on mid-flight unmount is a stale
    // setState warning — never a duplicate accept or duplicate toast.
  }, [teams, refreshTeams, setCurrentTeam])

  return null
}
