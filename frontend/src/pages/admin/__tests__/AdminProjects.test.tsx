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
import { MemoryRouter, useLocation } from 'react-router'
import type { Mocked } from 'vitest'

import type {
  AdminProjectListItem,
  AdminProjectListResponse,
  AdminTeamListItem,
  AdminTeamListResponse,
} from '@/services/adminService'

const mockNavigate = vi.hoisted(() => vi.fn())
vi.mock('react-router', async () => ({
  ...(await vi.importActual<typeof import('react-router')>('react-router')),
  useNavigate: () => mockNavigate,
}))

vi.mock('@/services/adminService', () => ({
  adminService: { listProjects: vi.fn(), listTeams: vi.fn() },
}))

import { adminService } from '@/services/adminService'

import { AdminProjects } from '../AdminProjects'

/** The #1138 count and configuration fields every team list row carries. */
const TEAM_COUNTS = {
  owner_count: 1,
  admin_count: 0,
  project_count: 1,
  resource_counts: {
    prompts: 0,
    memories: 0,
    artifacts: 0,
    blueprints: 0,
    agents: 0,
    feeds: 0,
    feed_items: 0,
    comments: 0,
    attachments: 0,
    total: 0,
  },
  configuration: {
    embedding_configured: false,
    llm_configured: false,
    ai_summary_enabled: false,
    email_configured: false,
    github_configured: false,
    search_settings_customized: false,
    freshness_enabled: false,
  },
} satisfies Partial<AdminTeamListItem>

const mockAdminService = adminService as Mocked<typeof adminService>

