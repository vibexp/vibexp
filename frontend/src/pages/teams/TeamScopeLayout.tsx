import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { useTeam } from '@/contexts/TeamContext'
import { TeamRoutes } from '@/pages/teams/TeamRoutes'
import { TeamScopeHeader } from '@/pages/teams/TeamScopeHeader'
import { TeamTabs } from '@/pages/teams/TeamTabs'
import type { Team } from '@/services/teamService'
import { teamService } from '@/services/teamService'
import { ApiError } from '@/types/errors'

type FallbackState = 'idle' | 'loading' | 'unavailable'

/**
 * Layout owning the `:id` in `/teams/:id/**` (#539).
 *
 * Responsibilities, in order:
 *  1. Resolve the team named by the URL — **not** the ambient current team.
 *  2. Fail closed if the user is not a member, or the id does not exist.
 *  3. Sync `TeamContext.currentTeam` to the URL's team, so the rest of the app
 *     (header switcher, project scope) agrees with the address bar.
 *  4. Render the team chrome (page header + tab bar) around the nested routes.
 *
 * Mirrors the `AdminShell` + `AdminRoutes` pairing rather than using
 * `<Outlet>`, which appears nowhere in this codebase.
 *
 * The chrome is one header block — escape hatch, identity, actions, tabs — with
 * the routes below it, so the tab bar reads as the seam between page chrome and
 * page content (#666).
 */
export function TeamScopeLayout() {
  const { id } = useParams<{ id: string }>()
  const { teams, currentTeam, setCurrentTeam, isLoading, refreshTeams } =
    useTeam()

  // Bumped whenever a header action changes the team or its roster, so the
  // active tab reloads its own data. The header owns the modals (it renders on
  // every team route); the tabs own their data — this is the seam between them.
  const [reloadToken, setReloadToken] = useState(0)
  const handleTeamChanged = useCallback(() => {
    setReloadToken(token => token + 1)
    void refreshTeams()
  }, [refreshTeams])

  const [fallbackTeam, setFallbackTeam] = useState<Team | null>(null)
  const [fallbackState, setFallbackState] = useState<FallbackState>('idle')

  // The team list only ever contains teams the user belongs to, so finding the
  // id here IS the membership check — and it costs no request.
  const teamFromList = useMemo(
    () => teams.find(team => team.id === id) ?? null,
    [teams, id]
  )

  const team = teamFromList ?? fallbackTeam

  // A stale list (team joined in another tab) must not read as "forbidden", so
  // fall back to fetching once the list has settled without a match. A 403/404
  // from here is authoritative: not a member, or no such team.
  const requestSeq = useRef(0)
  useEffect(() => {
    if (isLoading || !id || teamFromList) return

    const seq = ++requestSeq.current
    setFallbackState('loading')

    teamService
      .getTeamDetails(id)
      .then(fetched => {
        if (seq !== requestSeq.current) return
        setFallbackTeam(fetched)
        setFallbackState('idle')
      })
      .catch((error: unknown) => {
        if (seq !== requestSeq.current) return
        // Anything else (network, 5xx) is also unrenderable for this scope;
        // log it so a genuine outage isn't reported to the user as "no access".
        if (!(error instanceof ApiError && [403, 404].includes(error.status))) {
          console.error('Failed to resolve team for /teams/:id', error)
        }
        setFallbackTeam(null)
        setFallbackState('unavailable')
      })

    return () => {
      // Invalidate in-flight work for a previous id.
      requestSeq.current++
    }
  }, [id, isLoading, teamFromList])

  // Keep the ambient team pointing at the URL's team (epic #536 decision 3:
  // this persists, so leaving the subtree leaves you in that team). Comparing
  // ids first is what stops the effect re-running on the context change it
  // just caused.
  useEffect(() => {
    if (team && currentTeam?.id !== team.id) {
      setCurrentTeam(team)
    }
  }, [team, currentTeam?.id, setCurrentTeam])

  // Never 404 while the team list is still hydrating — a premature not-found
  // flash is the most likely bug in this component.
  if (isLoading || fallbackState === 'loading') {
    return (
      <div className="space-y-6" data-testid="team-scope-loading">
        <Skeleton className="h-9 w-64" />
        <Skeleton className="h-8 w-full max-w-md" />
        <Skeleton className="h-64 w-full" />
      </div>
    )
  }

  if (!team) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Team unavailable</AlertTitle>
        <AlertDescription>
          This team does not exist, or you are not a member of it.
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <div>
      <TeamScopeHeader team={team} onTeamChanged={handleTeamChanged} />
      {/* The rule under the tabs is the seam into the content, so it runs the
          full width of the main region: the negative inline margins cancel the
          app shell's horizontal padding, and the inner div puts it back so the
          tabs still line up with the title above them. The `max-w-screen-xl`
          cap in `components/layout/Layout.tsx` still applies — the layout keeps
          owning the container and nothing overflows. */}
      <div className="border-border -mx-4 border-b md:-mx-6 lg:-mx-8">
        <div className="px-4 md:px-6 lg:px-8">
          <TeamTabs teamId={team.id} />
        </div>
      </div>
      <div className="pt-6">
        <TeamRoutes team={team} reloadToken={reloadToken} />
      </div>
    </div>
  )
}
