import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

import type { Team } from '@/services/teamService'

import { TeamSettings } from '../TeamSettings'

const team: Team = {
  id: 'team-a',
  owner_id: 'owner-1',
  name: 'Engineering',
  slug: 'engineering',
  description: '',
  is_personal: false,
  permissions: [],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

function renderHub() {
  return render(
    <MemoryRouter>
      <TeamSettings team={team} />
    </MemoryRouter>
  )
}

describe('TeamSettings', () => {
  it('names the team being configured', () => {
    renderHub()
    // The AC's point: a user landing here must be able to tell WHICH team they
    // are configuring, since the hub looks identical to the personal one.
    expect(
      screen.getByRole('heading', { level: 1, name: /team settings/i })
    ).toBeInTheDocument()
    expect(screen.getByText(/Engineering/)).toBeInTheDocument()
  })

  it('explains the empty state while pages are still being relocated', () => {
    // Ships with zero cards by design: #540 and #541 move the team-scoped
    // settings pages here. Replace this with card assertions when they do.
    renderHub()
    expect(
      screen.getByText(/no team settings have moved here yet/i)
    ).toBeInTheDocument()
    expect(screen.queryAllByRole('button')).toHaveLength(0)
  })
})
