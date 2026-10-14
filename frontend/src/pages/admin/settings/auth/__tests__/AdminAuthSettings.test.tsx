import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import { authService } from '@/services/authService'
import { authSettingsService } from '@/services/authSettingsService'

const mockUseAuth = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/useAuth', () => ({
  useAuth: () => mockUseAuth(),
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('@/services/authSettingsService', () => ({
  authSettingsService: {
    listProviders: vi.fn(),
    updateProvider: vi.fn(),
    getAllowlist: vi.fn(),
    updateAllowlist: vi.fn(),
    previewAllowlist: vi.fn(),
    listAdmins: vi.fn(),
    listAudit: vi.fn(),
    getPolicy: vi.fn(),
    updatePolicy: vi.fn(),
  },
}))

vi.mock('@/services/authService', () => ({
  authService: { getSignInMethods: vi.fn() },
}))

import { AdminAuthSettings } from '../AdminAuthSettings'
import { allowlist, google, policy, provider } from './fixtures'

const service = vi.mocked(authSettingsService)
const auth = vi.mocked(authService)

const signedInAs = (flags: {
  is_instance_admin: boolean
  is_root_instance_admin: boolean
}) => {
  mockUseAuth.mockReturnValue({ user: { id: 'u1', ...flags } })
}

beforeEach(() => {
  vi.clearAllMocks()
  service.listProviders.mockResolvedValue({
    providers: [provider(), google],
    version: 7,
  })
  service.getAllowlist.mockResolvedValue(allowlist())
  service.listAdmins.mockResolvedValue({
    root_admins: ['root@example.com'],
    admins: [
      {
        user_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
        email: 'ada@example.com',
        name: null,
        granted_by_user_id: null,
        granted_at: '2026-10-01T10:00:00Z',
      },
    ],
  })
  service.listAudit.mockResolvedValue({ entries: [], next_cursor: null })
  service.getPolicy.mockResolvedValue(policy())
  auth.getSignInMethods.mockResolvedValue({
    providers: [],
    password: { enabled: false, reset: 'email' },
  })
})

describe('AdminAuthSettings', () => {
  it('renders the five sections, the sign-in policy first', async () => {
    signedInAs({ is_instance_admin: true, is_root_instance_admin: true })
    render(<AdminAuthSettings />, { wrapper: MemoryRouter })
    const policySection = await screen.findByTestId('auth-policy-section')
    const providersSection = await screen.findByTestId('auth-providers-section')
    expect(
      policySection.compareDocumentPosition(providersSection) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(await screen.findByTestId('auth-providers-section')).toBeVisible()
    expect(await screen.findByTestId('auth-allowlist-section')).toBeVisible()
    expect(await screen.findByTestId('auth-admins-section')).toBeVisible()
    expect(screen.getByTestId('auth-audit-section')).toBeVisible()
    await waitFor(() => {
      expect(service.listAudit).toHaveBeenCalledWith(
        'auth_providers',
        expect.anything()
      )
    })
  })

  it('offers grant and revoke to a root admin', async () => {
    signedInAs({ is_instance_admin: true, is_root_instance_admin: true })
    render(<AdminAuthSettings />, { wrapper: MemoryRouter })
    await screen.findByTestId('auth-admins-section')
    expect(
      screen.getByRole('button', { name: 'Remove ada@example.com' })
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Grant' })).toBeVisible()
  })

  it('hides grant and revoke from an admin who is not root', async () => {
    signedInAs({ is_instance_admin: true, is_root_instance_admin: false })
    render(<AdminAuthSettings />, { wrapper: MemoryRouter })
    await screen.findByTestId('auth-admins-section')
    expect(
      screen.queryByRole('button', { name: 'Remove ada@example.com' })
    ).toBeNull()
    expect(screen.queryByRole('button', { name: 'Grant' })).toBeNull()
    // The rest of the page is theirs to use.
    expect(screen.getByRole('button', { name: 'Add provider' })).toBeEnabled()
  })

  it('hides them while the flag is absent', async () => {
    mockUseAuth.mockReturnValue({ user: { id: 'u1', is_instance_admin: true } })
    render(<AdminAuthSettings />, { wrapper: MemoryRouter })
    await screen.findByTestId('auth-admins-section')
    expect(screen.queryByRole('button', { name: 'Grant' })).toBeNull()
  })

  it('re-reads the providers and the history after an allowlist save', async () => {
    signedInAs({ is_instance_admin: true, is_root_instance_admin: true })
    service.previewAllowlist.mockResolvedValue({
      count: 0,
      sample: [],
      sample_truncated: false,
    })
    service.updateAllowlist.mockResolvedValue(
      allowlist({ domains: ['example.com'], active: true, version: 1 })
    )
    // The allowlist save bumped the shared version the providers save against.
    service.listProviders
      .mockResolvedValueOnce({ providers: [provider(), google], version: 7 })
      .mockResolvedValue({ providers: [provider(), google], version: 8 })
    service.updateProvider.mockResolvedValue({ provider: google, version: 9 })

    const user = userEvent.setup()
    render(<AdminAuthSettings />, { wrapper: MemoryRouter })
    await screen.findByTestId('auth-allowlist-section')
    await waitFor(() => {
      expect(service.listAudit).toHaveBeenCalledTimes(1)
    })

    await user.type(screen.getByLabelText('Allowed domains'), 'example.com,')
    await user.click(screen.getByRole('button', { name: 'Save allowlist' }))
    await waitFor(() => {
      expect(service.listProviders).toHaveBeenCalledTimes(2)
    })
    await waitFor(() => {
      expect(service.listAudit).toHaveBeenCalledTimes(2)
    })

    await user.click(screen.getByRole('switch', { name: 'Google enabled' }))
    await waitFor(() => {
      expect(service.updateProvider).toHaveBeenCalledTimes(1)
    })
    expect(service.updateProvider.mock.calls[0][1]).toMatchObject({
      expected_version: 8,
    })
  })

  it('re-reads the history and the providers after a sign-in policy save', async () => {
    signedInAs({ is_instance_admin: true, is_root_instance_admin: true })
    service.updatePolicy.mockResolvedValue(
      policy({ password_login_enabled: true, version: 1 })
    )
    const user = userEvent.setup()
    render(<AdminAuthSettings />, { wrapper: MemoryRouter })
    await screen.findByTestId('auth-policy-section')
    await waitFor(() => {
      expect(service.listAudit).toHaveBeenCalledTimes(1)
    })
    expect(service.listProviders).toHaveBeenCalledTimes(1)

    await user.click(screen.getByRole('switch', { name: 'Password sign-in' }))
    await user.click(
      screen.getByRole('button', { name: 'Save sign-in policy' })
    )
    await waitFor(() => {
      expect(service.listAudit).toHaveBeenCalledTimes(2)
    })
    // A policy save bumps the version the providers are saved against: with a
    // stale one the admin's next provider change would be a false conflict.
    await waitFor(() => {
      expect(service.listProviders).toHaveBeenCalledTimes(2)
    })
  })
})
