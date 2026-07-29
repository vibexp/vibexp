import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Mocked } from 'vitest'

import type { Team } from '@/services/teamService'
import type { Type } from '@/services/typeService'

// The team TeamScopeLayout resolved from the URL (#584).
const urlTeam = { id: 'team-1', name: 'Test Team' } as unknown as Team

const mockUseTeam = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

vi.mock('@/services/typeService', () => ({
  typeService: {
    getTypes: vi.fn(),
    createType: vi.fn(),
    deleteType: vi.fn(),
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

beforeEach(() => {
  vi.clearAllMocks()
  mockUseTeam.mockReturnValue({
    currentTeam: { id: 'team-1', name: 'Test Team' },
    teams: [{ id: 'team-1', name: 'Test Team' }],
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
