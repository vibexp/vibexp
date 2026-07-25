/**
 * AdminUsers (#459): server-driven filtering, sorting, pagination, and the create
 * flow.
 *
 * The assertions are about the query the page issues and the URL it keeps — a
 * filter that renders but sends nothing would otherwise pass unnoticed.
 */
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router-dom'

import type {
  AdminUserDetail,
  AdminUserListItem,
  AdminUserListResponse,
} from '@/services/adminService'

const mockNavigate = jest.fn()
jest.mock('react-router-dom', () => ({
  ...jest.requireActual<typeof import('react-router-dom')>('react-router-dom'),
  useNavigate: () => mockNavigate,
}))

jest.mock('@/services/adminService', () => ({
  adminService: { listUsers: jest.fn(), createUser: jest.fn() },
}))

import { adminService } from '@/services/adminService'

import { AdminUsers } from '../AdminUsers'

const mockAdminService = adminService as jest.Mocked<typeof adminService>

beforeAll(() => {
  Element.prototype.scrollIntoView = jest.fn()
  Element.prototype.hasPointerCapture = jest.fn()
  Element.prototype.releasePointerCapture = jest.fn()
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
  jest.clearAllMocks()
  mockAdminService.listUsers.mockResolvedValue(page())
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
  expect(mockAdminService.listUsers.mock.calls.length).toBe(before + 1)
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

it('shows an error state on failure', async () => {
  mockAdminService.listUsers.mockRejectedValue(new Error('boom'))
  renderUsers()

  expect(await screen.findByText('Failed to load users')).toBeInTheDocument()
})
