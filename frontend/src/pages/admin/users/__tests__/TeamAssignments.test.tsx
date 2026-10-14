import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'

import { adminService } from '@/services/adminService'

vi.mock('@/services/adminService', () => ({
  adminService: { listTeams: vi.fn() },
}))

import { type TeamAssignment, TeamAssignments } from '../TeamAssignments'

const service = vi.mocked(adminService)

function team(id: string, name: string, isPersonal = false) {
  return { id, name, slug: id, is_personal: isPersonal } as never
}

function Harness({
  errors,
  onValue,
}: Readonly<{
  errors?: Record<number, string>
  onValue?: (value: TeamAssignment[]) => void
}>) {
  const [value, setValue] = useState<TeamAssignment[]>([])
  return (
    <TeamAssignments
      value={value}
      onChange={next => {
        setValue(next)
        onValue?.(next)
      }}
      errors={errors}
    />
  )
}

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
})

beforeEach(() => {
  vi.clearAllMocks()
  service.listTeams.mockResolvedValue({
    teams: [
      team('t-1', 'Engineering'),
      team('t-2', 'Ada personal', true),
      team('t-3', 'Design'),
    ],
    page: 1,
    total_pages: 1,
  } as never)
})

async function addTeam(user: ReturnType<typeof userEvent.setup>, name: string) {
  await user.click(screen.getByRole('button', { name: 'Add a team' }))
  await user.click(await screen.findByRole('option', { name }))
}

describe('TeamAssignments', () => {
  it('does not search teams until a team is being added', () => {
    render(<Harness />)
    expect(service.listTeams).not.toHaveBeenCalled()
  })

  it('adds shared teams only, as members, and lets the role and the row change', async () => {
    const onValue = vi.fn()
    const user = userEvent.setup()
    render(<Harness onValue={onValue} />)

    await user.click(screen.getByRole('button', { name: 'Add a team' }))
    expect(
      await screen.findByRole('option', { name: 'Engineering' })
    ).toBeVisible()
    expect(screen.queryByRole('option', { name: 'Ada personal' })).toBeNull()
    await user.click(screen.getByRole('option', { name: 'Engineering' }))
    await addTeam(user, 'Design')

    const rows = within(screen.getByTestId('team-assignments')).getAllByRole(
      'listitem'
    )
    expect(rows).toHaveLength(2)
    await user.click(
      within(
        screen.getByRole('radiogroup', { name: 'Role in Design' })
      ).getByRole('radio', { name: 'Admin' })
    )
    expect(onValue).toHaveBeenLastCalledWith([
      { team_id: 't-1', team_name: 'Engineering', role: 'member' },
      { team_id: 't-3', team_name: 'Design', role: 'admin' },
    ])

    // A chosen team is not offered again.
    await user.click(screen.getByRole('button', { name: 'Add a team' }))
    await screen.findByText('No teams to add.')

    await user.keyboard('{Escape}')
    await user.click(screen.getByRole('button', { name: 'Remove Engineering' }))
    await waitFor(() => {
      expect(onValue).toHaveBeenLastCalledWith([
        { team_id: 't-3', team_name: 'Design', role: 'admin' },
      ])
    })
  })

  it("shows the server's refusal on its row", async () => {
    const user = userEvent.setup()
    render(<Harness errors={{ 0: 'This team cannot be used' }} />)
    await addTeam(user, 'Engineering')

    expect(
      within(screen.getByTestId('team-assignments')).getByRole('alert')
    ).toHaveTextContent('This team cannot be used')
  })
})
