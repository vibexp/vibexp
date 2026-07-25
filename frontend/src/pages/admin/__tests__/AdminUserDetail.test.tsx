/**
 * AdminUserDetail (#459): the four mutation flows and the safety behaviour around
 * the portal's only destructive action.
 */
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import type { AdminUserDetail as AdminUserDetailType } from '@/services/adminService'

const mockNavigate = jest.fn()
jest.mock('react-router-dom', () => ({
  ...jest.requireActual<typeof import('react-router-dom')>('react-router-dom'),
  useNavigate: () => mockNavigate,
}))

const mockUseAuth = jest.fn()
jest.mock('@/contexts/useAuth', () => ({
  useAuth: () => mockUseAuth(),
}))

jest.mock('@/services/adminService', () => ({
  adminService: {
    getUser: jest.fn(),
    updateUser: jest.fn(),
    suspendUser: jest.fn(),
    reactivateUser: jest.fn(),
    deleteUser: jest.fn(),
  },
}))

jest.mock('sonner', () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}))

import { toast } from 'sonner'

import { adminService } from '@/services/adminService'

import { AdminUserDetail } from '../AdminUserDetail'

const mockAdminService = adminService as jest.Mocked<typeof adminService>

beforeAll(() => {
  Element.prototype.scrollIntoView = jest.fn()
  Element.prototype.hasPointerCapture = jest.fn()
  Element.prototype.releasePointerCapture = jest.fn()
})

function user(
  overrides: Partial<AdminUserDetailType> = {}
): AdminUserDetailType {
  return {
    id: 'u1',
    email: 'ada@example.com',
    name: 'Ada',
    idp_provider: 'google',
    status: 'active',
    created_at: '2026-01-01T00:00:00Z',
    memberships: [
      { team_id: 't1', team_name: 'Engineering', role: 'owner' },
      { team_id: 't2', team_name: 'Design', role: 'member' },
    ],
    ...overrides,
  }
}

function renderDetail(id = 'u1') {
  return render(
    <MemoryRouter initialEntries={[`/admin/users/${id}`]}>
      <Routes>
        <Route path="/admin/users/:id" element={<AdminUserDetail />} />
      </Routes>
    </MemoryRouter>
  )
}

beforeEach(() => {
  jest.clearAllMocks()
  mockUseAuth.mockReturnValue({
    user: {
      id: 'admin-1',
      email: 'admin@example.com',
      is_instance_admin: true,
    },
    isLoading: false,
  })
  mockAdminService.getUser.mockResolvedValue(user())
})

it('renders the profile, status and memberships', async () => {
  renderDetail()

  expect(
    await screen.findByRole('heading', { name: 'Ada' })
  ).toBeInTheDocument()
  expect(screen.getByText('ada@example.com')).toBeInTheDocument()
  expect(screen.getByText('Active')).toBeInTheDocument()
  expect(screen.getByText('Engineering')).toBeInTheDocument()
  expect(screen.getByText('Design')).toBeInTheDocument()
})

it('badges a suspended account and flips the action', async () => {
  mockAdminService.getUser.mockResolvedValue(user({ status: 'suspended' }))
  renderDetail()

  expect(await screen.findByText('Suspended')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /Reactivate/ })).toBeInTheDocument()
})

describe('editing the name', () => {
  it('saves and shows the updated user', async () => {
    mockAdminService.updateUser.mockResolvedValue(
      user({ name: 'Ada Lovelace' })
    )
    renderDetail()
    await screen.findByRole('heading', { name: 'Ada' })

    await userEvent.click(screen.getByRole('button', { name: /Edit/ }))
    const nameInput = await screen.findByLabelText('Name')
    await userEvent.clear(nameInput)
    await userEvent.type(nameInput, 'Ada Lovelace')
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => {
      expect(mockAdminService.updateUser).toHaveBeenCalledWith('u1', {
        name: 'Ada Lovelace',
      })
    })
    expect(
      await screen.findByRole('heading', { name: 'Ada Lovelace' })
    ).toBeInTheDocument()
  })

  it('offers no email field, because the API cannot change it', async () => {
    renderDetail()
    await screen.findByRole('heading', { name: 'Ada' })

    await userEvent.click(screen.getByRole('button', { name: /Edit/ }))

    await screen.findByLabelText('Name')
    expect(screen.queryByLabelText('Email')).not.toBeInTheDocument()
  })

  it('shows a save failure inline and keeps the dialog open', async () => {
    mockAdminService.updateUser.mockRejectedValue(new Error('name too long'))
    renderDetail()
    await screen.findByRole('heading', { name: 'Ada' })
    await userEvent.click(screen.getByRole('button', { name: /Edit/ }))
    const nameInput = await screen.findByLabelText('Name')
    await userEvent.type(nameInput, '!')

    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    expect(await screen.findByText('name too long')).toBeInTheDocument()
    // Still open, so the admin can correct it rather than start over.
    expect(screen.getByLabelText('Name')).toBeInTheDocument()
  })

  it('will not submit an empty name', async () => {
    renderDetail()
    await screen.findByRole('heading', { name: 'Ada' })
    await userEvent.click(screen.getByRole('button', { name: /Edit/ }))
    const nameInput = await screen.findByLabelText('Name')

    await userEvent.clear(nameInput)

    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  })
})

