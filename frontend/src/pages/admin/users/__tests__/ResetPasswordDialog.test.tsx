import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { adminUserAccessService } from '@/services/adminUserAccessService'
import { ApiError } from '@/types/errors'

vi.mock('@/services/adminUserAccessService', () => ({
  adminUserAccessService: { issueResetLink: vi.fn() },
}))

import { ResetPasswordDialog } from '../ResetPasswordDialog'

const service = vi.mocked(adminUserAccessService)

function refusal(status: number, code: string, detail: string) {
  return new ApiError({
    type: 'about:blank',
    title: 'Refused',
    status,
    detail,
    code,
    request_id: 'r',
    timestamp: '2026-10-14T00:00:00Z',
  })
}

function renderDialog() {
  render(
    <ResetPasswordDialog
      open
      onOpenChange={vi.fn()}
      userId="u-1"
      email="grace@example.com"
    />
  )
  return userEvent.setup()
}

const invalidate = () =>
  screen.getByRole('checkbox', {
    name: 'Sign out and disable the current password now',
  })

beforeEach(() => {
  vi.clearAllMocks()
})

describe('ResetPasswordDialog', () => {
  it('emails a link and leaves the current password alone by default', async () => {
    service.issueResetLink.mockResolvedValue({
      delivery: 'email',
      expires_at: '2026-10-14T11:00:00Z',
      invalidated: false,
    })
    const user = renderDialog()
    expect(invalidate()).not.toBeChecked()
    await user.click(
      screen.getByRole('button', { name: 'Email it to grace@example.com' })
    )

    expect(await screen.findByTestId('reset-link-emailed')).toBeVisible()
    expect(service.issueResetLink).toHaveBeenCalledWith('u-1', {
      delivery: 'email',
      invalidate_now: false,
    })
    expect(screen.queryByTestId('reset-invalidated')).toBeNull()
  })

  it('asks a second time before disabling the password, and Back changes nothing', async () => {
    service.issueResetLink.mockResolvedValue({
      delivery: 'link',
      link_url: 'http://localhost/account/reset#token=secret-5',
      expires_at: '2026-10-15T10:00:00Z',
      invalidated: true,
    })
    const user = renderDialog()
    await user.click(invalidate())
    await user.click(screen.getByRole('button', { name: 'Show the link here' }))

    const confirm = screen.getByTestId('reset-confirm')
    expect(confirm).toHaveTextContent('signed out everywhere')
    expect(service.issueResetLink).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Back' }))
    expect(screen.queryByTestId('reset-confirm')).toBeNull()
    expect(service.issueResetLink).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Show the link here' }))
    await user.click(
      screen.getByRole('button', { name: 'Disable and show the link' })
    )
    expect(await screen.findByTestId('link-reveal-value')).toHaveTextContent(
      'secret-5'
    )
    expect(screen.getByTestId('reset-invalidated')).toHaveTextContent(
      'current password is disabled'
    )
    expect(service.issueResetLink).toHaveBeenCalledWith('u-1', {
      delivery: 'link',
      invalidate_now: true,
    })
  })

  it('suggests handing over a link when an email may not be sent', async () => {
    service.issueResetLink.mockRejectedValue(
      refusal(
        409,
        'email_unverified_shared',
        'This address has not been verified and someone signs in to it with a provider.'
      )
    )
    const user = renderDialog()
    await user.click(
      screen.getByRole('button', { name: 'Email it to grace@example.com' })
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Show the link here and hand it over instead'
    )
  })

  it('says the password may already be stopped when a disabling reset fails', async () => {
    service.issueResetLink.mockRejectedValue(
      refusal(500, 'INTERNAL', 'Internal error')
    )
    const user = renderDialog()
    await user.click(invalidate())
    await user.click(
      screen.getByRole('button', { name: 'Email it to grace@example.com' })
    )
    await user.click(
      screen.getByRole('button', { name: 'Disable and email the link' })
    )
    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(
        'may already be disabled'
      )
    })
  })
})
