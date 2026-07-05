import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { useTeam } from '@/contexts/TeamContext'
import type {
  ConflictPolicy,
  ResourceSelections,
} from '@/services/projectMigrationService'
import type { Project, ProjectListResponse } from '@/services/projectService'
import { projectService } from '@/services/projectService'

import { SelectDestinationStep } from './SelectDestinationStep'

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
  github_connected: false,
})

const listResponse = (projects: Project[]): ProjectListResponse => ({
  projects,
  total_count: projects.length,
  page: 1,
  per_page: 100,
  total_pages: 1,
})

const sourceProject = baseProject('src-id', 'Source Project', 'source-project')
const destProject = baseProject(
  'dest-id',
  'Destination Project',
  'destination-project'
)
const otherProject = baseProject('other-id', 'Other Project', 'other-project')

const emptyResources: ResourceSelections = {
  prompts: { all: false, ids: [] },
  artifacts: { all: false, ids: [] },
  blueprints: { all: false, ids: [] },
  feed_items: { all: false, ids: [] },
}

const inventoryCounts = {
  prompts: 2,
  artifacts: 1,
  blueprints: 0,
  feed_items: 3,
}

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
  destinationProjectId = '',
  conflictPolicy = 'skip',
  selectedResources = emptyResources,
  onDestinationSelect = jest.fn(),
  onConflictPolicyChange = jest.fn(),
  onBack = jest.fn(),
  onMigrate = jest.fn(),
  migrating = false,
}: {
  destinationProjectId?: string
  conflictPolicy?: ConflictPolicy
  selectedResources?: ResourceSelections
  onDestinationSelect?: jest.Mock
  onConflictPolicyChange?: jest.Mock
  onBack?: jest.Mock
  onMigrate?: jest.Mock
  migrating?: boolean
} = {}) {
  return render(
    <SelectDestinationStep
      sourceProjectId="src-id"
      sourceProjectName="Source Project"
      destinationProjectId={destinationProjectId}
      conflictPolicy={conflictPolicy}
      selectedResources={selectedResources}
      inventoryCounts={inventoryCounts}
      onDestinationSelect={onDestinationSelect}
      onConflictPolicyChange={onConflictPolicyChange}
      onBack={onBack}
      onMigrate={onMigrate}
      migrating={migrating}
    />
  )
}

