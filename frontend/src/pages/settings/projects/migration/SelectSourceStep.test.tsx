import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { useTeam } from '@/contexts/TeamContext'
import { projectService } from '@/services/projectService'
import type { Project, ProjectListResponse } from '@/types/project'

import { SelectSourceStep } from './SelectSourceStep'

jest.mock('@/contexts/TeamContext')
jest.mock('@/services/projectService')

const mockedUseTeam = useTeam as jest.MockedFunction<typeof useTeam>
const mockedGetProjects = projectService.getProjects as jest.MockedFunction<
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
})

const listResponse = (projects: Project[]): ProjectListResponse => ({
  projects,
  total_count: projects.length,
  page: 1,
  per_page: 100,
  total_pages: 1,
})

const alpha = baseProject('p1', 'Alpha Project', 'alpha-project')
const beta = baseProject('p2', 'Beta Project', 'beta-project')

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
    currentTeam: {
      id: 'team-1',
      name: 'Team One',
      slug: 'team-one',
    },
    teams: [],
    setCurrentTeam: jest.fn(),
    refreshTeams: jest.fn(),
    isLoading: false,
  } as unknown as ReturnType<typeof useTeam>)
}

function renderStep({
  resolvedSourceProject = alpha,
  selectedProjectId = 'p1',
  onSelect = jest.fn(),
  onNext = jest.fn(),
  loadingInventory = false,
}: {
  resolvedSourceProject?: Project | null
  selectedProjectId?: string
  onSelect?: jest.Mock
  onNext?: jest.Mock
  loadingInventory?: boolean
} = {}) {
  return render(
    <SelectSourceStep
      resolvedSourceProject={resolvedSourceProject}
      selectedProjectId={selectedProjectId}
      onSelect={onSelect}
      onNext={onNext}
      loadingInventory={loadingInventory}
    />
  )
}

describe('SelectSourceStep', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    jest.useFakeTimers()
    setTeam()
    mockedGetProjects.mockResolvedValue(listResponse([alpha, beta]))
  })

  afterEach(() => {
    jest.runOnlyPendingTimers()
    jest.useRealTimers()
  })

  it('renders the heading', () => {
    renderStep()

    expect(
      screen.getByRole('heading', { name: /select source project/i })
    ).toBeInTheDocument()
  })

  it('shows the resolved source project name in the combobox trigger', () => {
    renderStep({ selectedProjectId: 'p1' })

    expect(screen.getByRole('combobox')).toHaveTextContent('Alpha Project')
  })

  it('shows placeholder when no project is selected', () => {
    renderStep({ selectedProjectId: '', resolvedSourceProject: null })

    expect(screen.getByRole('combobox')).toHaveTextContent(/select project/i)
  })

  it('Next button is enabled when a project is selected', () => {
    renderStep({ selectedProjectId: 'p1' })

    expect(screen.getByRole('button', { name: /next/i })).not.toBeDisabled()
  })

  it('Next button is disabled when no project is selected', () => {
    renderStep({ selectedProjectId: '', resolvedSourceProject: null })

    expect(screen.getByRole('button', { name: /next/i })).toBeDisabled()
  })

  it('Next button is disabled while loading inventory', () => {
    renderStep({ selectedProjectId: 'p1', loadingInventory: true })

    expect(
      screen.getByRole('button', { name: /loading inventory/i })
    ).toBeDisabled()
  })

  it('calls onNext when Next button is clicked', async () => {
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime })
    const onNext = jest.fn()
    renderStep({ selectedProjectId: 'p1', onNext })

    await user.click(screen.getByRole('button', { name: /next/i }))

    expect(onNext).toHaveBeenCalledTimes(1)
  })

  it('searches the backend (debounced) when typing in the picker', async () => {
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime })
    renderStep({ resolvedSourceProject: null, selectedProjectId: '' })

    await user.click(screen.getByRole('combobox'))

    const input = screen.getByPlaceholderText(/search projects/i)
    await user.type(input, 'Beta')

    expect(mockedGetProjects).not.toHaveBeenLastCalledWith('team-1', {
      limit: 100,
      page: 1,
      search: 'Beta',
    })

    jest.advanceTimersByTime(300)

    await waitFor(() => {
      expect(mockedGetProjects).toHaveBeenLastCalledWith('team-1', {
        limit: 100,
        page: 1,
        search: 'Beta',
      })
    })
  })

  it('selecting a searched project calls onSelect with the project', async () => {
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime })
    mockedGetProjects.mockResolvedValue(listResponse([beta]))
    const onSelect = jest.fn()
    renderStep({ resolvedSourceProject: null, selectedProjectId: '', onSelect })

    await user.click(screen.getByRole('combobox'))
    jest.advanceTimersByTime(300)

    const item = await screen.findByText('Beta Project')
    await user.click(item)

    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 'p2' }))
  })

  it('keeps the resolved source visible even when not in search results', async () => {
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime })
    // Search results do not include the resolved source project.
    mockedGetProjects.mockResolvedValue(listResponse([beta]))
    renderStep({ resolvedSourceProject: alpha, selectedProjectId: 'p1' })

    await user.click(screen.getByRole('combobox'))
    jest.advanceTimersByTime(300)

    expect(await screen.findByText('Beta Project')).toBeInTheDocument()
    // Alpha (the resolved source) is prepended even though search omitted it.
    // It also appears in the trigger, hence getAllByText.
    expect(screen.getAllByText('Alpha Project').length).toBeGreaterThan(1)
  })

  it('shows a loading state while searching', async () => {
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime })
    let resolveFetch: (value: ProjectListResponse) => void = () => {}
    mockedGetProjects.mockReturnValue(
      new Promise<ProjectListResponse>(resolve => {
        resolveFetch = resolve
      })
    )
    renderStep({ resolvedSourceProject: null, selectedProjectId: '' })

    await user.click(screen.getByRole('combobox'))
    jest.advanceTimersByTime(300)

    expect(await screen.findByText(/searching/i)).toBeInTheDocument()

    resolveFetch(listResponse([beta]))
    await waitFor(() => {
      expect(screen.queryByText(/searching/i)).not.toBeInTheDocument()
    })
  })

  it('shows an empty state when no projects match', async () => {
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime })
    mockedGetProjects.mockResolvedValue(listResponse([]))
    renderStep({ resolvedSourceProject: null, selectedProjectId: '' })

    await user.click(screen.getByRole('combobox'))
    jest.advanceTimersByTime(300)

    expect(await screen.findByText(/no projects found/i)).toBeInTheDocument()
  })
})
