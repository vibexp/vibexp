import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import { accountLinkService } from '@/services/accountLinkService'
import { ApiError } from '@/types/errors'

vi.mock('@/services/accountLinkService', () => ({
  accountLinkService: { preview: vi.fn(), completeReset: vi.fn() },
}))

import { AccountResetPage } from '../AccountResetPage'

const links = vi.mocked(accountLinkService)
const go = vi.fn()
const PASSPHRASE = 'a long reset passphrase'

const PREVIEW = {
  purpose: 'reset' as const,
  email: 'ada@example.com',
  name: 'Ada',
  expires_at: '2026-10-14T10:00:00Z',
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
  render(<AccountResetPage go={go} />, { wrapper: MemoryRouter })
  return userEvent.setup()
}

async function submit(
  user: ReturnType<typeof userEvent.setup>,
  revoke = false
) {
  const form = within(
    await screen.findByRole('form', { name: 'Set a new password' })
  )
  await user.type(form.getByLabelText('Password'), PASSPHRASE)
  await user.type(form.getByLabelText('Repeat the password'), PASSPHRASE)
  if (revoke) {
    await user.click(
      form.getByRole('checkbox', {
        name: 'Also revoke API keys and connected tools',
      })
    )
  }
  await user.click(
    form.getByRole('button', { name: 'Set password and sign in' })
  )
  return form
}

beforeEach(() => {
  vi.clearAllMocks()
  links.preview.mockResolvedValue(PREVIEW)
  links.completeReset.mockResolvedValue(undefined)
})

afterEach(() => {
  globalThis.history.replaceState(null, '', '/')
})

describe('AccountResetPage', () => {
  it('strips the token before asking the server, sets the password and loads the app', async () => {
    let hashWhenAsked = 'not asked'
    links.preview.mockImplementation(() => {
      hashWhenAsked = globalThis.location.hash
      return Promise.resolve(PREVIEW)
    })
    const user = renderAt('/account/reset#token=tok-1')

    const form = await submit(user)
    expect(hashWhenAsked).toBe('')
    expect(globalThis.location.href).not.toContain('tok-1')
    expect(form.getByLabelText('Email')).toHaveValue('ada@example.com')
    await waitFor(() => {
      expect(links.completeReset).toHaveBeenCalledWith({
        token: 'tok-1',
        password: PASSPHRASE,
        revoke_api_access: false,
      })
    })
    expect(go).toHaveBeenCalledWith('/')
  })

  it('sends the revoke choice', async () => {
    const user = renderAt('/account/reset#token=t')
    await submit(user, true)

    await waitFor(() => {
      expect(links.completeReset).toHaveBeenCalledWith(
        expect.objectContaining({ revoke_api_access: true })
      )
    })
  })

  it.each([
    ['no token', '/account/reset', null],
    [
      'an expired, used or unknown link',
      '/account/reset#token=t',
      apiError(401, 'AUTH_INVALID', 'x'),
    ],
    [
      'a malformed token',
      '/account/reset#token=t',
      apiError(400, 'BAD_REQUEST', 'x'),
    ],
  ])(
    'shows one invalid state for %s, with a way on',
    async (_case, url, refusal) => {
      if (refusal) links.preview.mockRejectedValue(refusal)
      renderAt(url)

      const state = await screen.findByTestId('account-reset-invalid')
      expect(state).toHaveTextContent('Forgot password?')
      expect(
        within(state).getByRole('link', { name: 'Go to sign in' })
      ).toHaveAttribute('href', '/login')
    }
  )

  it('treats a setup link opened here as not valid', async () => {
    links.preview.mockResolvedValue({ ...PREVIEW, purpose: 'setup' })
    renderAt('/account/reset#token=t')

    expect(await screen.findByTestId('account-reset-invalid')).toBeVisible()
    expect(screen.queryByRole('form')).toBeNull()
  })

  it('says so while password sign-in is off (404)', async () => {
    links.preview.mockRejectedValue(apiError(404, 'NOT_FOUND', 'Not found'))
    renderAt('/account/reset#token=t')

    expect(
      await screen.findByTestId('account-reset-password-off')
    ).toHaveTextContent('does not offer password sign-in')
  })

  it('says the account cannot be used when the allowlist refuses it (403)', async () => {
    links.preview.mockRejectedValue(
      apiError(403, 'access_restricted', 'Access is restricted')
    )
    renderAt('/account/reset#token=t')

    expect(
      await screen.findByTestId('account-reset-restricted')
    ).toHaveTextContent('Access is restricted')
  })

  it('shows the link as not valid when it was used up meanwhile', async () => {
    links.completeReset.mockRejectedValue(
      apiError(401, 'AUTH_INVALID', 'Invalid')
    )
    const user = renderAt('/account/reset#token=t')
    await submit(user)

    expect(await screen.findByTestId('account-reset-invalid')).toBeVisible()
    expect(go).not.toHaveBeenCalled()
  })

  it('says the password was changed when the request failed after changing it', async () => {
    links.completeReset.mockRejectedValue(apiError(500, 'INTERNAL', 'x'))
    const user = renderAt('/account/reset#token=t')
    await submit(user)

    const state = await screen.findByTestId('account-reset-set-not-signed-in')
    expect(state).toHaveTextContent('Your password is changed')
    expect(state).toHaveTextContent('ada@example.com')
    expect(screen.queryByRole('form')).toBeNull()
  })

  it('keeps the form for another try when nothing was stored (503)', async () => {
    links.completeReset.mockRejectedValueOnce(
      apiError(503, 'SERVICE_UNAVAILABLE', 'x')
    )
    const user = renderAt('/account/reset#token=t')
    const form = await submit(user)

    expect(await screen.findByTestId('account-reset-error')).toHaveTextContent(
      'Nothing was saved'
    )
    expect(form.getByLabelText('Password')).toHaveValue(PASSPHRASE)
  })

  it('puts a policy error on the password field and focuses it', async () => {
    links.completeReset.mockRejectedValue(
      apiError(
        400,
        'VALIDATION_FAILED',
        'This password is too common',
        'password'
      )
    )
    const user = renderAt('/account/reset#token=t')
    const form = await submit(user)

    expect(await form.findByText('This password is too common')).toBeVisible()
    expect(form.getByLabelText('Password')).toHaveFocus()
  })

  it('shows password sign-in as off when it was switched off after the check', async () => {
    links.completeReset.mockRejectedValue(
      apiError(404, 'NOT_FOUND', 'Not found')
    )
    const user = renderAt('/account/reset#token=t')
    await submit(user)

    expect(
      await screen.findByTestId('account-reset-password-off')
    ).toBeVisible()
    expect(screen.queryByRole('form')).toBeNull()
  })
})
