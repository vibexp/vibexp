import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import {
  type AdminAuthProvider,
  type AdminInstanceAdmin,
  type AdminInstanceAdminList,
  type AdminInstanceProviderAdmin,
  authSettingsService,
} from '@/services/authSettingsService'

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('@/services/authSettingsService', () => ({
  authSettingsService: {
    listAdmins: vi.fn(),
    grantAdmin: vi.fn(),
    revokeAdmin: vi.fn(),
  },
}))

import { InstanceAdminsSection } from '../InstanceAdminsSection'
import { apiError, provider } from './fixtures'

const service = vi.mocked(authSettingsService)
const onChanged = vi.fn()

const ada: AdminInstanceAdmin = {
  user_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
  email: 'ada@example.com',
  name: 'Ada Admin',
  granted_by_user_id: null,
  granted_at: '2026-10-01T10:00:00Z',
}

const list: AdminInstanceAdminList = {
  root_admins: ['root@example.com'],
  admins: [ada],
  provider_admins: [],
}

const keycloak = provider({
  id: '44444444-4444-4444-8444-444444444444',
  slug: 'keycloak',
  display_name: 'Company SSO',
})

const grace: AdminInstanceProviderAdmin = {
  user_id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
  email: 'grace@example.com',
  name: 'Grace',
  provider_id: keycloak.id,
  provider_slug: 'keycloak',
  matched_value: 'VibeXP Admins',
  evaluated_at: '2026-11-01T10:00:00Z',
}