describe('SelectDestinationStep', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    jest.useFakeTimers()
    setTeam()
    mockedGetProjects.mockResolvedValue(
      listResponse([destProject, otherProject])
    )
  })

  afterEach(() => {
    jest.runOnlyPendingTimers()
    jest.useRealTimers()
  })

  it('renders the destination combobox trigger', () => {
    renderStep()

    expect(
      screen.getByRole('combobox', { name: /destination project/i })
    ).toBeInTheDocument()
  })

  it('renders three conflict policy options', () => {
    renderStep()

    expect(screen.getByText('Skip')).toBeInTheDocument()
    expect(screen.getByText('Rename')).toBeInTheDocument()
    expect(screen.getByText('Overwrite')).toBeInTheDocument()
  })

  it('shows overwrite warning when policy is overwrite', () => {
    renderStep({ conflictPolicy: 'overwrite' })

    expect(
      screen.getByText(/permanently replace resources/i)
    ).toBeInTheDocument()
  })

  it('does not show overwrite warning for skip policy', () => {
    renderStep({ conflictPolicy: 'skip' })

    expect(
      screen.queryByText(/permanently replace resources/i)
    ).not.toBeInTheDocument()
  })

  it('does not show summary when no destination selected', () => {
    renderStep()

    expect(screen.queryByText(/migration summary/i)).not.toBeInTheDocument()
  })

  it('calls onBack when Back button is clicked', async () => {
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime })
    const onBack = jest.fn()
    renderStep({ onBack })

    await user.click(screen.getByRole('button', { name: /back/i }))

    expect(onBack).toHaveBeenCalledTimes(1)
  })

  it('disables Migrate button when no destination is selected', () => {
    renderStep()

    expect(screen.getByRole('button', { name: /^migrate$/i })).toBeDisabled()
  })

  it('disables Back and shows migrating text when migrating', () => {
    renderStep({ migrating: true, destinationProjectId: 'dest-id' })

    expect(screen.getByRole('button', { name: /back/i })).toBeDisabled()
    expect(
      screen.getByRole('button', { name: /migrating/i })
    ).toBeInTheDocument()
  })

  it('calls onConflictPolicyChange when radio changes', async () => {
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime })
    const onConflictPolicyChange = jest.fn()
    renderStep({ onConflictPolicyChange })

    const renameRadio = screen.getByDisplayValue('rename')
    await user.click(renameRadio)

    expect(onConflictPolicyChange).toHaveBeenCalledWith('rename')
  })

  it('searches the backend (debounced) when typing and excludes the source', async () => {
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime })
    renderStep()

    await user.click(screen.getByRole('combobox', { name: /destination/i }))

    const input = screen.getByPlaceholderText(/search projects/i)
    await user.type(input, 'Dest')

    jest.advanceTimersByTime(300)

    await waitFor(() => {
      expect(mockedGetProjects).toHaveBeenLastCalledWith('team-1', {
        limit: 100,
        page: 1,
        search: 'Dest',
      })
    })
  })

  it('selecting a searched destination calls onDestinationSelect and shows the summary', async () => {
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime })
    const onDestinationSelect = jest.fn()
    const { rerender } = renderStep({ onDestinationSelect })

    await user.click(screen.getByRole('combobox', { name: /destination/i }))
    jest.advanceTimersByTime(300)

    const item = await screen.findByText('Destination Project')
    await user.click(item)

    expect(onDestinationSelect).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'dest-id' })
    )

    // The parent reflects the selection back via props.
    rerender(
      <SelectDestinationStep
        sourceProjectId="src-id"
        sourceProjectName="Source Project"
        destinationProjectId="dest-id"
        conflictPolicy="skip"
        selectedResources={emptyResources}
        inventoryCounts={inventoryCounts}
        onDestinationSelect={onDestinationSelect}
        onConflictPolicyChange={jest.fn()}
        onBack={jest.fn()}
        onMigrate={jest.fn()}
        migrating={false}
      />
    )

    expect(screen.getByText(/migration summary/i)).toBeInTheDocument()
  })

  it('excludes the source project from the search request', async () => {
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime })
    // Backend returns source too; the hook must filter it out.
    mockedGetProjects.mockResolvedValue(
      listResponse([sourceProject, destProject])
    )
    renderStep()

    await user.click(screen.getByRole('combobox', { name: /destination/i }))
    jest.advanceTimersByTime(300)

    expect(await screen.findByText('Destination Project')).toBeInTheDocument()
    expect(screen.queryByText('Source Project')).not.toBeInTheDocument()
  })

  it('shows a loading state while searching', async () => {
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime })
    let resolveFetch: (value: ProjectListResponse) => void = () => {}
    mockedGetProjects.mockReturnValue(
      new Promise<ProjectListResponse>(resolve => {
        resolveFetch = resolve
      })
    )
    renderStep()

    await user.click(screen.getByRole('combobox', { name: /destination/i }))
    jest.advanceTimersByTime(300)

    expect(await screen.findByText(/searching/i)).toBeInTheDocument()

    resolveFetch(listResponse([destProject]))
    await waitFor(() => {
      expect(screen.queryByText(/searching/i)).not.toBeInTheDocument()
    })
  })

  it('shows an empty state when no other projects match', async () => {
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime })
    mockedGetProjects.mockResolvedValue(listResponse([]))
    renderStep()

    await user.click(screen.getByRole('combobox', { name: /destination/i }))
    jest.advanceTimersByTime(300)

    expect(
      await screen.findByText(/no other projects found/i)
    ).toBeInTheDocument()
  })
})
