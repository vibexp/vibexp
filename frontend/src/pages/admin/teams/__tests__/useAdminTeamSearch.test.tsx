/**
 * useAdminTeamSearch (#461): the paginated, debounced team search behind the
 * admin team picker.
 *
 * Tested directly because the two behaviours that matter — bounded page size and
 * the stale-response guard — are invisible through the picker UI.
 */
import { act, render, screen, waitFor } from '@testing-library/react'

import type {
  AdminTeamListItem,
  AdminTeamListResponse,
} from '@/services/adminService'

jest.mock('@/services/adminService', () => ({
  adminService: { listTeams: jest.fn() },
}))

import { adminService } from '@/services/adminService'

import { useAdminTeamSearch } from '../useAdminTeamSearch'

const mockAdminService = adminService as jest.Mocked<typeof adminService>

function team(id: string, name: string): AdminTeamListItem {
  return {
    id,
    name,
    slug: name.toLowerCase(),
    is_personal: false,
    owner: { id: `o-${id}`, email: `${id}@example.com`, name: 'Owner' },
    member_count: 1,
    created_at: '2026-01-01T00:00:00Z',
  }
}

function pageOf(
  teams: AdminTeamListItem[],
  page: number,
  totalPages: number
): AdminTeamListResponse {
  return {
    teams,
    total_count: totalPages * 25,
    page,
    per_page: 25,
    total_pages: totalPages,
  }
}

let hook: ReturnType<typeof useAdminTeamSearch>

function Probe() {
  hook = useAdminTeamSearch()
  return (
    <ul data-testid="names">
      {hook.teams.map(t => (
        <li key={t.id}>{t.name}</li>
      ))}
    </ul>
  )
}

beforeEach(() => {
  jest.clearAllMocks()
})

it('loads the first page sorted by name, bounded in size', async () => {
  mockAdminService.listTeams.mockResolvedValue(
    pageOf([team('t1', 'Alpha')], 1, 1)
  )
  render(<Probe />)

  await waitFor(() => {
    expect(screen.getByText('Alpha')).toBeInTheDocument()
  })
  const query = mockAdminService.listTeams.mock.calls[0][0]
  // Bounded: "fetch every team to fill a dropdown" grows without limit on an
  // instance with a personal workspace per user.
  expect(query.limit).toBe(25)
  expect(query.page).toBe(1)
  expect(query.sort_by).toBe('name')
  expect(query.sort_order).toBe('asc')
  expect(query.search).toBeUndefined()
})

it('appends the next page instead of replacing the list', async () => {
  mockAdminService.listTeams
    .mockResolvedValueOnce(pageOf([team('t1', 'Alpha')], 1, 2))
    .mockResolvedValueOnce(pageOf([team('t2', 'Beta')], 2, 2))
  render(<Probe />)
  await waitFor(() => {
    expect(screen.getByText('Alpha')).toBeInTheDocument()
  })
  expect(hook.hasMore).toBe(true)

  act(() => {
    hook.loadMore()
  })

  await waitFor(() => {
    expect(screen.getByText('Beta')).toBeInTheDocument()
  })
  expect(screen.getByText('Alpha')).toBeInTheDocument()
  expect(hook.hasMore).toBe(false)
})

it('does not page past the end', async () => {
  mockAdminService.listTeams.mockResolvedValue(
    pageOf([team('t1', 'Alpha')], 1, 1)
  )
  render(<Probe />)
  await waitFor(() => {
    expect(screen.getByText('Alpha')).toBeInTheDocument()
  })

  act(() => {
    hook.loadMore()
    hook.loadMore()
  })

  // hasMore is false, so the scroll handler calling loadMore freely is safe.
  await waitFor(() => {
    expect(mockAdminService.listTeams).toHaveBeenCalledTimes(1)
  })
})

it('debounces a query change and resets to page 1', async () => {
  mockAdminService.listTeams.mockResolvedValue(
    pageOf([team('t1', 'Alpha')], 1, 3)
  )
  render(<Probe />)
  await waitFor(() => {
    expect(screen.getByText('Alpha')).toBeInTheDocument()
  })
  act(() => {
    hook.loadMore()
  })
  await waitFor(() => {
    expect(mockAdminService.listTeams).toHaveBeenCalledTimes(2)
  })

  act(() => {
    hook.setQuery('de')
    hook.setQuery('des')
  })

  await waitFor(() => {
    const last = mockAdminService.listTeams.mock.calls.at(-1)?.[0]
    expect(last?.search).toBe('des')
  })
  const last = mockAdminService.listTeams.mock.calls.at(-1)?.[0]
  // A new query must start from page 1, or the first screen of results is
  // whatever page the previous query happened to be on.
  expect(last?.page).toBe(1)
  // Two rapid keystrokes, one request.
  expect(mockAdminService.listTeams).toHaveBeenCalledTimes(3)
})

it('replaces the list on a new query rather than appending to it', async () => {
  mockAdminService.listTeams
    .mockResolvedValueOnce(pageOf([team('t1', 'Alpha')], 1, 1))
    .mockResolvedValueOnce(pageOf([team('t2', 'Beta')], 1, 1))
  render(<Probe />)
  await waitFor(() => {
    expect(screen.getByText('Alpha')).toBeInTheDocument()
  })

  act(() => {
    hook.setQuery('be')
  })

  await waitFor(() => {
    expect(screen.getByText('Beta')).toBeInTheDocument()
  })
  expect(screen.queryByText('Alpha')).not.toBeInTheDocument()
})

it('ignores a stale response that resolves after a newer one', async () => {
  let resolveFirst: (value: AdminTeamListResponse) => void = () => undefined
  mockAdminService.listTeams
    .mockReturnValueOnce(
      new Promise<AdminTeamListResponse>(resolve => {
        resolveFirst = resolve
      })
    )
    .mockResolvedValueOnce(pageOf([team('t2', 'Beta')], 1, 1))
  render(<Probe />)

  act(() => {
    hook.setQuery('be')
  })
  await waitFor(() => {
    expect(screen.getByText('Beta')).toBeInTheDocument()
  })

  // The first, now-obsolete request finally answers. Awaiting a resolved promise
  // inside act() lets its .then handlers flush before the assertions below.
  await act(async () => {
    resolveFirst(pageOf([team('t1', 'Alpha')], 1, 1))
    await Promise.resolve()
  })

  // It must not overwrite the newer query's results — the classic search race.
  expect(screen.queryByText('Alpha')).not.toBeInTheDocument()
  expect(screen.getByText('Beta')).toBeInTheDocument()
})

it('surfaces a load failure', async () => {
  mockAdminService.listTeams.mockRejectedValue(new Error('boom'))
  render(<Probe />)

  await waitFor(() => {
    expect(hook.error).toBe('boom')
  })
  expect(hook.loading).toBe(false)
})
