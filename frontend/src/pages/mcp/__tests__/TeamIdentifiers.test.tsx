import { fireEvent, render, screen, waitFor } from '@testing-library/react'

import type { Team } from '@/types/team'

import { TeamIdentifiers } from '../TeamIdentifiers'

// Mock clipboard API. CopyButton calls navigator.clipboard.writeText directly.
const writeText = jest.fn().mockResolvedValue(undefined)
Object.assign(navigator, { clipboard: { writeText } })

function makeTeam(overrides: Partial<Team> = {}): Team {
  return {
    id: 'uuid-aaa',
    name: 'Acme Team',
    slug: 'acme-team',
    description: '',
    member_count: 1,
    is_personal: false,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('TeamIdentifiers', () => {
  beforeEach(() => {
    writeText.mockClear()
  })

  it('renders each team name with its UUID and slug', () => {
    const teams = [
      makeTeam({ id: 'uuid-aaa', name: 'Acme Team', slug: 'acme-team' }),
      makeTeam({ id: 'uuid-bbb', name: 'Beta Squad', slug: 'beta-squad' }),
    ]

    render(<TeamIdentifiers teams={teams} />)

    expect(screen.getByText('Acme Team')).toBeInTheDocument()
    expect(screen.getByText('uuid-aaa')).toBeInTheDocument()
    expect(screen.getByText('acme-team')).toBeInTheDocument()
    expect(screen.getByText('Beta Squad')).toBeInTheDocument()
    expect(screen.getByText('uuid-bbb')).toBeInTheDocument()
    expect(screen.getByText('beta-squad')).toBeInTheDocument()
  })

  it('renders an empty-state hint and no copy rows when there are no teams', () => {
    render(<TeamIdentifiers teams={[]} />)

    expect(
      screen.getByText(/don.t belong to any teams yet/i)
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /^Copy / })
    ).not.toBeInTheDocument()
  })

  it('copies the UUID when its copy button is clicked', async () => {
    render(<TeamIdentifiers teams={[makeTeam({ id: 'uuid-aaa' })]} />)

    fireEvent.click(
      screen.getByRole('button', { name: 'Copy UUID for uuid-aaa' })
    )

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith('uuid-aaa')
    })
  })

  it('copies the slug when its copy button is clicked', async () => {
    render(<TeamIdentifiers teams={[makeTeam({ slug: 'acme-team' })]} />)

    fireEvent.click(
      screen.getByRole('button', { name: 'Copy Slug for acme-team' })
    )

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith('acme-team')
    })
  })
})
