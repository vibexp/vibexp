/**
 * UserMenu renders the instance-admin "Admin Portal" entry only when the
 * signed-in user's `is_instance_admin` flag is true (issue #315). Both branches
 * are covered; Settings/Sign out are always present as a control.
 */
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

import { UserMenu } from '@/components/layout/UserMenu'

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
