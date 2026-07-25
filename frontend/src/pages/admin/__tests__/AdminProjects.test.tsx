/**
 * AdminProjects (#461): server-driven filtering, sorting and pagination over
 * #453's listing.
 *
 * Same shape as AdminTeams.test.tsx — the assertions are about the query issued
 * and the URL kept, since a filter that renders but sends nothing is invisible on
 * screen.
 */
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router-dom'

import type {
  AdminProjectListItem,
  AdminProjectListResponse,
  AdminTeamListResponse,
} from '@/services/adminService'

const mockNavigate = jest.fn()
jest.mock('react-router-dom', () => ({
  ...jest.requireActual<typeof import('react-router-dom')>('react-router-dom'),
  useNavigate: () => mockNavigate,
}))

jest.mock('@/services/adminService', () => ({
  adminService: { listProjects: jest.fn(), listTeams: jest.fn() },
}))

import { adminService } from '@/services/adminService'

import { AdminProjects } from '../AdminProjects'

const mockAdminService = adminService as jest.Mocked<typeof adminService>

beforeAll(() => {
  // Radix Popover/Command rely on layout APIs jsdom does not implement.
  Element.prototype.scrollIntoView = jest.fn()
  Element.prototype.hasPointerCapture = jest.fn()
  Element.prototype.releasePointerCapture = jest.fn()
})

function project(
  overrides: Partial<AdminProjectListItem> = {}
): AdminProjectListItem {
  return {
    id: 'p1',
    name: 'Platform',
    slug: 'platform',
    team: { id: 't1', name: 'Engineering', slug: 'engineering' },
    owner: { id: 'u1', email: 'creator@example.com', name: 'Creator' },
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    ...overrides,
  }
}

function page(
  overrides: Partial<AdminProjectListResponse> = {}
): AdminProjectListResponse {
  return {
    projects: [project()],
    total_count: 1,
    page: 1,
    per_page: 20,
    total_pages: 1,
    ...overrides,
  }
}

const teamPage: AdminTeamListResponse = {
  teams: [
    {
      id: 't1',
      name: 'Engineering',
      slug: 'engineering',
      is_personal: false,
      owner: { id: 'o1', email: 'owner@example.com', name: 'Owner' },
      member_count: 4,
      created_at: '2026-01-01T00:00:00Z',
    },
    {
      id: 't2',
      name: 'Design',
      slug: 'design',
      is_personal: false,
      owner: { id: 'o2', email: 'owner2@example.com', name: 'Owner Two' },
      member_count: 2,
      created_at: '2026-01-02T00:00:00Z',
    },
  ],
  total_count: 2,
  page: 1,
  per_page: 25,
  total_pages: 1,
}

let currentSearch = ''

function LocationProbe() {
  currentSearch = useLocation().search
  return null
}

function renderProjects(initialEntry = '/admin/projects') {
  currentSearch = ''
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <AdminProjects />
      <LocationProbe />
    </MemoryRouter>
  )
}

const lastQuery = () => {
  const { calls } = mockAdminService.listProjects.mock
  return calls[calls.length - 1][0]
}

beforeEach(() => {
  jest.clearAllMocks()
  mockAdminService.listProjects.mockResolvedValue(page())
  mockAdminService.listTeams.mockResolvedValue(teamPage)
})

it('renders a row with its slug, team and owner', async () => {
  renderProjects()

  expect(await screen.findByText('Platform')).toBeInTheDocument()
  expect(screen.getByText('platform')).toBeInTheDocument()
  expect(screen.getByText('Engineering')).toBeInTheDocument()
  // The project's creator, which is a different column from Team on purpose:
  // projects carry both a team and a creating user, and they can differ.
  expect(screen.getByText('creator@example.com')).toBeInTheDocument()
})

it('navigates to the detail page on row click', async () => {
  renderProjects()

  await userEvent.click(await screen.findByText('Platform'))

  expect(mockNavigate).toHaveBeenCalledWith('/admin/projects/p1')
})

it('shows an error state on failure', async () => {
  mockAdminService.listProjects.mockRejectedValue(new Error('boom'))
  renderProjects()

  expect(await screen.findByText('Failed to load projects')).toBeInTheDocument()
})

it('requests the default sort and sends no filter params on first load', async () => {
  renderProjects()

  await waitFor(() => {
    expect(mockAdminService.listProjects).toHaveBeenCalled()
  })
  expect(lastQuery()).toEqual({
    page: 1,
    limit: 20,
    search: undefined,
    team_id: undefined,
    created_from: undefined,
    created_to: undefined,
    sort_by: 'created_at',
    sort_order: 'desc',
  })
  expect(currentSearch).toBe('')
})

