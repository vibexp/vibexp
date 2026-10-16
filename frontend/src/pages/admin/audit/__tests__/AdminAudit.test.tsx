/**
 * AdminAudit (#1517): the instance audit log page over #1516's API.
 *
 * Like the other admin list tests, the assertions are mostly about the query
 * issued and the URL kept: a filter that renders but sends nothing is invisible
 * on screen. jsdom applies no CSS, so the table and the phone-width cards both
 * render here; assertions scope to the table where it matters.
 */
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router'
import type { Mocked } from 'vitest'

import type {
  AdminAuditEvent,
  AdminAuditEventPage,
} from '@/services/adminAuditService'

vi.mock('@/services/adminAuditService', () => ({
  adminAuditService: {
    listEvents: vi.fn(),
    exportEvents: vi.fn(),
  },
}))

vi.mock('@/services/adminService', async () => ({
  ...(await vi.importActual<typeof import('@/services/adminService')>(
    '@/services/adminService'
  )),
  adminService: {
    listUsers: vi.fn(),
    listTeams: vi.fn(),
    listProjects: vi.fn(),
    getUser: vi.fn(),
    getTeam: vi.fn(),
    getProject: vi.fn(),
    getSavedFilters: vi.fn(),
    replaceSavedFilters: vi.fn(),
  },
}))

// jsdom has no object URLs; the download itself is covered by its own test.
vi.mock('@/utils/downloadBlob', () => ({ downloadBlob: vi.fn() }))

import { adminAuditService } from '@/services/adminAuditService'
import { adminService } from '@/services/adminService'

import { AdminAudit } from '../AdminAudit'
import { defaultAuditRange } from '../auditListParams'

const mockAudit = adminAuditService as Mocked<typeof adminAuditService>
const mockAdmin = adminService as Mocked<typeof adminService>