describe('suspend and reactivate', () => {
  it('suspends after confirmation, spelling out what stops working', async () => {
    mockAdminService.suspendUser.mockResolvedValue(
      user({ status: 'suspended' })
    )
    renderDetail()
    await screen.findByRole('heading', { name: 'Ada' })

    await userEvent.click(screen.getByRole('button', { name: /Suspend/ }))

    // The copy is a safety control: an admin needs to know sessions and API keys
    // die immediately, not at expiry.
    expect(
      await screen.findByText(/stop working immediately, not at expiry/)
    ).toBeInTheDocument()
    expect(mockAdminService.suspendUser).not.toHaveBeenCalled()

    await userEvent.click(screen.getByRole('button', { name: 'Suspend' }))

    await waitFor(() => {
      expect(mockAdminService.suspendUser).toHaveBeenCalledWith('u1')
    })
    expect(await screen.findByText('Suspended')).toBeInTheDocument()
    expect(toast.success).toHaveBeenCalledWith('User suspended')
  })

  it('reactivates a suspended account', async () => {
    mockAdminService.getUser.mockResolvedValue(user({ status: 'suspended' }))
    mockAdminService.reactivateUser.mockResolvedValue(user())
    renderDetail()
    await screen.findByText('Suspended')

    await userEvent.click(screen.getByRole('button', { name: /Reactivate/ }))
    await userEvent.click(screen.getByRole('button', { name: 'Reactivate' }))

    await waitFor(() => {
      expect(mockAdminService.reactivateUser).toHaveBeenCalledWith('u1')
    })
    expect(await screen.findByText('Active')).toBeInTheDocument()
  })

  it('surfaces the server refusal when suspending is rejected', async () => {
    // The client cannot tell a config-listed instance admin apart, so this arm is
    // how that case reaches the admin at all.
    mockAdminService.suspendUser.mockRejectedValue(
      new Error('instance admins cannot be suspended')
    )
    renderDetail()
    await screen.findByRole('heading', { name: 'Ada' })
    await userEvent.click(screen.getByRole('button', { name: /Suspend/ }))

    await userEvent.click(screen.getByRole('button', { name: 'Suspend' }))

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        'instance admins cannot be suspended'
      )
    })
    // Not silently marked suspended.
    expect(screen.getByText('Active')).toBeInTheDocument()
  })
})

