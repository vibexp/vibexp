import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Mocked } from 'vitest'

import type {
  StatusDefinition,
  StatusResourceType,
} from '@/services/statusService'
import type { Team } from '@/services/teamService'
import { ApiError } from '@/types/errors'

// `permissions` is what usePermissions reads — the hook is deliberately NOT
// mocked, so gating is exercised through the real hook against these fixtures.
const adminTeam = {
  id: 'team-1',
  name: 'Test Team',
  role: 'member',
  permissions: ['team.settings.update'],
} as unknown as Team

const memberTeam = {
  id: 'team-1',
  name: 'Test Team',
  // An owner without the permission: the gate must follow `permissions`, so a
  // check that read `role` would wrongly show the controls here.
  role: 'owner',
  permissions: ['resource.create'],
} as unknown as Team

const mockUseTeam = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

vi.mock('@/services/statusService', async () => {
  const actual = await vi.importActual<
    typeof import('@/services/statusService')
  >('@/services/statusService')
  return {
    ...actual,
    statusService: {
      list: vi.fn(),
      create: vi.fn(),
      rename: vi.fn(),
      remove: vi.fn(),
    },
  }
})

const mockHandleError = vi.hoisted(() => vi.fn(() => ({})))
vi.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))

const mockToastSuccess = vi.hoisted(() => vi.fn())
vi.mock('@/lib/toast', () => ({
  toast: {
    success: (...a: unknown[]) => mockToastSuccess(...a),
    error: vi.fn(),
  },
}))

import { statusService } from '@/services/statusService'

import { Statuses } from '../Statuses'

const mocked = statusService as Mocked<typeof statusService>

const system = (
  resourceType: StatusResourceType,
  slug: string,
  name: string,
  usage = 0
): StatusDefinition => ({
  resource_type: resourceType,
  slug,
  name,
  is_system: true,
  is_live: slug !== 'draft',
  usage_count: usage,
})

const custom = (
  overrides: Partial<StatusDefinition> = {}
): StatusDefinition => ({
  id: 'status-1',
  resource_type: 'prompt',
  slug: 'in-review',
  name: 'In review',
  is_system: false,
  is_live: true,
  usage_count: 12,
  created_at: '2026-10-01T00:00:00Z',
  updated_at: '2026-10-01T00:00:00Z',
  ...overrides,
})

const promptStatuses = [
  system('prompt', 'draft', 'Draft', 4),
  system('prompt', 'published', 'Published', 9),
  custom(),
]

const artifactStatuses = [
  system('artifact', 'active', 'Active', 7),
  system('artifact', 'draft', 'Draft'),
  system('artifact', 'archived', 'Archived'),
  custom({
    id: 'status-2',
    resource_type: 'artifact',
    slug: 'approved',
    name: 'Approved',
    usage_count: 1,
  }),
]

const apiError = (status: number, detail: string) =>
  new ApiError({
    type: 'about:blank',
    title: 'Error',
    status,
    detail,
    code: status === 409 ? 'RESOURCE_ALREADY_EXISTS' : 'BAD_REQUEST',
    request_id: 'req-1',
    timestamp: '2026-10-01T00:00:00Z',
  })

beforeEach(() => {
  vi.clearAllMocks()
  mockUseTeam.mockReturnValue({ currentTeam: adminTeam, teams: [adminTeam] })
  mocked.list.mockImplementation((_teamId, resourceType) =>
    Promise.resolve(
      resourceType === 'artifact' ? artifactStatuses : promptStatuses
    )
  )
})

const rows = () => screen.getAllByTestId('status-item')

