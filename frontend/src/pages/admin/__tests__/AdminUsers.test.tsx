/**
 * AdminUsers (#459): server-driven filtering, sorting, pagination, and the create
 * flow.
 *
 * The assertions are about the query the page issues and the URL it keeps — a
 * filter that renders but sends nothing would otherwise pass unnoticed.
 */
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router'
import type { Mocked } from 'vitest'

import type {
  AdminUserDetail,
  AdminUserListItem,
  AdminUserListResponse,
} from '@/services/adminService'

const mockNavigate = vi.hoisted(() => vi.fn())
vi.mock('react-router', async () => ({
  ...(await vi.importActual<typeof import('react-router')>('react-router')),
  useNavigate: () => mockNavigate,
}))

vi.mock('@/services/adminService', () => ({
  adminService: {
    listUsers: vi.fn(),
    createUser: vi.fn(),
    getSavedFilters: vi.fn(),
    replaceSavedFilters: vi.fn(),
    exportUsers: vi.fn(),
  },
}))

// jsdom has no object URLs; the download itself is covered by its own test.
vi.mock('@/utils/downloadBlob', () => ({ downloadBlob: vi.fn() }))

import { formatDate } from '@/lib/time'
import { advancedKeys } from '@/pages/admin/filters/advancedFilterParams'
import { USER_ADVANCED_FILTERS } from '@/pages/admin/users/userAdvancedFilters'
import { adminService } from '@/services/adminService'
import { storage, STORAGE_KEYS } from '@/utils/storage'

import { AdminUsers } from '../AdminUsers'

const mockAdminService = adminService as Mocked<typeof adminService>

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

function listItem(
  overrides: Partial<AdminUserListItem> = {}
): AdminUserListItem {
  return {
    id: 'u1',
    email: 'ada@example.com',
    name: 'Ada',
    idp_provider: 'google',
    status: 'active',
    created_at: '2026-01-01T00:00:00Z',
    team_count: 2,
    project_count: 3,
    // Distinct values so each column's cell can be found by its number.
    resource_counts: {
      prompts: 11,
      memories: 12,
      artifacts: 13,
      blueprints: 14,
      agents: 15,
      feeds: 16,
      feed_items: 17,
      comments: 18,
      attachments: 19,
      total: 135,
    },
    last_resource_created_at: null,
    ...overrides,
  }
}

