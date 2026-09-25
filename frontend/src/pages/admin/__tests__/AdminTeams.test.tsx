/**
 * AdminTeams (#460): server-driven filtering, sorting and pagination.
 *
 * Most assertions are about the **query the page issues** and the URL it keeps,
 * because that is where the contract with #452's endpoint lives — a filter that
 * renders but sends nothing looks identical on screen.
 */
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router'
import type { Mocked } from 'vitest'

import type {
  AdminTeamListItem,
  AdminTeamListResponse,
} from '@/services/adminService'

const mockNavigate = vi.hoisted(() => vi.fn())
vi.mock('react-router', async () => ({
  ...(await vi.importActual<typeof import('react-router')>('react-router')),
  useNavigate: () => mockNavigate,
}))

vi.mock('@/services/adminService', () => ({
  adminService: { listTeams: vi.fn() },
}))

import { adminService } from '@/services/adminService'

import { AdminTeams } from '../AdminTeams'

const mockAdminService = adminService as Mocked<typeof adminService>

// Radix Select relies on layout APIs jsdom does not implement, same as
// BlueprintForm.test.tsx.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

function team(overrides: Partial<AdminTeamListItem> = {}): AdminTeamListItem {
  return {
    id: 't1',
    name: 'Engineering',
    slug: 'engineering',
    is_personal: false,
    owner: { id: 'o1', email: 'owner@example.com', name: 'Owner' },
    member_count: 4,
    owner_count: 1,
    admin_count: 2,
    project_count: 3,
    resource_counts: {
      prompts: 5,
      memories: 6,
      artifacts: 0,
      blueprints: 0,
      agents: 0,
      feeds: 0,
      feed_items: 0,
      comments: 0,
      attachments: 0,
      total: 11,
    },
    configuration: {
      embedding_configured: true,
      llm_configured: false,
      ai_summary_enabled: false,
      email_configured: false,
      github_configured: false,
      search_settings_customized: false,
      freshness_enabled: false,
    },
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function page(
  overrides: Partial<AdminTeamListResponse> = {}
): AdminTeamListResponse {
  return {
    teams: [team()],
    total_count: 1,
    page: 1,
    per_page: 20,
    total_pages: 1,
    ...overrides,
  }
}

let currentSearch = ''

function LocationProbe() {
  currentSearch = useLocation().search
  return null
}

function renderTeams(initialEntry = '/admin/teams') {
  currentSearch = ''
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <AdminTeams />
      <LocationProbe />
    </MemoryRouter>
  )
}

/** The query object of the most recent listTeams call. */
const lastQuery = () => {
  const { calls } = mockAdminService.listTeams.mock
  return calls[calls.length - 1][0]
}

beforeEach(() => {
  vi.clearAllMocks()
  mockAdminService.listTeams.mockResolvedValue(page())
})

it('renders a row with its slug, owner and member count', async () => {
  renderTeams()

  expect(await screen.findByText('Engineering')).toBeInTheDocument()
  expect(screen.getByText('engineering')).toBeInTheDocument()
  expect(screen.getByText('owner@example.com')).toBeInTheDocument()
  expect(screen.getByText('4')).toBeInTheDocument()
})

it('marks a personal workspace and leaves a shared team unmarked', async () => {
  mockAdminService.listTeams.mockResolvedValue(
    page({
      teams: [
        team({ id: 't1', name: 'Shared Team', is_personal: false }),
        team({
          id: 't2',
          name: 'Private Workspace',
          slug: 'private-workspace',
          is_personal: true,
        }),
      ],
    })
  )
  renderTeams()

  const personalRow = (await screen.findByText('Private Workspace')).closest(
    'tr'
  )
  const sharedRow = screen.getByText('Shared Team').closest('tr')
  expect(personalRow).not.toBeNull()
  expect(sharedRow).not.toBeNull()
  // The badge is what makes the personal/shared filter verifiable on screen.
  expect(within(personalRow as HTMLElement).getByText('Personal')).toBeVisible()
  expect(
    within(sharedRow as HTMLElement).queryByText('Personal')
  ).not.toBeInTheDocument()
})

it('navigates to the detail page on row click', async () => {
  renderTeams()

  await userEvent.click(await screen.findByText('Engineering'))

  expect(mockNavigate).toHaveBeenCalledWith('/admin/teams/t1')
})

it('shows an error state on failure', async () => {
  mockAdminService.listTeams.mockRejectedValue(new Error('boom'))
  renderTeams()

  expect(await screen.findByText('Failed to load teams')).toBeInTheDocument()
})

it('requests the default sort on first load and sends no filter params', async () => {
  renderTeams()

  await waitFor(() => {
    expect(mockAdminService.listTeams).toHaveBeenCalled()
  })
  expect(lastQuery()).toEqual({
    page: 1,
    limit: 20,
    search: undefined,
    is_personal: undefined,
    owner_email: undefined,
    created_from: undefined,
    created_to: undefined,
    sort_by: 'created_at',
    sort_order: 'desc',
  })
  // Defaults stay out of the URL, so an unfiltered page has a clean address bar.
  expect(currentSearch).toBe('')
})

describe('the tri-state team-type filter', () => {
  const selectKind = async (label: string) => {
    await userEvent.click(screen.getByRole('combobox', { name: 'Team type' }))
    await userEvent.click(await screen.findByRole('option', { name: label }))
  }

  it('sends is_personal=false for "Shared only"', async () => {
    renderTeams()
    await screen.findByText('Engineering')

    await selectKind('Shared only')

    await waitFor(() => {
      expect(lastQuery().is_personal).toBe(false)
    })
    expect(currentSearch).toContain('kind=shared')
  })

  it('sends is_personal=true for "Personal only"', async () => {
    renderTeams()
    await screen.findByText('Engineering')

    await selectKind('Personal only')

    await waitFor(() => {
      expect(lastQuery().is_personal).toBe(true)
    })
  })

  it('sends NO is_personal param for "All teams"', async () => {
    // The regression that matters: `is_personal=false` means "shared only", so
    // sending it for "All" would silently hide every personal workspace.
    renderTeams('/admin/teams?kind=personal')
    await screen.findByText('Engineering')
    await waitFor(() => {
      expect(lastQuery().is_personal).toBe(true)
    })

    await selectKind('All teams')

    await waitFor(() => {
      expect(lastQuery().is_personal).toBeUndefined()
    })
    expect(currentSearch).not.toContain('kind')
  })
})

it('debounces the search box into a single request', async () => {
  renderTeams()
  await screen.findByText('Engineering')
  const initialCalls = mockAdminService.listTeams.mock.calls.length

  await userEvent.type(
    screen.getByRole('textbox', { name: 'Search teams' }),
    'eng'
  )

  // Three keystrokes must not become three requests and three URL writes.
  await waitFor(() => {
    expect(lastQuery().search).toBe('eng')
  })
  expect(mockAdminService.listTeams.mock.calls).toHaveLength(initialCalls + 1)
  expect(currentSearch).toContain('search=eng')
})

it('rehydrates every filter from the URL on mount', async () => {
  renderTeams(
    '/admin/teams?search=eng&kind=shared&created_from=2026-07-01&created_to=2026-07-24&sort_by=name&sort_order=asc&page=3'
  )

  await waitFor(() => {
    expect(mockAdminService.listTeams).toHaveBeenCalled()
  })
  const query = lastQuery()
  expect(query.page).toBe(3)
  expect(query.search).toBe('eng')
  expect(query.is_personal).toBe(false)
  expect(query.sort_by).toBe('name')
  expect(query.sort_order).toBe('asc')
  // A shared link must reproduce the filter bar, not just the query.
  expect(screen.getByRole('textbox', { name: 'Search teams' })).toHaveValue(
    'eng'
  )
})

describe('the created-date range', () => {
  it('sends local-day instants, with the upper bound at end of day', async () => {
    renderTeams('/admin/teams?created_from=2026-07-01&created_to=2026-07-24')

    await waitFor(() => {
      expect(mockAdminService.listTeams).toHaveBeenCalled()
    })
    const query = lastQuery()
    const from = new Date(query.created_from!)
    const to = new Date(query.created_to!)
    // Local midnight to local end-of-day. A bare midnight upper bound would
    // exclude everything created during the 24th, making a one-day filter empty.
    expect(from.getHours()).toBe(0)
    expect(from.getDate()).toBe(1)
    expect(to.getHours()).toBe(23)
    expect(to.getMinutes()).toBe(59)
    expect(to.getDate()).toBe(24)
  })

  it('applies a range picked in the UI to both the URL and the query', async () => {
    renderTeams()
    await screen.findByText('Engineering')

    await userEvent.click(
      screen.getByRole('button', { name: 'Filter by creation date' })
    )
    await userEvent.click(
      await screen.findByRole('button', { name: 'Last 7 days' })
    )

    // The URL carries date-only local days — readable and shareable — while the
    // request carries the instants they denote.
    await waitFor(() => {
      expect(currentSearch).toMatch(/created_from=\d{4}-\d{2}-\d{2}/)
    })
    expect(currentSearch).toMatch(/created_to=\d{4}-\d{2}-\d{2}/)
    const query = lastQuery()
    expect(new Date(query.created_from!).getHours()).toBe(0)
    expect(new Date(query.created_to!).getHours()).toBe(23)
    // Seven days inclusive of today.
    const spanDays =
      (new Date(query.created_to!).getTime() -
        new Date(query.created_from!).getTime()) /
      86_400_000
    expect(Math.round(spanDays)).toBe(7)
  })

  it('ignores a malformed date param instead of sending garbage', async () => {
    renderTeams('/admin/teams?created_from=not-a-date&created_to=2026-02-31')

    await waitFor(() => {
      expect(mockAdminService.listTeams).toHaveBeenCalled()
    })
    // 2026-02-31 is well-shaped but impossible; JS would roll it into March.
    expect(lastQuery().created_from).toBeUndefined()
    expect(lastQuery().created_to).toBeUndefined()
  })
})

describe('sorting', () => {
  const clickHeader = async (name: string) => {
    await userEvent.click(
      screen.getByRole('button', { name: new RegExp(name) })
    )
  }

  it('sorts a new column descending', async () => {
    renderTeams()
    await screen.findByText('Engineering')

    await clickHeader('Members')

    await waitFor(() => {
      expect(lastQuery().sort_by).toBe('member_count')
    })
    expect(lastQuery().sort_order).toBe('desc')
  })

  it('flips direction when the active column is clicked again', async () => {
    renderTeams('/admin/teams?sort_by=name&sort_order=desc')
    await screen.findByText('Engineering')

    await clickHeader('Name')

    await waitFor(() => {
      expect(lastQuery().sort_order).toBe('asc')
    })
    expect(lastQuery().sort_by).toBe('name')
  })

  it('falls back to the default when the URL names an unknown column', async () => {
    renderTeams('/admin/teams?sort_by=owner_email')

    await waitFor(() => {
      expect(mockAdminService.listTeams).toHaveBeenCalled()
    })
    // The API rejects a sort_by outside its enum with a 400, so the page must
    // not forward whatever the URL happens to contain.
    expect(lastQuery().sort_by).toBe('created_at')
  })
})

it('resets to page 1 when a filter changes', async () => {
  renderTeams('/admin/teams?page=4')
  await screen.findByText('Engineering')
  await waitFor(() => {
    expect(lastQuery().page).toBe(4)
  })

  await userEvent.click(screen.getByRole('combobox', { name: 'Team type' }))
  await userEvent.click(
    await screen.findByRole('option', { name: 'Shared only' })
  )

  await waitFor(() => {
    expect(lastQuery().page).toBe(1)
  })
})

describe('empty states', () => {
  beforeEach(() => {
    mockAdminService.listTeams.mockResolvedValue(
      page({ teams: [], total_count: 0, total_pages: 0 })
    )
  })

  it('says "no teams yet" for an empty instance, with no way out to offer', async () => {
    renderTeams()

    expect(await screen.findByText('No teams yet')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Clear filters' })
    ).not.toBeInTheDocument()
  })

  it('counts a domain filter alone as filtered, with no search term', async () => {
    // Without this the team-type filter would show the "no teams yet" state,
    // which says the instance is empty and offers no way back.
    renderTeams('/admin/teams?kind=shared')

    expect(
      await screen.findByText('No teams match your filters')
    ).toBeInTheDocument()
    expect(screen.queryByText('No teams yet')).not.toBeInTheDocument()
  })

  it('offers a way out of a filtered-empty result', async () => {
    renderTeams('/admin/teams?search=nope&kind=shared')

    expect(
      await screen.findByText('No teams match your filters')
    ).toBeInTheDocument()

    const [clear] = screen.getAllByRole('button', { name: 'Clear filters' })
    await userEvent.click(clear)

    await waitFor(() => {
      expect(lastQuery().search).toBeUndefined()
    })
    expect(lastQuery().is_personal).toBeUndefined()
    expect(currentSearch).toBe('')
  })

  it('clears the search box too, not just the URL', async () => {
    renderTeams('/admin/teams?search=nope')
    await screen.findByText('No teams match your filters')

    const [clear] = screen.getAllByRole('button', { name: 'Clear filters' })
    await userEvent.click(clear)

    // Stale text left in the box would re-commit on the next debounce tick and
    // undo the clear.
    await waitFor(() => {
      expect(screen.getByRole('textbox', { name: 'Search teams' })).toHaveValue(
        ''
      )
    })
  })
})

describe('advanced filters (#1139)', () => {
  it('sends advanced params from the URL and opens the panel', async () => {
    renderTeams(
      '/admin/teams?member_count_min=10&embedding_configured=false&owner_email=boss@example.com&prompt_count_max=3'
    )

    await waitFor(() => {
      expect(mockAdminService.listTeams).toHaveBeenCalled()
    })
    expect(lastQuery()).toMatchObject({
      member_count_min: 10,
      embedding_configured: false,
      owner_email: 'boss@example.com',
      prompt_count_max: 3,
    })
    // The rest stay absent: "any" and empty bounds send nothing.
    expect(lastQuery()).not.toHaveProperty('member_count_max')
    expect(lastQuery()).not.toHaveProperty('llm_configured')
    // A shared link carrying an advanced filter opens with the panel visible.
    expect(
      await screen.findByRole('spinbutton', { name: 'Members minimum' })
    ).toHaveValue(10)
    expect(
      screen.getByRole('textbox', { name: 'Owner email (any owner)' })
    ).toHaveValue('boss@example.com')
    // Two ranges, one tri-state and the owner email; a range counts once.
    expect(screen.getByTestId('advanced-filters-count')).toHaveTextContent('4')
  })

  it('drops an invalid range from the request', async () => {
    renderTeams(
      '/admin/teams?owner_count_min=5&owner_count_max=2&admin_count_min=-1'
    )

    await waitFor(() => {
      expect(mockAdminService.listTeams).toHaveBeenCalled()
    })
    expect(lastQuery()).not.toHaveProperty('owner_count_min')
    expect(lastQuery()).not.toHaveProperty('owner_count_max')
    expect(lastQuery()).not.toHaveProperty('admin_count_min')
  })

  it('commits a tri-state picked in the panel to the URL and the query', async () => {
    renderTeams()
    await screen.findByText('Engineering')

    await userEvent.click(
      screen.getByRole('button', { name: /Advanced filters/ })
    )
    const group = await screen.findByRole('radiogroup', {
      name: 'LLM configured',
    })
    await userEvent.click(within(group).getByRole('radio', { name: 'No' }))

    await waitFor(() => {
      expect(lastQuery().llm_configured).toBe(false)
    })
    expect(currentSearch).toContain('llm_configured=false')
  })

  it('commits the owner email on Enter, trimmed, not per keystroke', async () => {
    renderTeams()
    await screen.findByText('Engineering')
    await userEvent.click(
      screen.getByRole('button', { name: /Advanced filters/ })
    )
    const initialCalls = mockAdminService.listTeams.mock.calls.length

    const input = await screen.findByRole('textbox', {
      name: 'Owner email (any owner)',
    })
    await userEvent.type(input, '  x@corp.com  ')
    expect(mockAdminService.listTeams.mock.calls).toHaveLength(initialCalls)
    await userEvent.keyboard('{Enter}')

    await waitFor(() => {
      expect(lastQuery().owner_email).toBe('x@corp.com')
    })
    expect(mockAdminService.listTeams.mock.calls).toHaveLength(initialCalls + 1)
    expect(currentSearch).toContain('owner_email=x%40corp.com')
  })

  it('clears every advanced key from the URL', async () => {
    mockAdminService.listTeams.mockResolvedValue(
      page({ teams: [], total_count: 0, total_pages: 0 })
    )
    renderTeams(
      '/admin/teams?project_count_min=2&freshness_enabled=true&owner_email=a@b.co'
    )
    await screen.findByText('No teams match your filters')

    const [clear] = screen.getAllByRole('button', { name: 'Clear filters' })
    await userEvent.click(clear)

    await waitFor(() => {
      expect(currentSearch).toBe('')
    })
    expect(lastQuery()).not.toHaveProperty('project_count_min')
    expect(lastQuery()).not.toHaveProperty('freshness_enabled')
    expect(lastQuery().owner_email).toBeUndefined()
    expect(
      screen.getByRole('textbox', { name: 'Owner email (any owner)' })
    ).toHaveValue('')
  })
})

describe('count and setup columns (#1139)', () => {
  const clickHeader = async (name: string) => {
    await userEvent.click(
      screen.getByRole('button', { name: new RegExp(name) })
    )
  }

  it('renders the role, project and resource counts', async () => {
    renderTeams()
    const row = (await screen.findByText('Engineering')).closest('tr')
    const cells = within(row as HTMLElement)
    expect(cells.getByText('1')).toBeInTheDocument()
    expect(cells.getByText('2')).toBeInTheDocument()
    expect(cells.getByText('3')).toBeInTheDocument()
    expect(cells.getByText('11')).toBeInTheDocument()
    expect(
      cells.getByRole('img', { name: 'Embedding: configured' })
    ).toBeInTheDocument()
    expect(
      cells.getByRole('img', { name: 'LLM: not configured' })
    ).toBeInTheDocument()
  })

  it.each([
    ['Owners', 'owner_count'],
    ['Admins', 'admin_count'],
    ['Projects', 'project_count'],
    ['Total resources', 'total_resource_count'],
  ])('sorts by %s', async (header, sortBy) => {
    renderTeams()
    await screen.findByText('Engineering')

    await clickHeader(header)

    await waitFor(() => {
      expect(lastQuery().sort_by).toBe(sortBy)
    })
    expect(currentSearch).toContain(`sort_by=${sortBy}`)
  })

  it('keeps a count sort from the URL', async () => {
    renderTeams('/admin/teams?sort_by=admin_count&sort_order=asc')

    await waitFor(() => {
      expect(mockAdminService.listTeams).toHaveBeenCalled()
    })
    expect(lastQuery().sort_by).toBe('admin_count')
    expect(lastQuery().sort_order).toBe('asc')
  })

  it('does not accept a per-type resource sort it has no column for', async () => {
    renderTeams('/admin/teams?sort_by=prompt_count')

    await waitFor(() => {
      expect(mockAdminService.listTeams).toHaveBeenCalled()
    })
    expect(lastQuery().sort_by).toBe('created_at')
  })

  it('navigates on a click on a setup indicator, like the rest of the row', async () => {
    renderTeams()
    await screen.findByText('Engineering')

    await userEvent.click(
      screen.getByRole('img', { name: 'Embedding: configured' })
    )

    expect(mockNavigate).toHaveBeenCalledWith('/admin/teams/t1')
  })
})