describe('Statuses — reading', () => {
  it('lists system statuses then custom ones, with slug, origin and usage', async () => {
    render(<Statuses team={adminTeam} />)

    await screen.findByText('In review')
    expect(mocked.list).toHaveBeenCalledWith('team-1', 'prompt')

    const [draft, published, inReview] = rows()
    expect(within(draft).getByText('Draft')).toBeInTheDocument()
    expect(within(draft).getByText('draft')).toBeInTheDocument()
    expect(within(draft).getByText('System')).toBeInTheDocument()
    expect(within(draft).getByTestId('status-usage')).toHaveTextContent('4')
    expect(within(published).getByText('System')).toBeInTheDocument()
    expect(within(inReview).getByText('in-review')).toBeInTheDocument()
    expect(within(inReview).getByText('Custom')).toBeInTheDocument()
    expect(within(inReview).getByTestId('status-usage')).toHaveTextContent('12')
  })

  it('offers one tab per resource type', () => {
    render(<Statuses team={adminTeam} />)

    expect(screen.getAllByRole('tab').map(tab => tab.textContent)).toEqual([
      'Prompts',
      'Blueprints',
      'Memories',
      'Artifacts',
    ])
  })

  it('loads another resource type only when its tab is opened', async () => {
    const user = userEvent.setup()
    render(<Statuses team={adminTeam} />)
    await screen.findByText('In review')
    expect(mocked.list).toHaveBeenCalledTimes(1)

    await user.click(screen.getByRole('tab', { name: 'Artifacts' }))

    expect(await screen.findByText('Approved')).toBeInTheDocument()
    expect(mocked.list).toHaveBeenLastCalledWith('team-1', 'artifact')
    expect(screen.queryByText('In review')).not.toBeInTheDocument()
  })

  it('shows a skeleton while the list is loading', () => {
    mocked.list.mockReturnValue(new Promise(() => {}))
    render(<Statuses team={adminTeam} />)

    expect(screen.getByTestId('statuses-loading')).toBeInTheDocument()
    expect(screen.queryByTestId('status-item')).not.toBeInTheDocument()
  })

  it('shows the failure with a retry that refetches', async () => {
    const user = userEvent.setup()
    mocked.list.mockRejectedValueOnce(new Error('statuses down'))
    render(<Statuses team={adminTeam} />)

    const alert = await screen.findByTestId('statuses-error')
    expect(alert).toHaveTextContent("Couldn't load the prompt statuses")
    expect(alert).toHaveTextContent('statuses down')

    await user.click(within(alert).getByRole('button', { name: 'Try again' }))

    expect(await screen.findByText('In review')).toBeInTheDocument()
    expect(screen.queryByTestId('statuses-error')).not.toBeInTheDocument()
  })

  // #584 — on a cold deep-link the ambient team is still the previous one.
  it('reads the team from its prop, never the ambient one', async () => {
    mockUseTeam.mockReturnValue({
      currentTeam: { id: 'team-a', name: 'Team A', permissions: [] },
      teams: [],
    })
    const urlTeam: Team = { ...adminTeam, id: 'team-b' }

    render(<Statuses team={urlTeam} />)

    await screen.findByText('In review')
    expect(mocked.list).toHaveBeenCalledWith('team-b', 'prompt')
    expect(mocked.list).not.toHaveBeenCalledWith('team-a', 'prompt')
    // And the URL team's permissions decide, not the ambient team's empty set.
    expect(screen.getByTestId('create-status-button')).toBeInTheDocument()
  })
})

describe('Statuses — permission gating', () => {
  it('shows a member the full list and no write control', async () => {
    render(<Statuses team={memberTeam} />)

    await screen.findByText('In review')
    expect(rows()).toHaveLength(3)
    expect(screen.queryByTestId('create-status-button')).not.toBeInTheDocument()
    expect(screen.queryByTestId('rename-status-button')).not.toBeInTheDocument()
    expect(screen.queryByTestId('delete-status-button')).not.toBeInTheDocument()
  })

  it('shows create, and rename and delete on custom rows only, with team.settings.update', async () => {
    render(<Statuses team={adminTeam} />)

    await screen.findByText('In review')
    expect(screen.getByTestId('create-status-button')).toBeInTheDocument()

    // One of each, on the single custom row — never on a system row.
    const renameButtons = screen.getAllByTestId('rename-status-button')
    const deleteButtons = screen.getAllByTestId('delete-status-button')
    expect(renameButtons).toHaveLength(1)
    expect(deleteButtons).toHaveLength(1)
    expect(renameButtons[0]).toHaveAttribute('aria-label', 'Rename In review')
    expect(deleteButtons[0]).toHaveAttribute('aria-label', 'Delete In review')
    const [draft, published] = rows()
    expect(within(draft).queryByRole('button')).not.toBeInTheDocument()
    expect(within(published).queryByRole('button')).not.toBeInTheDocument()
  })
})