it('rehydrates every filter from the URL on mount', async () => {
  renderProjects(
    '/admin/projects?search=plat&team_id=t1&created_from=2026-07-01&created_to=2026-07-24&sort_by=name&sort_order=asc&page=2'
  )

  await waitFor(() => {
    expect(mockAdminService.listProjects).toHaveBeenCalled()
  })
  const query = lastQuery()
  expect(query.page).toBe(2)
  expect(query.search).toBe('plat')
  expect(query.team_id).toBe('t1')
  expect(query.sort_by).toBe('name')
  expect(query.sort_order).toBe('asc')
  expect(screen.getByRole('textbox', { name: 'Search projects' })).toHaveValue(
    'plat'
  )
})

it('debounces the search box into a single request', async () => {
  renderProjects()
  await screen.findByText('Platform')
  const before = mockAdminService.listProjects.mock.calls.length

  await userEvent.type(
    screen.getByRole('textbox', { name: 'Search projects' }),
    'plat'
  )

  await waitFor(() => {
    expect(lastQuery().search).toBe('plat')
  })
  expect(mockAdminService.listProjects.mock.calls.length).toBe(before + 1)
})

it('sends local-day instants for the created range, upper bound at end of day', async () => {
  renderProjects(
    '/admin/projects?created_from=2026-07-01&created_to=2026-07-24'
  )

  await waitFor(() => {
    expect(mockAdminService.listProjects).toHaveBeenCalled()
  })
  const from = new Date(lastQuery().created_from!)
  const to = new Date(lastQuery().created_to!)
  expect(from.getHours()).toBe(0)
  expect(from.getDate()).toBe(1)
  expect(to.getHours()).toBe(23)
  expect(to.getDate()).toBe(24)
})

describe('sorting', () => {
  it('sorts a new column descending', async () => {
    renderProjects()
    await screen.findByText('Platform')

    await userEvent.click(screen.getByRole('button', { name: /Name/ }))

    await waitFor(() => {
      expect(lastQuery().sort_by).toBe('name')
    })
    expect(lastQuery().sort_order).toBe('desc')
  })

  it('flips direction on the active column', async () => {
    renderProjects('/admin/projects?sort_by=name&sort_order=desc')
    await screen.findByText('Platform')

    await userEvent.click(screen.getByRole('button', { name: /Name/ }))

    await waitFor(() => {
      expect(lastQuery().sort_order).toBe('asc')
    })
  })

  it('rejects a sort column the API does not accept', async () => {
    // #453 allows created_at and name only; team_name would be a 400.
    renderProjects('/admin/projects?sort_by=team_name')

    await waitFor(() => {
      expect(mockAdminService.listProjects).toHaveBeenCalled()
    })
    expect(lastQuery().sort_by).toBe('created_at')
  })
})

