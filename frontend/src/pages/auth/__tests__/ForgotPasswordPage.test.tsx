import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'

import {
  deriveSiteInfo,
  EMPTY_SITE_INFO,
  type SiteInfo,
} from '@/features/site-info/siteInfo'
import { SiteInfoContext } from '@/features/site-info/SiteInfoContext'
import { authService } from '@/services/authService'
import { ApiError } from '@/types/errors'

vi.mock('@/services/authService', () => ({
  authService: { getSignInMethods: vi.fn(), requestPasswordReset: vi.fn() },
}))

import { ForgotPasswordPage } from '../ForgotPasswordPage'

const auth = vi.mocked(authService)

function methods(enabled: boolean, reset: 'email' | 'admin') {
  return { providers: [], password: { enabled, reset } } as never
}

function renderPage(info: SiteInfo = EMPTY_SITE_INFO) {
  render(
    <SiteInfoContext.Provider value={deriveSiteInfo(info)}>
      <MemoryRouter>
        <ForgotPasswordPage />
      </MemoryRouter>
    </SiteInfoContext.Provider>
  )
  return userEvent.setup()
}

beforeEach(() => {
  vi.clearAllMocks()
  auth.requestPasswordReset.mockResolvedValue(undefined)
})

describe('ForgotPasswordPage', () => {
  it('with email: sends the request and answers the same whatever the address', async () => {
    auth.getSignInMethods.mockResolvedValue(methods(true, 'email'))
    const user = renderPage()

    const form = within(
      await screen.findByRole('form', { name: 'Request a password reset' })
    )
    await user.type(form.getByLabelText('Email'), 'ada@example.com')
    await user.click(
      form.getByRole('button', { name: 'Email me a reset link' })
    )

    const sent = await screen.findByTestId('forgot-password-sent')
    expect(sent).toHaveTextContent(
      'If ada@example.com belongs to an account here'
    )
    expect(auth.requestPasswordReset).toHaveBeenCalledWith('ada@example.com')
  })

  it('checks the address before sending', async () => {
    auth.getSignInMethods.mockResolvedValue(methods(true, 'email'))
    const user = renderPage()

    const form = within(
      await screen.findByRole('form', { name: 'Request a password reset' })
    )
    await user.click(
      form.getByRole('button', { name: 'Email me a reset link' })
    )

    expect(await form.findByText('Enter your email address.')).toHaveAttribute(
      'role',
      'alert'
    )
    expect(form.getByLabelText('Email')).toHaveAttribute('aria-invalid', 'true')
    expect(auth.requestPasswordReset).not.toHaveBeenCalled()
  })

  it('says to wait when the device is throttled', async () => {
    auth.getSignInMethods.mockResolvedValue(methods(true, 'email'))
    auth.requestPasswordReset.mockRejectedValue(
      new ApiError({
        type: 'about:blank',
        title: 'Too Many Requests',
        status: 429,
        detail: 'x',
        code: 'RATE_LIMITED',
        request_id: 'r',
        timestamp: '2026-10-14T00:00:00Z',
      })
    )
    const user = renderPage()

    const form = within(
      await screen.findByRole('form', { name: 'Request a password reset' })
    )
    await user.type(form.getByLabelText('Email'), 'ada@example.com')
    await user.click(
      form.getByRole('button', { name: 'Email me a reset link' })
    )

    expect(
      await screen.findByTestId('forgot-password-error')
    ).toHaveTextContent('Wait a minute')
  })

  it('without email: asks the administrator, with the support contact when set', async () => {
    auth.getSignInMethods.mockResolvedValue(methods(true, 'admin'))
    renderPage({ ...EMPTY_SITE_INFO, company_email: 'help@example.com' })

    const state = await screen.findByTestId('forgot-password-admin')
    expect(state).toHaveTextContent('Ask your administrator')
    expect(screen.queryByRole('form')).toBeNull()
    expect(
      within(state).getByRole('link', { name: 'Contact support' })
    ).toHaveAttribute('href', 'mailto:help@example.com')
  })

  it('without email and no contact: no support link', async () => {
    auth.getSignInMethods.mockResolvedValue(methods(true, 'admin'))
    renderPage()

    await screen.findByTestId('forgot-password-admin')
    expect(screen.queryByTestId('forgot-password-support')).toBeNull()
  })

  it.each([
    [
      'password sign-in is off',
      () => auth.getSignInMethods.mockResolvedValue(methods(false, 'email')),
    ],
    [
      'the methods cannot be read',
      () => auth.getSignInMethods.mockRejectedValue(new Error('down')),
    ],
  ])('points back to sign-in when %s', async (_case, arrange) => {
    arrange()
    renderPage()

    const state = await screen.findByTestId('forgot-password-unavailable')
    await waitFor(() => {
      expect(
        within(state).getByRole('link', { name: 'Go to sign in' })
      ).toHaveAttribute('href', '/login')
    })
  })
})
