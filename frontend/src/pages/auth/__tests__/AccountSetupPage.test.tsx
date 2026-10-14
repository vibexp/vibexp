import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import { accountLinkService } from '@/services/accountLinkService'
import { authService } from '@/services/authService'
import { ApiError } from '@/types/errors'

vi.mock('@/services/accountLinkService', () => ({
  accountLinkService: { preview: vi.fn(), completeSetup: vi.fn() },
}))
vi.mock('@/services/authService', () => ({
  authService: { getSignInMethods: vi.fn() },
}))
vi.mock('@/contexts/useAuth', () => ({ useAuth: () => ({ login: vi.fn() }) }))

import { AccountSetupPage } from '../AccountSetupPage'

const links = vi.mocked(accountLinkService)
const auth = vi.mocked(authService)
const go = vi.fn()
const PASSPHRASE = 'a long setup passphrase'

const PREVIEW = {
  purpose: 'setup' as const,
  email: 'ada@example.com',
  name: 'Ada',
  expires_at: '2026-10-21T10:00:00Z',
}

function apiError(
  status: number,
  code: string,
  detail: string,
  field?: string
) {
  return new ApiError({
    type: 'about:blank',
    title: 'Refused',
    status,
    detail,
    code,
    request_id: 'r',
    timestamp: '2026-10-14T00:00:00Z',
    ...(field
      ? { validation_errors: [{ field, message: detail, code: 'common' }] }
      : {}),
  })
}

function renderAt(url: string) {
  globalThis.history.replaceState(null, '', url)
  render(<AccountSetupPage go={go} />, { wrapper: MemoryRouter })
  return userEvent.setup()
}

async function submit(user: ReturnType<typeof userEvent.setup>) {
  const form = within(
    await screen.findByRole('form', { name: 'Set up your account' })
  )
  await user.type(form.getByLabelText('Password'), PASSPHRASE)
  await user.type(form.getByLabelText('Repeat the password'), PASSPHRASE)
  await user.click(
    form.getByRole('button', { name: 'Create password and sign in' })
  )
  return form
}

beforeEach(() => {
  vi.clearAllMocks()
  links.preview.mockResolvedValue(PREVIEW)
  links.completeSetup.mockResolvedValue(undefined)
  auth.getSignInMethods.mockResolvedValue({
    providers: [{ name: 'mock-sso', display_name: 'Mock SSO', type: 'oidc' }],
    password: { enabled: true, reset: 'email' },
  } as never)
})

afterEach(() => {
  globalThis.history.replaceState(null, '', '/')
})

