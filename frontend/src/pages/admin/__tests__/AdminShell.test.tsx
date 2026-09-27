/**
 * AdminShell (#456): the shell's own contract — its nav, its section heading,
 * and above all what it must NOT contain, since the whole point of the decoupled
 * branch is that nothing team-scoped reaches an instance-scoped page.
 */
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import { STORAGE_KEYS } from '@/constants/storageKeys'
import { ThemeProvider } from '@/lib/theme'
import { ADMIN_NAV_ITEMS } from '@/pages/admin/admin-nav'
import { AdminShell } from '@/pages/admin/AdminShell'
import { adminService } from '@/services/adminService'
import { sessionStore } from '@/utils/storage'

// The instance email warning banner (#1192) reads this on mount.
vi.mock('@/services/adminService', () => ({
  adminService: { getInstanceEmailSettings: vi.fn() },
}))
const mockEmailSettings = vi.mocked(adminService.getInstanceEmailSettings)

const mockUseAuth = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => mockUseAuth(),
}))

/**
 * Fails the render if the shell (or anything it pulls in) mounts a team- or
 * project-scoped switcher. Asserting on rendered text would not catch a
 * switcher that renders nothing while its data loads.
 */
vi.mock('@/components/layout/TeamSwitcher', () => ({
  TeamSwitcher: () => {
    throw new Error('AdminShell must not mount TeamSwitcher')
  },
}))
vi.mock('@/components/layout/ProjectSwitcher', () => ({
  ProjectSwitcher: () => {
    throw new Error('AdminShell must not mount ProjectSwitcher')
  },
}))

function renderShell(path = '/admin/users') {
  return render(
    <ThemeProvider defaultTheme="light">
      <MemoryRouter initialEntries={[path]}>
        <AdminShell>
          <div>page body</div>
        </AdminShell>
      </MemoryRouter>
    </ThemeProvider>
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  sessionStore.remove(STORAGE_KEYS.ADMIN_EMAIL_BANNER_DISMISSED)
  mockEmailSettings.mockResolvedValue({
    configured: true,
    provider_type: 'smtp',
    has_credential: true,
    is_healthy: true,
  })
  mockUseAuth.mockReturnValue({
    user: {
      id: 'u1',
      email: 'admin@example.com',
      name: 'Ada Admin',
      is_instance_admin: true,
    },
    isLoading: false,
    logout: vi.fn(),
  })
})

it('renders every admin nav item as a link to its section', () => {
  renderShell()

  const nav = screen.getByRole('navigation', { name: 'Admin sections' })
  for (const item of ADMIN_NAV_ITEMS) {
    // getAllBy: the label also appears in the rail tooltip content.
    const [link] = screen.getAllByRole('link', { name: item.label })
    expect(link).toHaveAttribute('href', item.href)
    expect(nav).toContainElement(link)
  }
})

it('includes a Projects entry even before #461 adds the pages', () => {
  renderShell()

  expect(ADMIN_NAV_ITEMS.map(i => i.href)).toEqual([
    '/admin',
    '/admin/users',
    '/admin/teams',
    '/admin/projects',
    '/admin/settings/email',
    '/admin/settings/search',
    '/admin/settings/ai-summary',
  ])
})

it('lists Email under a "Settings" group after the Administration items (#1191)', () => {
  renderShell()

  const nav = screen.getByRole('navigation', { name: 'Admin sections' })
  const labels = within(nav).getAllByText(/^(administration|settings)$/i)
  expect(labels.map(l => l.textContent)).toEqual(['Administration', 'Settings'])
  const settingsGroup = labels[1].parentElement
  if (!settingsGroup) throw new Error('the Settings label has no group')
  const [emailLink] = within(settingsGroup).getAllByRole('link', {
    name: 'Email',
  })
  expect(emailLink).toHaveAttribute('href', '/admin/settings/email')
  // Search and AI Summary (#1202) join the same group.
  expect(
    within(settingsGroup).getAllByRole('link', { name: 'Search' })[0]
  ).toHaveAttribute('href', '/admin/settings/search')
  expect(
    within(settingsGroup).getAllByRole('link', { name: 'AI Summary' })[0]
  ).toHaveAttribute('href', '/admin/settings/ai-summary')
  expect(
    within(settingsGroup).queryByRole('link', { name: 'Users' })
  ).not.toBeInTheDocument()
})

it.each([
  ['/admin/settings/email', 'Email'],
  ['/admin/settings/search', 'Search'],
  ['/admin/settings/ai-summary', 'AI Summary'],
])('titles the Settings page at %s in the shell', (path, heading) => {
  renderShell(path)

  expect(
    screen.getByRole('heading', { name: heading, level: 1 })
  ).toBeInTheDocument()
})

it('renders the section heading for a section path', () => {
  renderShell('/admin/users')

  expect(
    screen.getByRole('heading', { name: 'Users', level: 1 })
  ).toBeInTheDocument()
  expect(screen.getByText('page body')).toBeInTheDocument()
})

it('renders no section heading for a detail path, which titles itself', () => {
  renderShell('/admin/users/u1')

  expect(screen.queryByRole('heading', { level: 1 })).not.toBeInTheDocument()
  expect(screen.getByText('page body')).toBeInTheDocument()
})

it('offers a way back to the product app', () => {
  renderShell()

  expect(screen.getByRole('link', { name: 'Back to app' })).toHaveAttribute(
    'href',
    '/'
  )
})

it('keeps the user menu and theme toggle, both context-free', async () => {
  renderShell()

  expect(screen.getByTestId('user-menu')).toBeInTheDocument()
  await userEvent.click(screen.getByTestId('user-menu'))
  expect(await screen.findByText('admin@example.com')).toBeInTheDocument()
})

it('mounts no search or notification affordance — both are team-scoped', () => {
  renderShell()

  expect(
    screen.queryByRole('button', { name: /search/i })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: /notification/i })
  ).not.toBeInTheDocument()
})

it('opens a mobile drawer with the same nav items', async () => {
  renderShell()

  await userEvent.click(
    screen.getByRole('button', { name: 'Open admin navigation' })
  )

  const dialog = await screen.findByRole('dialog')
  for (const item of ADMIN_NAV_ITEMS) {
    expect(
      screen
        .getAllByRole('link', { name: item.label })
        .some(link => dialog.contains(link))
    ).toBe(true)
  }
})

it('shows the instance email warning above the page when email is unconfigured (#1192)', async () => {
  mockEmailSettings.mockResolvedValue({
    configured: false,
    provider_type: null,
    has_credential: false,
    is_healthy: null,
  })
  renderShell('/admin/users')

  const banner = await screen.findByTestId('admin-email-warning')
  const heading = screen.getByRole('heading', { name: 'Users', level: 1 })
  // The banner sits above the section heading and the page content.
  expect(
    banner.compareDocumentPosition(heading) & Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
  expect(
    banner.compareDocumentPosition(screen.getByText('page body')) &
      Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
})
