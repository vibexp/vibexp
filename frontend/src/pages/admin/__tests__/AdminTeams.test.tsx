/**
 * AdminTeams (#316): renders rows, navigates on row click, and shows the error
 * state (pagination shares ListPage.Footer, covered by AdminUsers).
 */
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import type { AdminTeamListResponse } from '@/services/adminService'

const mockNavigate = jest.fn()
jest.mock('react-router-dom', () => ({
  ...jest.requireActual<typeof import('react-router-dom')>('react-router-dom'),
  useNavigate: () => mockNavigate,
}))

jest.mock('@/services/adminService', () => ({
  adminService: { listTeams: jest.fn() },
}))

import { adminService } from '@/services/adminService'

import { AdminTeams } from '../AdminTeams'

const mockAdminService = adminService as jest.Mocked<typeof adminService>

const teamsPage: AdminTeamListResponse = {
  teams: [
    {
      id: 't1',
      name: 'Engineering',
      owner: { id: 'o1', email: 'owner@example.com', name: 'Owner' },
      member_count: 4,
      created_at: '2026-01-01T00:00:00Z',
    },
  ],
  total_count: 1,
  page: 1,
  per_page: 20,
  total_pages: 1,
}

function renderTeams() {
  return render(
    <MemoryRouter>
      <AdminTeams />
    </MemoryRouter>
  )
}

afterEach(() => {
  jest.clearAllMocks()
})

it('renders team rows with the owner email', async () => {
  mockAdminService.listTeams.mockResolvedValue(teamsPage)
  renderTeams()

  expect(await screen.findByText('Engineering')).toBeInTheDocument()
  expect(screen.getByText('owner@example.com')).toBeInTheDocument()
})

it('navigates to the detail page on row click', async () => {
  mockAdminService.listTeams.mockResolvedValue(teamsPage)
  renderTeams()

  await userEvent.click(await screen.findByText('Engineering'))

  expect(mockNavigate).toHaveBeenCalledWith('/admin/teams/t1')
})

it('shows an error state on failure', async () => {
  mockAdminService.listTeams.mockRejectedValue(new Error('boom'))
  renderTeams()

  expect(await screen.findByText('Failed to load teams')).toBeInTheDocument()
})