beforeAll(() => {
  // Radix Popover/Command/Sheet rely on layout APIs jsdom does not implement.
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

function auditEvent(overrides: Partial<AdminAuditEvent> = {}): AdminAuditEvent {
  return {
    id: '11111111-1111-4111-8111-111111111111',
    created_at: new Date().toISOString(),
    actor_user_id: '22222222-2222-4222-8222-222222222222',
    actor_email: 'ada@example.com',
    actor_name: 'Ada Lovelace',
    actor_type: 'user',
    team_id: 't-1',
    team_name: 'Platform',
    project_id: 'p-1',
    project_name: 'Core',
    operation: 'prompt.update',
    resource_type: 'prompt',
    resource_id: 'r-1',
    resource_name: 'Release notes',
    source: 'mcp',
    assistant_name: 'claude-code',
    api_key_id: null,
    api_key_name: null,
    outcome: 'success',
    ip: null,
    user_agent: null,
    request_id: 'req-1',
    metadata: {
      changed_fields: ['title', 'body'],
      title: { old: 'Draft', new: 'Release notes' },
    },
    ...overrides,
  }
}

function eventPage(
  items: AdminAuditEvent[],
  next: string | null = null
): AdminAuditEventPage {
  return { items, next_cursor: next }
}

let currentSearch = ''

function LocationProbe() {
  currentSearch = useLocation().search
  return null
}

function renderAudit(initialEntry = '/admin/audit') {
  currentSearch = ''
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <AdminAudit />
      <LocationProbe />
    </MemoryRouter>
  )
}

const lastQuery = () => {
  const { calls } = mockAudit.listEvents.mock
  return calls[calls.length - 1][0]
}

beforeEach(() => {
  vi.clearAllMocks()
  mockAudit.listEvents.mockResolvedValue(eventPage([auditEvent()]))
  mockAdmin.listUsers.mockResolvedValue({
    users: [],
    total_count: 0,
    page: 1,
    per_page: 25,
    total_pages: 0,
  })
  mockAdmin.listTeams.mockResolvedValue({
    teams: [],
    total_count: 0,
    page: 1,
    per_page: 25,
    total_pages: 0,
  })
  mockAdmin.listProjects.mockResolvedValue({
    projects: [],
    total_count: 0,
    page: 1,
    per_page: 25,
    total_pages: 0,
  })
  mockAdmin.getSavedFilters.mockResolvedValue({
    list: 'audit',
    presets: [],
    version: 0,
  })
})

it('renders an event row with its human operation label, actor, resource and channel', async () => {
  renderAudit()

  const table = within(await screen.findByRole('table'))
  expect(table.getByText('Prompt updated')).toBeInTheDocument()
  expect(table.getByText('Ada Lovelace')).toBeInTheDocument()
  expect(table.getByText('Release notes')).toBeInTheDocument()
  expect(table.getByText('Platform / Core')).toBeInTheDocument()
  expect(table.getByText('MCP · claude-code')).toBeInTheDocument()
  expect(table.getByText('Success')).toBeInTheDocument()
})

it('shows the absolute UTC instant on hover of a row time', async () => {
  mockAudit.listEvents.mockResolvedValue(
    eventPage([auditEvent({ created_at: '2026-01-02T03:04:05Z' })])
  )
  renderAudit()

  const table = within(await screen.findByRole('table'))
  expect(table.getByText(/Jan 2, 2026/)).toHaveAttribute(
    'title',
    '2026-01-02 03:04:05 UTC'
  )
})

it('applies the last seven days when the URL carries no range, without writing it to the URL', async () => {
  renderAudit()

  await waitFor(() => {
    expect(mockAudit.listEvents).toHaveBeenCalled()
  })
  const { from, to } = defaultAuditRange(new Date())
  const query = lastQuery()
  expect(query.from).toBe(from?.toISOString())
  expect(new Date(query.to ?? '').getDate()).toBe(to?.getDate())
  expect(query).toMatchObject({ limit: 50 })
  expect(query).not.toHaveProperty('cursor')
  expect(currentSearch).toBe('')
})

it('round-trips every filter from the URL into the request (a reload keeps them)', async () => {
  renderAudit(
    '/admin/audit?created_from=2026-01-01&created_to=2026-01-31&search=ada' +
      '&team_id=t-1&actor_user_id=u-1&project_id=p-1' +
      '&operation=prompt.update,authz.denied&resource_type=prompt' +
      '&source=mcp&outcome=denied,failure'
  )

  await waitFor(() => {
    expect(mockAudit.listEvents).toHaveBeenCalled()
  })
  expect(lastQuery()).toMatchObject({
    q: 'ada',
    team_id: 't-1',
    actor_user_id: 'u-1',
    project_id: 'p-1',
    operation: ['prompt.update', 'authz.denied'],
    resource_type: 'prompt',
    source: ['mcp'],
    outcome: ['denied', 'failure'],
    from: new Date(2026, 0, 1).toISOString(),
    to: new Date(2026, 0, 31, 23, 59, 59, 999).toISOString(),
  })
})

it('shows the picked user by name when a shared link carries only the id', async () => {
  mockAdmin.getUser.mockResolvedValue({
    id: 'u-1',
    name: 'Grace Hopper',
    email: 'grace@example.com',
  } as Awaited<ReturnType<typeof adminService.getUser>>)
  renderAudit('/admin/audit?actor_user_id=u-1')

  expect(
    await screen.findByRole('combobox', { name: 'Filter by user' })
  ).toHaveTextContent('Grace Hopper')
  expect(mockAdmin.getUser).toHaveBeenCalledWith('u-1')
})

it('writes a chosen operation to the URL and resends the query', async () => {
  renderAudit()
  await screen.findByRole('table')

  await userEvent.click(
    screen.getByRole('combobox', { name: 'Filter by operation' })
  )
  await userEvent.click(await screen.findByText('Permission denied'))

  await waitFor(() => {
    expect(lastQuery().operation).toEqual(['authz.denied'])
  })
  expect(new URLSearchParams(currentSearch).get('operation')).toBe(
    'authz.denied'
  )
})

it('pages forward along next_cursor and back to the cursor each page was loaded with', async () => {
  mockAudit.listEvents
    .mockResolvedValueOnce(eventPage([auditEvent()], 'cursor-2'))
    .mockResolvedValueOnce(
      eventPage(
        [auditEvent({ id: 'e2', resource_name: 'Second page' })],
        'cursor-3'
      )
    )
    .mockResolvedValueOnce(
      eventPage([auditEvent({ id: 'e3', resource_name: 'Third page' })])
    )
  renderAudit()
  await screen.findByRole('table')

  await userEvent.click(screen.getByRole('button', { name: /Next/ }))
  await waitFor(() => {
    expect(lastQuery().cursor).toBe('cursor-2')
  })
  expect(await screen.findByText('Page 2')).toBeInTheDocument()

  await userEvent.click(screen.getByRole('button', { name: /Next/ }))
  await waitFor(() => {
    expect(lastQuery().cursor).toBe('cursor-3')
  })
  expect(await screen.findByText('Page 3')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /Next/ })).toBeDisabled()

  mockAudit.listEvents.mockResolvedValue(eventPage([auditEvent()], 'cursor-3'))
  await userEvent.click(screen.getByRole('button', { name: /Previous/ }))
  await waitFor(() => {
    expect(lastQuery().cursor).toBe('cursor-2')
  })
  await userEvent.click(screen.getByRole('button', { name: /Previous/ }))
  await waitFor(() => {
    expect(lastQuery()).not.toHaveProperty('cursor')
  })
  expect(await screen.findByText('Page 1')).toBeInTheDocument()
})

