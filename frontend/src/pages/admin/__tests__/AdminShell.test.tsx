/**
 * AdminShell (#456): the shell's own contract — its nav, its section heading,
 * and above all what it must NOT contain, since the whole point of the decoupled
 * branch is that nothing team-scoped reaches an instance-scoped page.
 */
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import { ThemeProvider } from '@/lib/theme'
import { ADMIN_NAV_ITEMS } from '@/pages/admin/admin-nav'
import { AdminShell } from '@/pages/admin/AdminShell'

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
  ])
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
