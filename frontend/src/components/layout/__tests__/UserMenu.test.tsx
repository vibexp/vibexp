/**
 * UserMenu renders the instance-admin "Admin Portal" entry only when the
 * signed-in user's `is_instance_admin` flag is true (issue #315). Both branches
 * are covered; Settings/Sign out are always present as a control.
 */
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import { UserMenu } from '@/components/layout/UserMenu'
import {
  deriveSiteInfo,
  EMPTY_SITE_INFO,
  type SiteInfo,
} from '@/features/site-info/siteInfo'
import { SiteInfoContext } from '@/features/site-info/SiteInfoContext'

const mockTheme = vi.hoisted(() => ({
  theme: 'system',
  setTheme: vi.fn(),
}))
vi.mock('@/lib/theme', () => ({
  useTheme: () => mockTheme,
}))

const mockUseAuth = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => mockUseAuth(),
}))

// Radix DropdownMenu needs these pointer/scroll APIs in jsdom.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
  Element.prototype.hasPointerCapture = vi.fn()
  Element.prototype.releasePointerCapture = vi.fn()
})

function renderMenu() {
  return render(
    <MemoryRouter>
      <UserMenu />
    </MemoryRouter>
  )
}

function authUser(isInstanceAdmin: boolean) {
  return {
    user: {
      id: 'user-1',
      name: 'Alice Admin',
      email: 'alice@example.com',
      is_instance_admin: isInstanceAdmin,
    },
    logout: vi.fn(),
  }
}

afterEach(() => {
  vi.clearAllMocks()
})

it('shows the Admin Portal item for an instance admin', async () => {
  mockUseAuth.mockReturnValue(authUser(true))
  renderMenu()

  await userEvent.click(screen.getByTestId('user-menu'))

  expect(await screen.findByText('Admin Portal')).toBeInTheDocument()
  expect(screen.getByText('Settings')).toBeInTheDocument()
})

it('hides the Admin Portal item for a non-admin', async () => {
  mockUseAuth.mockReturnValue(authUser(false))
  renderMenu()

  await userEvent.click(screen.getByTestId('user-menu'))

  expect(await screen.findByText('Settings')).toBeInTheDocument()
  expect(screen.queryByText('Admin Portal')).not.toBeInTheDocument()
})

// ---------------------------------------------------------------------------
// The instance's own links, from site info (#1360)
// ---------------------------------------------------------------------------

async function openMenuWith(info: Partial<SiteInfo>) {
  mockUseAuth.mockReturnValue(authUser(false))
  render(
    <SiteInfoContext.Provider
      value={deriveSiteInfo({ ...EMPTY_SITE_INFO, ...info })}
    >
      <MemoryRouter>
        <UserMenu />
      </MemoryRouter>
    </SiteInfoContext.Provider>
  )
  await userEvent.click(screen.getByTestId('user-menu'))
  await screen.findByText('Sign out')
}

const siteLink = (label: string) => screen.queryByText(label)?.closest('a')

describe('site info links', () => {
  it('adds no item and no extra separator when nothing is configured', async () => {
    await openMenuWith({})

    expect(screen.queryByText('Support')).not.toBeInTheDocument()
    expect(screen.queryByText('Terms')).not.toBeInTheDocument()
    expect(screen.queryByText('Privacy')).not.toBeInTheDocument()
    // Label | Settings | Theme | Sign out: three separators, and no fourth
    // for a links group that is not there.
    expect(screen.getAllByRole('separator')).toHaveLength(3)
  })

  it('shows all three as links opening in a new tab', async () => {
    await openMenuWith({
      support_url: 'https://acme.test/help',
      terms_url: 'https://acme.test/terms',
      privacy_url: 'https://acme.test/privacy',
    })

    for (const [label, href] of [
      ['Support', 'https://acme.test/help'],
      ['Terms', 'https://acme.test/terms'],
      ['Privacy', 'https://acme.test/privacy'],
    ]) {
      const link = siteLink(label)
      expect(link).toHaveAttribute('href', href)
      expect(link).toHaveAttribute('target', '_blank')
      expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    }
    expect(screen.getAllByRole('separator')).toHaveLength(4)
  })

  it('falls back to a mailto for Support when only an email is configured', async () => {
    await openMenuWith({ company_email: 'hello@acme.test' })

    expect(siteLink('Support')).toHaveAttribute(
      'href',
      'mailto:hello@acme.test'
    )
    expect(screen.queryByText('Terms')).not.toBeInTheDocument()
    expect(screen.queryByText('Privacy')).not.toBeInTheDocument()
  })

  it.each([
    ['Terms', { terms_url: 'https://acme.test/terms' }, ['Support', 'Privacy']],
    [
      'Privacy',
      { privacy_url: 'https://acme.test/privacy' },
      ['Support', 'Terms'],
    ],
  ])(
    'shows %s alone when it is the only one set',
    async (shown, info, hidden) => {
      await openMenuWith(info)

      expect(siteLink(shown)).not.toBeNull()
      for (const label of hidden) {
        expect(screen.queryByText(label)).not.toBeInTheDocument()
      }
    }
  )

  it('keeps Settings and Sign out', async () => {
    await openMenuWith({ support_url: 'https://acme.test/help' })

    expect(screen.getByText('Settings')).toBeInTheDocument()
    expect(screen.getByText('Sign out')).toBeInTheDocument()
  })
})

describe('theme', () => {
  beforeEach(() => {
    mockTheme.theme = 'system'
    mockTheme.setTheme.mockClear()
  })

  it('offers Light, Dark and System, with the current one checked', async () => {
    await openMenuWith({})

    const choices = screen.getAllByRole('menuitemradio')
    expect(choices.map(choice => choice.textContent)).toEqual([
      'Light',
      'Dark',
      'System',
    ])
    expect(
      screen.getByRole('menuitemradio', { name: 'System' })
    ).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('menuitemradio', { name: 'Dark' })).toHaveAttribute(
      'aria-checked',
      'false'
    )
  })

  it('sets the theme and keeps the menu open, so the change can be seen', async () => {
    await openMenuWith({})

    await userEvent
      .setup()
      .click(screen.getByRole('menuitemradio', { name: 'Dark' }))

    expect(mockTheme.setTheme).toHaveBeenCalledWith('dark')
    expect(screen.getByRole('menu')).toBeInTheDocument()
  })
})
