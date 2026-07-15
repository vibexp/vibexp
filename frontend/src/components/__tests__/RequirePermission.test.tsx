import { render, screen } from '@testing-library/react'

import { RequirePermission } from '@/components/RequirePermission'
import type { Team } from '@/services/teamService'

const mockUseTeam = jest.fn()
jest.mock('@/contexts/TeamContext', () => ({
  useTeam: () => mockUseTeam(),
}))

jest.mock('@/contexts/useAuth', () => ({
  useAuth: () => ({ user: { id: 'user-1' } }),
}))

const teamWith = (permissions: Team['permissions']) =>
  ({
    id: 'team-1',
    owner_id: 'owner-1',
    name: 'Team',
    slug: 'team',
    description: '',
    is_personal: false,
    permissions,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  }) satisfies Team

describe('RequirePermission', () => {
  it('renders children when the team grants the permission', () => {
    mockUseTeam.mockReturnValue({ currentTeam: teamWith(['project.create']) })

    render(
      <RequirePermission permission="project.create">
        <button>New project</button>
      </RequirePermission>
    )

    expect(
      screen.getByRole('button', { name: 'New project' })
    ).toBeInTheDocument()
  })

  it('renders nothing when the permission is absent', () => {
    mockUseTeam.mockReturnValue({ currentTeam: teamWith(['resource.create']) })

    render(
      <RequirePermission permission="project.create">
        <button>New project</button>
      </RequirePermission>
    )

    expect(
      screen.queryByRole('button', { name: 'New project' })
    ).not.toBeInTheDocument()
  })

  it('renders the fallback instead when one is given', () => {
    mockUseTeam.mockReturnValue({ currentTeam: teamWith([]) })

    render(
      <RequirePermission
        permission="project.create"
        fallback={<span>Ask an admin</span>}
      >
        <button>New project</button>
      </RequirePermission>
    )

    expect(screen.getByText('Ask an admin')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'New project' })
    ).not.toBeInTheDocument()
  })

  it('fails closed while no team is loaded', () => {
    mockUseTeam.mockReturnValue({ currentTeam: null })

    render(
      <RequirePermission permission="project.create">
        <button>New project</button>
      </RequirePermission>
    )

    expect(
      screen.queryByRole('button', { name: 'New project' })
    ).not.toBeInTheDocument()
  })
})