async function renderSection(
  canManage: boolean,
  admins: AdminInstanceAdminList = list,
  providers?: AdminAuthProvider[]
) {
  service.listAdmins.mockResolvedValue(admins)
  render(
    <InstanceAdminsSection
      canManage={canManage}
      providers={providers}
      onChanged={onChanged}
    />
  )
  await screen.findByTestId('auth-admins-section')
  return userEvent.setup()
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('InstanceAdminsSection', () => {
  it('lists root admins read-only with their source, then the grants', async () => {
    await renderSection(true)
    const root = within(screen.getByTestId('auth-root-admin'))
    expect(root.getByText('root@example.com')).toBeVisible()
    expect(root.getByText('Server configuration')).toBeVisible()
    // A root admin comes from the server configuration: nothing to remove.
    expect(root.queryByRole('button')).toBeNull()

    const granted = within(screen.getByTestId('auth-db-admin'))
    expect(granted.getByText('Ada Admin')).toBeVisible()
    expect(granted.getByText('ada@example.com')).toBeVisible()
    expect(granted.getByText('Granted')).toBeVisible()
    expect(granted.queryByText('Server configuration')).toBeNull()
  })

  it('lists provider-sourced admins read-only, naming the provider (#1645)', async () => {
    await renderSection(true, { ...list, provider_admins: [grace] }, [keycloak])
    expect(screen.getAllByRole('listitem')).toHaveLength(3)
    const row = screen.getByTestId('auth-provider-admin')
    const sourced = within(row)
    expect(sourced.getByText('Grace')).toBeVisible()
    expect(sourced.getByText('grace@example.com')).toBeVisible()
    expect(sourced.getByText('Identity provider')).toBeVisible()
    expect(sourced.getByText('VibeXP Admins')).toBeVisible()
    expect(row).toHaveTextContent('via Company SSO')
    expect(row).toHaveTextContent(
      'Managed by Company SSO: change it at the identity provider or in the role mapping.'
    )
    // Only the mapping changes it, so there is nothing to remove.
    expect(sourced.queryByRole('button')).toBeNull()
    expect(
      screen.queryByRole('button', { name: 'Remove grace@example.com' })
    ).toBeNull()
  })

  it('falls back to the provider slug without the providers list', async () => {
    await renderSection(true, { ...list, provider_admins: [grace] })
    expect(screen.getByTestId('auth-provider-admin')).toHaveTextContent(
      'via keycloak'
    )
  })

  it('keeps Remove on the grant of a user the mapping also made an admin', async () => {
    const both: AdminInstanceProviderAdmin = {
      ...grace,
      user_id: ada.user_id,
      email: ada.email,
      name: ada.name,
    }
    await renderSection(true, { ...list, provider_admins: [both] }, [keycloak])
    expect(
      within(screen.getByTestId('auth-db-admin')).getByRole('button', {
        name: 'Remove ada@example.com',
      })
    ).toBeVisible()
    expect(
      within(screen.getByTestId('auth-provider-admin')).queryByRole('button')
    ).toBeNull()
  })

  it('shows the server message when a grant targets a provider-sourced admin', async () => {
    service.grantAdmin.mockRejectedValue(
      apiError(409, 'CONFLICT', {
        detail: 'grace@example.com is an instance admin through keycloak',
      })
    )
    const user = await renderSection(true, {
      ...list,
      provider_admins: [grace],
    })
    await user.type(
      screen.getByLabelText('Grant instance admin to'),
      'grace@example.com'
    )
    await user.click(screen.getByRole('button', { name: 'Grant' }))
    expect(await screen.findByTestId('auth-admins-error')).toHaveTextContent(
      'grace@example.com is an instance admin through keycloak'
    )
    expect(onChanged).not.toHaveBeenCalled()
  })

  it('renders no grant or revoke control for a non-root admin', async () => {
    await renderSection(false)
    expect(screen.getByText('Ada Admin')).toBeVisible()
    expect(screen.queryByRole('button', { name: /Remove/ })).toBeNull()
    expect(screen.queryByRole('form')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Grant' })).toBeNull()
    expect(screen.getByTestId('auth-admins-readonly')).toBeVisible()
  })

  it('renders the grant form and a Remove per grant for a root admin', async () => {
    await renderSection(true)
    expect(
      screen.getByRole('form', { name: 'Grant instance admin' })
    ).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Remove ada@example.com' })
    ).toBeVisible()
    expect(screen.queryByTestId('auth-admins-readonly')).toBeNull()
  })

  it('says so when nobody has been granted', async () => {
    await renderSection(true, { ...list, admins: [] })
    expect(screen.getByTestId('auth-admins-empty')).toBeVisible()
  })

  it('grants by email and re-reads the list', async () => {
    service.grantAdmin.mockResolvedValue({ ...ada, email: 'bob@example.com' })
    const user = await renderSection(true)
    await user.type(
      screen.getByLabelText('Grant instance admin to'),
      ' Bob@Example.com '
    )
    await user.click(screen.getByRole('button', { name: 'Grant' }))

    await waitFor(() => {
      expect(service.grantAdmin).toHaveBeenCalledTimes(1)
    })
    expect(service.grantAdmin.mock.calls[0][0]).toEqual({
      email: expect.stringMatching(/^bob@example\.com$/i) as string,
    })
    await waitFor(() => {
      expect(service.listAdmins).toHaveBeenCalledTimes(2)
    })
    expect(onChanged).toHaveBeenCalledTimes(1)
    expect(screen.getByLabelText('Grant instance admin to')).toHaveValue('')
  })

  it('never sends a malformed address', async () => {
    const user = await renderSection(true)
    await user.type(screen.getByLabelText('Grant instance admin to'), 'bob@')
    await user.click(screen.getByRole('button', { name: 'Grant' }))
    expect(
      screen.getByText('Enter the full email address of an existing user.')
    ).toBeVisible()
    expect(service.grantAdmin).not.toHaveBeenCalled()
  })

  it('shows why a grant was refused', async () => {
    service.grantAdmin.mockRejectedValue(
      apiError(404, 'RESOURCE_NOT_FOUND', { detail: 'user not found' })
    )
    const user = await renderSection(true)
    await user.type(
      screen.getByLabelText('Grant instance admin to'),
      'ghost@example.com'
    )
    await user.click(screen.getByRole('button', { name: 'Grant' }))
    expect(await screen.findByTestId('auth-admins-error')).toHaveTextContent(
      'user not found'
    )
    expect(onChanged).not.toHaveBeenCalled()
  })

  it('revokes after a confirmation', async () => {
    service.revokeAdmin.mockResolvedValue(undefined)
    const user = await renderSection(true)
    await user.click(
      screen.getByRole('button', { name: 'Remove ada@example.com' })
    )
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent(
      'Remove ada@example.com as an instance admin?'
    )
    expect(service.revokeAdmin).not.toHaveBeenCalled()

    await user.click(
      within(dialog).getByRole('button', { name: 'Remove admin' })
    )
    await waitFor(() => {
      expect(service.revokeAdmin).toHaveBeenCalledWith(ada.user_id)
    })
    await waitFor(() => {
      expect(onChanged).toHaveBeenCalledTimes(1)
    })
    expect(service.listAdmins).toHaveBeenCalledTimes(2)
  })

  it('shows a refused revoke', async () => {
    service.revokeAdmin.mockRejectedValue(
      apiError(403, 'FORBIDDEN', { detail: 'only a root admin may revoke' })
    )
    const user = await renderSection(true)
    await user.click(
      screen.getByRole('button', { name: 'Remove ada@example.com' })
    )
    await user.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', {
        name: 'Remove admin',
      })
    )
    expect(await screen.findByTestId('auth-admins-error')).toHaveTextContent(
      'only a root admin may revoke'
    )
  })

  it('shows a load failure', async () => {
    service.listAdmins.mockRejectedValue(new Error('boom'))
    render(<InstanceAdminsSection canManage />)
    expect(await screen.findByText('boom')).toBeVisible()
  })
})