it('hides the pager when everything fits on one page', async () => {
  renderAudit()
  await screen.findByRole('table')

  expect(
    screen.queryByRole('navigation', { name: 'Audit log pages' })
  ).not.toBeInTheDocument()
})

it('renders the empty state for no matching events', async () => {
  mockAudit.listEvents.mockResolvedValue(eventPage([]))
  renderAudit()

  expect(
    await screen.findByText('No audit events match these filters')
  ).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /Export CSV/ })).toBeDisabled()
})

it('shows loading skeletons until the first page arrives', () => {
  mockAudit.listEvents.mockReturnValue(new Promise(() => undefined))
  renderAudit()

  expect(
    screen.getAllByTestId('list-page-skeleton-row').length
  ).toBeGreaterThan(0)
})

it('shows an error state on failure', async () => {
  mockAudit.listEvents.mockRejectedValue(new Error('boom'))
  renderAudit()

  expect(
    await screen.findByText('Failed to load the audit log')
  ).toBeInTheDocument()
})

it('opens the detail drawer with old → new changes and uncaptured client info', async () => {
  renderAudit()
  const table = within(await screen.findByRole('table'))

  await userEvent.click(table.getByText('Prompt updated'))

  const drawer = await screen.findByRole('dialog')
  const changes = within(drawer).getByRole('list', { name: 'Changes' })
  expect(changes).toHaveTextContent('title')
  expect(changes).toHaveTextContent('Draft')
  expect(changes).toHaveTextContent('Release notes')
  expect(changes).toHaveTextContent('body: changed (content is not recorded)')
  // Neither the IP address nor the user agent was captured for this event.
  expect(within(drawer).getAllByText('Not captured')).toHaveLength(2)
  expect(within(drawer).getByText('claude-code')).toBeInTheDocument()
})

it('exports the filter on screen, without the cursor or page size', async () => {
  mockAudit.exportEvents.mockResolvedValue({
    blob: new Blob(['x']),
    filename: 'admin-audit.csv',
    totalCount: 1,
    truncated: false,
  })
  renderAudit('/admin/audit?outcome=denied&team_id=t-1')
  await screen.findByRole('table')

  await userEvent.click(screen.getByRole('button', { name: /Export CSV/ }))

  await waitFor(() => {
    expect(mockAudit.exportEvents).toHaveBeenCalled()
  })
  const exported = mockAudit.exportEvents.mock.calls[0][0]
  expect(exported).toMatchObject({ outcome: ['denied'], team_id: 't-1' })
  expect(exported).not.toHaveProperty('cursor')
  expect(exported).not.toHaveProperty('limit')
  expect(exported.from).toBe(lastQuery().from)
})

it('offers the saved filters of the audit list', async () => {
  renderAudit()
  await screen.findByRole('table')

  await waitFor(() => {
    expect(mockAdmin.getSavedFilters).toHaveBeenCalledWith('audit')
  })
  expect(screen.getByRole('button', { name: /Presets/ })).toBeInTheDocument()
})
