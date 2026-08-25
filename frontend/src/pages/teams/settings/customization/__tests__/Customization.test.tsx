import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Mocked } from 'vitest'

import type { Team } from '@/services/teamService'
import type { Type } from '@/services/typeService'

// The team TeamScopeLayout resolved from the URL (#584). `permissions` is what
// usePermissions reads — it is deliberately NOT mocked, so gating is exercised
// through the real hook against this fixture.
const urlTeam = {
  id: 'team-1',
  name: 'Test Team',
  permissions: ['resource.create'],
} as unknown as Team

const otherTeam = {
  id: 'team-2',
  name: 'Other Team',
  permissions: ['resource.create'],
} as unknown as Team

const mockUseTeam = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

vi.mock('@/services/typeService', () => ({
  typeService: {
    getTypes: vi.fn(),
    createType: vi.fn(),
    deleteType: vi.fn(),
    copyTypesFromTeam: vi.fn(),
  },
}))

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

import { typeService } from '@/services/typeService'

import { Customization } from '../Customization'

const systemType: Type = {
  id: 'type-sys',
  resource_type: 'artifacts',
  slug: 'general',
  name: 'General',
  is_system: true,
  created_at: '2026-06-15T09:00:00Z',
}

const customType: Type = {
  id: 'type-custom',
  team_id: 'team-1',
  resource_type: 'artifacts',
  slug: 'bug-report',
  name: 'Bug report',
  is_system: false,
  created_at: '2026-06-15T10:00:00Z',
}

const mockedService = typeService as Mocked<typeof typeService>

// cmdk (inside the copy dialog's popover) needs APIs jsdom does not provide.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
})

beforeEach(() => {
  vi.clearAllMocks()
  mockUseTeam.mockReturnValue({
    currentTeam: urlTeam,
    teams: [urlTeam, otherTeam],
    isLoading: false,
    setCurrentTeam: vi.fn(),
    refreshTeams: vi.fn(),
  })
  mockedService.getTypes.mockResolvedValue([systemType, customType])
})

it('lists system and custom types; only custom types are deletable', async () => {
  render(<Customization team={urlTeam} />)

  expect(await screen.findByText('General')).toBeInTheDocument()
  expect(screen.getByText('Bug report')).toBeInTheDocument()
  expect(mockedService.getTypes).toHaveBeenCalledWith('team-1', 'artifacts')

  // Exactly one delete button — the custom type (system default is read-only).
  const deleteButtons = screen.getAllByTestId('delete-type-button')
  expect(deleteButtons).toHaveLength(1)
  expect(deleteButtons[0]).toHaveAttribute('aria-label', 'Delete Bug report')
})

it('deletes a custom type and reloads, surfacing the reassignment in the confirm copy', async () => {
  mockedService.deleteType.mockResolvedValue()
  const user = userEvent.setup()
  render(<Customization team={urlTeam} />)

  await screen.findByText('Bug report')
  await user.click(screen.getByTestId('delete-type-button'))

  const dialog = await screen.findByRole('alertdialog')
  expect(within(dialog).getByText(/moved to/i)).toBeInTheDocument()

  await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

  await waitFor(() => {
    expect(mockedService.deleteType).toHaveBeenCalledWith(
      'team-1',
      'type-custom'
    )
  })
  expect(mockToastSuccess).toHaveBeenCalled()
  // List reloaded: initial mount + after delete.
  expect(mockedService.getTypes).toHaveBeenCalledTimes(2)
})

// ---------------------------------------------------------------------------
// #584 — cold deep-link: act on the URL's team, never the ambient one
// ---------------------------------------------------------------------------
// React fires child effects before parent effects, so on a cold deep-link to
// /teams/B/... this page's load effect runs BEFORE TeamScopeLayout's
// setCurrentTeam sync and the ambient team is still A. These tests pin that the
// page reads the team from its prop by driving the two apart.

describe('cold deep-link (#584)', () => {
  it('fetches artifact types for the URL team, never the ambient one', async () => {
    mockUseTeam.mockReturnValue({
      currentTeam: { id: 'team-a', name: 'Team A' },
      teams: [],
    })
    const urlTeamB = { id: 'team-b', name: 'Team B' } as unknown as Team

    render(<Customization team={urlTeamB} />)

    await waitFor(() => {
      expect(mockedService.getTypes).toHaveBeenCalledWith('team-b', 'artifacts')
    })
    expect(mockedService.getTypes).not.toHaveBeenCalledWith(
      'team-a',
      'artifacts'
    )
  })
})

// ---------------------------------------------------------------------------
// #833 — copy artifact types from another team
// ---------------------------------------------------------------------------

