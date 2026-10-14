import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { adminUserAccessService } from '@/services/adminUserAccessService'
import { ApiError } from '@/types/errors'

vi.mock('@/services/adminUserAccessService', () => ({
  adminUserAccessService: { removePassword: vi.fn() },
}))

import { RemovePasswordDialog } from '../RemovePasswordDialog'

const service = vi.mocked(adminUserAccessService)
const onRemoved = vi.fn()
const onOpenChange = vi.fn()

function renderDialog() {
  render(
    <RemovePasswordDialog
      open
      onOpenChange={onOpenChange}
      userId="u-1"
      email="grace@example.com"
      onRemoved={onRemoved}
    />
  )
  return userEvent.setup()
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('RemovePasswordDialog', () => {
  it('removes the password', async () => {
    service.removePassword.mockResolvedValue(undefined)
    const user = renderDialog()
    await user.click(screen.getByRole('button', { name: 'Remove password' }))

    await waitFor(() => {
      expect(onRemoved).toHaveBeenCalledTimes(1)
    })
    expect(service.removePassword).toHaveBeenCalledWith('u-1', false)
  })

  it('asks again when it is the last way in, then confirms it', async () => {
    service.removePassword
      .mockRejectedValueOnce(
        new ApiError({
          type: 'about:blank',
          title: 'Conflict',
          status: 409,
          detail: 'It is the only way in',
          code: 'last_sign_in_method',
          request_id: 'r',
          timestamp: '2026-10-14T00:00:00Z',
        })
      )
      .mockResolvedValueOnce(undefined)
    const user = renderDialog()
    await user.click(screen.getByRole('button', { name: 'Remove password' }))

    const dialog = await screen.findByRole('alertdialog')
    await within(dialog).findByText('This is their only way to sign in')
    expect(onRemoved).not.toHaveBeenCalled()
    await user.click(
      within(dialog).getByRole('button', { name: 'Remove it anyway' })
    )
    await waitFor(() => {
      expect(onRemoved).toHaveBeenCalledTimes(1)
    })
    expect(service.removePassword).toHaveBeenLastCalledWith('u-1', true)
  })

  it('shows another refusal and stays open', async () => {
    service.removePassword.mockRejectedValue(new Error('No password to remove'))
    const user = renderDialog()
    await user.click(screen.getByRole('button', { name: 'Remove password' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'No password to remove'
    )
    expect(onRemoved).not.toHaveBeenCalled()
  })
})
