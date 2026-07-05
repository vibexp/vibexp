import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { Type } from '@/services/typeService'

const mockUseTeam = jest.fn()
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

jest.mock('@/services/typeService', () => ({
  typeService: {
    getTypes: jest.fn(),
    createType: jest.fn(),
    deleteType: jest.fn(),
  },
}))

const mockHandleError = jest.fn(() => ({}))
jest.mock('@/hooks/useErrorHandler', () => ({
  useErrorHandler: () => ({ handleError: mockHandleError }),
}))

const mockToastSuccess = jest.fn()
jest.mock('@/lib/toast', () => ({
  toast: {
    success: (...a: unknown[]) => mockToastSuccess(...a),
    error: jest.fn(),
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

const mockedService = typeService as jest.Mocked<typeof typeService>

beforeEach(() => {
  jest.clearAllMocks()
  mockUseTeam.mockReturnValue({
    currentTeam: { id: 'team-1', name: 'Test Team' },
    teams: [{ id: 'team-1', name: 'Test Team' }],
    isLoading: false,
    setCurrentTeam: jest.fn(),
    refreshTeams: jest.fn(),
  })
  mockedService.getTypes.mockResolvedValue([systemType, customType])
})

it('lists system and custom types; only custom types are deletable', async () => {
  render(<Customization />)

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
  render(<Customization />)

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
