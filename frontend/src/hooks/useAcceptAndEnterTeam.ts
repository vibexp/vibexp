import { useCallback } from 'react'
import { useNavigate } from 'react-router-dom'

import { emitInvitationsChanged } from '@/components/invitations/invitationEvents'
import { STORAGE_KEYS } from '@/constants/storageKeys'
import { useTeam } from '@/contexts/TeamContext'
import { toast } from '@/lib/toast'
import { teamService } from '@/services/teamService'
import type { Team } from '@/types/team'
import { mapInvitationError } from '@/utils/invitationErrors'
import { sessionStore } from '@/utils/storage'

/**
 * Outcome of {@link AcceptAndEnterTeamFn}.
 *
 * - `ok: true` — the invitation was accepted, the active team was switched
 *   and the user navigated to the team's details page; a success toast was
 *   shown.
 * - `ok: false` — accept failed; an error toast was shown by the hook itself
 *   and the failure is surfaced through the return value so the caller can
 *   stop a spinner / mark UI as idle.
 */
export type AcceptAndEnterTeamResult =
  | { ok: true; team: Team | null; teamId: string; teamName: string }
  | { ok: false; error: unknown }

export type AcceptAndEnterTeamFn = (
  token: string
) => Promise<AcceptAndEnterTeamResult>

/**
 * Hook that runs the full "accept invitation → enter team" flow:
 *   1. POST /invitations/{token}/accept
 *   2. Refresh the team list and switch the active team to the joined one
 *   3. Navigate to `/settings/teams/:id`
 *   4. Show a success toast
 *
 * Must be used inside `TeamProvider` because it depends on `useTeam`.
 *
 * Errors are surfaced as a typed result *and* as a toast — callers don't
 * need to wire their own error handling unless they want richer recovery.
 */
export function useAcceptAndEnterTeam(): AcceptAndEnterTeamFn {
  const { refreshTeams, setCurrentTeam } = useTeam()
  const navigate = useNavigate()

  return useCallback(
    async (token: string): Promise<AcceptAndEnterTeamResult> => {
      try {
        const response = await teamService.acceptInvitation(token)

        // Always clear the post-auth resume token if it's still around so we
        // don't bounce the user back into accept-loop on the next reload.
        sessionStore.remove(STORAGE_KEYS.PENDING_INVITATION_TOKEN)
        // Also clear the cross-context handshake stash so the in-shell handshake
        // doesn't double-toast / re-switch when this in-context flow already did
        // both. Keeps the two accept paths cleanly disjoint.
        sessionStore.remove(STORAGE_KEYS.INVITATION_JUST_ACCEPTED)

        const refreshed = await refreshTeams()
        let team = refreshed.find(t => t.id === response.team_id) ?? null

        if (!team) {
          // Team-list endpoint may lag behind the join (eventual consistency
          // in tests / cache miss in prod). Fall back to a direct lookup so
          // we still switch to the joined team.
          try {
            team = await teamService.getTeamDetails(response.team_id)
          } catch (lookupErr) {
            console.error('Failed to load just-joined team details:', lookupErr)
          }
        }

        if (team) {
          setCurrentTeam(team)
        }

        emitInvitationsChanged()
        toast.success(`Welcome to ${response.team_name}!`)
        void navigate(`/settings/teams/${response.team_id}`)

        return {
          ok: true,
          team,
          teamId: response.team_id,
          teamName: response.team_name,
        }
      } catch (error) {
        const view = mapInvitationError(error)
        toast.error(view.title, { description: view.description })
        return { ok: false, error }
      }
    },
    [navigate, refreshTeams, setCurrentTeam]
  )
}
