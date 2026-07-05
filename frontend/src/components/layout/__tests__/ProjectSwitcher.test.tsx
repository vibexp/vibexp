import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import { ProjectSwitcher } from '@/components/layout/ProjectSwitcher'
import { useProject } from '@/contexts/ProjectContext'
import { useTeam } from '@/contexts/TeamContext'
import type { Project, ProjectListResponse } from '@/services/projectService'
import { projectService } from '@/services/projectService'

jest.mock('@/contexts/TeamContext')
jest.mock('@/contexts/ProjectContext')
jest.mock('@/services/projectService')

const mockedUseTeam = useTeam as jest.MockedFunction<typeof useTeam>
const mockedUseProject = useProject as jest.MockedFunction<typeof useProject>
const mockedGetProjects = projectService.getProjects as jest.MockedFunction<
  typeof projectService.getProjects
>

const alpha: Project = {
  id: 'p1',
  user_id: 'user-1',
  team_id: 'team-1',
  name: 'Alpha Project',
  slug: 'alpha-project',
  description: '',
  git_url: '',
  homepage: '',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  version: 1,
  github_connected: false,
}

const listResponse = (projects: Project[]): ProjectListResponse => ({
  projects,
  total_count: projects.length,
  page: 1,
  per_page: 100,
  total_pages: 1,
})

// cmdk (used inside the popover) relies on browser APIs jsdom doesn't provide.
beforeAll(() => {
  global.ResizeObserver = class {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  Element.prototype.scrollIntoView = jest.fn()
})

function setTeam(): void {
  mockedUseTeam.mockReturnValue({
    currentTeam: { id: 'team-1', name: 'Team One', slug: 'team-one' },
    teams: [],
    setCurrentTeam: jest.fn(),
    refreshTeams: jest.fn(),
    isLoading: false,
  } as unknown as ReturnType<typeof useTeam>)
}

function setProject(currentProject: Project | null, isLoading = false) {
  const setCurrentProject = jest.fn()
  mockedUseProject.mockReturnValue({
    currentProject,
    setCurrentProject,
    isLoading,
  })
  return setCurrentProject
}

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <ProjectSwitcher />
    </MemoryRouter>
  )
}

describe('ProjectSwitcher', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    setTeam()
    mockedGetProjects.mockResolvedValue(listResponse([alpha]))
  })

  it('defaults to "All projects" on a project-scoped page', () => {
    setProject(null)
    renderAt('/artifacts')

    const trigger = screen.getByTestId('project-switcher')
    expect(trigger).toHaveTextContent('All projects')
    expect(trigger).toBeEnabled()
  })

  it('shows the selected project name in the trigger', () => {
    setProject(alpha)
    renderAt('/prompts')

    expect(screen.getByTestId('project-switcher')).toHaveTextContent(
      'Alpha Project'
    )
  })

  it('selecting a project updates the shared context', async () => {
    const user = userEvent.setup()
    const setCurrentProject = setProject(null)
    renderAt('/memories')

    await user.click(screen.getByTestId('project-switcher'))
    await user.click(await screen.findByText('Alpha Project'))

    expect(setCurrentProject).toHaveBeenCalledWith(alpha)
  })

  it('clearing back to "All projects" updates the shared context with null', async () => {
    const user = userEvent.setup()
    const setCurrentProject = setProject(alpha)
    renderAt('/blueprints')

    await user.click(screen.getByTestId('project-switcher'))
    const [allOption] = await screen.findAllByText('All projects')
    await user.click(allOption)

    expect(setCurrentProject).toHaveBeenCalledWith(null)
  })

  it('is visible but inactive on a non-project page', () => {
    setProject(alpha)
    renderAt('/settings/team')

    const trigger = screen.getByTestId('project-switcher')
    expect(trigger).toBeDisabled()
    // Selection is still shown so users can see the global state
    expect(trigger).toHaveTextContent('Alpha Project')
  })

  it('is inactive on the search page (URL-driven filter overrides)', () => {
    setProject(null)
    renderAt('/search')

    expect(screen.getByTestId('project-switcher')).toBeDisabled()
  })

  it('renders a loading state while the selection is restoring', () => {
    setProject(null, true)
    renderAt('/artifacts')

    expect(screen.getByText('Loading…')).toBeDisabled()
  })

  it('renders nothing when there is no current team', () => {
    mockedUseTeam.mockReturnValue({
      currentTeam: null,
      teams: [],
      setCurrentTeam: jest.fn(),
      refreshTeams: jest.fn(),
      isLoading: false,
    })
    setProject(null)

    const { container } = renderAt('/artifacts')
    expect(container).toBeEmptyDOMElement()
  })
})
