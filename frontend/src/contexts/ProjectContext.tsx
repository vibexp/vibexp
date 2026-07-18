import type { ReactNode } from 'react'
import { createContext, useContext, useEffect, useRef, useState } from 'react'

import type { Project } from '@/services/projectService'

import { STORAGE_KEYS } from '../constants/storageKeys'
import { projectService } from '../services/projectService'
import { sessionStore, storage } from '../utils/storage'
import { useTeam } from './TeamContext'

interface ProjectContextValue {
  /**
   * The globally selected project driving the header project selector and the
   * project-scoped list pages, or `null` when "All projects" is selected.
   */
  currentProject: Project | null
  setCurrentProject: (project: Project | null) => void
  isLoading: boolean
}

const ProjectContext = createContext<ProjectContextValue | undefined>(undefined)

interface ProjectProviderProps {
  children: ReactNode
}

/**
 * Holds the globally selected project (mirror of {@link TeamProvider} for
 * projects). Must be mounted inside `TeamProvider`: the selection is scoped to
 * the current team, restored from storage on the first team resolution and
 * cleared whenever the team changes (a project always belongs to one team).
 */
export function ProjectProvider({ children }: Readonly<ProjectProviderProps>) {
  const { currentTeam } = useTeam()
  const [currentProject, setCurrentProjectState] = useState<Project | null>(
    null
  )
  // Loading only means "a persisted selection is being restored" — with
  // nothing stored there is nothing to wait for, so consumers (list pages)
  // can fetch immediately.
  const [isLoading, setIsLoading] = useState(
    () => getStoredProjectId() !== null
  )
  // Team id the selection was last resolved for; distinguishes the initial
  // restore-from-storage from a later team switch (which resets instead).
  const resolvedTeamIdRef = useRef<string | null>(null)
  // Set once the user picks a project; a slow in-flight restore must never
  // overwrite an explicit selection made while it was pending.
  const userSelectedRef = useRef(false)

  useEffect(() => {
    if (!currentTeam) return
    const prevTeamId = resolvedTeamIdRef.current
    if (prevTeamId === currentTeam.id) return
    resolvedTeamIdRef.current = currentTeam.id

    if (prevTeamId !== null) {
      // Team switch: the selected project belonged to the previous team.
      setCurrentProjectState(null)
      clearProjectId()
      setIsLoading(false)
      return
    }

    const storedId = getStoredProjectId()
    if (!storedId) {
      setIsLoading(false)
      return
    }

    // First team resolution: restore the persisted selection, validating it
    // against the team's projects so a deleted project (or one stored for a
    // different team) is dropped instead of silently filtering everything out.
    let cancelled = false
    const restore = async () => {
      try {
        const response = await projectService.getProjects(currentTeam.id, {
          limit: 100,
        })
        if (cancelled || userSelectedRef.current) return
        const stored = response.projects.find(p => p.id === storedId)
        if (stored) {
          setCurrentProjectState(stored)
        } else {
          clearProjectId()
        }
      } catch (error) {
        console.error('Failed to restore current project:', error)
      } finally {
        if (!cancelled) setIsLoading(false)
      }
    }
    void restore()
    return () => {
      cancelled = true
    }
  }, [currentTeam])

  const setCurrentProject = (project: Project | null) => {
    userSelectedRef.current = true
    setCurrentProjectState(project)
    if (project) {
      saveProjectId(project.id)
    } else {
      clearProjectId()
    }
  }

  const value: ProjectContextValue = {
    currentProject,
    setCurrentProject,
    isLoading,
  }

  return (
    <ProjectContext.Provider value={value}>{children}</ProjectContext.Provider>
  )
}

export function useProject(): ProjectContextValue {
  const context = useContext(ProjectContext)
  if (context === undefined) {
    throw new Error('useProject must be used within ProjectProvider')
  }
  return context
}

// Helper functions for storage operations with fallbacks
function getStoredProjectId(): string | null {
  try {
    const value = storage.get(STORAGE_KEYS.CURRENT_PROJECT_ID)
    if (value) return value
    return null
  } catch (error) {
    console.error('Failed to read from storage:', error)
    try {
      const value = sessionStore.get(STORAGE_KEYS.CURRENT_PROJECT_ID)
      if (value) return value
      return null
    } catch (sessionError) {
      console.error('Failed to read from session storage:', sessionError)
      return null
    }
  }
}

function saveProjectId(projectId: string): void {
  try {
    storage.set(STORAGE_KEYS.CURRENT_PROJECT_ID, projectId)
  } catch (error) {
    console.error('Failed to write to storage:', error)
    try {
      sessionStore.set(STORAGE_KEYS.CURRENT_PROJECT_ID, projectId)
    } catch (sessionError) {
      console.error('Failed to write to session storage:', sessionError)
    }
  }
}

function clearProjectId(): void {
  try {
    storage.remove(STORAGE_KEYS.CURRENT_PROJECT_ID)
  } catch (error) {
    console.error('Failed to remove from storage:', error)
  }
  try {
    sessionStore.remove(STORAGE_KEYS.CURRENT_PROJECT_ID)
  } catch (error) {
    console.error('Failed to remove from session storage:', error)
  }
}
