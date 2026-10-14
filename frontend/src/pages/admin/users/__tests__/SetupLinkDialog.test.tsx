import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { adminService } from '@/services/adminService'
import { ApiError } from '@/types/errors'

vi.mock('@/services/adminService', () => ({
  adminService: { issueSetupLink: vi.fn() },
}))

import { SetupLinkDialog } from '../SetupLinkDialog'

const service = vi.mocked(adminService)
const onOpenChange = vi.fn()

function renderDialog() {
  render(
    <SetupLinkDialog
      open
      onOpenChange={onOpenChange}
      userId="u-1"
      email="grace@example.com"
    />
  )
  return userEvent.setup()
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('SetupLinkDialog', () => {
  it('emails a new link and says until when it works', async () => {
    service.issueSetupLink.mockResolvedValue({
      delivery: 'email',
      expires_at: '2026-10-21T10:00:00Z',
    })
    const user = renderDialog()
    await user.click(
      screen.getByRole('button', { name: 'Email it to grace@example.com' })
    )

    expect(await screen.findByTestId('setup-link-emailed')).toHaveTextContent(
      'Sent to grace@example.com'
    )
    expect(service.issueSetupLink).toHaveBeenCalledWith('u-1', {
      delivery: 'email',
    })
    expect(screen.queryByTestId('link-reveal')).toBeNull()
  })

  it('shows a link once, and closing drops it', async () => {
    service.issueSetupLink.mockResolvedValue({
      delivery: 'link',
      link_url: 'http://localhost/account/setup#token=secret-2',
      expires_at: '2026-10-21T10:00:00Z',
    })
    const user = renderDialog()
    await user.click(screen.getByRole('button', { name: 'Show the link here' }))

    expect(await screen.findByTestId('link-reveal-value')).toHaveTextContent(
      'secret-2'
    )
    await user.click(screen.getByRole('button', { name: 'Done' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
    expect(screen.queryByText(/secret-2/)).toBeNull()
  })

  it("shows the server's sentence for a refusal and lets the admin try the other way", async () => {
    service.issueSetupLink
      .mockRejectedValueOnce(
        new ApiError({
          type: 'about:blank',
          title: 'Conflict',
          status: 409,
          detail:
            'No email provider is configured, so the link cannot be emailed.',
          code: 'email_unavailable',
          request_id: 'r',
          timestamp: '2026-10-14T00:00:00Z',
        })
      )
      .mockResolvedValueOnce({
        delivery: 'link',
        link_url: 'http://localhost/account/setup#token=secret-3',
        expires_at: '2026-10-21T10:00:00Z',
      })
    const user = renderDialog()
    await user.click(
      screen.getByRole('button', { name: 'Email it to grace@example.com' })
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'No email provider is configured'
    )
    await user.click(screen.getByRole('button', { name: 'Show the link here' }))
    await waitFor(() => {
      expect(screen.getByTestId('link-reveal-value')).toHaveTextContent(
        'secret-3'
      )
    })
  })
})
