import { useEffect, useState } from 'react'

import { useProject } from '@/contexts/ProjectContext'
import { useTeam } from '@/contexts/TeamContext'
import type { Project } from '@/services/projectService'
import { projectService } from '@/services/projectService'

/**
 * The project a create form opens on: the one chosen in the header filter, or
 * the workspace's only project when there is exactly one. With several projects
 * and no filter there is no honest default, so the form opens on its
 * placeholder and the author picks.
 */
export function pickDefaultProject(
  filtered: Project | null,
  projects: readonly Project[]
): Project | null {
  if (filtered) return filtered
  return projects.length === 1 ? projects[0] : null
}

interface DefaultProject {
  /** The project to preselect, or `null` when the author has to pick. */
  project: Project | null
  /** False once the workspace is known to have no project at all. */
  hasProjects: boolean
  /**
   * True until the default is settled. A form must wait for it: seeding
   * `initialValues` after first paint is a reset, which discards anything
   * already typed.
   */
  loading: boolean
}

interface Loaded {
  teamId: string
  projects: Project[]
}

/**
 * Resolves {@link pickDefaultProject} for the current workspace. Pass
 * `enabled: false` where no default applies (an edit page), and nothing is
 * fetched.
 */
export function useDefaultProject({
  enabled = true,
}: { enabled?: boolean } = {}): DefaultProject {
  const { currentTeam, isLoading: isLoadingTeam } = useTeam()
  const { currentProject, isLoading: isRestoringProject } = useProject()
  const [loaded, setLoaded] = useState<Loaded | null>(null)

  const teamId = currentTeam?.id ?? null
  const hasFilter = currentProject !== null

  useEffect(() => {
    // A filtered project answers the question on its own.
    if (!enabled || !teamId || isRestoringProject || hasFilter) return
    let cancelled = false
    const load = async () => {
      let projects: Project[] = []
      try {
        // Two is enough to tell "exactly one" from "several".
        const response = await projectService.getProjects(teamId, { limit: 2 })
        projects = response.projects
      } catch (error) {
        console.error('Failed to fetch projects:', error)
      }
      if (!cancelled) setLoaded({ teamId, projects })
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [enabled, teamId, isRestoringProject, hasFilter])

  if (!enabled) return { project: null, hasProjects: true, loading: false }
  if (currentProject) {
    return { project: currentProject, hasProjects: true, loading: false }
  }
  if (isLoadingTeam || isRestoringProject) {
    return { project: null, hasProjects: true, loading: true }
  }
  if (!teamId) return { project: null, hasProjects: false, loading: false }
  if (loaded?.teamId !== teamId) {
    return { project: null, hasProjects: true, loading: true }
  }
  return {
    project: pickDefaultProject(null, loaded.projects),
    hasProjects: loaded.projects.length > 0,
    loading: false,
  }
}
