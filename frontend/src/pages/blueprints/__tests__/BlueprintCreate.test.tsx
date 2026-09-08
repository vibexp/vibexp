import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import type { Mock } from 'vitest'

import type { Blueprint } from '@/services/blueprintService'

const mockUseTeam = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

vi.mock('@/services/blueprintService', () => ({
  blueprintService: { createBlueprint: vi.fn() },
}))

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

vi.mock('@/hooks', () => ({
  useAlerts: () => ({ showSuccess: vi.fn(), showError: vi.fn() }),
  useAnalytics: () => ({ trackEvent: vi.fn() }),
}))

vi.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: vi.fn() }),
}))

import { blueprintService } from '@/services/blueprintService'

import { BlueprintCreate } from '../BlueprintCreate'

// Radix Select relies on layout APIs jsdom doesn't implement.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

const created: Blueprint = {
  id: 'bp-1',
  project_id: 'project-1',
  slug: 'my-blueprint',
  path: 'my-blueprint.md',
  user_id: 'user-1',
  content: 'Rules',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  status: 'active',
  title: 'My blueprint',
  description: '',
  type: 'general',
  metadata: {},
  labels: [],
}

function renderCreate() {
  return render(
    <MemoryRouter>
      <BlueprintCreate />
    </MemoryRouter>
  )
}

describe('BlueprintCreate', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseTeam.mockReturnValue({
      currentTeam: { id: 'team-1', name: 'Test Team' },
      teams: [{ id: 'team-1', name: 'Test Team' }],
      isLoading: false,
      setCurrentTeam: vi.fn(),
      refreshTeams: vi.fn() as () => Promise<void>,
    })
    ;(blueprintService.createBlueprint as Mock).mockResolvedValue(created)
  })

  it('offers the status and labels controls the old form never had', () => {
    renderCreate()
    // The gap #915 actually closes: the API has always accepted a blueprint
    // status, and only the form was missing one.
    expect(screen.getByTestId('blueprint-status-select')).toBeInTheDocument()
    expect(screen.getByTestId('blueprint-labels-input')).toBeInTheDocument()
  })

  it('auto-fills the slug from the title, which the old form never did', async () => {
    const user = userEvent.setup()
    renderCreate()
    await user.type(screen.getByTestId('blueprint-title-input'), 'My blueprint')
    await waitFor(() => {
      expect(screen.getByTestId('blueprint-slug-input')).toHaveValue(
        'my-blueprint'
      )
    })
  })

  it('sends the whole form, status and labels included', async () => {
    const user = userEvent.setup()
    renderCreate()

    await user.type(screen.getByTestId('blueprint-title-input'), 'My blueprint')
    await user.type(screen.getByTestId('blueprint-content-textarea'), 'Rules')
    await user.click(screen.getByTestId('blueprint-project-select'))
    await user.type(screen.getByTestId('blueprint-labels-input'), 'rules')
    await user.keyboard('{Enter}')

    await user.click(screen.getByRole('button', { name: /create blueprint/i }))

    await waitFor(() => {
      expect(blueprintService.createBlueprint).toHaveBeenCalledWith('team-1', {
        title: 'My blueprint',
        slug: 'my-blueprint',
        description: '',
        project_id: 'project-1',
        type: 'general',
        status: 'active',
        content: 'Rules',
        labels: ['rules'],
        metadata: undefined,
      })
    })
  })

  it('offers exactly the two statuses the spec allows, and saves one', async () => {
    const user = userEvent.setup()
    renderCreate()

    await user.click(screen.getByTestId('blueprint-status-select'))

    // #912 kept `BlueprintStatus` at [active, expired]; draft/archived belong to
    // artifacts and memories, and offering them here would 400.
    expect(
      await screen.findByRole('option', { name: 'Active' })
    ).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Expired' })).toBeInTheDocument()
    expect(
      screen.queryByRole('option', { name: 'Draft' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('option', { name: 'Archived' })
    ).not.toBeInTheDocument()

    await user.click(screen.getByRole('option', { name: 'Expired' }))
    await user.type(screen.getByTestId('blueprint-title-input'), 'My blueprint')
    await user.type(screen.getByTestId('blueprint-content-textarea'), 'Rules')
    await user.click(screen.getByTestId('blueprint-project-select'))
    await user.click(screen.getByRole('button', { name: /create blueprint/i }))

    await waitFor(() => {
      expect(blueprintService.createBlueprint).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ status: 'expired' })
      )
    })
  })
})
