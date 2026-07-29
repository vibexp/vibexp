import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { MockedFunction } from 'vitest'

import { useTeam } from '@/contexts/TeamContext'
import type { Project, ProjectListResponse } from '@/services/projectService'
import { projectService } from '@/services/projectService'

import { ProjectPicker } from './ProjectPicker'

vi.mock('@/contexts/TeamContext')
vi.mock('@/services/projectService')

const mockedUseTeam = useTeam as MockedFunction<typeof useTeam>
const mockedGetProjects = projectService.getProjects as MockedFunction<
  typeof projectService.getProjects
>

const baseProject = (id: string, name: string, slug: string): Project => ({
  id,
  user_id: 'user-1',
  team_id: 'team-1',
  name,
  slug,
  description: '',
  git_url: '',
  homepage: '',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  version: 1,
  github_connected: false,
})

const pagedResponse = (
  projects: Project[],
  page: number,
  totalPages: number
): ProjectListResponse => ({
  projects,
  total_count: totalPages,
  page,
  per_page: projects.length,
  total_pages: totalPages,
})

const listResponse = (projects: Project[]): ProjectListResponse =>
  pagedResponse(projects, 1, 1)

const alpha = baseProject('p1', 'Alpha Project', 'alpha-project')
const beta = baseProject('p2', 'Beta Project', 'beta-project')

// cmdk (used inside the popover) relies on browser APIs jsdom doesn't provide.
beforeAll(() => {
  global.ResizeObserver = class {
    observe(): void {}
    unobserve(): void {}
    disconnect(): void {}
  }
  Element.prototype.scrollIntoView = vi.fn()
})

function setTeam(): void {
  mockedUseTeam.mockReturnValue({
    currentTeam: { id: 'team-1', name: 'Team One', slug: 'team-one' },
    teams: [],
    setCurrentTeam: vi.fn(),
    refreshTeams: vi.fn(),
    isLoading: false,
  } as unknown as ReturnType<typeof useTeam>)
}

describe('ProjectPicker', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    setTeam()
    mockedGetProjects.mockResolvedValue(listResponse([alpha, beta]))
  })

  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
  })

  it('shows the placeholder when nothing is selected', () => {
    render(
      <ProjectPicker value={null} onChange={vi.fn()} placeholder="Pick one…" />
    )

    expect(screen.getByRole('combobox')).toHaveTextContent('Pick one…')
  })

  it('shows the seeded selected project name in the trigger', () => {
    render(
      <ProjectPicker value="p1" onChange={vi.fn()} selectedProject={alpha} />
    )

    expect(screen.getByRole('combobox')).toHaveTextContent('Alpha Project')
  })

  it('searches the backend (debounced) when typing', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    render(<ProjectPicker value={null} onChange={vi.fn()} />)

    await user.click(screen.getByRole('combobox'))
    await user.type(screen.getByPlaceholderText(/search projects/i), 'Beta')

    vi.advanceTimersByTime(300)

    await waitFor(() => {
      expect(mockedGetProjects).toHaveBeenLastCalledWith('team-1', {
        limit: 100,
        page: 1,
        search: 'Beta',
      })
    })
  })

  it('calls onChange with the project id and project when a project is selected', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    const onChange = vi.fn()
    render(<ProjectPicker value={null} onChange={onChange} />)

    await user.click(screen.getByRole('combobox'))
    vi.advanceTimersByTime(300)

    await user.click(await screen.findByText('Beta Project'))

    expect(onChange).toHaveBeenCalledWith('p2', beta)
  })

  it('renders an "All projects" option that clears the selection', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    const onChange = vi.fn()
    render(<ProjectPicker value="p1" onChange={onChange} includeAllOption />)

    await user.click(screen.getByRole('combobox'))
    vi.advanceTimersByTime(300)

    await user.click(await screen.findByText('All projects'))

    expect(onChange).toHaveBeenCalledWith(null, null)
  })

  it('forwards aria-* props onto the trigger (FormControl wiring)', () => {
    render(
      <ProjectPicker
        value={null}
        onChange={vi.fn()}
        aria-invalid
        aria-describedby="project-error"
      />
    )

    const trigger = screen.getByRole('combobox')
    expect(trigger).toHaveAttribute('aria-invalid', 'true')
    expect(trigger).toHaveAttribute('aria-describedby', 'project-error')
  })

  it('shows the all-option label in the trigger when cleared', () => {
    render(
      <ProjectPicker
        value={null}
        onChange={vi.fn()}
        includeAllOption
        allOptionLabel="All projects"
      />
    )

    expect(screen.getByRole('combobox')).toHaveTextContent('All projects')
  })

  it('renders a height-bounded, scrollable result list', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    const { container } = render(
      <ProjectPicker value={null} onChange={vi.fn()} />
    )

    await user.click(screen.getByRole('combobox'))
    vi.advanceTimersByTime(300)
    await screen.findByText('Alpha Project')

    const list = container.ownerDocument.querySelector('[cmdk-list]')
    expect(list).not.toBeNull()
    expect(list?.className).toContain('max-h-72')
    expect(list?.className).toContain('overflow-y-auto')
  })

  it('appends the next page when "Load more" is selected', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    mockedGetProjects.mockResolvedValueOnce(pagedResponse([alpha], 1, 2))

    render(<ProjectPicker value={null} onChange={vi.fn()} />)

    await user.click(screen.getByRole('combobox'))
    vi.advanceTimersByTime(300)
    await screen.findByText('Alpha Project')

    mockedGetProjects.mockResolvedValueOnce(pagedResponse([beta], 2, 2))

    await user.click(await screen.findByText('Load more'))

    expect(await screen.findByText('Beta Project')).toBeInTheDocument()
    expect(mockedGetProjects).toHaveBeenLastCalledWith('team-1', {
      limit: 100,
      page: 2,
    })
  })

  it('shows an empty state when no projects match', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    mockedGetProjects.mockResolvedValue(listResponse([]))
    const { container } = render(
      <ProjectPicker value={null} onChange={vi.fn()} />
    )

    await user.click(screen.getByRole('combobox'))
    vi.advanceTimersByTime(300)

    const list = await waitFor(() => {
      const el =
        container.ownerDocument.querySelector<HTMLElement>('[cmdk-list]')
      expect(el).not.toBeNull()
      return el!
    })
    await waitFor(() => {
      expect(within(list).getByText(/no projects found/i)).toBeInTheDocument()
    })
  })
})