describe('Statuses — create', () => {
  const openCreate = async (user: ReturnType<typeof userEvent.setup>) => {
    await screen.findByText('In review')
    await user.click(screen.getByTestId('create-status-button'))
    await user.type(screen.getByTestId('status-name-input'), 'Blocked')
  }

  it('creates a status for the open tab and reloads the list', async () => {
    const user = userEvent.setup()
    mocked.create.mockResolvedValue(
      custom({ slug: 'blocked', name: 'Blocked' })
    )
    render(<Statuses team={adminTeam} />)

    await openCreate(user)
    await user.click(screen.getByTestId('submit-status-button'))

    await waitFor(() => {
      expect(mocked.create).toHaveBeenCalledWith('team-1', {
        resource_type: 'prompt',
        slug: 'blocked',
        name: 'Blocked',
      })
    })
    expect(mockToastSuccess).toHaveBeenCalledWith('Status created')
    await waitFor(() => {
      expect(screen.queryByTestId('status-form')).not.toBeInTheDocument()
    })
    expect(mocked.list).toHaveBeenCalledTimes(2)
  })

  it('sends the resource type of the tab that is open', async () => {
    const user = userEvent.setup()
    mocked.create.mockResolvedValue(custom())
    render(<Statuses team={adminTeam} />)
    await screen.findByText('In review')
    await user.click(screen.getByRole('tab', { name: 'Artifacts' }))
    await screen.findByText('Approved')

    await user.click(screen.getByTestId('create-status-button'))
    expect(
      screen.getByRole('heading', { name: 'Create artifact status' })
    ).toBeInTheDocument()
    await user.type(screen.getByTestId('status-name-input'), 'Blocked')
    await user.click(screen.getByTestId('submit-status-button'))

    await waitFor(() => {
      expect(mocked.create).toHaveBeenCalledWith(
        'team-1',
        expect.objectContaining({ resource_type: 'artifact' })
      )
    })
  })

  it.each([
    'slug is a built-in status and cannot be used',
    'A status with this slug already exists for this resource type',
  ])('puts the 409 message %j on the slug field', async message => {
    const user = userEvent.setup()
    mocked.create.mockRejectedValue(apiError(409, message))
    render(<Statuses team={adminTeam} />)

    await openCreate(user)
    await user.click(screen.getByTestId('submit-status-button'))

    expect(await screen.findByText(message)).toBeInTheDocument()
    expect(screen.getByTestId('status-slug-input')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
    // The dialog stays open on the error, and no toast duplicates it.
    expect(screen.getByTestId('status-form')).toBeInTheDocument()
    expect(mockHandleError).not.toHaveBeenCalled()
    expect(mockToastSuccess).not.toHaveBeenCalled()
  })

  it('shows the 400 for the 20-status limit as a form error', async () => {
    const user = userEvent.setup()
    const message =
      'a team can have at most 20 custom statuses per resource type'
    mocked.create.mockRejectedValue(apiError(400, message))
    render(<Statuses team={adminTeam} />)

    await openCreate(user)
    await user.click(screen.getByTestId('submit-status-button'))

    expect(await screen.findByTestId('status-form-error')).toHaveTextContent(
      message
    )
    expect(mockHandleError).not.toHaveBeenCalled()
  })

  it('hands any other failure to the shared error handler', async () => {
    const user = userEvent.setup()
    const failure = new Error('network down')
    mocked.create.mockRejectedValue(failure)
    render(<Statuses team={adminTeam} />)

    await openCreate(user)
    await user.click(screen.getByTestId('submit-status-button'))

    await waitFor(() => {
      expect(mockHandleError).toHaveBeenCalledWith(
        failure,
        'Failed to create status'
      )
    })
    expect(screen.queryByTestId('status-form-error')).not.toBeInTheDocument()
  })
})

describe('Statuses — rename', () => {
  const openRename = async (user: ReturnType<typeof userEvent.setup>) => {
    await screen.findByText('In review')
    await user.click(screen.getByTestId('rename-status-button'))
    const name = screen.getByTestId('status-name-input')
    await user.clear(name)
    await user.type(name, 'Under review')
  }

  it('renames by id, sending the name only, and reloads', async () => {
    const user = userEvent.setup()
    mocked.rename.mockResolvedValue(custom({ name: 'Under review' }))
    render(<Statuses team={adminTeam} />)

    await openRename(user)
    expect(screen.getByTestId('status-slug-input')).toHaveAttribute('readonly')
    await user.click(screen.getByTestId('submit-status-button'))

    await waitFor(() => {
      expect(mocked.rename).toHaveBeenCalledWith(
        'team-1',
        'status-1',
        'Under review'
      )
    })
    expect(mockToastSuccess).toHaveBeenCalledWith('Status renamed')
    await waitFor(() => {
      expect(screen.queryByTestId('status-form')).not.toBeInTheDocument()
    })
    expect(mocked.list).toHaveBeenCalledTimes(2)
  })

  it('shows a 400 as a form error and keeps the dialog open', async () => {
    const user = userEvent.setup()
    mocked.rename.mockRejectedValue(
      apiError(400, 'name must be at most 50 characters')
    )
    render(<Statuses team={adminTeam} />)

    await openRename(user)
    await user.click(screen.getByTestId('submit-status-button'))

    expect(await screen.findByTestId('status-form-error')).toHaveTextContent(
      'name must be at most 50 characters'
    )
    expect(mockHandleError).not.toHaveBeenCalled()
  })

  it('hands any other failure to the shared error handler, closes and reloads', async () => {
    const user = userEvent.setup()
    const failure = apiError(404, 'No custom status with this id')
    mocked.rename.mockRejectedValue(failure)
    render(<Statuses team={adminTeam} />)

    await openRename(user)
    await user.click(screen.getByTestId('submit-status-button'))

    await waitFor(() => {
      expect(mockHandleError).toHaveBeenCalledWith(
        failure,
        'Failed to rename status'
      )
    })
    // A 404 means the status is gone: the dialog closes and the list reloads.
    await waitFor(() => {
      expect(screen.queryByTestId('status-form')).not.toBeInTheDocument()
    })
    expect(mocked.list).toHaveBeenCalledTimes(2)
  })
})

describe('Statuses — delete', () => {
  it('states how many items move and where, then reports what the server moved', async () => {
    const user = userEvent.setup()
    mocked.remove.mockResolvedValue({
      reassigned_count: 12,
      reassigned_to: 'draft',
    })
    render(<Statuses team={adminTeam} />)
    await screen.findByText('In review')

    await user.click(screen.getByTestId('delete-status-button'))

    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent(
      'This will permanently delete In review. 12 prompts will move to Draft.'
    )

    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

    await waitFor(() => {
      expect(mocked.remove).toHaveBeenCalledWith('team-1', 'status-1')
    })
    expect(mockToastSuccess).toHaveBeenCalledWith(
      'Status deleted. 12 prompts moved to Draft'
    )
    // Mount, the refresh when the dialog opened, and the reload after.
    await waitFor(() => {
      expect(mocked.list).toHaveBeenCalledTimes(3)
    })
  })

  it('names Active as the destination outside prompts, and uses the singular for one item', async () => {
    const user = userEvent.setup()
    render(<Statuses team={adminTeam} />)
    await screen.findByText('In review')
    await user.click(screen.getByRole('tab', { name: 'Artifacts' }))
    await screen.findByText('Approved')

    await user.click(screen.getByTestId('delete-status-button'))

    expect(await screen.findByRole('alertdialog')).toHaveTextContent(
      '1 artifact will move to Active.'
    )
  })

  it('says nothing will move when the status is unused', async () => {
    const user = userEvent.setup()
    mocked.list.mockResolvedValue([
      system('prompt', 'draft', 'Draft'),
      custom({ usage_count: 0 }),
    ])
    mocked.remove.mockResolvedValue({
      reassigned_count: 0,
      reassigned_to: 'draft',
    })
    render(<Statuses team={adminTeam} />)
    await screen.findByText('In review')

    await user.click(screen.getByTestId('delete-status-button'))
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent('No prompts use it, so nothing will move.')

    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))
    await waitFor(() => {
      expect(mockToastSuccess).toHaveBeenCalledWith('Status deleted')
    })
  })

  // The count on the row can be stale; opening the dialog refetches it.
  it('shows the usage count as refreshed when the dialog opened', async () => {
    const user = userEvent.setup()
    mocked.list
      .mockResolvedValueOnce(promptStatuses)
      .mockResolvedValueOnce([
        system('prompt', 'draft', 'Draft'),
        custom({ usage_count: 40 }),
      ])
    render(<Statuses team={adminTeam} />)
    await screen.findByText('In review')

    await user.click(screen.getByTestId('delete-status-button'))

    const dialog = await screen.findByRole('alertdialog')
    await waitFor(() => {
      expect(dialog).toHaveTextContent('40 prompts will move to Draft.')
    })
  })

  it('reports a failed delete through the shared error handler and reloads', async () => {
    const user = userEvent.setup()
    const failure = apiError(404, 'No custom status with this id')
    mocked.remove.mockRejectedValue(failure)
    render(<Statuses team={adminTeam} />)
    await screen.findByText('In review')

    await user.click(screen.getByTestId('delete-status-button'))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

    await waitFor(() => {
      expect(mockHandleError).toHaveBeenCalledWith(
        failure,
        'Failed to delete status'
      )
    })
    expect(mockToastSuccess).not.toHaveBeenCalled()
    await waitFor(() => {
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    })
    expect(mocked.list).toHaveBeenCalledTimes(3)
  })

  it('deletes nothing when the dialog is cancelled', async () => {
    const user = userEvent.setup()
    render(<Statuses team={adminTeam} />)
    await screen.findByText('In review')

    await user.click(screen.getByTestId('delete-status-button'))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))

    await waitFor(() => {
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    })
    expect(mocked.remove).not.toHaveBeenCalled()
  })
})
