import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import type { Mock } from 'vitest'

import type { Artifact } from '@/services/artifactService'

const mockUseTeam = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

vi.mock('@/services/artifactService', () => ({
  artifactService: { createArtifact: vi.fn() },
}))

// The picker and the type catalog are fetched, not typed into; the generated
// form's own suite covers their wiring, so both are stubbed to one value here.
vi.mock('@/components/ProjectPicker', () => ({
  ProjectPicker: ({
    onChange,
    'data-testid': testId,
  }: {
    onChange: (id: string) => void
    'data-testid'?: string
  }) => (
    <button
      type="button"
      data-testid={testId}
      onClick={() => {
        onChange('project-1')
      }}
    >
      pick project
    </button>
  ),
}))

vi.mock('@/hooks/useTypes', () => ({
  useTypes: () => ({
    types: [{ slug: 'general', name: 'General' }],
    isLoading: false,
  }),
}))

vi.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess: vi.fn(), showError: vi.fn() }),
  useAnalytics: () => ({ trackEvent: vi.fn() }),
}))

vi.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: vi.fn() }),
}))

import { artifactService } from '@/services/artifactService'

import { ArtifactCreate } from '../ArtifactCreate'

const created: Artifact = {
  id: 'artifact-1',
  project_id: 'project-1',
  slug: 'my-artifact',
  user_id: 'user-1',
  content: 'Body',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  status: 'active',
  title: 'My artifact',
  description: '',
  type: 'general',
  metadata: {},
  labels: [],
}

async function fillRequiredFields(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByTestId('artifact-title-input'), 'My artifact')
  await user.type(screen.getByTestId('artifact-content-textarea'), 'Body')
  await user.click(screen.getByTestId('artifact-project-select'))
}

function renderCreate() {
  return render(
    <MemoryRouter>
      <ArtifactCreate />
    </MemoryRouter>
  )
}

describe('ArtifactCreate', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseTeam.mockReturnValue({
      currentTeam: { id: 'team-1', name: 'Test Team' },
      teams: [{ id: 'team-1', name: 'Test Team' }],
      isLoading: false,
      setCurrentTeam: vi.fn(),
      refreshTeams: vi.fn() as () => Promise<void>,
    })
    ;(artifactService.createArtifact as Mock).mockResolvedValue(created)
  })

  it('renders the generated form rather than a page-local one', () => {
    renderCreate()
    expect(screen.getByTestId('resource-form')).toBeInTheDocument()
    expect(screen.getByTestId('artifact-status-select')).toBeInTheDocument()
    // #915 gave artifacts a labels editor they never had.
    expect(screen.getByTestId('artifact-labels-input')).toBeInTheDocument()
  })

  it('auto-fills the slug from the title', async () => {
    const user = userEvent.setup()
    renderCreate()
    await user.type(screen.getByTestId('artifact-title-input'), 'My artifact')
    await waitFor(() => {
      expect(screen.getByTestId('artifact-slug-input')).toHaveValue(
        'my-artifact'
      )
    })
  })

  it('sends the whole form, labels and metadata included', async () => {
    const user = userEvent.setup()
    renderCreate()

    await fillRequiredFields(user)
    await user.type(screen.getByTestId('artifact-labels-input'), 'onboarding')
    await user.keyboard('{Enter}')
    await user.click(screen.getByTestId('metadata-add-pair'))
    await user.type(screen.getByTestId('metadata-key-0'), 'source')
    await user.type(screen.getByTestId('metadata-value-0'), 'wiki')

    await user.click(screen.getByRole('button', { name: /create artifact/i }))

    await waitFor(() => {
      expect(artifactService.createArtifact).toHaveBeenCalledWith('team-1', {
        title: 'My artifact',
        slug: 'my-artifact',
        description: '',
        project_id: 'project-1',
        type: 'general',
        status: 'active',
        content: 'Body',
        labels: ['onboarding'],
        metadata: { source: 'wiki' },
      })
    })
  })

  it('omits metadata entirely when the bag is empty', async () => {
    const user = userEvent.setup()
    renderCreate()

    await fillRequiredFields(user)
    await user.click(screen.getByRole('button', { name: /create artifact/i }))
    await waitFor(() => {
      expect(artifactService.createArtifact).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ metadata: undefined, labels: [] })
      )
    })
  })

  it('blocks the submit while a required field is blank', async () => {
    const user = userEvent.setup()
    renderCreate()

    await user.click(screen.getByRole('button', { name: /create artifact/i }))

    expect(await screen.findByText('Title is required')).toBeInTheDocument()
    expect(artifactService.createArtifact).not.toHaveBeenCalled()
  })
})