describe('deleting', () => {
  const openDeleteDialog = async () => {
    await userEvent.click(screen.getByRole('button', { name: /Delete/ }))
    return screen.findByRole('dialog')
  }

  it('names the user and what will be removed before deleting anything', async () => {
    renderDetail()
    await screen.findByRole('heading', { name: 'Ada' })

    await openDeleteDialog()

    expect(screen.getByText('Delete Ada?')).toBeInTheDocument()
    expect(
      screen.getByText(/personal workspace and everything in it/)
    ).toBeInTheDocument()
    // The membership count comes from the data, not a guess.
    expect(screen.getByText(/2 teams/)).toBeInTheDocument()
    expect(mockAdminService.deleteUser).not.toHaveBeenCalled()
  })

  it('offers suspension as the reversible alternative', async () => {
    renderDetail()
    await screen.findByRole('heading', { name: 'Ada' })
    await openDeleteDialog()

    await userEvent.click(
      screen.getByRole('button', { name: 'Suspend instead' })
    )

    // Hands off to the suspend confirmation rather than deleting.
    expect(
      await screen.findByText(/stop working immediately, not at expiry/)
    ).toBeInTheDocument()
    expect(mockAdminService.deleteUser).not.toHaveBeenCalled()
  })

  it('deletes and returns to the list', async () => {
    mockAdminService.deleteUser.mockResolvedValue({ deleted: true })
    renderDetail()
    await screen.findByRole('heading', { name: 'Ada' })
    await openDeleteDialog()

    await userEvent.click(
      screen.getByRole('button', { name: 'Delete permanently' })
    )

    await waitFor(() => {
      expect(mockAdminService.deleteUser).toHaveBeenCalledWith('u1')
    })
    expect(mockNavigate).toHaveBeenCalledWith('/admin/users')
    expect(toast.success).toHaveBeenCalledWith('User deleted')
  })

  it('renders the blockers on a 409 and attempts nothing further', async () => {
    mockAdminService.deleteUser.mockResolvedValue({
      deleted: false,
      refusal: {
        message: 'This user owns shared teams with other members.',
        blockers: [
          { team_id: 't1', team_name: 'Acme Engineering', member_count: 4 },
          { team_id: 't9', team_name: 'Platform Guild', member_count: 12 },
        ],
      },
    })
    renderDetail()
    await screen.findByRole('heading', { name: 'Ada' })
    await openDeleteDialog()

    await userEvent.click(
      screen.getByRole('button', { name: 'Delete permanently' })
    )

    expect(
      await screen.findByText('Cannot delete this user')
    ).toBeInTheDocument()
    // Nothing was deleted, and the blocking teams are named with their sizes so
    // the admin knows what to transfer.
    expect(screen.getByText(/Nothing was deleted/)).toBeInTheDocument()
    expect(screen.getByText('Acme Engineering')).toBeInTheDocument()
    expect(screen.getByText('4 members')).toBeInTheDocument()
    expect(screen.getByText('Platform Guild')).toBeInTheDocument()
    expect(screen.getByText('12 members')).toBeInTheDocument()
    // No destructive button left to press, and no second attempt was made.
    expect(
      screen.queryByRole('button', { name: 'Delete permanently' })
    ).not.toBeInTheDocument()
    expect(mockAdminService.deleteUser).toHaveBeenCalledTimes(1)
    expect(mockNavigate).not.toHaveBeenCalledWith('/admin/users')
  })

  it('forgets a refusal when the dialog is reopened', async () => {
    mockAdminService.deleteUser.mockResolvedValue({
      deleted: false,
      refusal: { message: 'Blocked.', blockers: [] },
    })
    renderDetail()
    await screen.findByRole('heading', { name: 'Ada' })
    await openDeleteDialog()
    await userEvent.click(
      screen.getByRole('button', { name: 'Delete permanently' })
    )
    await screen.findByText('Cannot delete this user')

    await userEvent.click(screen.getByRole('button', { name: 'Done' }))
    await openDeleteDialog()

    // Back to the confirm state — a stale refusal would leave the dialog
    // permanently unusable after one blocked attempt.
    expect(await screen.findByText('Delete Ada?')).toBeInTheDocument()
  })
})

describe('acting on yourself', () => {
  beforeEach(() => {
    mockUseAuth.mockReturnValue({
      user: { id: 'u1', email: 'ada@example.com', is_instance_admin: true },
      isLoading: false,
    })
  })

  it('disables suspend and delete, and says why', async () => {
    renderDetail()
    await screen.findByRole('heading', { name: 'Ada' })

    expect(screen.getByRole('button', { name: /Suspend/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: /Delete/ })).toBeDisabled()
    expect(
      screen.getByText('You cannot suspend or delete your own account.')
    ).toBeInTheDocument()
  })

  it('still allows editing your own name', async () => {
    renderDetail()
    await screen.findByRole('heading', { name: 'Ada' })

    // Editing is not destructive and the server permits it.
    expect(screen.getByRole('button', { name: /Edit/ })).toBeEnabled()
  })
})

it('shows the error state for an unknown id', async () => {
  mockAdminService.getUser.mockRejectedValue(new Error('404'))
  renderDetail('missing')

  expect(await screen.findByText('Failed to load user')).toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: /Delete/ })
  ).not.toBeInTheDocument()
})
