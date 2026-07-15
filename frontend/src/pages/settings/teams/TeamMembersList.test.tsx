import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { TeamMember } from '@/services/teamService'

import { TeamMembersList } from './TeamMembersList'

// Radix Select needs these in jsdom (same shim as ModelProviderDialog.test).
beforeAll(() => {
  Element.prototype.scrollIntoView = jest.fn()
  Element.prototype.hasPointerCapture = jest.fn()
  Element.prototype.releasePointerCapture = jest.fn()
})

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
    render(<TeamMembersList members={[]} />)
    expect(screen.getByText(/no members found/i)).toBeInTheDocument()
  })

  it('renders an Accepted badge for members without an invitation_status (legacy active members)', () => {
    render(<TeamMembersList members={[makeMember()]} canRemoveMember />)
    // Two badges per row: role + status. We only assert the status one.
    expect(screen.getByText('Accepted')).toBeInTheDocument()
    expect(screen.queryByText('Pending')).not.toBeInTheDocument()
  })

  it('renders a Pending badge for invitation rows when the viewer manages members', () => {
    const pending = makeMember({
      user_id: 'inv:abc',
      email: 'pending@example.com',
      name: 'pending',
      invitation_status: 'pending',
    })
    render(<TeamMembersList members={[pending]} canRemoveMember />)
    expect(screen.getByText('Pending')).toBeInTheDocument()
    expect(screen.getByText('pending@example.com')).toBeInTheDocument()
  })

  it('does not show the Status column to viewers who cannot manage members', () => {
    const pending = makeMember({
      user_id: 'inv:abc',
      email: 'pending@example.com',
      name: 'pending',
      invitation_status: 'pending',
    })
    render(<TeamMembersList members={[pending]} />)
    // Member-management column hidden — badge text absent.
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
        canRemoveMember
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
        canRemoveMember
        onRemoveMember={onRemoveMember}
      />
    )

    expect(
      screen.getByRole('button', { name: /remove bob/i })
    ).toBeInTheDocument()
  })

  it('hides the Remove action without the member.remove permission', () => {
    const accepted = makeMember({ user_id: 'user-2', name: 'Bob' })
    const onRemoveMember = jest.fn().mockResolvedValue(undefined)

    render(
      <TeamMembersList
        members={[accepted]}
        canRemoveMember={false}
        onRemoveMember={onRemoveMember}
      />
    )

    expect(
      screen.queryByRole('button', { name: /remove bob/i })
    ).not.toBeInTheDocument()
  })

  it('prefixes the Joined column with "Invited" for pending rows', () => {
    const pending = makeMember({
      user_id: 'inv:abc',
      email: 'pending@example.com',
      name: 'pending',
      invitation_status: 'pending',
      joined_at: '2024-01-01T00:00:00Z',
    })
    render(<TeamMembersList members={[pending]} canRemoveMember />)
    expect(screen.getByText(/^Invited\s/)).toBeInTheDocument()
  })

  it('does not prefix the Joined column for accepted rows', () => {
    render(<TeamMembersList members={[makeMember()]} canRemoveMember />)
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
        canRemoveMember
        onRemoveMember={onRemoveMember}
      />
    )

    expect(
      screen.queryByRole('button', { name: /remove carol/i })
    ).not.toBeInTheDocument()
  })

  describe('role management (#225)', () => {
    it('renders a role dropdown for a member when the viewer may update roles', () => {
      render(
        <TeamMembersList
          members={[makeMember({ name: 'Alice' })]}
          canManageRoles
          onChangeRole={jest.fn()}
        />
      )

      expect(
        screen.getByRole('combobox', { name: /change role for alice/i })
      ).toBeInTheDocument()
    })

    it('renders a static badge, not a dropdown, without the member.role.update permission', () => {
      // An admin fixture: "Member" is also the first column's header, so the
      // badge text would be ambiguous for a member row.
      render(
        <TeamMembersList
          members={[makeMember({ name: 'Alice', role: 'admin' })]}
          canManageRoles={false}
          onChangeRole={jest.fn()}
        />
      )

      expect(
        screen.queryByRole('combobox', { name: /change role for alice/i })
      ).not.toBeInTheDocument()
      expect(screen.getByText('Admin')).toBeInTheDocument()
    })

    it('keeps the owner row immutable — ownership moves only via transfer', () => {
      render(
        <TeamMembersList
          members={[makeMember({ name: 'Carol', role: 'owner' })]}
          canManageRoles
          onChangeRole={jest.fn()}
        />
      )

      expect(
        screen.queryByRole('combobox', { name: /change role for carol/i })
      ).not.toBeInTheDocument()
      expect(screen.getByText('Owner')).toBeInTheDocument()
    })

    it('keeps pending invitation rows immutable — there is no membership to update yet', () => {
      render(
        <TeamMembersList
          members={[
            makeMember({ name: 'pending', invitation_status: 'pending' }),
          ]}
          canManageRoles
          onChangeRole={jest.fn()}
        />
      )

      expect(
        screen.queryByRole('combobox', { name: /change role for pending/i })
      ).not.toBeInTheDocument()
    })

    it('reports the picked role to the caller', async () => {
      const user = userEvent.setup()
      const onChangeRole = jest.fn().mockResolvedValue(undefined)

      render(
        <TeamMembersList
          members={[makeMember({ user_id: 'user-2', name: 'Bob' })]}
          canManageRoles
          onChangeRole={onChangeRole}
        />
      )

      await user.click(
        screen.getByRole('combobox', { name: /change role for bob/i })
      )
      await user.click(screen.getByRole('option', { name: 'Admin' }))

      await waitFor(() => {
        expect(onChangeRole).toHaveBeenCalledWith('user-2', 'admin')
      })
    })
  })
})
