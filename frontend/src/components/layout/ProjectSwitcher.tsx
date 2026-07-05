import { useLocation } from 'react-router-dom'

import { ProjectPicker } from '@/components/ProjectPicker'
import { Button } from '@/components/ui/button'
import { useProject } from '@/contexts/ProjectContext'
import { useTeam } from '@/contexts/TeamContext'

// Route sections whose list pages filter by the globally selected project.
// Everywhere else the selector stays visible but inactive, so it never looks
// like it is filtering a page it cannot affect (Agents, Settings, Search's
// URL-driven filter, …).
const PROJECT_SCOPED_PREFIXES = [
  '/prompts',
  '/artifacts',
  '/blueprints',
  '/memories',
  '/feeds',
  '/feed-items',
]

function isProjectScopedPath(pathname: string): boolean {
  return PROJECT_SCOPED_PREFIXES.some(
    prefix => pathname === prefix || pathname.startsWith(`${prefix}/`)
  )
}

/**
 * Header project selector beside the team switcher: binds the shared current
 * project (default "All projects") to the reusable {@link ProjectPicker}.
 */
export function ProjectSwitcher() {
  const { currentTeam, isLoading: isTeamLoading } = useTeam()
  const { currentProject, setCurrentProject, isLoading } = useProject()
  const { pathname } = useLocation()

  if (isTeamLoading || (currentTeam && isLoading)) {
    return (
      <Button variant="outline" size="sm" disabled className="h-8">
        Loading…
      </Button>
    )
  }

  if (!currentTeam) {
    return null
  }

  const active = isProjectScopedPath(pathname)

  return (
    <span
      title={
        active ? undefined : 'The project filter does not apply to this page'
      }
    >
      <ProjectPicker
        value={currentProject?.id ?? null}
        selectedProject={currentProject}
        onChange={(_, project) => {
          setCurrentProject(project)
        }}
        includeAllOption
        allOptionLabel="All projects"
        disabled={!active}
        triggerClassName="h-8 w-auto max-w-[220px]"
        data-testid="project-switcher"
        aria-label="Filter by project"
      />
    </span>
  )
}