describe('AccountSetupPage', () => {
  it('strips the token from the address bar before asking the server, and sets the password', async () => {
    let hashWhenAsked = 'not asked'
    links.preview.mockImplementation(() => {
      hashWhenAsked = globalThis.location.hash
      return Promise.resolve(PREVIEW)
    })
    const user = renderAt('/account/setup#token=tok-1')

    const form = await submit(user)
    expect(hashWhenAsked).toBe('')
    expect(globalThis.location.href).not.toContain('tok-1')
    expect(links.preview).toHaveBeenCalledWith('tok-1')
    expect(form.getByLabelText('Email')).toHaveValue('ada@example.com')
    await waitFor(() => {
      expect(links.completeSetup).toHaveBeenCalledWith({
        token: 'tok-1',
        password: PASSPHRASE,
        name: 'Ada',
      })
    })
    expect(go).toHaveBeenCalledWith('/')
    expect(
      screen.getByRole('button', { name: 'Continue with Mock SSO' })
    ).toBeVisible()
  })

  it.each([
    ['no token', '/account/setup', null],
    [
      'an unusable link',
      '/account/setup#token=t',
      apiError(401, 'AUTH_INVALID', 'x'),
    ],
  ])('shows one invalid state for %s', async (_case, url, refusal) => {
    if (refusal) links.preview.mockRejectedValue(refusal)
    renderAt(url)

    const invalid = await screen.findByTestId('account-setup-invalid')
    expect(invalid).toHaveTextContent('Ask your administrator for a new one')
    expect(
      within(invalid).getByRole('link', { name: 'Go to sign in' })
    ).toHaveAttribute('href', '/login')
    expect(within(invalid).getByTestId('public-instance-name')).toBeVisible()
  })

  it('treats a reset link opened here as not valid', async () => {
    links.preview.mockResolvedValue({ ...PREVIEW, purpose: 'reset' })
    renderAt('/account/setup#token=t')

    expect(await screen.findByTestId('account-setup-invalid')).toBeVisible()
    expect(screen.queryByRole('form')).toBeNull()
  })

  it('points at the providers while password sign-in is off', async () => {
    links.preview.mockRejectedValue(apiError(404, 'NOT_FOUND', 'Not found'))
    renderAt('/account/setup#token=t')

    const off = await screen.findByTestId('account-setup-password-off')
    expect(off).toHaveTextContent('does not offer password sign-in')
    expect(
      within(off).getByRole('button', { name: 'Continue with Mock SSO' })
    ).toBeVisible()
    expect(
      within(off).getByRole('link', { name: 'Go to sign in' })
    ).toHaveAttribute('href', '/login')
  })

  it('says the password was set when the request failed after setting it, and offers no retry', async () => {
    links.completeSetup.mockRejectedValue(
      apiError(500, 'INTERNAL', 'The request could not be finished')
    )
    const user = renderAt('/account/setup#token=t')
    await submit(user)

    const state = await screen.findByTestId('account-setup-set-not-signed-in')
    expect(state).toHaveTextContent('Your password is set')
    expect(state).toHaveTextContent('ada@example.com')
    expect(screen.queryByRole('form')).toBeNull()
    expect(go).not.toHaveBeenCalled()
  })

  it('keeps the form for another try when nothing was stored (503)', async () => {
    links.completeSetup
      .mockRejectedValueOnce(
        apiError(503, 'SERVICE_UNAVAILABLE', 'Unavailable')
      )
      .mockResolvedValueOnce(undefined)
    const user = renderAt('/account/setup#token=t')
    const form = await submit(user)

    expect(await screen.findByTestId('account-setup-error')).toHaveTextContent(
      'Nothing was saved'
    )
    expect(form.getByLabelText('Password')).toHaveValue(PASSPHRASE)
    await user.click(
      form.getByRole('button', { name: 'Create password and sign in' })
    )
    await waitFor(() => {
      expect(go).toHaveBeenCalledWith('/')
    })
    expect(links.completeSetup).toHaveBeenCalledTimes(2)
  })

  it('puts a policy error on the password field', async () => {
    links.completeSetup.mockRejectedValue(
      apiError(
        400,
        'VALIDATION_FAILED',
        'This password is too common',
        'password'
      )
    )
    const user = renderAt('/account/setup#token=t')
    const form = await submit(user)

    expect(await form.findByText('This password is too common')).toBeVisible()
  })

  it('shows the link as not valid when it was used up meanwhile', async () => {
    links.completeSetup.mockRejectedValue(
      apiError(401, 'AUTH_INVALID', 'Invalid')
    )
    const user = renderAt('/account/setup#token=t')
    await submit(user)

    expect(await screen.findByTestId('account-setup-invalid')).toBeVisible()
  })

  it('offers to try again when the link could not be checked', async () => {
    links.preview
      .mockRejectedValueOnce(apiError(503, 'SERVICE_UNAVAILABLE', 'Try later'))
      .mockResolvedValueOnce(PREVIEW)
    const user = renderAt('/account/setup#token=t')

    await user.click(await screen.findByRole('button', { name: 'Try again' }))
    expect(
      await screen.findByRole('form', { name: 'Set up your account' })
    ).toBeVisible()
    expect(links.preview).toHaveBeenLastCalledWith('t')
  })
})

describe('AccountSetupPage: after review', () => {
  it('says a valid link for an address the allowlist refuses cannot be used, with no retry', async () => {
    links.preview.mockRejectedValue(
      apiError(
        403,
        'access_restricted',
        'Access to this instance is restricted.'
      )
    )
    renderAt('/account/setup#token=t')

    const state = await screen.findByTestId('account-setup-restricted')
    expect(state).toHaveTextContent('Access to this instance is restricted.')
    expect(
      within(state).getByRole('link', { name: 'Go to sign in' })
    ).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull()
  })

  it('treats a malformed token as an invalid link', async () => {
    links.preview.mockRejectedValue(apiError(400, 'BAD_REQUEST', 'Invalid'))
    renderAt('/account/setup#token=t')

    expect(await screen.findByTestId('account-setup-invalid')).toBeVisible()
  })

  it('has one page heading in every state', async () => {
    renderAt('/account/setup#token=t')
    expect(
      await screen.findByRole('heading', {
        level: 1,
        name: 'Set up your account',
      })
    ).toBeVisible()
  })
})
