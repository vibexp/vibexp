import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { toast } from '@/lib/toast'
import type { Team } from '@/services/teamService'
import { teamService } from '@/services/teamService'
import type { APIErrorResponse } from '@/types/errors'
import { ApiError } from '@/types/errors'

import { DeleteTeamModal } from './DeleteTeamModal'

jest.mock('@/services/teamService', () => ({
  teamService: {
    deleteTeam: jest.fn(),
  },
}))

jest.mock('@/lib/toast', () => ({
  toast: {
    success: jest.fn(),
    error: jest.fn(),
  },
}))

const mockedDeleteTeam = teamService.deleteTeam as jest.MockedFunction<
  typeof teamService.deleteTeam
>
const mockedToastError = toast.error as jest.MockedFunction<typeof toast.error>
const mockedToastSuccess = toast.success as jest.MockedFunction<
  typeof toast.success
>

const makeTeam = (overrides: Partial<Team> = {}): Team => ({
  id: 'team-1',
  owner_id: 'owner-1',
  name: 'Acme',
  slug: 'acme',
  description: '',
  member_count: 1,
  is_personal: false,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  ...overrides,
})

const buildApiError = (
  status: number,
  overrides: Partial<APIErrorResponse> = {}
): ApiError =>
  new ApiError({
    type: 'https://api.vibexp.io/errors/test',
    title: 'Conflict',
    status,
    detail: 'detail',
    code: 'TEST',
    request_id: 'req-1',
    timestamp: '2024-01-01T00:00:00Z',
    ...overrides,
  })

const renderModal = (team: Team = makeTeam()) => {
  const onClose = jest.fn()
  const onSuccess = jest.fn()
  render(
    <DeleteTeamModal
      isOpen
      team={team}
      onClose={onClose}
      onSuccess={onSuccess}
    />
  )
  return { onClose, onSuccess }
}

const clickDelete = async () => {
  await userEvent.click(screen.getByTestId('confirm-delete-team-button'))
}

describe('DeleteTeamModal', () => {
  beforeEach(() => {
    jest.clearAllMocks()
  })

  it('deletes the team and reports success on the happy path', async () => {
    mockedDeleteTeam.mockResolvedValueOnce(undefined)
    const { onClose, onSuccess } = renderModal()

    await clickDelete()

    await waitFor(() => {
      expect(mockedDeleteTeam).toHaveBeenCalledWith('team-1')
    })
    expect(mockedToastSuccess).toHaveBeenCalledWith('Team deleted successfully')
    expect(onSuccess).toHaveBeenCalledTimes(1)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('renders the member-count message for TEAM_HAS_MEMBERS', async () => {
    mockedDeleteTeam.mockRejectedValueOnce(
      buildApiError(409, {
        code: 'TEAM_HAS_MEMBERS',
        detail: 'Team has members',
      })
    )
    renderModal(makeTeam({ member_count: 3 }))

    await clickDelete()

    expect(
      await screen.findByText(/remove all team members before deleting/i)
    ).toBeInTheDocument()
    expect(mockedToastError).not.toHaveBeenCalled()
  })

  it('falls back to a toast for an unknown error code', async () => {
    mockedDeleteTeam.mockRejectedValueOnce(
      buildApiError(500, {
        code: 'UNKNOWN_ERROR',
        detail: 'Something broke',
      })
    )
    renderModal()

    await clickDelete()

    await waitFor(() => {
      expect(mockedToastError).toHaveBeenCalledWith('Something broke')
    })
    expect(screen.queryByText('Cannot Delete Team')).not.toBeInTheDocument()
  })

  it('falls back to a toast for non-ApiError failures', async () => {
    mockedDeleteTeam.mockRejectedValueOnce(new Error('network down'))
    renderModal()

    await clickDelete()

    await waitFor(() => {
      expect(mockedToastError).toHaveBeenCalledWith('network down')
    })
  })
})