describe('the team filter', () => {
  it('does not fetch every team on the instance', async () => {
    renderProjects()
    await screen.findByText('Platform')

    await userEvent.click(
      screen.getByRole('combobox', { name: 'Filter by team' })
    )

    await waitFor(() => {
      expect(mockAdminService.listTeams).toHaveBeenCalled()
    })
    // One bounded page, not an unbounded "give me everything" request. On an
    // instance with a personal workspace per user, the latter grows forever.
    const teamQuery = mockAdminService.listTeams.mock.calls[0][0]
    expect(teamQuery.limit).toBeLessThanOrEqual(25)
    expect(teamQuery.page).toBe(1)
  })

  it('narrows the listing to the chosen team', async () => {
    renderProjects()
    await screen.findByText('Platform')

    await userEvent.click(
      screen.getByRole('combobox', { name: 'Filter by team' })
    )
    await userEvent.click(await screen.findByText('Design'))

    await waitFor(() => {
      expect(lastQuery().team_id).toBe('t2')
    })
    expect(currentSearch).toContain('team_id=t2')
  })

  it('searches teams server-side rather than filtering the loaded page', async () => {
    renderProjects()
    await screen.findByText('Platform')
    await userEvent.click(
      screen.getByRole('combobox', { name: 'Filter by team' })
    )
    await waitFor(() => {
      expect(mockAdminService.listTeams).toHaveBeenCalled()
    })

    await userEvent.type(screen.getByPlaceholderText('Search teams…'), 'des')

    await waitFor(() => {
      const searches = mockAdminService.listTeams.mock.calls.map(
        ([q]) => q.search
      )
      expect(searches).toContain('des')
    })
  })

  it('pulls the next page when the list is scrolled near the bottom', async () => {
    mockAdminService.listTeams
      .mockResolvedValueOnce({ ...teamPage, total_pages: 2 })
      .mockResolvedValueOnce({
        ...teamPage,
        teams: [
          {
            id: 't3',
            name: 'Research',
            slug: 'research',
            is_personal: false,
            owner: { id: 'o3', email: 'owner3@example.com', name: 'Owner 3' },
            member_count: 1,
            created_at: '2026-01-03T00:00:00Z',
          },
        ],
        page: 2,
        total_pages: 2,
      })
    renderProjects()
    await screen.findByText('Platform')
    await userEvent.click(
      screen.getByRole('combobox', { name: 'Filter by team' })
    )
    // Scoped to the picker: "Engineering" is also this project's Team column.
    const input = await screen.findByPlaceholderText('Search teams…')
    const list = input
      .closest('[cmdk-root]')!
      .querySelector<HTMLElement>('[cmdk-list]')!
    expect(within(list).getByText('Engineering')).toBeInTheDocument()

    // jsdom reports every element as zero-height, so the scroll geometry has to
    // be stubbed for the near-the-bottom check to mean anything.
    Object.defineProperty(list, 'scrollHeight', {
      value: 600,
      configurable: true,
    })
    Object.defineProperty(list, 'clientHeight', {
      value: 200,
      configurable: true,
    })
    Object.defineProperty(list, 'scrollTop', { value: 380, configurable: true })
    fireEvent.scroll(list)

    await waitFor(() => {
      expect(within(list).getByText('Research')).toBeInTheDocument()
    })
    // Appended, not replaced.
    expect(within(list).getByText('Engineering')).toBeInTheDocument()
  })

  it('clears back to all teams', async () => {
    renderProjects('/admin/projects?team_id=t2')
    await screen.findByText('Platform')
    await waitFor(() => {
      expect(lastQuery().team_id).toBe('t2')
    })

    await userEvent.click(
      screen.getByRole('combobox', { name: 'Filter by team' })
    )
    await userEvent.click(await screen.findByText('All teams'))

    await waitFor(() => {
      expect(lastQuery().team_id).toBeUndefined()
    })
    expect(currentSearch).not.toContain('team_id')
  })
})

it('applies a range picked in the UI to both the URL and the query', async () => {
  renderProjects()
  await screen.findByText('Platform')

  await userEvent.click(
    screen.getByRole('button', { name: 'Filter by creation date' })
  )
  await userEvent.click(
    await screen.findByRole('button', { name: 'Last 30 days' })
  )

  await waitFor(() => {
    expect(currentSearch).toMatch(/created_from=\d{4}-\d{2}-\d{2}/)
  })
  const query = lastQuery()
  expect(new Date(query.created_from!).getHours()).toBe(0)
  expect(new Date(query.created_to!).getHours()).toBe(23)
})

it('resets to page 1 when a filter changes', async () => {
  renderProjects('/admin/projects?page=3')
  await screen.findByText('Platform')
  await waitFor(() => {
    expect(lastQuery().page).toBe(3)
  })

  await userEvent.click(
    screen.getByRole('combobox', { name: 'Filter by team' })
  )
  await userEvent.click(await screen.findByText('Design'))

  await waitFor(() => {
    expect(lastQuery().page).toBe(1)
  })
})

describe('empty states', () => {
  beforeEach(() => {
    mockAdminService.listProjects.mockResolvedValue(
      page({ projects: [], total_count: 0, total_pages: 0 })
    )
  })

  it('says "no projects yet" for an empty instance', async () => {
    renderProjects()

    expect(await screen.findByText('No projects yet')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Clear filters' })
    ).not.toBeInTheDocument()
  })

  it('counts a domain filter alone as filtered, with no search term', async () => {
    // Without this the team filter would show the "no projects yet" state, which
    // says the instance is empty and offers no way back.
    renderProjects('/admin/projects?team_id=t1')

    expect(
      await screen.findByText('No projects match your filters')
    ).toBeInTheDocument()
    expect(screen.queryByText('No projects yet')).not.toBeInTheDocument()
  })

  it('counts a date range alone as filtered', async () => {
    renderProjects('/admin/projects?created_from=2026-07-01')

    expect(
      await screen.findByText('No projects match your filters')
    ).toBeInTheDocument()
  })

  it('offers a way out of a filtered-empty result', async () => {
    renderProjects('/admin/projects?search=nope&team_id=t1')

    expect(
      await screen.findByText('No projects match your filters')
    ).toBeInTheDocument()

    const [clear] = screen.getAllByRole('button', { name: 'Clear filters' })
    await userEvent.click(clear)

    await waitFor(() => {
      expect(lastQuery().search).toBeUndefined()
    })
    expect(lastQuery().team_id).toBeUndefined()
    expect(currentSearch).toBe('')
    expect(
      screen.getByRole('textbox', { name: 'Search projects' })
    ).toHaveValue('')
  })
})