function page(
  overrides: Partial<AdminUserListResponse> = {}
): AdminUserListResponse {
  return {
    users: [listItem()],
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

function renderUsers(initialEntry = '/admin/users') {
  currentSearch = ''
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <AdminUsers />
      <LocationProbe />
    </MemoryRouter>
  )
}

const lastQuery = () => {
  const { calls } = mockAdminService.listUsers.mock
  return calls[calls.length - 1][0]
}

beforeEach(() => {
  vi.clearAllMocks()
  storage.remove(STORAGE_KEYS.ADMIN_USERS_COLUMNS)
  mockAdminService.listUsers.mockResolvedValue(page())
  mockAdminService.getSavedFilters.mockResolvedValue({
    list: 'users',
    presets: [],
    version: 0,
  })
})

it('renders a row with provider and team count, unbadged when active', async () => {
  renderUsers()

  expect(await screen.findByText('ada@example.com')).toBeInTheDocument()
  expect(screen.getByText('Ada')).toBeInTheDocument()
  expect(screen.getByText('google')).toBeInTheDocument()
  expect(screen.getByText('2')).toBeInTheDocument()
  expect(screen.queryByText('Suspended')).not.toBeInTheDocument()
})

it('badges a suspended account in the list', async () => {
  mockAdminService.listUsers.mockResolvedValue(
    page({
      users: [
        listItem({ id: 'u1', email: 'active@example.com', status: 'active' }),
        listItem({ id: 'u2', email: 'gone@example.com', status: 'suspended' }),
      ],
    })
  )
  renderUsers()

  const row = (await screen.findByText('gone@example.com')).closest('tr')
  expect(row).not.toBeNull()
  // Scanning the list has to show which accounts cannot sign in at all.
  expect(row?.textContent).toContain('Suspended')
})

it('navigates to the detail page on row click', async () => {
  renderUsers()

  await userEvent.click(await screen.findByText('ada@example.com'))

  expect(mockNavigate).toHaveBeenCalledWith('/admin/users/u1')
})

it('sends no filter params and keeps a clean URL on first load', async () => {
  renderUsers()

  await waitFor(() => {
    expect(mockAdminService.listUsers).toHaveBeenCalled()
  })
  expect(lastQuery()).toEqual({
    page: 1,
    limit: 20,
    search: undefined,
    status: undefined,
    idp_provider: undefined,
    created_from: undefined,
    created_to: undefined,
    sort_by: 'created_at',
    sort_order: 'desc',
  })
  expect(currentSearch).toBe('')
})

describe('the status filter', () => {
  const selectStatus = async (label: string) => {
    await userEvent.click(
      screen.getByRole('combobox', { name: 'Account status' })
    )
    await userEvent.click(await screen.findByRole('option', { name: label }))
  }

  it('narrows to suspended accounts', async () => {
    renderUsers()
    await screen.findByText('ada@example.com')

    await selectStatus('Suspended')

    await waitFor(() => {
      expect(lastQuery().status).toBe('suspended')
    })
    expect(currentSearch).toContain('status=suspended')
  })

  it('sends no status param for "All statuses"', async () => {
    renderUsers('/admin/users?status=active')
    await screen.findByText('ada@example.com')
    await waitFor(() => {
      expect(lastQuery().status).toBe('active')
    })

    await selectStatus('All statuses')

    await waitFor(() => {
      expect(lastQuery().status).toBeUndefined()
    })
    expect(currentSearch).not.toContain('status')
  })

  it('ignores a status the API would reject', async () => {
    // The API's enum is active|suspended; anything else is a 400.
    renderUsers('/admin/users?status=deleted')

    await waitFor(() => {
      expect(mockAdminService.listUsers).toHaveBeenCalled()
    })
    expect(lastQuery().status).toBeUndefined()
  })
})

it('filters by identity provider', async () => {
  renderUsers()
  await screen.findByText('ada@example.com')

  await userEvent.click(
    screen.getByRole('combobox', { name: 'Identity provider' })
  )
  await userEvent.click(await screen.findByRole('option', { name: 'oidc' }))

  await waitFor(() => {
    expect(lastQuery().idp_provider).toBe('oidc')
  })
})

it('rehydrates every filter from the URL on mount', async () => {
  renderUsers(
    '/admin/users?search=ada&status=suspended&idp_provider=google&created_from=2026-07-01&created_to=2026-07-24&sort_by=team_count&sort_order=asc&page=2'
  )

  await waitFor(() => {
    expect(mockAdminService.listUsers).toHaveBeenCalled()
  })
  expect(lastQuery()).toMatchObject({
    page: 2,
    search: 'ada',
    status: 'suspended',
    idp_provider: 'google',
    sort_by: 'team_count',
    sort_order: 'asc',
  })
  expect(screen.getByRole('textbox', { name: 'Search users' })).toHaveValue(
    'ada'
  )
})

it('debounces the search box into a single request', async () => {
  renderUsers()
  await screen.findByText('ada@example.com')
  const before = mockAdminService.listUsers.mock.calls.length

  await userEvent.type(
    screen.getByRole('textbox', { name: 'Search users' }),
    'ada'
  )

  await waitFor(() => {
    expect(lastQuery().search).toBe('ada')
  })
  expect(mockAdminService.listUsers.mock.calls).toHaveLength(before + 1)
})

describe('sorting', () => {
  it('sorts by team count descending on first click', async () => {
    renderUsers()
    await screen.findByText('ada@example.com')

    await userEvent.click(screen.getByRole('button', { name: /Teams/ }))

    await waitFor(() => {
      expect(lastQuery().sort_by).toBe('team_count')
    })
    expect(lastQuery().sort_order).toBe('desc')
  })

  it('flips direction on the active column', async () => {
    renderUsers('/admin/users?sort_by=email&sort_order=desc')
    await screen.findByText('ada@example.com')

    await userEvent.click(screen.getByRole('button', { name: /Email/ }))

    await waitFor(() => {
      expect(lastQuery().sort_order).toBe('asc')
    })
  })

  it('falls back to the default for a column the API rejects', async () => {
    renderUsers('/admin/users?sort_by=provider')

    await waitFor(() => {
      expect(mockAdminService.listUsers).toHaveBeenCalled()
    })
    expect(lastQuery().sort_by).toBe('created_at')
  })
})

it('resets to page 1 when a filter changes', async () => {
  renderUsers('/admin/users?page=5')
  await screen.findByText('ada@example.com')
  await waitFor(() => {
    expect(lastQuery().page).toBe(5)
  })

  await userEvent.click(
    screen.getByRole('combobox', { name: 'Account status' })
  )
  await userEvent.click(
    await screen.findByRole('option', { name: 'Suspended' })
  )

  await waitFor(() => {
    expect(lastQuery().page).toBe(1)
  })
})

describe('empty states', () => {
  beforeEach(() => {
    mockAdminService.listUsers.mockResolvedValue(
      page({ users: [], total_count: 0, total_pages: 0 })
    )
  })

  it('says "no users yet" for an empty instance', async () => {
    renderUsers()

    expect(await screen.findByText('No users yet')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Clear filters' })
    ).not.toBeInTheDocument()
  })

  it('counts a status filter alone as filtered', async () => {
    renderUsers('/admin/users?status=suspended')

    expect(
      await screen.findByText('No users match your filters')
    ).toBeInTheDocument()
    expect(screen.queryByText('No users yet')).not.toBeInTheDocument()
  })

  it('clears the filters and the search box together', async () => {
    renderUsers('/admin/users?search=nope&status=suspended')
    await screen.findByText('No users match your filters')

    const [clear] = screen.getAllByRole('button', { name: 'Clear filters' })
    await userEvent.click(clear)

    await waitFor(() => {
      expect(lastQuery().search).toBeUndefined()
    })
    expect(lastQuery().status).toBeUndefined()
    expect(currentSearch).toBe('')
    expect(screen.getByRole('textbox', { name: 'Search users' })).toHaveValue(
      ''
    )
  })
})

describe('creating a user', () => {
  const created: AdminUserDetail = {
    id: 'new-1',
    email: 'new.user@example.com',
    name: 'New User',
    idp_provider: null,
    status: 'active',
    created_at: '2026-07-25T00:00:00Z',
    memberships: [],
  }

  const openDialog = async () => {
    await userEvent.click(screen.getByRole('button', { name: /New user/ }))
    return screen.findByRole('dialog')
  }

  it('creates the account and opens it', async () => {
    mockAdminService.createUser.mockResolvedValue(created)
    renderUsers()
    await screen.findByText('ada@example.com')

    await openDialog()
    await userEvent.type(screen.getByLabelText('Email'), 'new.user@example.com')
    await userEvent.type(screen.getByLabelText('Name'), 'New User')
    await userEvent.click(screen.getByRole('button', { name: 'Create user' }))

    await waitFor(() => {
      expect(mockAdminService.createUser).toHaveBeenCalledWith({
        email: 'new.user@example.com',
        name: 'New User',
      })
    })
    expect(mockNavigate).toHaveBeenCalledWith('/admin/users/new-1')
  })

  it('omits the provider when none is chosen', async () => {
    mockAdminService.createUser.mockResolvedValue(created)
    renderUsers()
    await screen.findByText('ada@example.com')

    await openDialog()
    await userEvent.type(screen.getByLabelText('Email'), 'a@example.com')
    await userEvent.type(screen.getByLabelText('Name'), 'A')
    await userEvent.click(screen.getByRole('button', { name: 'Create user' }))

    await waitFor(() => {
      expect(mockAdminService.createUser).toHaveBeenCalled()
    })
    // Absent rather than an empty string: the field is optional and the API
    // records whatever it is given verbatim.
    expect(mockAdminService.createUser.mock.calls[0][0]).not.toHaveProperty(
      'idp_provider'
    )
  })

  it('includes the provider when one is chosen', async () => {
    mockAdminService.createUser.mockResolvedValue(created)
    renderUsers()
    await screen.findByText('ada@example.com')

    await openDialog()
    await userEvent.type(screen.getByLabelText('Email'), 'a@example.com')
    await userEvent.type(screen.getByLabelText('Name'), 'A')
    await userEvent.type(
      screen.getByLabelText('Expected identity provider (optional)'),
      'oidc'
    )
    await userEvent.click(screen.getByRole('button', { name: 'Create user' }))

    await waitFor(() => {
      expect(mockAdminService.createUser).toHaveBeenCalledWith(
        expect.objectContaining({ idp_provider: 'oidc' })
      )
    })
  })

  it('requires both email and name', async () => {
    renderUsers()
    await screen.findByText('ada@example.com')
    await openDialog()

    expect(screen.getByRole('button', { name: 'Create user' })).toBeDisabled()

    await userEvent.type(screen.getByLabelText('Email'), 'a@example.com')
    expect(screen.getByRole('button', { name: 'Create user' })).toBeDisabled()

    await userEvent.type(screen.getByLabelText('Name'), 'A')
    expect(screen.getByRole('button', { name: 'Create user' })).toBeEnabled()
  })

  it('shows a duplicate-email failure inline and keeps the form open', async () => {
    mockAdminService.createUser.mockRejectedValue(
      new Error('a user with that email already exists')
    )
    renderUsers()
    await screen.findByText('ada@example.com')

    await openDialog()
    await userEvent.type(screen.getByLabelText('Email'), 'ada@example.com')
    await userEvent.type(screen.getByLabelText('Name'), 'Ada')
    await userEvent.click(screen.getByRole('button', { name: 'Create user' }))

    expect(
      await screen.findByText('a user with that email already exists')
    ).toBeInTheDocument()
    // The email is the thing to correct, so the form stays put.
    expect(screen.getByLabelText('Email')).toBeInTheDocument()
    expect(mockNavigate).not.toHaveBeenCalled()
  })

  it('does not carry a failed attempt into the next one', async () => {
    mockAdminService.createUser.mockRejectedValue(new Error('boom'))
    renderUsers()
    await screen.findByText('ada@example.com')
    await openDialog()
    await userEvent.type(screen.getByLabelText('Email'), 'a@example.com')
    await userEvent.type(screen.getByLabelText('Name'), 'A')
    await userEvent.click(screen.getByRole('button', { name: 'Create user' }))
    await screen.findByText('boom')

    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await openDialog()

    expect(screen.getByLabelText('Email')).toHaveValue('')
    expect(screen.queryByText('boom')).not.toBeInTheDocument()
  })
})

describe('advanced filters (#1134)', () => {
  const openPanel = async () => {
    await userEvent.click(
      screen.getByRole('button', { name: /Advanced filters/ })
    )
  }

  it('declares all twelve count ranges and the last-resource range', () => {
    expect(advancedKeys(USER_ADVANCED_FILTERS)).toHaveLength(26)
    expect(advancedKeys(USER_ADVANCED_FILTERS)).toEqual(
      expect.arrayContaining([
        'total_resource_count_min',
        'attachment_count_max',
        'last_resource_created_from',
        'last_resource_created_to',
      ])
    )
  })

  it('sends a team-count range and a per-type range under their API names', async () => {
    renderUsers()
    await screen.findByText('ada@example.com')
    await openPanel()

    await userEvent.type(
      screen.getByRole('spinbutton', { name: 'Teams minimum' }),
      '2'
    )
    await userEvent.tab()
    await waitFor(() => {
      expect(lastQuery().team_count_min).toBe(2)
    })

    await userEvent.type(
      screen.getByRole('spinbutton', { name: 'Feed items maximum' }),
      '5{Enter}'
    )
    await waitFor(() => {
      expect(lastQuery().feed_item_count_max).toBe(5)
    })
    expect(lastQuery()).toMatchObject({ team_count_min: 2, page: 1 })
    expect(currentSearch).toContain('team_count_min=2')
    expect(currentSearch).toContain('feed_item_count_max=5')
  })

  it('rehydrates advanced filters from the URL with the panel open', async () => {
    renderUsers(
      '/admin/users?prompt_count_min=3&total_resource_count_max=9&project_count_min=1&last_resource_created_from=2026-07-01&last_resource_created_to=2026-07-24'
    )

    await waitFor(() => {
      expect(mockAdminService.listUsers).toHaveBeenCalled()
    })
    const query = lastQuery()
    expect(query).toMatchObject({
      prompt_count_min: 3,
      total_resource_count_max: 9,
      project_count_min: 1,
    })
    // Local days become instants, the upper bound at end of day.
    expect(query.last_resource_created_from).toBe(
      new Date(2026, 6, 1).toISOString()
    )
    expect(query.last_resource_created_to).toBe(
      new Date(2026, 6, 24, 23, 59, 59, 999).toISOString()
    )
    expect(
      screen.getByRole('spinbutton', { name: 'Prompts minimum' })
    ).toHaveValue(3)
    expect(screen.getByTestId('advanced-filters-count')).toHaveTextContent('4')
  })

  it('never sends an invalid URL value', async () => {
    renderUsers(
      '/admin/users?prompt_count_min=-1&team_count_min=5&team_count_max=2&memory_count_max=1.5'
    )

    await waitFor(() => {
      expect(mockAdminService.listUsers).toHaveBeenCalled()
    })
    const query = lastQuery()
    expect(query).not.toHaveProperty('prompt_count_min')
    expect(query).not.toHaveProperty('team_count_min')
    expect(query).not.toHaveProperty('team_count_max')
    expect(query).not.toHaveProperty('memory_count_max')
  })

  describe('empty state', () => {
    beforeEach(() => {
      mockAdminService.listUsers.mockResolvedValue(
        page({ users: [], total_count: 0, total_pages: 0 })
      )
    })

    it('counts an advanced-only filter as filtered, and Clear removes it', async () => {
      renderUsers('/admin/users?memory_count_min=1')

      expect(
        await screen.findByText('No users match your filters')
      ).toBeInTheDocument()
      expect(lastQuery().memory_count_min).toBe(1)

      const [clear] = screen.getAllByRole('button', { name: 'Clear filters' })
      await userEvent.click(clear)

      await waitFor(() => {
        expect(lastQuery()).not.toHaveProperty('memory_count_min')
      })
      expect(currentSearch).toBe('')
      expect(await screen.findByText('No users yet')).toBeInTheDocument()
    })
  })
})

describe('activity columns (#1134)', () => {
  // Sortable headers carry role="button", so read the cells, not the role.
  const headers = () =>
    [...document.querySelectorAll('thead th')].map(th => th.textContent)

  it('shows the default count columns and hides the rest', async () => {
    renderUsers()
    await screen.findByText('ada@example.com')

    expect(headers()).toEqual([
      'Email',
      'Name',
      'Provider',
      'Teams',
      'Projects',
      'Total resources',
      'Prompts',
      'Memories',
      'Artifacts',
      'Last resource',
      'Created',
    ])
    const row = screen.getByText('ada@example.com').closest('tr')
    const cells = [...(row?.querySelectorAll('td') ?? [])].map(
      td => td.textContent
    )
    expect(cells.slice(3, 10)).toEqual(['2', '3', '135', '11', '12', '13', '—'])
  })

  it('renders the last-resource timestamp when there is one', async () => {
    mockAdminService.listUsers.mockResolvedValue(
      page({
        users: [listItem({ last_resource_created_at: '2026-07-20T10:00:00Z' })],
      })
    )
    renderUsers()
    await screen.findByText('ada@example.com')

    const row = screen.getByText('ada@example.com').closest('tr')
    expect(row?.querySelectorAll('td')[9].textContent).toBe(
      formatDate('2026-07-20T10:00:00Z')
    )
  })

  it('sorts a count column descending, then ascending', async () => {
    renderUsers()
    await screen.findByText('ada@example.com')

    await userEvent.click(screen.getByRole('button', { name: /Prompts/ }))
    await waitFor(() => {
      expect(lastQuery()).toMatchObject({
        sort_by: 'prompt_count',
        sort_order: 'desc',
      })
    })

    await userEvent.click(screen.getByRole('button', { name: /Prompts/ }))
    await waitFor(() => {
      expect(lastQuery().sort_order).toBe('asc')
    })
    expect(lastQuery().sort_by).toBe('prompt_count')
  })

  it('sorts by last resource', async () => {
    renderUsers()
    await screen.findByText('ada@example.com')

    await userEvent.click(screen.getByRole('button', { name: /Last resource/ }))

    await waitFor(() => {
      expect(lastQuery().sort_by).toBe('last_resource_created_at')
    })
  })

  it('always shows the column it is sorted by', async () => {
    renderUsers('/admin/users?sort_by=comment_count&sort_order=asc')
    await screen.findByText('ada@example.com')

    expect(lastQuery().sort_by).toBe('comment_count')
    expect(headers()).toContain('Comments')
    expect(screen.getByText('18')).toBeInTheDocument()
  })

  it('reveals a hidden column through the chooser and remembers it', async () => {
    const { unmount } = renderUsers()
    await screen.findByText('ada@example.com')
    expect(headers()).not.toContain('Blueprints')

    await userEvent.click(screen.getByRole('button', { name: /Columns/ }))
    await userEvent.click(
      await screen.findByRole('menuitemcheckbox', { name: 'Blueprints' })
    )

    await waitFor(() => {
      expect(headers()).toContain('Blueprints')
    })
    expect(screen.getByText('14')).toBeInTheDocument()

    unmount()
    renderUsers()
    await screen.findByText('ada@example.com')
    expect(headers()).toContain('Blueprints')
  })

  it('hides a default column through the chooser', async () => {
    renderUsers()
    await screen.findByText('ada@example.com')

    await userEvent.click(screen.getByRole('button', { name: /Columns/ }))
    await userEvent.click(
      await screen.findByRole('menuitemcheckbox', { name: 'Memories' })
    )

    await waitFor(() => {
      expect(headers()).not.toContain('Memories')
    })
  })

  it('ignores unknown or malformed stored visibility', async () => {
    storage.set(STORAGE_KEYS.ADMIN_USERS_COLUMNS, {
      agent_count: true,
      prompt_count: 'no',
      bogus: true,
    })
    renderUsers()
    await screen.findByText('ada@example.com')

    expect(headers()).toContain('Agents')
    expect(headers()).toContain('Prompts')
    expect(headers()).not.toContain('bogus')
  })

  it('falls back to the defaults for a non-object stored value', async () => {
    storage.set(STORAGE_KEYS.ADMIN_USERS_COLUMNS, ['agent_count'])
    renderUsers()
    await screen.findByText('ada@example.com')

    expect(headers()).not.toContain('Agents')
    expect(headers()).toContain('Prompts')
  })
})

it('shows an error state on failure', async () => {
  mockAdminService.listUsers.mockRejectedValue(new Error('boom'))
  renderUsers()

  expect(await screen.findByText('Failed to load users')).toBeInTheDocument()
})

it('applies a saved preset as the whole URL query (#1148)', async () => {
  mockAdminService.getSavedFilters.mockResolvedValue({
    list: 'users',
    presets: [{ id: 'p1', name: 'Suspended', query: { status: 'suspended' } }],
    version: 1,
  })
  renderUsers('/admin/users?idp_provider=google&page=3')
  await screen.findByTestId('saved-filters-count')
  expect(mockAdminService.getSavedFilters).toHaveBeenCalledWith('users')

  await userEvent.click(screen.getByRole('button', { name: /Presets/ }))
  await userEvent.click(
    await screen.findByRole('menuitem', { name: 'Suspended' })
  )

  await waitFor(() => {
    expect(lastQuery()).toMatchObject({ status: 'suspended', page: 1 })
  })
  expect(lastQuery().idp_provider).toBeUndefined()
  expect(currentSearch).toBe('?status=suspended')
})

describe('CSV export (#1150)', () => {
  const exportButton = () => screen.getByRole('button', { name: /export csv/i })

  it('exports exactly the list query, minus page and limit', async () => {
    mockAdminService.exportUsers.mockResolvedValue({
      blob: new Blob([]),
      filename: 'admin-users.csv',
      totalCount: 1,
      truncated: false,
    })
    renderUsers(
      '/admin/users?page=2&search=ada&status=suspended&idp_provider=github&prompt_count_min=3&total_resource_count_max=9&last_resource_created_from=2026-07-01&last_resource_created_to=2026-07-24&sort_by=team_count&sort_order=asc'
    )

    await waitFor(() => {
      expect(exportButton()).toBeEnabled()
    })
    await userEvent.click(exportButton())

    await waitFor(() => {
      expect(mockAdminService.exportUsers).toHaveBeenCalledTimes(1)
    })
    const listFilters = Object.fromEntries(
      Object.entries(lastQuery()).filter(
        ([key]) => key !== 'page' && key !== 'limit'
      )
    )
    const exported = mockAdminService.exportUsers.mock.calls[0][0]
    expect(exported).toStrictEqual(listFilters)
    expect(exported).not.toHaveProperty('page')
    expect(exported).not.toHaveProperty('limit')
    // Guard against both sides being empty: the URL's filters really are there.
    expect(exported).toMatchObject({
      search: 'ada',
      status: 'suspended',
      idp_provider: 'github',
      prompt_count_min: 3,
      total_resource_count_max: 9,
      sort_by: 'team_count',
      sort_order: 'asc',
    })
    expect(exported.last_resource_created_from).toEqual(expect.any(String))
    expect(exported.last_resource_created_to).toEqual(expect.any(String))
  })

  it('is disabled when the filtered list is empty', async () => {
    mockAdminService.listUsers.mockResolvedValue(
      page({ users: [], total_count: 0, total_pages: 0 })
    )
    renderUsers()

    // Settled on the empty result, not still loading.
    expect(await screen.findByText('No users yet')).toBeInTheDocument()
    expect(exportButton()).toBeDisabled()
    expect(mockAdminService.exportUsers).not.toHaveBeenCalled()
  })
})
