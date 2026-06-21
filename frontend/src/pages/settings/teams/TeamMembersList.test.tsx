import { render, screen } from '@testing-library/react'

import type { TeamMember } from '@/types/team'

import { TeamMembersList } from './TeamMembersList'

const makeMember = (overrides: Partial<TeamMember> = {}): TeamMember => ({
  user_id: 'user-1',
  email: 'alice@example.com',
  name: 'Alice',
  role: 'member',
  joined_at: '2024-01-01T00:00:00Z',
  ...overrides,
})

describe('TeamMembersList', () => {
  it('renders the empty state when there are no members', () => {
    render(<TeamMembersList members={[]} isOwner={false} />)
    expect(screen.getByText(/no members found/i)).toBeInTheDocument()
  })

  it('renders an Accepted badge for members without an invitation_status (legacy active members)', () => {
    render(<TeamMembersList members={[makeMember()]} isOwner />)
    // Two badges per row: role + status. We only assert the status one.
    expect(screen.getByText('Accepted')).toBeInTheDocument()
    expect(screen.queryByText('Pending')).not.toBeInTheDocument()
  })

  it('renders a Pending badge for invitation rows when the viewer is the owner', () => {
    const pending = makeMember({
      user_id: 'inv:abc',
      email: 'pending@example.com',
      name: 'pending',
      invitation_status: 'pending',
    })
    render(<TeamMembersList members={[pending]} isOwner />)
    expect(screen.getByText('Pending')).toBeInTheDocument()
    expect(screen.getByText('pending@example.com')).toBeInTheDocument()
  })

  it('does not show the Status column to non-owners', () => {
    const pending = makeMember({
      user_id: 'inv:abc',
      email: 'pending@example.com',
      name: 'pending',
      invitation_status: 'pending',
    })
    render(<TeamMembersList members={[pending]} isOwner={false} />)
    // Owner-only column hidden — badge text absent.
    expect(screen.queryByText('Pending')).not.toBeInTheDocument()
    expect(screen.queryByText('Accepted')).not.toBeInTheDocument()
  })

  it('suppresses the Remove action for pending invitation rows', () => {
    const pending = makeMember({
      user_id: 'inv:abc',
      email: 'pending@example.com',
      name: 'pending',
      invitation_status: 'pending',
    })
    const onRemoveMember = jest.fn().mockResolvedValue(undefined)

    render(
      <TeamMembersList
        members={[pending]}
        isOwner
        onRemoveMember={onRemoveMember}
      />
    )

    expect(
      screen.queryByRole('button', { name: /remove pending/i })
    ).not.toBeInTheDocument()
  })

  it('shows the Remove action for accepted, non-owner members', () => {
    const accepted = makeMember({
      user_id: 'user-2',
      name: 'Bob',
      email: 'bob@example.com',
      invitation_status: 'accepted',
    })
    const onRemoveMember = jest.fn().mockResolvedValue(undefined)

    render(
      <TeamMembersList
        members={[accepted]}
        isOwner
        onRemoveMember={onRemoveMember}
      />
    )

    expect(
      screen.getByRole('button', { name: /remove bob/i })
    ).toBeInTheDocument()
  })

  it('prefixes the Joined column with "Invited" for pending rows', () => {
    const pending = makeMember({
      user_id: 'inv:abc',
      email: 'pending@example.com',
      name: 'pending',
      invitation_status: 'pending',
      joined_at: '2024-01-01T00:00:00Z',
    })
    render(<TeamMembersList members={[pending]} isOwner />)
    expect(screen.getByText(/^Invited\s/)).toBeInTheDocument()
  })

  it('does not prefix the Joined column for accepted rows', () => {
    render(<TeamMembersList members={[makeMember()]} isOwner />)
    expect(screen.queryByText(/^Invited\s/)).not.toBeInTheDocument()
  })

  it('does not show the Remove action for the team owner row', () => {
    const owner = makeMember({
      user_id: 'user-owner',
      name: 'Carol',
      role: 'owner',
    })
    const onRemoveMember = jest.fn().mockResolvedValue(undefined)

    render(
      <TeamMembersList
        members={[owner]}
        isOwner
        onRemoveMember={onRemoveMember}
      />
    )

    expect(
      screen.queryByRole('button', { name: /remove carol/i })
    ).not.toBeInTheDocument()
  })
})