beforeAll(() => {
  // Radix Popover/Command rely on layout APIs jsdom does not implement.
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
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
    // Distinct per type, so a transposed breakdown cannot pass unnoticed.
    resource_counts: {
      prompts: 3,
      memories: 7,
      artifacts: 2,
      blueprints: 1,
      feed_items: 5,
      total: 18,
    },
    last_resource_created_at: '2026-03-04T10:00:00Z',
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
      ...TEAM_COUNTS,
      created_at: '2026-01-01T00:00:00Z',
    },
    {
      id: 't2',
      name: 'Design',
      slug: 'design',
      is_personal: false,
      owner: { id: 'o2', email: 'owner2@example.com', name: 'Owner Two' },
      member_count: 2,
      ...TEAM_COUNTS,
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
  vi.clearAllMocks()
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
    owner_email: undefined,
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
  expect(mockAdminService.listProjects.mock.calls).toHaveLength(before + 1)
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
    // team_name is not in the published sort_by enum; it would be a 400.
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
            ...TEAM_COUNTS,
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

describe('advanced filters (#1144)', () => {
  it('sends advanced params from the URL and opens the panel', async () => {
    renderProjects(
      '/admin/projects?prompt_count_min=5&total_resource_count_max=100&last_resource_created_from=2026-01-01&owner_email=creator@example.com&team_id=t1'
    )

    await waitFor(() => {
      expect(mockAdminService.listProjects).toHaveBeenCalled()
    })
    expect(lastQuery()).toMatchObject({
      team_id: 't1',
      prompt_count_min: 5,
      total_resource_count_max: 100,
      owner_email: 'creator@example.com',
    })
    // A local-day lower bound, sent as the start of that day.
    const from = new Date(String(lastQuery().last_resource_created_from))
    expect(from.getDate()).toBe(1)
    expect(from.getHours()).toBe(0)
    // Empty bounds send nothing.
    expect(lastQuery()).not.toHaveProperty('prompt_count_max')
    expect(lastQuery()).not.toHaveProperty('memory_count_min')
    expect(lastQuery()).not.toHaveProperty('last_resource_created_to')
    // A shared link carrying an advanced filter opens with the panel visible.
    expect(
      await screen.findByRole('spinbutton', { name: 'Prompts minimum' })
    ).toHaveValue(5)
    expect(screen.getByRole('textbox', { name: 'Creator email' })).toHaveValue(
      'creator@example.com'
    )
    expect(
      screen.getByRole('group', { name: 'Last resource created' })
    ).toBeInTheDocument()
    // Two ranges, the date range and the creator email; team_id is main-row.
    expect(screen.getByTestId('advanced-filters-count')).toHaveTextContent('4')
  })

  it('offers no control for a type that is not project-scoped', async () => {
    renderProjects('/admin/projects?prompt_count_min=1')

    await screen.findByRole('spinbutton', { name: 'Prompts minimum' })
    for (const label of ['Agents', 'Feeds', 'Comments', 'Attachments']) {
      expect(
        screen.queryByRole('spinbutton', { name: `${label} minimum` })
      ).not.toBeInTheDocument()
    }
    expect(
      screen.getByRole('spinbutton', { name: 'Feed items minimum' })
    ).toBeInTheDocument()
  })

  it('does not send a malformed creator email restored from the URL', async () => {
    renderProjects('/admin/projects?owner_email=boss')

    await waitFor(() => {
      expect(mockAdminService.listProjects).toHaveBeenCalled()
    })
    expect(lastQuery().owner_email).toBeUndefined()
    const input = await screen.findByRole('textbox', { name: 'Creator email' })
    expect(input).toHaveValue('boss')
    expect(input).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByTestId('advanced-filters-count')).toHaveTextContent('1')
  })

  it('commits the creator email on Enter, trimmed', async () => {
    renderProjects()
    await screen.findByText('Platform')
    await userEvent.click(
      screen.getByRole('button', { name: /Advanced filters/ })
    )
    const initialCalls = mockAdminService.listProjects.mock.calls.length

    const input = await screen.findByRole('textbox', { name: 'Creator email' })
    await userEvent.type(input, '  x@corp.com  ')
    expect(mockAdminService.listProjects.mock.calls).toHaveLength(initialCalls)
    await userEvent.keyboard('{Enter}')

    await waitFor(() => {
      expect(lastQuery().owner_email).toBe('x@corp.com')
    })
    expect(currentSearch).toContain('owner_email=x%40corp.com')
  })

  it('clears every advanced key and a rejected creator-email draft', async () => {
    renderProjects(
      '/admin/projects?blueprint_count_min=2&last_resource_created_to=2026-02-01'
    )
    await screen.findByText('Platform')
    const input = await screen.findByRole('textbox', { name: 'Creator email' })
    await userEvent.type(input, 'boss{Enter}')
    expect(await screen.findByRole('alert')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Clear filters' }))

    await waitFor(() => {
      expect(currentSearch).toBe('')
    })
    expect(lastQuery()).not.toHaveProperty('blueprint_count_min')
    expect(lastQuery()).not.toHaveProperty('last_resource_created_to')
    expect(lastQuery().owner_email).toBeUndefined()
    expect(screen.getByRole('textbox', { name: 'Creator email' })).toHaveValue(
      ''
    )
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})

describe('resource columns (#1144)', () => {
  it('renders the total and the last resource date', async () => {
    renderProjects()
    const row = (await screen.findByText('Platform')).closest('tr')
    const cells = within(row as HTMLElement)
    expect(
      cells.getByRole('img', {
        name: '18 resources: 3 prompts, 7 memories, 2 artifacts, 1 blueprint, 5 feed items',
      })
    ).toHaveTextContent('18')
    expect(cells.getByText('Mar 4, 2026')).toBeInTheDocument()
  })

  it('shows a dash for a project with no resources yet', async () => {
    mockAdminService.listProjects.mockResolvedValue(
      page({ projects: [project({ last_resource_created_at: null })] })
    )
    renderProjects()
    const row = (await screen.findByText('Platform')).closest('tr')
    expect(within(row as HTMLElement).getByText('—')).toBeInTheDocument()
  })

  it('places Resources and Last resource before Created', async () => {
    renderProjects()
    await screen.findByText('Platform')
    const headers = Array.from(document.querySelectorAll('thead th')).map(th =>
      th.textContent.trim()
    )
    expect(headers).toEqual([
      expect.stringMatching(/^Name/),
      'Team',
      'Owner',
      expect.stringMatching(/^Resources/),
      expect.stringMatching(/^Last resource/),
      expect.stringMatching(/^Created/),
    ])
  })

  it.each([
    ['Resources', 'total_resource_count'],
    ['Last resource', 'last_resource_created_at'],
  ])('sorts by %s', async (header, sortBy) => {
    renderProjects()
    await screen.findByText('Platform')

    await userEvent.click(
      screen.getByRole('button', { name: new RegExp(`^${header}`) })
    )

    await waitFor(() => {
      expect(lastQuery().sort_by).toBe(sortBy)
    })
    expect(currentSearch).toContain(`sort_by=${sortBy}`)
  })

  it('restores a count sort from the URL', async () => {
    renderProjects(
      '/admin/projects?sort_by=last_resource_created_at&sort_order=asc'
    )

    await waitFor(() => {
      expect(mockAdminService.listProjects).toHaveBeenCalled()
    })
    expect(lastQuery().sort_by).toBe('last_resource_created_at')
    expect(lastQuery().sort_order).toBe('asc')
  })

  it('falls back from a per-type sort that has no column', async () => {
    // prompt_count is in the API enum but not displayed, so not sortable here.
    renderProjects('/admin/projects?sort_by=prompt_count')

    await waitFor(() => {
      expect(mockAdminService.listProjects).toHaveBeenCalled()
    })
    expect(lastQuery().sort_by).toBe('created_at')
  })
})
