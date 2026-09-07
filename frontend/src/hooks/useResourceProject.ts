import { useEffect, useState } from 'react'

import type { Project } from '@/services/projectService'
import { projectService } from '@/services/projectService'

/**
 * Resolves the project a resource belongs to, for the Metadata section's
 * Project row.
 *
 * Every resource payload carries `project_id` but no project name, and the row
 * has to read "Design system", not a UUID. `MemoryView` already did this by
 * hand; #903 gives the row to all four resources, so the fetch moves here
 * rather than being copy-pasted three more times.
 *
 * Best-effort by design: the project name is supplemental, so a failed lookup
 * resolves to `null` (no row) instead of surfacing as a page error.
 */
export function useResourceProject(
  teamId: string | undefined,
  projectId: string | undefined
): Project | null {
  const [project, setProject] = useState<Project | null>(null)

  useEffect(() => {
    if (!teamId || !projectId) {
      setProject(null)
      return
    }
    // Guard against stale responses: if the team/project changes mid-flight, a
    // slower earlier request must not overwrite the newer project.
    let active = true
    const load = async () => {
      try {
        const res = await projectService.getProjects(teamId, { limit: 100 })
        if (!active) return
        setProject(res.projects.find(p => p.id === projectId) ?? null)
      } catch {
        if (active) setProject(null)
      }
    }
    void load()
    return () => {
      active = false
    }
  }, [teamId, projectId])

  return project
}