describe('copy from another team (#833)', () => {
  const sourceCustomType: Type = {
    id: 'type-source',
    team_id: 'team-2',
    resource_type: 'artifacts',
    slug: 'design-doc',
    name: 'Design doc',
    is_system: false,
    created_at: '2026-06-15T11:00:00Z',
  }

  // Same slug as the destination's existing custom type — the server would
  // report this one as skipped.
  const sourceCollidingType: Type = {
    ...customType,
    id: 'type-source-collide',
    team_id: 'team-2',
  }

  const openCopyDialog = async (user: ReturnType<typeof userEvent.setup>) => {
    await screen.findByText('Bug report')
    await user.click(screen.getByTestId('copy-types-button'))
    await user.click(await screen.findByTestId('source-team-picker'))
    await user.click(await screen.findByText('Other Team'))
  }

  it('hides the entry point without the permission the server requires', async () => {
    const noPermTeam = {
      id: 'team-1',
      name: 'Test Team',
      permissions: [],
    } as unknown as Team

    render(<Customization team={noPermTeam} />)

    await screen.findByText('Bug report')
    expect(screen.queryByTestId('copy-types-button')).not.toBeInTheDocument()
  })

  it('hides the entry point when there is no other team to copy from', async () => {
    mockUseTeam.mockReturnValue({
      currentTeam: urlTeam,
      teams: [urlTeam],
      isLoading: false,
      setCurrentTeam: vi.fn(),
      refreshTeams: vi.fn(),
    })

    render(<Customization team={urlTeam} />)

    await screen.findByText('Bug report')
    expect(screen.queryByTestId('copy-types-button')).not.toBeInTheDocument()
  })

  it('previews what the copy will add and what already exists', async () => {
    const user = userEvent.setup()
    mockedService.getTypes.mockImplementation((teamId: string) =>
      Promise.resolve(
        teamId === 'team-2'
          ? [systemType, sourceCustomType, sourceCollidingType]
          : [systemType, customType]
      )
    )

    render(<Customization team={urlTeam} />)
    await openCopyDialog(user)

    const preview = await screen.findByTestId('copy-preview')
    // "Design doc" is new; "Bug report" collides; the system default is never
    // part of the source set.
    expect(preview).toHaveTextContent(
      '1 type will be added, 1 already exists here and will be skipped.'
    )
    const added = within(preview).getAllByTestId('copy-preview-add')
    expect(added).toHaveLength(1)
    expect(added[0]).toHaveTextContent('Design doc')
    expect(mockedService.getTypes).toHaveBeenCalledWith('team-2', 'artifacts')
  })

  it('copies from the chosen team and reloads the destination list', async () => {
    const user = userEvent.setup()
    mockedService.getTypes.mockImplementation((teamId: string) =>
      Promise.resolve(
        teamId === 'team-2' ? [sourceCustomType] : [systemType, customType]
      )
    )
    mockedService.copyTypesFromTeam.mockResolvedValue({
      added: [sourceCustomType],
      skipped: [],
      added_count: 1,
      skipped_count: 0,
    })

    render(<Customization team={urlTeam} />)
    await openCopyDialog(user)

    await user.click(await screen.findByTestId('confirm-copy-from-team-button'))

    await waitFor(() => {
      expect(mockedService.copyTypesFromTeam).toHaveBeenCalledWith(
        'team-1',
        'team-2'
      )
    })
    expect(mockToastSuccess).toHaveBeenCalledWith(
      expect.stringContaining('Other Team')
    )
    // Destination list reloaded: mount + after the copy (the source fetch for
    // the preview is a different team).
    await waitFor(() => {
      expect(
        mockedService.getTypes.mock.calls.filter(([id]) => id === 'team-1')
      ).toHaveLength(2)
    })
  })

  it('copies into the URL team, never the ambient one', async () => {
    const user = userEvent.setup()
    const urlTeamB = {
      id: 'team-b',
      name: 'Team B',
      permissions: ['resource.create'],
    } as unknown as Team
    mockUseTeam.mockReturnValue({
      currentTeam: urlTeam,
      teams: [urlTeam, otherTeam, urlTeamB],
      isLoading: false,
      setCurrentTeam: vi.fn(),
      refreshTeams: vi.fn(),
    })
    mockedService.getTypes.mockResolvedValue([customType])
    mockedService.copyTypesFromTeam.mockResolvedValue({
      added: [],
      skipped: [],
      added_count: 0,
      skipped_count: 0,
    })

    render(<Customization team={urlTeamB} />)

    await screen.findByText('Bug report')
    await user.click(screen.getByTestId('copy-types-button'))
    await user.click(await screen.findByTestId('source-team-picker'))
    await user.click(await screen.findByText('Other Team'))
    await user.click(await screen.findByTestId('confirm-copy-from-team-button'))

    await waitFor(() => {
      expect(mockedService.copyTypesFromTeam).toHaveBeenCalledWith(
        'team-b',
        'team-2'
      )
    })
    expect(mockedService.copyTypesFromTeam).not.toHaveBeenCalledWith(
      'team-1',
      'team-2'
    )
  })
})
