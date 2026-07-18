/**
 * AdminUsers (#316): renders rows, navigates on row click, paginates via the
 * footer, and shows empty/error states.
 */
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import type { AdminUserListResponse } from '@/services/adminService'

const mockNavigate = jest.fn()
jest.mock('react-router-dom', () => ({
  ...jest.requireActual<typeof import('react-router-dom')>('react-router-dom'),
  useNavigate: () => mockNavigate,
}))

jest.mock('@/services/adminService', () => ({
  adminService: { listUsers: jest.fn() },
}))

import { adminService } from '@/services/adminService'

import { AdminUsers } from '../AdminUsers'

const mockAdminService = adminService as jest.Mocked<typeof adminService>

function page(
  overrides: Partial<AdminUserListResponse>
): AdminUserListResponse {
  return {
    users: [
      {
        id: 'u1',
        email: 'alice@example.com',
        name: 'Alice',
        idp_provider: 'google',
        created_at: '2026-01-01T00:00:00Z',
        team_count: 2,
      },
    ],
    total_count: 1,
    page: 1,
    per_page: 20,
    total_pages: 1,
    ...overrides,
  }
}

function renderUsers() {
  return render(
    <MemoryRouter>
      <AdminUsers />
    </MemoryRouter>
  )
}

afterEach(() => {
  jest.clearAllMocks()
})

it('renders user rows', async () => {
  mockAdminService.listUsers.mockResolvedValue(page({}))
  renderUsers()

  expect(await screen.findByText('alice@example.com')).toBeInTheDocument()
  expect(screen.getByText('Alice')).toBeInTheDocument()
})

it('navigates to the detail page on row click', async () => {
  mockAdminService.listUsers.mockResolvedValue(page({}))
  renderUsers()

  await userEvent.click(await screen.findByText('alice@example.com'))

  expect(mockNavigate).toHaveBeenCalledWith('/admin/users/u1')
})

it('requests the next page when Next is clicked', async () => {
  mockAdminService.listUsers.mockResolvedValue(page({ total_pages: 3 }))
  renderUsers()

  await userEvent.click(await screen.findByRole('button', { name: 'Next' }))

  await waitFor(() => {
    expect(mockAdminService.listUsers).toHaveBeenCalledWith(2, 20)
  })
})

it('shows the empty state when there are no users', async () => {
  mockAdminService.listUsers.mockResolvedValue(
    page({ users: [], total_count: 0, total_pages: 0 })
  )
  renderUsers()

  expect(await screen.findByText('No users yet')).toBeInTheDocument()
})

it('shows an error state on failure', async () => {
  mockAdminService.listUsers.mockRejectedValue(new Error('boom'))
  renderUsers()

  expect(await screen.findByText('Failed to load users')).toBeInTheDocument()
})
