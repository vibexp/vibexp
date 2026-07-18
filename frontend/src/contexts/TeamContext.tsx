import type { ReactNode } from 'react'
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react'

import { STORAGE_KEYS } from '../constants/storageKeys'
import type { Team } from '../services/teamService'
import { teamService } from '../services/teamService'
import { sessionStore, storage } from '../utils/storage'

interface TeamContextValue {
  currentTeam: Team | null
  teams: Team[]
  setCurrentTeam: (team: Team) => void
  /**
   * Refetch the user's teams. Returns the newly fetched list so callers that
   * need to react to the post-refresh state (e.g. switch to a just-joined
   * team) can do so without a second round-trip. The internal state is also
   * mutated as before, so existing callers that ignore the return value still
   * work.
   */
  refreshTeams: () => Promise<Team[]>
  isLoading: boolean
}

const TeamContext = createContext<TeamContextValue | undefined>(undefined)

interface TeamProviderProps {
  children: ReactNode
}

export function TeamProvider({ children }: Readonly<TeamProviderProps>) {
  const [currentTeam, setCurrentTeam] = useState<Team | null>(null)
  const [teams, setTeams] = useState<Team[]>([])
  const [isLoading, setIsLoading] = useState(true)

  // Fetch teams function (extracted for reuse; also serves as `refreshTeams`)
  const fetchTeams = useCallback(async (): Promise<Team[]> => {
    try {
      setIsLoading(true)
      const fetchedTeams = await teamService.getTeams()
      setTeams(fetchedTeams)

      // Try to load team from localStorage
      const storedTeamId = getStoredTeamId()

      if (storedTeamId && fetchedTeams.length > 0) {
        // Find the stored team in the fetched teams
        const storedTeam = fetchedTeams.find(t => t.id === storedTeamId)
        if (storedTeam) {
          setCurrentTeam(storedTeam)
        } else {
          // Stored team not found (maybe user left the team), default to first
          setCurrentTeam(fetchedTeams[0])
          saveTeamId(fetchedTeams[0].id)
        }
      } else if (fetchedTeams.length > 0) {
        // No stored team, default to first team
        setCurrentTeam(fetchedTeams[0])
        saveTeamId(fetchedTeams[0].id)
      }
      return fetchedTeams
    } catch (error) {
      console.error('Failed to fetch teams:', error)
      return []
    } finally {
      setIsLoading(false)
    }
  }, [])

  // Fetch teams on mount
  useEffect(() => {
    void fetchTeams()
  }, [fetchTeams])

  const value = useMemo<TeamContextValue>(
    () => ({
      currentTeam,
      teams,
      setCurrentTeam: (team: Team) => {
        setCurrentTeam(team)
        saveTeamId(team.id)
      },
      refreshTeams: fetchTeams,
      isLoading,
    }),
    [currentTeam, teams, isLoading, fetchTeams]
  )

  return <TeamContext.Provider value={value}>{children}</TeamContext.Provider>
}

export function useTeam(): TeamContextValue {
  const context = useContext(TeamContext)
  if (context === undefined) {
    throw new Error('useTeam must be used within TeamProvider')
  }
  return context
}

// Helper functions for storage operations with fallbacks
function getStoredTeamId(): string | null {
  try {
    const value = storage.get(STORAGE_KEYS.CURRENT_TEAM_ID)
    if (value) return value
    return null
  } catch (error) {
    console.error('Failed to read from storage:', error)
    try {
      const value = sessionStore.get(STORAGE_KEYS.CURRENT_TEAM_ID)
      if (value) return value
      return null
    } catch (sessionError) {
      console.error('Failed to read from session storage:', sessionError)
      return null
    }
  }
}

function saveTeamId(teamId: string): void {
  try {
    storage.set(STORAGE_KEYS.CURRENT_TEAM_ID, teamId)
  } catch (error) {
    console.error('Failed to write to storage:', error)
    try {
      sessionStore.set(STORAGE_KEYS.CURRENT_TEAM_ID, teamId)
    } catch (sessionError) {
      console.error('Failed to write to session storage:', sessionError)
    }
  }
}
